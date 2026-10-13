package gitwiring

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/cli"
	"github.com/tstapler/stapler-squad/session/tmux"
)

var germanLocale = []string{"LANG=de_DE.UTF-8", "LC_ALL=de_DE.UTF-8", "LANGUAGE=de"}

func germanRunner() gitRunner {
	return gitRunner{environ: func() []string { return append([]string{"PATH=" + os.Getenv("PATH")}, germanLocale...) }}
}

func TestGitRunnerEnvForGitOverridesLocale(t *testing.T) {
	env := germanRunner().envFor("git")

	// os/exec keeps the last duplicate, so the required entries must come after the locale.
	last := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		last[k] = v
	}
	assert.Equal(t, "C", last["LC_ALL"])
	assert.Equal(t, "0", last["GIT_TERMINAL_PROMPT"])
	assert.Equal(t, env[len(env)-2:], backend.RequiredGitEnv())
}

func TestGitRunnerEnvForNonGitIsInherited(t *testing.T) {
	for _, name := range []string{"tmux", "ls", "/usr/bin/ssh", "gitk"} {
		assert.Nil(t, germanRunner().envFor(name), name)
	}
	assert.NotNil(t, germanRunner().envFor("/usr/bin/git"), "an absolute git path is still git")
}

// A stub git that speaks German unless LC_ALL=C, standing in for a machine with a German locale
// (a real git only translates when that locale is installed). Classification must still work.
func TestLocalClassificationSurvivesGermanLocale(t *testing.T) {
	installGermanUnlessCStubGit(t)

	_, err := cli.New(germanRunner()).RepoRoot(context.Background(), backend.Local{Root: backend.RepoRoot(t.TempDir())})
	assert.ErrorIs(t, err, backend.ErrNotARepo)
}

// NewRouter must upgrade a bare tmux.LocalRunner: with a German process locale, classification
// through the router only works if the env-applying runner was swapped in.
func TestNewRouterWithBareLocalRunnerClassifiesUnderGermanLocale(t *testing.T) {
	installGermanUnlessCStubGit(t)
	t.Setenv("LANG", "de_DE.UTF-8")
	t.Setenv("LC_ALL", "de_DE.UTF-8")

	_, err := NewRouter(backend.CohortMap{}, tmux.LocalRunner{}).RepoRoot(context.Background(), backend.Local{Root: backend.RepoRoot(t.TempDir())})

	assert.ErrorIs(t, err, backend.ErrNotARepo)
}

func installGermanUnlessCStubGit(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	stub := `#!/bin/sh
if [ "$LC_ALL" = "C" ]; then
  echo "fatal: not a git repository (or any of the parent directories): .git" >&2
else
  echo "fatal: kein Git-Repository (oder irgendein Elternverzeichnis): .git" >&2
fi
exit 128
`
	require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte(stub), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// Smoke test with real git and the locale forced German. Not discriminating where that locale is
// not installed (git then speaks English anyway); the stub-git test above is the one that bites.
func TestLocalClassificationRealGitSmokeUnderForcedLocale(t *testing.T) {
	dir := t.TempDir()
	r := gitRunner{environ: func() []string {
		return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "GIT_CEILING_DIRECTORIES=" + filepath.Dir(dir)}, germanLocale...)
	}}
	b := cli.New(r)

	_, err := b.RepoRoot(context.Background(), backend.Local{Root: backend.RepoRoot(dir)})
	assert.ErrorIs(t, err, backend.ErrNotARepo)
}

func TestWithGitEnvUpgradesOnlyBareLocalRunner(t *testing.T) {
	assert.IsType(t, gitRunner{}, withGitEnv(tmux.LocalRunner{}))
	custom := &fakeLocal{}
	assert.Same(t, custom, withGitEnv(custom))
}
