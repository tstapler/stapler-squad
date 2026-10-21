package unfinished

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

// First user of raceEnabled: a TTL cache hit must stay allocation-free.
func TestHasUncommitted_ShouldNotAllocate_WhenResultIsWithinTTL(t *testing.T) {
	if raceEnabled {
		t.Skip("AllocsPerRun is not meaningful under -race")
	}
	repo, dir := testrepo.NewRepo(t)
	testrepo.Commit(t, repo, map[string]string{"a.txt": "a\n"}, "init", testrepo.Epoch)
	r := &GoGitVCSReader{}
	if _, err := r.HasUncommitted(dir); err != nil {
		t.Fatal(err)
	}
	if allocs := testing.AllocsPerRun(100, func() { _, _ = r.HasUncommitted(dir) }); allocs != 0 {
		t.Errorf("HasUncommitted cache hit allocs = %.0f, want 0", allocs)
	}
}
