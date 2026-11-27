package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/Dynatrace/dtctl/pkg/config"
	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

const fmReleaseStageJSON = `{"key":"qa","name":"QA","description":"","type":"STAGING","displayOrder":1,"color":"blue","version":3}`

func newFMReleaseStageServer(t *testing.T) *fmWriteServer {
	return newFMResourceServer(t, fmReleaseStageJSON, "/projects/checkout/release-stages", "/projects/checkout/release-stages/qa")
}

func TestFMReleaseStageWrites_SafetyLevels(t *testing.T) {
	writes := []struct {
		args             []string
		group            func() *cobra.Command
		allowed, refused config.SafetyLevel
	}{
		{[]string{"release-stage", "qa", "--name", "QA", "--type", "staging"}, newCreateFMCmd, config.SafetyLevelReadWriteAll, config.SafetyLevelReadWriteMine},
		{[]string{"release-stage", "qa", "--guarded"}, newUpdateFMCmd, config.SafetyLevelReadWriteAll, config.SafetyLevelReadWriteMine},
		{[]string{"release-stage", "qa", "-y"}, newDeleteFMCmd, config.SafetyLevelDangerouslyUnrestricted, config.SafetyLevelReadWriteAll},
	}
	for _, w := range writes {
		srv := newFMReleaseStageServer(t)
		_, err := runFMWrite(t, srv, w.refused, w.group(), w.args...)
		require.Error(t, err, "%v at %s", w.args, w.refused)
		require.Empty(t, srv.puts)

		_, err = runFMWrite(t, srv, w.allowed, w.group(), w.args...)
		require.NoError(t, err, "%v at %s", w.args, w.allowed)
		require.Len(t, srv.puts, 1, "%v", w.args)
	}
}

func TestFMUpdateReleaseStage_KeepsUnknownFields(t *testing.T) {
	srv := newFMReleaseStageServer(t)
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newUpdateFMCmd(), "release-stage", "qa", "--guarded")
	require.NoError(t, err)
	require.Len(t, srv.puts, 1)
	require.Equal(t, "blue", srv.puts[0]["color"])
	require.Equal(t, true, srv.puts[0]["guarded"])
	require.Equal(t, "3", srv.puts[0]["_version"])
}

// newFMStagesServer serves three stages and records reorders.
func newFMStagesServer(t *testing.T) *fmWriteServer {
	t.Helper()
	s := &fmWriteServer{}
	list := `{"items":[{"key":"dev","name":"Dev","type":"DEVELOPMENT","displayOrder":0,"version":1},
	 {"key":"qa","name":"QA","type":"STAGING","displayOrder":1,"version":4},
	 {"key":"prod","name":"Prod","type":"PRODUCTION","displayOrder":2,"version":2}],"totalCount":3}`
	mux := http.NewServeMux()
	mux.HandleFunc(sdkfm.BasePath+"/projects/checkout/release-stages", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(list))
	})
	mux.HandleFunc(sdkfm.BasePath+"/projects/checkout/release-stages/reorder", func(w http.ResponseWriter, r *http.Request) {
		var sent map[string]any
		_ = json.NewDecoder(r.Body).Decode(&sent)
		s.puts = append(s.puts, sent)
		_, _ = w.Write([]byte(list))
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func TestFMUpdateReleaseStages_Order(t *testing.T) {
	srv := newFMStagesServer(t)
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newUpdateFMCmd(), "release-stages", "--order", "qa,dev,prod")
	require.NoError(t, err)
	require.Len(t, srv.puts, 1)
	items := srv.puts[0]["items"].([]any)
	require.Equal(t, map[string]any{"key": "qa", "displayOrder": 0.0, "version": 4.0}, items[0])
	require.Equal(t, map[string]any{"key": "dev", "displayOrder": 1.0, "version": 1.0}, items[1])

	srv = newFMStagesServer(t)
	_, err = runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newUpdateFMCmd(), "release-stages", "--order", "dev,qa,prod")
	require.NoError(t, err)
	require.Empty(t, srv.puts, "the stages already were in that order")

	for _, bad := range []string{"dev,qa", "dev,qa,prod,qa", "dev,qa,nope"} {
		_, err = runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newUpdateFMCmd(), "release-stages", "--order", bad)
		require.Error(t, err, bad)
	}
	_, err = runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newUpdateFMCmd(), "release-stage", "qa", "--order", "dev,qa,prod")
	require.ErrorContains(t, err, "takes no stage")
	require.Empty(t, srv.puts)
}
