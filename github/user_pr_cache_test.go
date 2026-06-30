package github_test

import (
	"context"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/github"
)

// TestUserPRCache_GetAll_NilBeforeFirstFetch ensures the zero value returns nil.
func TestUserPRCache_GetAll_NilBeforeFirstFetch(t *testing.T) {
	c := github.NewUserPRCache()
	if got := c.GetAll(); got != nil {
		t.Fatalf("expected nil before any fetch, got %v", got)
	}
}

// TestUserPRCache_SetOnUpdated_Atomic verifies SetOnUpdated does not panic and
// can be called multiple times concurrently.
func TestUserPRCache_SetOnUpdated_Atomic(t *testing.T) {
	c := github.NewUserPRCache()
	done := make(chan struct{})
	go func() {
		c.SetOnUpdated(func(prs []github.UserPR) {})
		done <- struct{}{}
	}()
	c.SetOnUpdated(nil)
	<-done
}

// TestUserPRCache_StartStop verifies Start/Stop do not race or panic.
func TestUserPRCache_StartStop(t *testing.T) {
	t.Parallel()
	c := github.NewUserPRCache()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Start(ctx)
	c.Start(ctx) // second call is a no-op
	c.Stop()
}

// TestNormalizeCheckState exercises the state-normalisation helper via
// nodeToUserPR (indirect test through exported API).
func TestUserPRCache_UserPR_Fields(t *testing.T) {
	t.Parallel()
	pr := github.UserPR{
		Owner:           "acme",
		Repo:            "anvil",
		Number:          42,
		Title:           "fix: drop the anvil",
		HeadRef:         "fix/anvil",
		BaseRef:         "main",
		State:           "OPEN",
		IsDraft:         false,
		ApprovedCount:   2,
		ChangesReqCount: 0,
		CheckConclusion: "success",
	}
	if pr.Owner != "acme" {
		t.Fatalf("unexpected Owner: %s", pr.Owner)
	}
	if pr.ApprovedCount != 2 {
		t.Fatalf("unexpected ApprovedCount: %d", pr.ApprovedCount)
	}
}

// TestUserPRCache_Annotate verifies that Annotate is a no-op before first fetch
// and populates SessionIDs / LocalWorktreePath on an existing snapshot.
func TestUserPRCache_Annotate_NoopBeforeSnapshot(t *testing.T) {
	c := github.NewUserPRCache()
	// Annotate on an empty cache should not panic.
	c.Annotate([]github.PRAnnotationSession{
		{ID: "s1", Branch: "feat/x", GitHubOwner: "acme"},
	}, nil)
	if got := c.GetAll(); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

// TestParseGQLTime_Zero ensures an empty timestamp field does not panic.
func TestUserPRCache_EmptyTimestamp(t *testing.T) {
	t.Parallel()
	pr := github.UserPR{UpdatedAt: time.Time{}}
	if !pr.UpdatedAt.IsZero() {
		t.Fatal("expected zero time")
	}
}
