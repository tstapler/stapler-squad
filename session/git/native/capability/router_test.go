package capability

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

// served records which backend answered a Commit.
type served struct {
	backend.Backend // any other method panics on the nil interface
	name            string
	log             *[]string
}

func (s served) Commit(context.Context, backend.RepoLocation, backend.CommitRequest) error {
	*s.log = append(*s.log, s.name)
	return nil
}

// TestRouterSendsHookedSignedAndPlainReposWhereADR006Says drives the real Router with the real
// Detector: a hooked or signing repository is served by the CLI, a plain one in-process.
func TestRouterSendsHookedSignedAndPlainReposWhereADR006Says(t *testing.T) {
	cases := map[string]struct {
		setup func(f *fx)
		want  string
	}{
		"plain":           {func(f *fx) {}, "gogit"},
		"executable hook": {func(f *fx) { f.hook("pre-commit", 0o755) }, "cli"},
		"sample hook":     {func(f *fx) { f.hook("pre-commit.sample", 0o755) }, "gogit"},
		"commit.gpgsign":  {func(f *fx) { f.appendConfig("[commit]\n\tgpgsign = true\n") }, "cli"},
		"unreadable repo config include": {func(f *fx) {
			f.appendConfig("[include]\n\tpath = " + filepath.Join(f.home, "locked.cfg") + "\n")
			f.write(filepath.Join(f.home, "locked.cfg"), "x")
			require.NoError(f.t, os.Chmod(filepath.Join(f.home, "locked.cfg"), 0))
		}, "cli"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if name == "unreadable repo config include" && os.Geteuid() == 0 {
				t.Skip("permission rows need a non-root user")
			}
			f := newFx(t)
			c.setup(f)
			var log []string
			cohorts := backend.CohortMap{}.With(backend.CohortLocalWrite, backend.BackendGoGit)
			r, err := backend.NewRouter(backend.RouterConfig{
				Cohorts: cohorts, CLI: served{name: "cli", log: &log}, GoGit: served{name: "gogit", log: &log},
				LocalWriteRepos: []backend.RepoRoot{backend.RepoRoot(f.root)},
				LiveSessions:    func(string) bool { return false },
				Capabilities:    f.detector(),
				// Commit is not a verified worktree writer by default; it does not write files.
			})
			require.NoError(t, err)
			require.NoError(t, r.Commit(context.Background(), backend.Local{Root: backend.RepoRoot(f.root)}, backend.CommitRequest{}))
			assert.Equal(t, []string{c.want}, log)
		})
	}
}
