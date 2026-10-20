package unfinished

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

func TestAheadBehindFrom_ShouldMatchAheadBehind_WhenParentMemoIsTheSource(t *testing.T) {
	_, dir, h := testrepo.Diverged(t, 200, cmAhead, cmBehind)
	r := &GoGitVCSReader{}
	entry, err := r.openRepoEntry(dir)
	if err != nil {
		t.Fatal(err)
	}

	entry.mu.Lock()
	head, base, err := resolveAheadBehindRefs(entry, "base")
	entry.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if head != h.Head || base != h.Base {
		t.Fatalf("resolveAheadBehindRefs = (%s, %s), want (%s, %s)", head, base, h.Head, h.Base)
	}

	entry.mu.Lock()
	ahead, behind, err := aheadBehindFrom(newParentMemo(entry.repo), head, base)
	entry.mu.Unlock()
	if err != nil || ahead != cmAhead || behind != cmBehind {
		t.Fatalf("aheadBehindFrom = (%d, %d, %v), want (%d, %d, nil)", ahead, behind, err, cmAhead, cmBehind)
	}
	gotA, gotB, err := r.AheadBehind(dir, "base")
	if err != nil || gotA != ahead || gotB != behind {
		t.Fatalf("AheadBehind = (%d, %d, %v), want (%d, %d, nil)", gotA, gotB, err, ahead, behind)
	}
}
