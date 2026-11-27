package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/Dynatrace/dtctl/pkg/config"
	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

// newFMFeatureServer answers feature reads and records feature writes.
func newFMFeatureServer(t *testing.T) *fmWriteServer {
	t.Helper()
	s := &fmWriteServer{}
	feature := `{"projectKey":"checkout","key":"new-checkout","name":"New checkout","description":"","variants":[{"key":"off","name":"off","displayOrder":0},{"key":"on","name":"on","displayOrder":1}],"flags":[],"owner":"","maintainers":[],"status":"ACTIVE","type":"RELEASE","version":3}`
	mux := http.NewServeMux()
	record := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(feature))
			return
		}
		body, _ := io.ReadAll(r.Body)
		var sent map[string]any
		_ = json.Unmarshal(body, &sent)
		if sent == nil {
			sent = map[string]any{}
		}
		sent["_method"], sent["_path"] = r.Method, r.URL.Path
		s.puts = append(s.puts, sent)
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(feature))
	}
	mux.HandleFunc(sdkfm.BasePath+"/projects/checkout/features", record)
	mux.HandleFunc(sdkfm.BasePath+"/projects/checkout/features/new-checkout", record)
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func TestFMFeatureWrites_SafetyLevels(t *testing.T) {
	writes := [][]string{
		{"create", "feature", "new-checkout", "--name", "New checkout"},
		{"update", "feature", "new-checkout", "--remove-variant", "on"},
		{"delete", "feature", "new-checkout", "-y"},
	}
	groups := map[string]func() *cobra.Command{"create": newCreateFMCmd, "update": newUpdateFMCmd, "delete": newDeleteFMCmd}
	for _, w := range writes {
		srv := newFMFeatureServer(t)
		_, err := runFMWrite(t, &fmWriteServer{Server: srv.Server}, config.SafetyLevelReadWriteMine, groups[w[0]](), w[1:]...)
		require.Error(t, err, "%v at readwrite-mine", w)
		require.Empty(t, srv.puts)

		_, err = runFMWrite(t, srv, config.SafetyLevelReadWriteAll, groups[w[0]](), w[1:]...)
		require.NoError(t, err, "%v at readwrite-all", w)
		require.Len(t, srv.puts, 1, "%v", w)
	}
}

func TestFMUpdateFeature_OneKindOfChange(t *testing.T) {
	srv := newFMFeatureServer(t)
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newUpdateFMCmd(), "feature", "new-checkout", "--owner", "x", "--status", "ARCHIVED")
	require.ErrorContains(t, err, "separate commands")
	_, err = runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newUpdateFMCmd(), "feature", "new-checkout")
	require.ErrorContains(t, err, "nothing to update")
	require.Empty(t, srv.puts)
}

func TestFMCreateFeature_FromFile(t *testing.T) {
	srv := newFMFeatureServer(t)
	path := filepath.Join(t.TempDir(), "feature.yaml")
	require.NoError(t, os.WriteFile(path, []byte("kind: fm/feature\nprojectKey: checkout\nkey: new-checkout\nname: New checkout\nversion: 7\nvariants: [{key: off, name: Off}]\n"), 0o600))
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newCreateFMCmd(), "feature", "-f", path)
	require.NoError(t, err)
	require.Len(t, srv.puts, 1)
	require.Equal(t, "new-checkout", srv.puts[0]["key"])
	require.NotContains(t, srv.puts[0], "version")
	require.NotContains(t, srv.puts[0], "kind")

	_, err = runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newCreateFMCmd(), "feature", "-f", path, "--name", "x")
	require.ErrorContains(t, err, "-f cannot be combined")
}

func TestFMDeleteFeature_NeedsYesWithoutATerminal(t *testing.T) {
	origPlain := plainMode(context.Background())
	t.Cleanup(func() { gFlags.plainMode = origPlain })
	gFlags.plainMode = true
	srv := newFMFeatureServer(t)
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newDeleteFMCmd(), "feature", "new-checkout")
	require.ErrorContains(t, err, "pass -y to confirm")
	require.Empty(t, srv.puts)
}
