//go:build gitoracle

package gitconfig

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/testutil/gitoracle"
)

// fixture is a repository with one commit and a linked worktree, under a fresh HOME. The
// repository sits inside HOME (home/work/repo) so `~/work/` patterns can match it.
type fixture struct {
	home, root, linked, linkedName string
	g                              gitoracle.Git
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	gitoracle.RequireGit(t)
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	f := fixture{home: home, root: filepath.Join(home, "work", "repo"), g: gitoracle.Git{T: t, Home: home}}
	require.NoError(t, os.MkdirAll(f.root, 0o700))
	f.g.Run(f.root, "init", "-q", "-b", "main", "--template=")
	f.g.Write(f.root, "a.txt", "1\n")
	f.g.Commit(f.root, "first")
	f.linkedName = "linked"
	f.linked = filepath.Join(home, "elsewhere", "linked")
	f.g.Run(f.root, "worktree", "add", "-q", "-b", "wt", f.linked)
	return f
}

// place returns the directory git runs in and the Repo the resolver is given for it.
func (f fixture) place(which string) (string, Repo) {
	common := filepath.Join(f.root, ".git")
	if which == "linked" {
		return f.linked, Repo{GitDir: filepath.Join(common, "worktrees", f.linkedName), CommonDir: common}
	}
	return f.root, Repo{GitDir: common, CommonDir: common}
}

func (f fixture) write(rel, content string) string {
	f.g.T.Helper()
	full := filepath.Join(f.home, rel)
	writeTestFile(f.g.T, full, content)
	return full
}

func (f fixture) appendLocal(text string) {
	f.g.T.Helper()
	p := filepath.Join(f.root, ".git", "config")
	data, err := os.ReadFile(p)
	require.NoError(f.g.T, err)
	require.NoError(f.g.T, os.WriteFile(p, append(data, text...), 0o600))
}

type scenario struct {
	name  string
	setup func(t *testing.T, f fixture) []string // returns extra environment entries
	// drop names environment variables removed from the hermetic base (GIT_CONFIG_NOSYSTEM ...).
	drop []string
	// places defaults to main and linked.
	places []string
	// wantSub means git must fail (unless gitOK) and the resolver must fail with this Sub.
	wantSub Sub
	// gitOK: git succeeds anyway; only the resolver is required to fail closed (rows git cannot
	// be compared on: unported glob, git crashing, inferred rows).
	gitOK bool
	// gitAny: do not constrain git's exit status at all.
	gitAny bool
}

func scenarioEnv(f fixture, sc scenario, extra []string) []string {
	var env []string
outer:
	for _, kv := range cleanEnv(f.home) {
		for _, d := range sc.drop {
			if strings.HasPrefix(kv, d+"=") {
				continue outer
			}
		}
		env = append(env, kv)
	}
	return append(env, extra...)
}

// vendorFiles are the system files git reads even with GIT_CONFIG_SYSTEM set (Apple git).
func vendorFiles(t testing.TB, f fixture) []string {
	t.Helper()
	env := append(cleanEnv(f.home), "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_GLOBAL=/dev/null")
	var kept []string
	for _, kv := range env {
		if !strings.HasPrefix(kv, "GIT_CONFIG_NOSYSTEM=") {
			kept = append(kept, kv)
		}
	}
	out, _, err := gitRun(t, f.home, kept, "config", "--list", "--show-origin", "--no-includes", "-z")
	require.NoError(t, err)
	var files []string
	for i, rec := range strings.Split(strings.TrimSuffix(out, "\x00"), "\x00") {
		if i%2 == 0 {
			if p, ok := strings.CutPrefix(rec, "file:"); ok && !contains(files, p) {
				files = append(files, p)
			}
		}
	}
	return files
}

func contains(l []string, s string) bool {
	for _, v := range l {
		if v == s {
			return true
		}
	}
	return false
}

// gitListing runs `git config --list --show-origin --show-scope --includes -z` and returns
// "scope|origin|key=value" rows with relative origins made absolute and Apple's "unknown"
// scope read as system.
func gitListing(t testing.TB, dir string, env []string) ([]string, error) {
	t.Helper()
	out, _, err := gitRun(t, dir, env, "config", "--list", "--show-origin", "--show-scope", "--includes", "-z")
	if err != nil {
		return nil, err
	}
	fields := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	if out == "" {
		return nil, nil
	}
	require.Zero(t, len(fields)%3, "unexpected -z shape: %q", out)
	var rows []string
	for i := 0; i < len(fields); i += 3 {
		scope, origin, kv := fields[i], fields[i+1], fields[i+2]
		if scope == "unknown" {
			scope = "system"
		}
		if p, ok := strings.CutPrefix(origin, "file:"); ok && !filepath.IsAbs(p) {
			origin = "file:" + filepath.Join(dir, p)
		}
		key, val, has := strings.Cut(kv, "\n")
		row := scope + "|" + origin + "|" + key
		if has {
			row += "=" + val
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func resolverListing(res *Result) []string {
	var rows []string
	for _, s := range res.Entries {
		origin := s.Origin
		if origin != CommandOrigin {
			origin = "file:" + origin
		}
		row := string(s.Scope) + "|" + origin + "|" + s.Name()
		if s.HasValue {
			row += "=" + s.Value
		}
		rows = append(rows, row)
	}
	return rows
}

func runScenario(t *testing.T, sc scenario) {
	t.Helper()
	f := newFixture(t)
	extra := sc.setup(t, f)
	env := scenarioEnv(f, sc, extra)
	places := sc.places
	if len(places) == 0 {
		places = []string{"main", "linked"}
	}
	for _, place := range places {
		dir, repo := f.place(place)
		res, rerr := Resolve(context.Background(), Env{Environ: env, SystemVendor: vendorFiles(t, f)}, repo)
		want, gerr := gitListing(t, dir, env)
		label := sc.name + "/" + place
		if sc.wantSub != "" {
			if !sc.gitOK && !sc.gitAny {
				assert.Error(t, gerr, "%s: git must reject this fixture", label)
			}
			var ce *Error
			if assert.ErrorAs(t, rerr, &ce, label) {
				assert.Equal(t, sc.wantSub, ce.Sub, label)
			}
			continue
		}
		require.NoError(t, gerr, "%s: git rejected a fixture the resolver must accept", label)
		require.NoError(t, rerr, label)
		assert.Equal(t, want, resolverListing(res), label)
	}
}

func TestResolverAgreesWithGit(t *testing.T) {
	for _, sc := range scenarios() {
		t.Run(sc.name, func(t *testing.T) {
			t.Parallel()
			runScenario(t, sc)
		})
	}
}

// requireNotRoot skips rows that depend on file permissions, which root ignores.
func requireNotRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("permission rows need a non-root user")
	}
}

var _ = exec.Command // keep os/exec imported for helpers in other oracle files
