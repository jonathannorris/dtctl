package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Dynatrace/dtctl/pkg/config"
)

func writeFMApplyFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "feature.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

const fmApplyFeatureYAML = "key: new-checkout\nname: Renamed\ndescription: ''\ntype: RELEASE\nstatus: ACTIVE\n" +
	"variants: [{key: off, name: off, displayOrder: 0}, {key: on, name: on, displayOrder: 1}]\nflags: []\nowner: ''\nmaintainers: []\nversion: 3\n"

func TestFMApplyFeature_SafetyLevelsAndDryRun(t *testing.T) {
	path := writeFMApplyFile(t, fmApplyFeatureYAML)

	srv := newFMFeatureServer(t)
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteMine, newApplyFMCmd(), "feature", "-f", path)
	require.Error(t, err)
	require.Empty(t, srv.puts)

	// A dry run reads and sends nothing, so it needs no safety level.
	out, err := runFMWrite(t, srv, config.SafetyLevelReadOnly, newApplyFMCmd(), "feature", "-f", path, "--dry-run")
	require.NoError(t, err)
	require.Empty(t, srv.puts)
	require.Contains(t, out, `"dryRun": true`)
	require.Contains(t, out, "Renamed")

	_, err = runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newApplyFMCmd(), "feature", "-f", path)
	require.NoError(t, err)
	require.Len(t, srv.puts, 1)
	require.Equal(t, "PUT", srv.puts[0]["_method"])
	require.Equal(t, "Renamed", srv.puts[0]["name"])
}

func TestFMApplyFeatureTargeting_NeedsAStage(t *testing.T) {
	path := writeFMApplyFile(t, "targeting: {rules: []}\n")
	srv := newFMWriteServer(t)
	_, err := runFMWrite(t, srv, config.SafetyLevelReadWriteAll, newApplyFMCmd(), "feature-targeting", "new-checkout", "-f", path)
	require.ErrorContains(t, err, "--stage is required")
	require.Empty(t, srv.puts)
}
