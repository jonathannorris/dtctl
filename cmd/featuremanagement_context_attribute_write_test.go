package cmd

import (
	"net/http"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/Dynatrace/dtctl/pkg/config"
	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

const fmContextAttributeJSON = `{"projectKey":"checkout","key":"country","name":"Country","description":"","schema":"",
 "type":"STRING","contextKey":"country","builtIn":false,"examples":["AT"],"version":2}`

// newFMContextAttributeServer serves the country attribute, used by the
// features in usage (none when empty), and records writes.
func newFMContextAttributeServer(t *testing.T, usage string) *fmWriteServer {
	t.Helper()
	srv := newFMResourceServer(t, fmContextAttributeJSON, "/projects/checkout/context-attributes", "/projects/checkout/context-attributes/country")
	mux := srv.Config.Handler.(*http.ServeMux)
	mux.HandleFunc(sdkfm.BasePath+"/projects/checkout/context-attributes/country/usage", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[` + usage + `]}`))
	})
	return srv
}

func TestFMContextAttributeWrites_SafetyLevels(t *testing.T) {
	writes := []struct {
		args  []string
		group func() *cobra.Command
	}{
		{[]string{"context-attribute", "country", "--name", "Country", "--type", "string"}, newCreateFMCmd},
		{[]string{"context-attribute", "country", "--description", "ISO code"}, newUpdateFMCmd},
		{[]string{"context-attribute", "country", "-y"}, newDeleteFMCmd},
	}
	for _, w := range writes {
		srv := newFMContextAttributeServer(t, "")
		_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteMine, w.group(), w.args...)
		require.Error(t, err, "%v at readwrite-mine", w.args)
		require.Empty(t, srv.puts)

		_, err = runFMWrite(t, srv, config.SafetyLevelReadWriteAll, w.group(), w.args...)
		require.NoError(t, err, "%v at readwrite-all", w.args)
		require.Len(t, srv.puts, 1, "%v", w.args)
	}
}

func TestFMCreateContextAttribute_ContextKeyDefaultsToKey(t *testing.T) {
	srv := newFMContextAttributeServer(t, "")
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newCreateFMCmd(), "context-attribute", "country", "--name", "Country", "--type", "STRING")
	require.NoError(t, err)
	require.Equal(t, "country", srv.puts[0]["contextKey"])
	require.Equal(t, "", srv.puts[0]["schema"], "schema is required, so it is sent empty")
}

func TestFMUpdateContextAttribute_SendsOnlyWhatAnUpdateTakes(t *testing.T) {
	srv := newFMContextAttributeServer(t, "")
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newUpdateFMCmd(), "context-attribute", "country", "--name", "Land")
	require.NoError(t, err)
	sent := srv.puts[0]
	require.Equal(t, "Land", sent["name"])
	require.Equal(t, []any{"AT"}, sent["examples"])
	for _, readOnly := range []string{"type", "contextKey", "builtIn", "projectKey", "version"} {
		require.NotContains(t, sent, readOnly)
	}
}

func TestFMDeleteContextAttribute_RefusedWhileUsed(t *testing.T) {
	srv := newFMContextAttributeServer(t, `{"featureKey":"new-checkout","releaseStages":[]}`)
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newDeleteFMCmd(), "context-attribute", "country", "-y")
	require.ErrorContains(t, err, "still used by the targeting of new-checkout")
	require.Empty(t, srv.puts)
}
