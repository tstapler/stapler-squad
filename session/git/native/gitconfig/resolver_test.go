package gitconfig

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func TestIncludeDepthBoundary(t *testing.T) {
	chain := func(n int) []string {
		files := map[string]string{".gitconfig": "[include]\n\tpath = c1.cfg\n"}
		for i := 1; i <= n; i++ {
			body := "[c]\n\tk = 1\n"
			if i < n {
				body += "[include]\n\tpath = c" + strconv.Itoa(i+1) + ".cfg\n"
			}
			files["c"+strconv.Itoa(i)+".cfg"] = body
		}
		return fixtureHome(t, files)
	}
	_, err := Resolve(context.Background(), Env{Environ: chain(10)}, Repo{})
	require.NoError(t, err, "ten nested includes are allowed")
	_, err = Resolve(context.Background(), Env{Environ: chain(11)}, Repo{})
	var ce *Error
	require.ErrorAs(t, err, &ce, "an eleventh nested include is where git dies")
	assert.Equal(t, SubIncludeDepth, ce.Sub)
}

func TestExponentialIncludeFanOutIsBounded(t *testing.T) {
	files := map[string]string{".gitconfig": "[include]\n\tpath = f0.cfg\n"}
	for i := range 9 { // depth stays under git's limit, the fan-out is 4^9
		body := ""
		for range 4 {
			body += "[include]\n\tpath = f" + strconv.Itoa(i+1) + ".cfg\n"
		}
		files["f"+strconv.Itoa(i)+".cfg"] = body
	}
	files["f9.cfg"] = "[a]\n\tk = 1\n"
	env := fixtureHome(t, files)
	_, err := Resolve(context.Background(), Env{Environ: env}, Repo{})
	var ce *Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, SubIncludeDepth, ce.Sub)
}

func TestScopeSelectionVariables(t *testing.T) {
	env := fixtureHome(t, map[string]string{".gitconfig": "[a]\n\tk = home\n", "g.cfg": "[a]\n\tk = global-env\n", "s.cfg": "[a]\n\tk = system-env\n", "v.cfg": "[a]\n\tk = vendor\n"})
	home := env[0][len("HOME="):]
	value := func(extra ...string) string {
		e := append([]string{env[0]}, extra...)
		res, err := Resolve(context.Background(), Env{Environ: e, SystemVendor: []string{filepath.Join(home, "v.cfg")}}, Repo{})
		require.NoError(t, err)
		last, _ := res.Lookup("a.k")
		return last.Value
	}
	assert.Equal(t, "home", value("GIT_CONFIG_NOSYSTEM=1"))
	noSys, err := Resolve(context.Background(), Env{Environ: []string{env[0], "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull}, SystemVendor: []string{filepath.Join(home, "v.cfg")}}, Repo{})
	require.NoError(t, err)
	assert.Empty(t, noSys.All("a.k"), "GIT_CONFIG_NOSYSTEM drops vendor files too")
	assert.Equal(t, "global-env", value("GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(home, "g.cfg")))
	assert.Equal(t, "home", value("GIT_CONFIG_SYSTEM="+filepath.Join(home, "s.cfg")), "global beats system")
	res, err := Resolve(context.Background(), Env{Environ: []string{env[0], "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_SYSTEM=" + filepath.Join(home, "s.cfg")}, SystemVendor: []string{filepath.Join(home, "v.cfg")}}, Repo{})
	require.NoError(t, err)
	all := res.All("a.k")
	order := make([]string, 0, len(all))
	for _, s := range all {
		order = append(order, s.Value)
	}
	assert.Equal(t, []string{"vendor", "system-env"}, order, "vendor files come first and GIT_CONFIG_SYSTEM does not replace them")
	_, err = Resolve(context.Background(), Env{Environ: []string{env[0], "GIT_CONFIG_NOSYSTEM=banana"}}, Repo{})
	require.Error(t, err)
}

func TestTildeIncludeAndHasconfigAndGitdirFold(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir()) // macOS temp dirs sit behind a symlink
	require.NoError(t, err)
	root := filepath.Join(base, "Work", "Repo")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "config"), []byte("[remote \"o\"]\n\turl = https://git.corp.example/x\n"), 0o600))
	home := filepath.Join(base, "home")
	write := func(rel, content string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(home, rel)), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(home, rel), []byte(content), 0o600))
	}
	write("tilde.cfg", "[t]\n\tk = tilde\n")
	write("hit.cfg", "[h]\n\tk = hit\n")
	write(".gitconfig", "[include]\n\tpath = ~/tilde.cfg\n"+
		"[includeIf \"hasconfig:remote.*.url:https://**.corp.example/**\"]\n\tpath = ~/hit.cfg\n"+
		"[includeIf \"gitdir/i:"+strings.ToLower(root)+"/\"]\n\tpath = ~/fold.cfg\n"+
		"[includeIf \"gitdir:"+strings.ToLower(root)+"/\"]\n\tpath = ~/nofold.cfg\n")
	write("fold.cfg", "[f]\n\tk = folded\n")
	write("nofold.cfg", "[n]\n\tk = sensitive\n")
	repo := Repo{GitDir: filepath.Join(root, ".git"), CommonDir: filepath.Join(root, ".git")}
	res, err := Resolve(context.Background(), Env{Environ: []string{"HOME=" + home, "GIT_CONFIG_NOSYSTEM=1"}}, repo)
	require.NoError(t, err)
	has := func(key string) bool { _, ok := res.Lookup(key); return ok }
	assert.True(t, has("t.k"), "~/ include")
	assert.True(t, has("h.k"), "hasconfig:remote.*.url matches the repo's remote")
	assert.True(t, has("f.k"), "gitdir/i folds case")
	if !isCaseInsensitiveFS(root) {
		assert.False(t, has("n.k"), "gitdir: is case sensitive")
	}
}

func isCaseInsensitiveFS(dir string) bool {
	_, err := os.Stat(strings.ToLower(dir))
	_, err2 := os.Stat(strings.ToUpper(dir))
	return err == nil && err2 == nil
}

func TestWorktreeConfigIsReadOnlyWhenTheRepositoryEnablesIt(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "r")
	gitDir := filepath.Join(root, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "config.worktree"), []byte("[w]\n\tk = 1\n"), 0o600))
	env := Env{Environ: []string{"HOME=" + base, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull}}
	repo := Repo{GitDir: gitDir, CommonDir: gitDir}
	write := func(cfg string) {
		require.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte(cfg), 0o600))
	}

	write("[core]\n\tbare = false\n")
	res, err := Resolve(context.Background(), env, repo)
	require.NoError(t, err)
	_, ok := res.Lookup("w.k")
	assert.False(t, ok, "config.worktree is ignored without extensions.worktreeConfig")

	write("[extensions]\n\tworktreeConfig = true\n")
	res, err = Resolve(context.Background(), env, repo)
	require.NoError(t, err)
	e, ok := res.Lookup("w.k")
	require.True(t, ok)
	assert.Equal(t, ScopeWorktree, e.Scope)
}

func TestRemoteURLInsideAnIncludeIfFileFailsClosedWhenHasconfigRuns(t *testing.T) {
	env := fixtureHome(t, map[string]string{
		"bad.cfg": "[remote \"x\"]\n\turl = https://m.example/r\n",
		"t.cfg":   "[t]\n\tk = 1\n",
	})
	home := env[0][len("HOME="):]
	cfg := "[includeIf \"gitdir:/nowhere/\"]\n\tpath = " + filepath.Join(home, "bad.cfg") + "\n" +
		"[include]\n\tpath = " + filepath.Join(home, "plain.cfg") + "\n" +
		"[includeIf \"hasconfig:remote.*.url:x\"]\n\tpath = " + filepath.Join(home, "t.cfg") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(cfg), 0o600))
	// the gitdir condition is false here, so bad.cfg is not read and nothing is wrong
	_, err := Resolve(context.Background(), Env{Environ: env}, Repo{})
	require.NoError(t, err)

	// a true includeIf file (gitdir matches) that defines a remote URL is where git dies
	root := filepath.Join(home, "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o700))
	realHome, err := filepath.EvalSymlinks(home)
	require.NoError(t, err)
	cfg = strings.ReplaceAll(cfg, "gitdir:/nowhere/", "gitdir:"+filepath.Join(realHome, "repo")+"/")
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(cfg), 0o600))
	_, err = Resolve(context.Background(), Env{Environ: env}, Repo{GitDir: filepath.Join(realHome, "repo", ".git"), CommonDir: filepath.Join(realHome, "repo", ".git")})
	var ce *Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, SubRemoteURLInInclude, ce.Sub)
}
