package gitwiring

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

func TestCapabilityProbeReadsTheDiscoveredSystemConfig(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "config"), []byte("[core]\n\tbare = false\n"), 0o600))
	sys := filepath.Join(dir, "system.cfg")
	require.NoError(t, os.WriteFile(sys, []byte("[commit]\n\tgpgsign = true\n"), 0o600))
	t.Setenv("HOME", filepath.Join(dir, "home"))
	for _, k := range []string{"GIT_CONFIG_SYSTEM", "GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_GLOBAL", "GIT_DIR"} {
		t.Setenv(k, "")
		require.NoError(t, os.Unsetenv(k))
	}

	probe := newCapabilityProbe(SystemConfigFiles{Default: []string{sys}}, nil)
	caps, err := probe.Capabilities(context.Background(), backend.Local{Root: backend.RepoRoot(root)}, backend.OpCommit)
	require.NoError(t, err)
	require.Len(t, caps, 1)
	assert.Equal(t, backend.CapGPGSign, caps[0].Kind)
}

func TestCapabilityProbeFailsClosedWhenDiscoveryFailed(t *testing.T) {
	probe := newCapabilityProbe(SystemConfigFiles{}, errors.New("no git"))
	_, err := probe.Capabilities(context.Background(), backend.Local{Root: "/x"}, backend.OpCommit)
	var de *backend.DetectError
	require.ErrorAs(t, err, &de)
	assert.Equal(t, backend.DetectIO, de.Sub)
}
