//go:build !race

package unfinished

import (
	"context"
	"testing"

	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

func TestCommitMessages_ShouldAllocZero_WhenPrimedWithinTTL(t *testing.T) {
	_, dir, _ := testrepo.Diverged(t, 50, cmAhead, cmBehind)
	r := &GoGitVCSReader{}
	if _, err := r.CommitMessages(context.Background(), dir, "base", 20); err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(100, func() {
		if _, err := r.CommitMessages(context.Background(), dir, "base", 20); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Errorf("primed CommitMessages allocated %v times per call, want 0", allocs)
	}
}

func TestAheadBehind_ShouldAllocZero_WhenCachedWithinTTL(t *testing.T) {
	_, dir, _ := testrepo.Diverged(t, 50, cmAhead, cmBehind)
	r := &GoGitVCSReader{}
	if _, _, err := r.AheadBehind(context.Background(), dir, "base"); err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(100, func() {
		if _, _, err := r.AheadBehind(context.Background(), dir, "base"); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Errorf("cached AheadBehind allocated %v times per call, want 0", allocs)
	}
}
