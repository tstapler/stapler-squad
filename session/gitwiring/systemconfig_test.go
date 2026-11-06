package gitwiring

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

func TestParseOriginList(t *testing.T) {
	list := "file:/sys/a\x00credential.helper\nosxkeychain\x00" +
		"file:/sys/b\x00bare.key\x00" + // a bare key has no value part
		"file:/sys/a\x00init.defaultbranch\nmain\x00" + // repeat origin collapses
		"command line:\x00x.y\nz\x00" // non-file origins are ignored
	got, err := parseOriginList([]byte(list))
	require.NoError(t, err)
	assert.Equal(t, []string{"/sys/a", "/sys/b"}, got)
}

func TestParseOriginListEmptyAndMalformed(t *testing.T) {
	got, err := parseOriginList(nil)
	require.NoError(t, err)
	assert.Empty(t, got)
	_, err = parseOriginList([]byte("file:/x\x00k\nv\x00orphan\x00"))
	require.Error(t, err)
}

type fakeGit struct {
	all       string // listing with no override
	vendor    string // listing with GIT_CONFIG_SYSTEM=/dev/null
	listErr   error
	sysStderr string
	sysErr    error
	calls     [][]string
}

func (f *fakeGit) run(extraEnv []string, args ...string) ([]byte, []byte, error) {
	f.calls = append(f.calls, args)
	switch {
	case len(args) > 1 && args[1] == "--system":
		return nil, []byte(f.sysStderr), f.sysErr
	case len(extraEnv) > 0:
		return []byte(f.vendor), nil, f.listErr
	}
	return []byte(f.all), nil, f.listErr
}

func TestDiscoverSystemConfig(t *testing.T) {
	t.Run("compiled-in default first, vendor files separate, deduplicated", func(t *testing.T) {
		f := &fakeGit{
			all:       "file:/etc/gitconfig\x00a.b\nc\x00file:/vendor/gitconfig\x00d.e\nf\x00",
			vendor:    "file:/vendor/gitconfig\x00d.e\nf\x00",
			sysStderr: "fatal: unable to read config file '/etc/gitconfig': No such file or directory\n",
			sysErr:    errors.New("exit status 128"),
		}
		got, err := discoverSystemConfig(f.run)
		require.NoError(t, err)
		assert.Equal(t, SystemConfigFiles{Default: []string{"/etc/gitconfig"}, Vendor: []string{"/vendor/gitconfig"}}, got)
	})
	t.Run("missing compiled-in default is still reported", func(t *testing.T) {
		f := &fakeGit{
			all:       "file:/vendor/gitconfig\x00d.e\nf\x00",
			vendor:    "file:/vendor/gitconfig\x00d.e\nf\x00",
			sysStderr: "fatal: unable to read config file '/opt/etc/gitconfig': No such file or directory\n",
			sysErr:    errors.New("exit status 128"),
		}
		got, err := discoverSystemConfig(f.run)
		require.NoError(t, err)
		assert.Equal(t, SystemConfigFiles{Default: []string{"/opt/etc/gitconfig"}, Vendor: []string{"/vendor/gitconfig"}}, got)
	})
	t.Run("an existing default system file is not named by an error", func(t *testing.T) {
		f := &fakeGit{all: "file:/etc/gitconfig\x00a.b\nc\x00"}
		got, err := discoverSystemConfig(f.run)
		require.NoError(t, err)
		assert.Equal(t, SystemConfigFiles{Default: []string{"/etc/gitconfig"}}, got)
	})
	t.Run("no system config at all is an empty, non-error answer", func(t *testing.T) {
		f := &fakeGit{sysStderr: "unrelated", sysErr: errors.New("x")}
		got, err := discoverSystemConfig(f.run)
		require.NoError(t, err)
		assert.Equal(t, SystemConfigFiles{}, got)
	})
	t.Run("a failing git is an error so the caller keeps the conservative default", func(t *testing.T) {
		f := &fakeGit{listErr: errors.New("git: not found")}
		_, err := discoverSystemConfig(f.run)
		require.Error(t, err)
	})
	t.Run("never asks for includes to be expanded", func(t *testing.T) {
		f := &fakeGit{}
		_, _ = discoverSystemConfig(f.run)
		require.NotEmpty(t, f.calls)
		assert.Contains(t, f.calls[0], "--no-includes")
	})
}

func TestScrubbedGitEnvDropsEveryConfigSelector(t *testing.T) {
	base := []string{
		"PATH=/usr/bin", "HOME=/home/u", "XDG_CONFIG_HOME=/x", "GIT_CONFIG_GLOBAL=/g", "GIT_CONFIG_SYSTEM=/s",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_PARAMETERS='a=b'", "GIT_DIR=/d", "GIT_WORK_TREE=/w",
		"GIT_COMMON_DIR=/c", "GIT_INDEX_FILE=/i", "LANG=C",
	}
	sandbox := filepath.Join(t.TempDir(), "sb")
	env := scrubbedGitEnv(base, sandbox)
	got := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		got[k] = v
	}
	assert.Equal(t, "/usr/bin", got["PATH"])
	assert.Equal(t, sandbox, got["HOME"])
	assert.Equal(t, filepath.Join(sandbox, "xdg"), got["XDG_CONFIG_HOME"])
	for _, k := range []string{"GIT_CONFIG_SYSTEM", "GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS", "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"} {
		assert.NotContains(t, got, k)
	}
	assert.Contains(t, got, "GIT_CONFIG_GLOBAL")
	assert.NotEqual(t, "/g", got["GIT_CONFIG_GLOBAL"])
}

func TestDiscoveryEnvForcesTheCLocale(t *testing.T) {
	env := discoveryEnv([]string{"LC_ALL=de_DE.UTF-8"}, t.TempDir(), []string{"GIT_CONFIG_SYSTEM=/x"})
	last := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		last[k] = v // os/exec keeps the last duplicate
	}
	assert.Equal(t, "C", last["LC_ALL"])
	assert.Equal(t, "/x", last["GIT_CONFIG_SYSTEM"])
}

func TestNewGoGitReadsTheDiscoveredFiles(t *testing.T) {
	dir := t.TempDir()
	def, vendor := filepath.Join(dir, "default.cfg"), filepath.Join(dir, "vendor.cfg")
	require.NoError(t, os.WriteFile(def, []byte("[zz]\n\tdefault = d\n"), 0o600))
	require.NoError(t, os.WriteFile(vendor, []byte("[zz]\n\tvendor = v\n"), 0o600))
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, k := range []string{"GIT_CONFIG_SYSTEM", "GIT_CONFIG_NOSYSTEM"} {
		t.Setenv(k, "") // registers the restore of the original value
		require.NoError(t, os.Unsetenv(k))
	}
	b := newGoGit(SystemConfigFiles{Default: []string{def}, Vendor: []string{vendor}}, nil)
	for key, want := range map[string]string{"zz.default": "d", "zz.vendor": "v"} {
		got, err := b.GetConfig(context.Background(), backend.Local{Root: backend.RepoRoot(home)}, backend.ConfigKey(key))
		require.NoError(t, err, key)
		assert.Equal(t, want, got, key)
	}
	assert.NotNil(t, newGoGit(SystemConfigFiles{}, errors.New("no git")), "a failed discovery still yields a usable backend")
}
