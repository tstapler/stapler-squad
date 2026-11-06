//go:build gitoracle

package gogit_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/cli"
	"github.com/tstapler/stapler-squad/session/git/backend/gogit"
	"github.com/tstapler/stapler-squad/testutil/gitoracle"
)

// envRunner runs real git with an explicit environment, so a scenario controls the global and
// system config sources the shared gitoracle runner pins to /dev/null.
type envRunner struct{ environ []string }

func (r envRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := safeexec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Env = dir, r.environ
	return cmd.CombinedOutput()
}

func (r envRunner) RunStdout(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := safeexec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Env = dir, r.environ
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		ee.Stderr = stderr.Bytes()
	}
	return stdout.Bytes(), err
}

// scenarioEnv is gitoracle.Env(home) without the variables a scenario sets itself.
func scenarioEnv(home string, drop []string, add ...string) []string {
	var env []string
outer:
	for _, kv := range gitoracle.Env(home) {
		for _, d := range drop {
			if strings.HasPrefix(kv, d+"=") {
				continue outer
			}
		}
		env = append(env, kv)
	}
	return append(env, add...)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

type configScenario struct {
	name string
	// setup writes config files under home and repo and returns the environment for both backends.
	setup func(t *testing.T, home, repo string) []string
	keys  []string
	// fallback reports keys the in-process reader must hand to the CLI.
	fallback func(key string) bool
}

// neverFallback: only a key with no section, which the reader leaves to git, may fall back.
func neverFallback(key string) bool { return key == "nodot" }

func configScenarios() []configScenario {
	plain := []string{"user.name", "user.email", "core.editor", "core.foobar", "inc.value", "remote.origin.url", "x.y", "missing.key", "nodot", "a.b.c.d", "core.bare", "branch.main.remote"}
	return []configScenario{
		{name: "global-and-local-precedence", keys: plain, fallback: neverFallback, setup: func(t *testing.T, home, repo string) []string {
			writeFile(t, filepath.Join(home, ".gitconfig"), "[user]\n\tname = Global\n\temail = g@example.com\n[core]\n\teditor = vim\n")
			writeFile(t, filepath.Join(home, "xdg", "git", "config"), "[user]\n\tname = Xdg\n[x]\n\ty = from-xdg\n")
			appendLocal(t, repo, "[user]\n\tname = Local\n[remote \"origin\"]\n\turl = u\n")
			return scenarioEnv(home, []string{"GIT_CONFIG_GLOBAL"})
		}},
		{name: "global-env-replaces-user-files", keys: plain, fallback: neverFallback, setup: func(t *testing.T, home, repo string) []string {
			writeFile(t, filepath.Join(home, ".gitconfig"), "[user]\n\tname = Ignored\n")
			writeFile(t, filepath.Join(home, "elsewhere"), "[user]\n\tname = FromEnvFile\n\temail = e@example.com\n")
			return scenarioEnv(home, []string{"GIT_CONFIG_GLOBAL"}, "GIT_CONFIG_GLOBAL="+filepath.Join(home, "elsewhere"))
		}},
		{name: "system-file-lowest-precedence", keys: plain, fallback: neverFallback, setup: func(t *testing.T, home, repo string) []string {
			sys := filepath.Join(home, "system.cfg")
			writeFile(t, sys, "[user]\n\tname = System\n\temail = s@example.com\n[core]\n\teditor = nano\n")
			writeFile(t, filepath.Join(home, ".gitconfig"), "[core]\n\teditor = vim\n")
			return scenarioEnv(home, []string{"GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM"}, "GIT_CONFIG_SYSTEM="+sys)
		}},
		{name: "nosystem-ignores-system-file", keys: plain, fallback: neverFallback, setup: func(t *testing.T, home, repo string) []string {
			sys := filepath.Join(home, "system.cfg")
			writeFile(t, sys, "[user]\n\tname = System\n")
			return scenarioEnv(home, []string{"GIT_CONFIG_GLOBAL"}, "GIT_CONFIG_SYSTEM="+sys, "GIT_CONFIG_NOSYSTEM=1")
		}},
		{name: "includes-unconditional-and-conditional", keys: append([]string{"cond.secret", "inc.deep", "inc.override", "user.signingkey"}, plain...), setup: func(t *testing.T, home, repo string) []string {
			writeFile(t, filepath.Join(home, "inc.cfg"), "[inc]\n\tvalue = from-home-include\n\toverride = inc\n[include]\n\tpath = nested/deep.cfg\n")
			writeFile(t, filepath.Join(home, "nested", "deep.cfg"), "[inc]\n\tdeep = nested-value\n")
			writeFile(t, filepath.Join(home, "cond.cfg"), "[cond]\n\tsecret = only-here\n[user]\n\tsigningkey = KEY\n")
			writeFile(t, filepath.Join(home, ".gitconfig"), "[inc]\n\toverride = first\n[include]\n\tpath = ~/inc.cfg\n[inc]\n\toverride = last\n"+
				"[includeIf \"gitdir:"+repo+"/\"]\n\tpath = "+filepath.Join(home, "cond.cfg")+"\n"+
				"[includeIf \"hasconfig:remote.*.url:*example.com*\"]\n\tpath = "+filepath.Join(home, "nonexistent.cfg")+"\n")
			return scenarioEnv(home, []string{"GIT_CONFIG_GLOBAL"})
		}, fallback: func(k string) bool { return k == "cond.secret" || k == "user.signingkey" || neverFallback(k) }},
		{name: "include-loop-is-a-git-error", keys: []string{"a.k", "loop.k"}, fallback: func(string) bool { return true }, setup: func(t *testing.T, home, repo string) []string {
			writeFile(t, filepath.Join(home, "loop.cfg"), "[include]\n\tpath = loop.cfg\n[loop]\n\tk = looped\n")
			writeFile(t, filepath.Join(home, ".gitconfig"), "[a]\n\tk = 1\n[include]\n\tpath = loop.cfg\n")
			return scenarioEnv(home, []string{"GIT_CONFIG_GLOBAL"})
		}},
		{name: "include-edge-cases", keys: []string{"a.k", "b.k", "rel.k", "tilde.k"}, fallback: neverFallback, setup: func(t *testing.T, home, repo string) []string {
			writeFile(t, filepath.Join(home, "loop.cfg"), "[include]\n\tpath = loop.cfg\n[loop]\n\tk = looped\n")
			writeFile(t, filepath.Join(home, "rel.cfg"), "[rel]\n\tk = relative\n")
			writeFile(t, filepath.Join(home, ".gitconfig"), "[a]\n\tk = 1\n[include]\n\tpath = missing-file.cfg\n[include]\n\tpath = rel.cfg\n[b]\n\tk = 2\n")
			return scenarioEnv(home, []string{"GIT_CONFIG_GLOBAL"})
		}},
		{name: "value-syntax", keys: []string{"val.quoted", "val.tab", "val.nl", "val.empty", "val.bare", "val.spaced", "val.multi", "val.bs", "val.inner", "VAL.QUOTED", "val.Quoted", "sec.sub.key", "sec.SUB.key", "sec.key", "SEC.KEY"}, fallback: neverFallback, setup: func(t *testing.T, home, repo string) []string {
			writeFile(t, filepath.Join(home, ".gitconfig"), "[val]\n\tquoted = \"a # b\" ; comment\n\ttab = x\\ty\n\tnl = l1\\nl2\n\tempty =\n\tbare\n\tspaced =   padded   \n\tmulti = one\n\tmulti = two\n\tbs = back\\\\slash\n\tinner = pre \"quoted\" post\n[sec \"sub\"]\n\tkey = lower-sub\n[SEC]\n\tKey = upper-section\n")
			return scenarioEnv(home, []string{"GIT_CONFIG_GLOBAL"})
		}},
		{name: "line-continuation-is-left-to-git", keys: []string{"val.cont", "val.other"}, fallback: func(string) bool { return true }, setup: func(t *testing.T, home, repo string) []string {
			writeFile(t, filepath.Join(home, ".gitconfig"), "[val]\n\tother = x\n\tcont = a\\\n b\n")
			return scenarioEnv(home, []string{"GIT_CONFIG_GLOBAL"})
		}},
		{name: "invalid-syntax-is-left-to-git", keys: []string{"ok.k", "user.name"}, fallback: func(string) bool { return true }, setup: func(t *testing.T, home, repo string) []string {
			writeFile(t, filepath.Join(home, ".gitconfig"), "[ok]\n\tk = 1\n[broken\n\tx = y\n")
			return scenarioEnv(home, []string{"GIT_CONFIG_GLOBAL"})
		}},
		{name: "unknown-escape-is-left-to-git", keys: []string{"esc.k", "esc.ok"}, fallback: func(string) bool { return true }, setup: func(t *testing.T, home, repo string) []string {
			writeFile(t, filepath.Join(home, ".gitconfig"), "[esc]\n\tk = bad\\qescape\n\tok = fine\n")
			return scenarioEnv(home, []string{"GIT_CONFIG_GLOBAL"})
		}},
		{name: "legacy-dotted-section-is-left-to-git", keys: []string{"remote.origin.url", "remote.x.url"}, fallback: func(string) bool { return true }, setup: func(t *testing.T, home, repo string) []string {
			writeFile(t, filepath.Join(home, ".gitconfig"), "[remote.origin]\n\turl = legacy\n")
			return scenarioEnv(home, []string{"GIT_CONFIG_GLOBAL"})
		}},
	}
}

func appendLocal(t *testing.T, repo, text string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(repo, ".git", "config"), os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = f.WriteString(text)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

// configDiffs compares cand with the CLI over every key of the scenario in a repository, a linked
// worktree and a non-repository, and returns the mismatches that are not documented fallbacks.
func configDiffs(t *testing.T, sc configScenario, cand func(env []string) backend.Backend) []string {
	t.Helper()
	home := t.TempDir()
	repoRoot, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	g := gitoracle.Git{T: t, Home: home}
	g.Run(repoRoot, "init", "-q", "-b", "main", "--template=")
	g.Write(repoRoot, "a.txt", "1\n")
	g.Commit(repoRoot, "first")
	linked := filepath.Join(repoRoot, "..", filepath.Base(repoRoot)+"-linked")
	g.Run(repoRoot, "worktree", "add", "-q", "-b", "wt", linked)
	nonRepo := t.TempDir()
	env := sc.setup(t, home, repoRoot)

	oracle := cli.New(envRunner{environ: env})
	candidate := cand(env)
	var out []string
	for name, dir := range map[string]string{"repo": repoRoot, "linked": linked, "non-repo": nonRepo} {
		for _, key := range sc.keys {
			loc := backend.Local{Root: backend.RepoRoot(dir)}
			want, wantErr := oracle.GetConfig(context.Background(), loc, backend.ConfigKey(key))
			got, gotErr := candidate.GetConfig(context.Background(), loc, backend.ConfigKey(key))
			label := fmt.Sprintf("%s/%s %s", sc.name, name, key)
			switch {
			case errors.Is(gotErr, backend.ErrFallbackEligible{}):
				if !sc.fallback(key) {
					out = append(out, fmt.Sprintf("%s: unexpected fallback: %v", label, gotErr))
				}
			case gitoracle.ErrorClass(wantErr) != gitoracle.ErrorClass(gotErr):
				out = append(out, fmt.Sprintf("%s: oracle err %v, candidate err %v", label, wantErr, gotErr))
			case want != got:
				out = append(out, fmt.Sprintf("%s: oracle %q, candidate %q", label, want, got))
			}
		}
	}
	return out
}

func TestParityGetConfig(t *testing.T) {
	for _, sc := range configScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			t.Parallel()
			diffs := configDiffs(t, sc, func(env []string) backend.Backend {
				return gogit.New(gogit.WithEnviron(func() []string { return env }))
			})
			require.Empty(t, diffs)
		})
	}
}
