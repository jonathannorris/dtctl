package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Dynatrace/dtctl/pkg/config"
	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

// newFMOverridesServer serves feature new-checkout (variants off and on),
// the caller's override of it in staging, and targeting keys user-1, and
// records the writes.
func newFMOverridesServer(t *testing.T) *fmWriteServer {
	t.Helper()
	s := &fmWriteServer{}
	project := sdkfm.BasePath + "/projects/checkout"
	mux := http.NewServeMux()
	record := func(r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var sent map[string]any
		_ = json.Unmarshal(b, &sent)
		if sent == nil {
			sent = map[string]any{}
		}
		sent["_method"], sent["_path"] = r.Method, r.URL.Path
		s.puts = append(s.puts, sent)
	}
	mux.HandleFunc(project+"/features/new-checkout", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"key":"new-checkout","name":"New checkout","variants":[{"key":"off","name":"Off"},{"key":"on","name":"On"}],"flags":[],"version":1}`))
	})
	mux.HandleFunc(project+"/features/new-checkout/my-overrides", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			record(r)
		}
		_, _ = w.Write([]byte(`{"overrides":[{"releaseStageKey":"development","variantKey":null},{"releaseStageKey":"staging","variantKey":"on"}]}`))
	})
	mux.HandleFunc(project+"/overrides-profile", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"targetingKeys":["user-1"]}`))
		case http.MethodDelete:
			record(r)
			w.WriteHeader(http.StatusNoContent)
		default:
			record(r)
		}
	})
	mux.HandleFunc(project+"/my-overrides", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		w.WriteHeader(http.StatusNoContent)
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func TestFMOverrideWrites_AllowedAtReadWriteMine(t *testing.T) {
	writes := []struct {
		args   []string
		update bool
	}{
		{[]string{"override", "new-checkout", "-s", "development", "--variant", "off"}, true},
		{[]string{"overrides", "--add-targeting-key", "device-2"}, true},
		{[]string{"overrides", "--all", "-y"}, false},
	}
	for _, w := range writes {
		group := newDeleteFMCmd
		if w.update {
			group = newUpdateFMCmd
		}
		srv := newFMOverridesServer(t)
		_, err := runFMWrite(t, srv, config.SafetyLevelReadOnly, group(), w.args...)
		require.Error(t, err, "%v at readonly", w.args)
		require.Empty(t, srv.puts)

		_, err = runFMWrite(t, srv, config.SafetyLevelReadWriteMine, group(), w.args...)
		require.NoError(t, err, "%v at readwrite-mine", w.args)
		require.Len(t, srv.puts, 1, "%v", w.args)
	}
}

func TestFMUpdateOverride_SendsTheFullSet(t *testing.T) {
	srv := newFMOverridesServer(t)
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteMine, newUpdateFMCmd(), "override", "new-checkout", "-s", "development", "--variant", "off")
	require.NoError(t, err)
	require.Equal(t, []any{
		map[string]any{"releaseStageKey": "development", "variantKey": "off"},
		map[string]any{"releaseStageKey": "staging", "variantKey": "on"},
	}, srv.puts[0]["overrides"])

	srv = newFMOverridesServer(t)
	_, err = runFMWrite(t, srv, config.SafetyLevelReadWriteMine, newUpdateFMCmd(), "override", "new-checkout", "-s", "staging", "--clear")
	require.NoError(t, err)
	require.Equal(t, []any{}, srv.puts[0]["overrides"])

	srv = newFMOverridesServer(t)
	_, err = runFMWrite(t, srv, config.SafetyLevelReadWriteMine, newUpdateFMCmd(), "override", "new-checkout", "-s", "staging", "--variant", "on")
	require.NoError(t, err)
	require.Empty(t, srv.puts, "the override already was on")

	for _, bad := range [][]string{
		{"override", "new-checkout", "-s", "staging", "--variant", "nope"},
		{"override", "new-checkout", "-s", "production", "--variant", "on"},
		{"override", "new-checkout", "-s", "staging"},
		{"override", "new-checkout", "-s", "staging", "--variant", "on", "--clear"},
		{"overrides", "--stage", "staging"},
	} {
		_, err = runFMWrite(t, srv, config.SafetyLevelReadWriteMine, newUpdateFMCmd(), bad...)
		require.Error(t, err, "%v", bad)
	}
	require.Empty(t, srv.puts)
}

func TestFMUpdateOverrides_TargetingKeys(t *testing.T) {
	srv := newFMOverridesServer(t)
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteMine, newUpdateFMCmd(), "overrides", "--add-targeting-key", "Device-2", "--remove-targeting-key", "user-1")
	require.NoError(t, err)
	require.Equal(t, []any{"Device-2"}, srv.puts[0]["targetingKeys"])

	srv = newFMOverridesServer(t)
	_, err = runFMWrite(t, srv, config.SafetyLevelReadWriteMine, newUpdateFMCmd(), "overrides", "--add-targeting-key", "user-1")
	require.NoError(t, err)
	require.Empty(t, srv.puts, "user-1 already was a targeting key")
}

func TestFMDeleteOverrides(t *testing.T) {
	srv := newFMOverridesServer(t)
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteMine, newDeleteFMCmd(), "override", "new-checkout", "-y")
	require.NoError(t, err)
	require.Len(t, srv.puts, 1)
	require.Equal(t, http.MethodPut, srv.puts[0]["_method"])
	require.Equal(t, []any{}, srv.puts[0]["overrides"])

	srv = newFMOverridesServer(t)
	_, err = runFMWrite(t, srv, config.SafetyLevelReadWriteMine, newDeleteFMCmd(), "overrides", "--all", "--targeting-keys", "-y")
	require.NoError(t, err)
	require.Len(t, srv.puts, 2)

	_, err = runFMWrite(t, srv, config.SafetyLevelReadWriteMine, newDeleteFMCmd(), "overrides", "-y")
	require.ErrorContains(t, err, "--all")
}
