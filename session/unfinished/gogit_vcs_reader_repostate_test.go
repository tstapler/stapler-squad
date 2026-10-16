package unfinished

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/unfinished/gogitstore"
	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

// The reader's repos must expose the store's canonical key: one string for a
// main checkout, a symlinked route to it, and a linked worktree.
func TestRepoKey_ShouldBeCanonical_WhenOpenedThroughReaderViaMainSymlinkAndLinked(t *testing.T) {
	t.Parallel()
	repo, main := testrepo.NewRepo(t)
	testrepo.Commit(t, repo, map[string]string{"a.txt": "a"}, "c1", testrepo.Epoch.Add(time.Minute))
	linked := testrepo.NewLinkedWorktree(t, main, "wt1", "wt-1")
	link := testrepo.Symlink(t, main)

	g := &GoGitVCSReader{}
	want := filepath.Join(main, ".git")
	for name, path := range map[string]string{"main": main, "symlink": link, "linked": linked} {
		entry, err := g.openRepoEntry(path)
		require.NoError(t, err, name)
		ws, ok := entry.repo.Storer.(*gogitstore.WorktreeStorer)
		require.True(t, ok, name)
		assert.Equal(t, want, ws.CommonDirKey(), name)
	}
	assert.Len(t, g.gogitstoreRegistry().RefCounts(), 1, "one registry entry for all three routes")
}
