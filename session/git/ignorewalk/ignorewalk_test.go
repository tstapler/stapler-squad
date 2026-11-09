package ignorewalk

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type listRecorder struct {
	billy.Filesystem
	mu     sync.Mutex
	listed []string
}

func (l *listRecorder) ReadDir(p string) ([]os.FileInfo, error) {
	l.mu.Lock()
	l.listed = append(l.listed, p)
	l.mu.Unlock()
	return l.Filesystem.ReadDir(p)
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
}

// fixture mirrors the real shape: the root ignores web-app/node_modules, whose
// parent web-app has its own .gitignore (so go-git's per-directory check misses it).
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, ".gitignore", "/web-app/node_modules\nworktrees/\n*.log\n")
	write(t, root, "web-app/.gitignore", ".next\n")
	write(t, root, "web-app/src/.gitignore", "gen/\n")
	write(t, root, "web-app/node_modules/pkg/.gitignore", "dist\n")
	write(t, root, "web-app/node_modules/pkg/deep/x.js", "")
	write(t, root, ".claude/worktrees/w1/.gitignore", "secret\n")
	write(t, root, ".git/info/exclude", "local-only\n")
	return root
}

func TestReadPatterns_SkipsSubtreesIgnoredByAncestorPatterns(t *testing.T) {
	t.Parallel()
	root := fixture(t)
	rec := &listRecorder{Filesystem: osfs.New(root)}

	_, err := ReadPatterns(rec)
	require.NoError(t, err)

	for _, p := range rec.listed {
		assert.NotContains(t, p, "node_modules", "ignored subtree must not be listed")
		assert.NotContains(t, p, "worktrees", "ignored subtree must not be listed")
	}
}

func TestReadPatterns_MatchesGoGitOutsideIgnoredSubtrees(t *testing.T) {
	t.Parallel()
	root := fixture(t)

	got, err := ReadPatterns(osfs.New(root))
	require.NoError(t, err)
	want, err := gitignore.ReadPatterns(osfs.New(root), nil)
	require.NoError(t, err)

	gm, wm := gitignore.NewMatcher(got), gitignore.NewMatcher(want)
	for _, c := range []struct {
		path  []string
		isDir bool
	}{
		{[]string{"a.log"}, false},
		{[]string{"web-app", ".next"}, true},
		{[]string{"web-app", "src", "gen"}, true},
		{[]string{"web-app", "src", "keep.ts"}, false},
		{[]string{"local-only"}, false},
		{[]string{"web-app", "node_modules"}, true},
		{[]string{"x", "worktrees"}, true},
	} {
		assert.Equal(t, wm.Match(c.path, c.isDir), gm.Match(c.path, c.isDir), fmt.Sprint(c.path))
	}
}

func TestReadPatterns_MissingRootReturnsError(t *testing.T) {
	t.Parallel()
	_, err := ReadPatterns(osfs.New(filepath.Join(t.TempDir(), "absent")))
	assert.Error(t, err)
}

func BenchmarkReadPatterns(b *testing.B) {
	root := b.TempDir()
	rootIgnore := "/web-app/node_modules\n"
	require.NoError(b, os.WriteFile(filepath.Join(root, ".gitignore"), []byte(rootIgnore), 0o644))
	for i := 0; i < 300; i++ {
		require.NoError(b, os.MkdirAll(filepath.Join(root, "web-app", "node_modules", fmt.Sprintf("p%d", i), "lib"), 0o755))
	}
	require.NoError(b, os.MkdirAll(filepath.Join(root, "web-app", "src"), 0o755))
	require.NoError(b, os.WriteFile(filepath.Join(root, "web-app", ".gitignore"), []byte(".next\n"), 0o644))

	b.Run("gogit", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = gitignore.ReadPatterns(osfs.New(root), nil)
		}
	})
	b.Run("pruned", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = ReadPatterns(osfs.New(root))
		}
	})
}
