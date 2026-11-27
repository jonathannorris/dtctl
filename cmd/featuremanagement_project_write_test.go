package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/Dynatrace/dtctl/pkg/config"
	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

// newFMResourceServer answers reads of paths with body and records every
// write to them, with its method and path under _method and _path.
func newFMResourceServer(t *testing.T, body string, paths ...string) *fmWriteServer {
	t.Helper()
	s := &fmWriteServer{}
	mux := http.NewServeMux()
	record := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(body))
			return
		}
		b, _ := io.ReadAll(r.Body)
		var sent map[string]any
		_ = json.Unmarshal(b, &sent)
		if sent == nil {
			sent = map[string]any{}
		}
		sent["_method"], sent["_path"] = r.Method, r.URL.Path
		sent["_version"] = r.URL.Query().Get("optimistic-locking-version")
		s.puts = append(s.puts, sent)
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(body))
	}
	for _, p := range paths {
		mux.HandleFunc(sdkfm.BasePath+p, record)
	}
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

const fmProjectJSON = `{"key":"checkout","name":"Checkout","description":"","owner":"","maintainers":[],
 "settings":{"changeRequests":{"enabled":false},"retention":"P30D"},"version":7}`

func newFMProjectServer(t *testing.T) *fmWriteServer {
	return newFMResourceServer(t, fmProjectJSON, "/projects", "/projects/checkout")
}

func TestFMProjectWrites_SafetyLevels(t *testing.T) {
	writes := []struct {
		args    []string
		group   func() *cobra.Command
		allowed config.SafetyLevel
		refused config.SafetyLevel
	}{
		{[]string{"project", "checkout", "--name", "Checkout"}, newCreateFMCmd, config.SafetyLevelReadWriteAll, config.SafetyLevelReadWriteMine},
		{[]string{"project", "checkout", "--owner", "team"}, newUpdateFMCmd, config.SafetyLevelReadWriteAll, config.SafetyLevelReadWriteMine},
		{[]string{"project", "checkout", "-y"}, newDeleteFMCmd, config.SafetyLevelDangerouslyUnrestricted, config.SafetyLevelReadWriteAll},
	}
	for _, w := range writes {
		srv := newFMProjectServer(t)
		_, err := runFMWrite(t, srv, w.refused, w.group(), w.args...)
		require.Error(t, err, "%v at %s", w.args, w.refused)
		require.Empty(t, srv.puts)

		_, err = runFMWrite(t, srv, w.allowed, w.group(), w.args...)
		require.NoError(t, err, "%v at %s", w.args, w.allowed)
		require.Len(t, srv.puts, 1, "%v", w.args)
	}
}

func TestFMDeleteProject_RefusalNamesProjectsNotBuckets(t *testing.T) {
	srv := newFMProjectServer(t)
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newDeleteFMCmd(), "project", "checkout", "-y")
	require.ErrorContains(t, err, "Feature Management project")
	require.NotContains(t, err.Error(), "bucket")
}

func TestFMUpdateProject_KeepsUnknownSettings(t *testing.T) {
	srv := newFMProjectServer(t)
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newUpdateFMCmd(), "project", "checkout", "--change-requests")
	require.NoError(t, err)
	require.Len(t, srv.puts, 1)
	sent := srv.puts[0]
	require.Equal(t, "7", sent["_version"])
	settings := sent["settings"].(map[string]any)
	require.Equal(t, "P30D", settings["retention"])
	require.Equal(t, true, settings["changeRequests"].(map[string]any)["enabled"])

	// Asking for what the project already is sends nothing.
	srv = newFMProjectServer(t)
	_, err = runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newUpdateFMCmd(), "project", "checkout", "--change-requests=false")
	require.NoError(t, err)
	require.Empty(t, srv.puts)
}
