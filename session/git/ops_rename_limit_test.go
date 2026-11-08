package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// renameFixture commits `dels` files under a/, then replaces them with `adds` files under b/.
// The first min(adds,dels) new files are content renames of old ones (one edited line, so not
// exact); with exact=true they are byte-identical instead. Basenames differ so git's
// basename-guess pre-pass (not implemented in the fork) cannot change the outcome.
func renameFixture(t *testing.T, adds, dels int, exact bool) (work, baseSHA, headSHA string) {
	t.Helper()
	origin := setupTestRepo(t)
	work = cloneTestRepo(t, origin)
	body := func(i int) string {
		return strings.Repeat(fmt.Sprintf("file %d unique line\n", i), 8)
	}
	require.NoError(t, os.MkdirAll(filepath.Join(work, "a"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(work, "b"), 0o755))
	for i := 0; i < dels; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(work, "a", fmt.Sprintf("f%04d.txt", i)), []byte(body(i)), 0o644))
	}
	runGit(t, work, "add", "a")
	runGit(t, work, "commit", "-m", "add files")
	baseSHA = strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	for i := 0; i < dels; i++ {
		require.NoError(t, os.Remove(filepath.Join(work, "a", fmt.Sprintf("f%04d.txt", i))))
	}
	for i := 0; i < adds; i++ {
		content := body(i)
		if !exact {
			content += "one edited line\n"
		}
		require.NoError(t, os.WriteFile(filepath.Join(work, "b", fmt.Sprintf("g%04d.txt", i)), []byte(content), 0o644))
	}
	runGit(t, work, "add", "-A")
	runGit(t, work, "commit", "-m", "move")
	headSHA = strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	return work, baseSHA, headSHA
}

// gitStdout is runGit without stderr: git prints its "exhaustive rename detection was skipped" warning there.
func gitStdout(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := safeexec.CommandContext(context.Background(), "git", append([]string{"-C", dir}, args...)...).Output()
	require.NoError(t, err)
	return string(out)
}

// gitCLIStat returns what `git diff --numstat -M` and `--name-status -M` report: the rename count
// and the added/deleted line totals.
func gitCLIStat(t *testing.T, work, baseSHA, headSHA string) (renames, additions, deletions int) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(gitStdout(t, work, "diff", "--numstat", "-M", baseSHA, headSHA)), "\n") {
		f := strings.Split(line, "\t")
		require.Len(t, f, 3, line)
		a, err := strconv.Atoi(f[0])
		require.NoError(t, err)
		d, err := strconv.Atoi(f[1])
		require.NoError(t, err)
		additions += a
		deletions += d
	}
	for _, line := range strings.Split(strings.TrimSpace(gitStdout(t, work, "diff", "--name-status", "-M", baseSHA, headSHA)), "\n") {
		if strings.HasPrefix(line, "R") {
			renames++
		}
	}
	return renames, additions, deletions
}

// FileStatsBetween must report what `git diff -M` reports, including at the boundary of git's
// rename limit (product rule: skip when dst*src > limit^2, or both exceed limit) and for
// exact renames, which are always detected. See session/git/AGENTS.md.
func TestFileStatsBetween_ShouldMatchGitCLI_AroundRenameLimit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		adds, dels int
		limit      int
		exact      bool
	}{
		{"product at limit squared is detected", 3, 3, 3, false},
		{"both sides above limit is skipped", 4, 4, 3, false},
		{"one side within limit but product above is skipped", 2, 5, 3, false},
		{"one add against many deletes stays detected", 1, 8, 3, false},
		{"exact renames above the limit are still detected", 6, 6, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			work, base, head := renameFixture(t, tc.adds, tc.dels, tc.exact)
			runGit(t, work, "config", "diff.renameLimit", strconv.Itoa(tc.limit))

			wantRenames, wantAdds, wantDels := gitCLIStat(t, work, base, head)
			stats, err := FileStatsBetween(work, base, head)
			require.NoError(t, err)

			var gotRenames, gotAdds, gotDels int
			for _, s := range stats {
				if s.Status == "renamed" {
					gotRenames++
				}
				gotAdds += s.Additions
				gotDels += s.Deletions
			}
			assert.Equal(t, wantRenames, gotRenames, "renames vs git diff -M")
			assert.Equal(t, wantAdds, gotAdds, "additions vs git diff --numstat -M")
			assert.Equal(t, wantDels, gotDels, "deletions vs git diff --numstat -M")
		})
	}
}

func TestGitRenameLimit_ShouldHonorDiffRenameLimitConfig(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		config string // empty means unset
		want   uint
	}{
		{"unset uses git's default of 1000", "", gitDefaultRenameLimit},
		{"configured value is used", "50", 50},
		{"zero means git's unlimited sentinel", "0", gitUnlimitedRenameLimit},
		{"unparseable falls back to the default", "lots", gitDefaultRenameLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			work := cloneTestRepo(t, setupTestRepo(t))
			if tc.config != "" {
				runGit(t, work, "config", "diff.renameLimit", tc.config)
			}
			repo, err := OpenRepo(work)
			require.NoError(t, err)
			assert.Equal(t, tc.want, gitRenameLimit(repo))
		})
	}
}
