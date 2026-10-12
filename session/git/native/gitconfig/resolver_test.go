package gitconfig

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDevNullIsAnEmptyConfigFile(t *testing.T) {
	home := t.TempDir()
	res, err := Resolve(context.Background(), Env{Environ: []string{"HOME=" + home, "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1"}}, Repo{})
	require.NoError(t, err)
	assert.Empty(t, res.Entries)
}

func fixtureHome(t *testing.T, files map[string]string) []string {
	t.Helper()
	home := t.TempDir()
	for name, content := range files {
		p := filepath.Join(home, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o700))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}
	return []string{"HOME=" + home, "GIT_CONFIG_NOSYSTEM=1"}
}

func TestResolveOrdersScopesAndKeepsIncludePosition(t *testing.T) {
	env := fixtureHome(t, map[string]string{
		".config/git/config": "[a]\n\tk = xdg\n",
		".gitconfig":         "[a]\n\tk = home1\n[include]\n\tpath = inc.cfg\n[a]\n\tk = home2\n",
		"inc.cfg":            "[a]\n\tk = included\n",
	})
	env = append(env, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=a.k", "GIT_CONFIG_VALUE_0=cmd")
	res, err := Resolve(context.Background(), Env{Environ: env}, Repo{})
	require.NoError(t, err)
	all := res.All("a.k")
	values := make([]string, 0, len(all))
	for _, s := range all {
		values = append(values, string(s.Scope)+":"+s.Value)
	}
	assert.Equal(t, []string{"global:xdg", "global:home1", "global:included", "global:home2", "command:cmd"}, values)
	last, _ := res.Lookup("a.k")
	assert.Equal(t, "cmd", last.Value)
}

func TestResolveRecordsMissingFilesInTheClosure(t *testing.T) {
	env := fixtureHome(t, map[string]string{".gitconfig": "[include]\n\tpath = later.cfg\n"})
	res, err := Resolve(context.Background(), Env{Environ: env}, Repo{})
	require.NoError(t, err)
	var missing bool
	for _, f := range res.Closure {
		if filepath.Base(f.Path) == "later.cfg" {
			missing = f.SHA256 == ""
		}
	}
	assert.True(t, missing, "an absent include must be in the closure with an empty hash so creating it invalidates a cache key: %+v", res.Closure)
}

func TestParseBoolMatchesGit(t *testing.T) {
	for _, c := range []struct {
		v      string
		has    bool
		want   bool
		wantOK bool
	}{
		{"", false, true, true}, {"", true, false, true}, {"true", true, true, true}, {"YES", true, true, true},
		{"On", true, true, true}, {"false", true, false, true}, {"no", true, false, true}, {"off", true, false, true},
		{"0", true, false, true}, {"1", true, true, true}, {"2", true, true, true}, {"-1", true, true, true},
		{"1k", true, true, true}, {"0x10", true, true, true}, {"banana", true, false, false},
	} {
		got, ok := ParseBool(c.v, c.has)
		assert.Equal(t, c.want, got, "%q has=%v", c.v, c.has)
		assert.Equal(t, c.wantOK, ok, "%q has=%v", c.v, c.has)
	}
}

func TestParseKey(t *testing.T) {
	s, sub, has, name, ok := ParseKey("Remote.Origin.URL")
	assert.True(t, ok)
	assert.Equal(t, []any{"remote", "Origin", true, "url"}, []any{s, sub, has, name})
	_, _, _, _, ok = ParseKey("nodot")
	assert.False(t, ok)
	_, _, _, _, ok = ParseKey("a.")
	assert.False(t, ok)
	_, _, _, _, ok = ParseKey("a.1b")
	assert.False(t, ok)
}

func TestEnvironmentOverridesFailClosed(t *testing.T) {
	for _, kv := range []string{"GIT_DIR=/x", "GIT_INDEX_FILE=/x", "GIT_COMMON_DIR=/x", "GIT_WORK_TREE=/x", "GIT_CONFIG_PARAMETERS='a=b'"} {
		_, err := Resolve(context.Background(), Env{Environ: []string{"HOME=/h", kv}}, Repo{})
		var ce *Error
		require.ErrorAs(t, err, &ce, kv)
	}
}
