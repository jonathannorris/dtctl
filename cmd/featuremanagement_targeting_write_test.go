package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/Dynatrace/dtctl/pkg/config"
	sdkfm "github.com/Dynatrace/dtctl/sdk/api/featuremanagement"
)

// fmWriteServer is a Feature Management backend with one feature's targeting
// in the production stage. It counts the targeting writes it receives.
type fmWriteServer struct {
	*httptest.Server
	puts []map[string]any
}

func newFMWriteServer(t *testing.T) *fmWriteServer {
	t.Helper()
	s := &fmWriteServer{}
	feature := sdkfm.BasePath + "/projects/checkout/features/new-checkout"
	ft := map[string]any{
		"projectKey": "checkout", "featureKey": "new-checkout", "releaseStageKey": "production",
		"enabled": true, "version": 4,
		"targeting": map[string]any{"rules": []any{
			map[string]any{"id": "6f1c2b9e-0000-4000-8000-000000000001", "serve": []any{map[string]any{"variantKey": "on", "percentage": 100}}},
		}},
	}
	mux := http.NewServeMux()
	mux.HandleFunc(feature, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(sdkfm.FeatureDetails{Key: "new-checkout", Variants: []sdkfm.Variant{{Key: "on"}, {Key: "off"}}})
	})
	mux.HandleFunc(sdkfm.BasePath+"/projects/checkout/context-attributes", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(sdkfm.ContextAttributeList{})
	})
	mux.HandleFunc(feature+"/release-stage-targeting/production", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			var sent map[string]any
			_ = json.Unmarshal(body, &sent)
			s.puts = append(s.puts, sent)
			ft["enabled"], ft["targeting"] = sent["enabled"], sent["targeting"]
			ft["version"] = ft["version"].(int) + 1
		}
		_ = json.NewEncoder(w).Encode(ft)
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

// runFMWrite runs one fm write command against srv in a context at level,
// with --project checkout, and returns its stdout.
func runFMWrite(t *testing.T, srv *fmWriteServer, level config.SafetyLevel, group *cobra.Command, args ...string) (string, error) {
	t.Helper()
	setupPlatformCmdTest(t, srv.Server, "json")
	// The shell running the tests may be an AI agent's, which turns agent mode
	// on by detection; these tests read the plain -o json result.
	origNoAgent := gFlags.noAgent
	t.Cleanup(func() { gFlags.noAgent = origNoAgent })
	gFlags.noAgent = true
	cfg, err := LoadConfig()
	require.NoError(t, err)
	cur, err := cfg.CurrentContextObj()
	require.NoError(t, err)
	cfg.SetContextWithOptions(cfg.CurrentContext, cur.Environment, "", &config.ContextOptions{SafetyLevel: level})
	require.NoError(t, cfg.SaveTo(cfgFile(context.Background())))

	group.SetArgs(append(args, "--project", "checkout"))
	group.SilenceUsage, group.SilenceErrors = true, true
	var runErr error
	out := capturePlatformStdout(t, func() { _, runErr = group.ExecuteC() })
	return out, runErr
}

func TestFMTargetingWrites_SafetyLevels(t *testing.T) {
	writes := []struct {
		name  string
		group func() *cobra.Command
		args  []string
	}{
		{"create", newCreateFMCmd, []string{"feature-targeting", "new-checkout", "-s", "production", "--variant", "off"}},
		{"update", newUpdateFMCmd, []string{"feature-targeting", "new-checkout", "-s", "production", "--position", "0", "--variant", "off"}},
		{"delete", newDeleteFMCmd, []string{"feature-targeting", "new-checkout", "-s", "production", "--position", "0"}},
		{"disable", newDisableFMCmd, []string{"feature", "new-checkout", "-s", "production"}},
	}
	for _, w := range writes {
		for _, level := range []config.SafetyLevel{config.SafetyLevelReadOnly, config.SafetyLevelReadWriteMine} {
			t.Run(w.name+" refused at "+string(level), func(t *testing.T) {
				srv := newFMWriteServer(t)
				_, err := runFMWrite(t, srv, level, w.group(), w.args...)
				require.Error(t, err)
				require.Empty(t, srv.puts, "a refused write reached the server")
			})
		}
		t.Run(w.name+" allowed at readwrite-all", func(t *testing.T) {
			srv := newFMWriteServer(t)
			_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, w.group(), w.args...)
			require.NoError(t, err)
			require.Len(t, srv.puts, 1)
		})
	}
}

func TestFMTargetingWrites_RequireTheStage(t *testing.T) {
	srv := newFMWriteServer(t)
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newEnableFMCmd(), "feature", "new-checkout")
	require.ErrorContains(t, err, "--stage is required")
	require.Empty(t, srv.puts)
}

func TestFMDisable_ResultAndNoOp(t *testing.T) {
	srv := newFMWriteServer(t)
	out, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newDisableFMCmd(), "feature", "new-checkout", "-s", "production")
	require.NoError(t, err)
	var res fmTargetingWrite
	require.NoError(t, json.Unmarshal([]byte(out), &res), out)
	require.True(t, res.Changed)
	require.Equal(t, "disable", res.Action)
	require.False(t, res.Targeting.Enabled)
	require.Equal(t, "6f1c2b9e-0000-4000-8000-000000000001", res.Targeting.Targeting.Rules[0].ID)

	out, err = runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newDisableFMCmd(), "feature", "new-checkout", "-s", "production")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(out), &res), out)
	require.False(t, res.Changed, "disabling a disabled stage must be a no-op")
	require.Len(t, srv.puts, 1)
}

func TestFMDeleteAll_NeedsConfirmationWithoutATerminal(t *testing.T) {
	origPlain := plainMode(context.Background())
	t.Cleanup(func() { gFlags.plainMode = origPlain })
	gFlags.plainMode = true

	srv := newFMWriteServer(t)
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newDeleteFMCmd(), "feature-targeting", "new-checkout", "-s", "production", "--all")
	require.ErrorContains(t, err, "pass -y to confirm")
	require.Empty(t, srv.puts)

	_, err = runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newDeleteFMCmd(), "feature-targeting", "new-checkout", "-s", "production", "--all", "-y")
	require.NoError(t, err)
	require.Len(t, srv.puts, 1)
	rules := srv.puts[0]["targeting"].(map[string]any)["rules"].([]any)
	require.Empty(t, rules)
	require.Equal(t, true, srv.puts[0]["enabled"], "--all must leave enabled as read")
}

func TestFMUpdate_NeedsSomethingToChange(t *testing.T) {
	srv := newFMWriteServer(t)
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newUpdateFMCmd(), "feature-targeting", "new-checkout", "-s", "production", "--position", "0")
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "nothing to update"), err.Error())
}
