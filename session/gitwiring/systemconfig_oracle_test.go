//go:build gitoracle

package gitwiring

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/gogit"
	"github.com/tstapler/stapler-squad/testutil/gitoracle"
)

// oracleEnv is the scrubbed environment without GIT_CEILING_DIRECTORIES, which the in-process
// backend deliberately hands to the CLI instead of honouring.
func oracleEnv(sandbox string) []string {
	var env []string
	for _, kv := range scrubbedGitEnv(os.Environ(), sandbox) {
		if !strings.HasPrefix(kv, "GIT_CEILING_DIRECTORIES=") {
			env = append(env, kv)
		}
	}
	return env
}

// realGit runs the real git in the scrubbed sandbox, with extra environment entries appended.
func realGit(t *testing.T, sandbox string, extra []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = sandbox
	cmd.Env = append(oracleEnv(sandbox), extra...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return strings.TrimRight(out.String(), "\n"), err
}

// TestDiscoveredSystemConfigAgreesWithGit: with the discovered files, the in-process reader gives
// the answer `git config --get` gives for every key git itself reports at system scope, on
// whatever git build runs the tests (Apple git keeps its file under share/git-core, which a
// guessed list missed).
func TestDiscoveredSystemConfigAgreesWithGit(t *testing.T) {
	gitoracle.RequireGit(t)
	files, err := DiscoverSystemConfig(context.Background())
	require.NoError(t, err)

	sandbox := t.TempDir()
	// `--system` on some builds fails when /etc/gitconfig is absent; ask without it.
	listing, err := realGit(t, sandbox, nil, "config", "--list", "--show-origin", "--no-includes")
	require.NoError(t, err)
	var keys []string
	for _, line := range strings.Split(listing, "\n") {
		_, kv, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		key, _, _ := strings.Cut(kv, "=")
		keys = append(keys, key)
	}
	if len(keys) > 0 {
		require.NotEmpty(t, append(append([]string(nil), files.Default...), files.Vendor...), "git reports system config entries but discovery found no file")
	}
	keys = append(keys, "user.name", "core.nosuchthing", "credential.helper")

	env := oracleEnv(sandbox)
	gg := gogit.New(gogit.WithEnviron(func() []string { return env }), gogit.WithSystemConfig(files.Default...), gogit.WithVendorSystemConfig(files.Vendor...))
	loc := backend.Local{Root: backend.RepoRoot(sandbox)}
	for _, key := range keys {
		want, wantErr := realGit(t, sandbox, nil, "config", "--get", key)
		got, gotErr := gg.GetConfig(context.Background(), loc, backend.ConfigKey(key))
		if wantErr != nil {
			assert.ErrorIs(t, gotErr, backend.ErrConfigUnset, key)
			continue
		}
		require.NoError(t, gotErr, key)
		assert.Equal(t, want, got, key)
	}
}

// TestDiscoveredSystemConfigHonoursOverrides: GIT_CONFIG_SYSTEM replaces the discovered files and
// GIT_CONFIG_NOSYSTEM drops them, exactly as for git.
func TestDiscoveredSystemConfigHonoursOverrides(t *testing.T) {
	gitoracle.RequireGit(t)
	files, err := DiscoverSystemConfig(context.Background())
	require.NoError(t, err)

	sandbox := t.TempDir()
	custom := filepath.Join(sandbox, "custom.cfg")
	require.NoError(t, os.WriteFile(custom, []byte("[zz]\n\tkey = from-custom\n"), 0o600))
	loc := backend.Local{Root: backend.RepoRoot(sandbox)}

	for name, extra := range map[string][]string{
		"GIT_CONFIG_SYSTEM":   {"GIT_CONFIG_SYSTEM=" + custom},
		"GIT_CONFIG_NOSYSTEM": {"GIT_CONFIG_NOSYSTEM=1"},
	} {
		t.Run(name, func(t *testing.T) {
			env := append(oracleEnv(sandbox), extra...)
			gg := gogit.New(gogit.WithEnviron(func() []string { return env }), gogit.WithSystemConfig(files.Default...), gogit.WithVendorSystemConfig(files.Vendor...))
			for _, key := range []string{"zz.key", "credential.helper", "init.defaultbranch"} {
				want, wantErr := realGit(t, sandbox, extra, "config", "--get", key)
				got, gotErr := gg.GetConfig(context.Background(), loc, backend.ConfigKey(key))
				if wantErr != nil {
					assert.ErrorIs(t, gotErr, backend.ErrConfigUnset, key)
					continue
				}
				require.NoError(t, gotErr, key)
				assert.Equal(t, want, got, key)
			}
		})
	}
}
