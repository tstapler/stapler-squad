package github

import (
	"sync"
	"testing"
	"time"
)

func TestAnnotate_should_LinkViaLegacyOnly_When_GHESessionHasUnsetStoredHost(t *testing.T) {
	t.Parallel()
	c := seedPRs(t, UserPR{Owner: "acme", Repo: "api", Number: 7, HeadRef: "fix", Host: "ghe.corp"})
	c.Annotate([]PRAnnotationSession{{ID: "ghe-unset", Branch: "fix", Repo: mustRef(t, "acme", "api", "")}}, nil)

	pr := c.GetAll()[0]
	if len(pr.LinkedSessions) != 1 || !pr.LinkedSessions[0].LegacyFallback {
		t.Fatalf("want one legacy-fallback link, got %+v", pr.LinkedSessions)
	}
}

func TestAnnotate_should_NotLinkAcrossKnownHost_When_SessionHostIsSetAndDiffers(t *testing.T) {
	t.Parallel()
	c := seedPRs(t, UserPR{Owner: "acme", Repo: "api", Number: 7, HeadRef: "fix", Host: "ghe.corp"})
	c.Annotate([]PRAnnotationSession{{ID: "other-host", Branch: "fix", Repo: mustRef(t, "acme", "api", "github.com")}}, nil)
	if got := c.GetAll()[0].LinkedSessions; len(got) != 0 {
		t.Fatalf("explicit different host must not link: %+v", got)
	}
}

func TestAnnotate_should_ResolveWorktreeViaLegacy_When_SessionsLinkedViaLegacy(t *testing.T) {
	t.Parallel()
	c := seedPRs(t, UserPR{Owner: "acme", Repo: "api", Number: 7, HeadRef: "fix", Host: "ghe.corp"})
	c.Annotate(
		[]PRAnnotationSession{{ID: "s", Branch: "fix", Repo: mustRef(t, "acme", "api", "")}},
		[]PRAnnotationWorktree{{Branch: "fix", Repo: mustRef(t, "acme", "api", ""), WorktreePath: "/wt/api"}},
	)
	if got := c.GetAll()[0].LocalWorktreePath; got != "/wt/api" {
		t.Fatalf("LocalWorktreePath = %q, want /wt/api", got)
	}

	// No legacy-linked session: a same-owner worktree must not attach.
	c2 := seedPRs(t, UserPR{Owner: "acme", Repo: "api", Number: 7, HeadRef: "fix", Host: "ghe.corp"})
	c2.Annotate(nil, []PRAnnotationWorktree{{Branch: "fix", Repo: mustRef(t, "acme", "api", ""), WorktreePath: "/wt/api"}})
	if got := c2.GetAll()[0].LocalWorktreePath; got != "" {
		t.Fatalf("LocalWorktreePath = %q, want empty", got)
	}
}

func TestAnnotate_should_KeepSessionIDsInMatchOrderAndSortOnlyLinkedSessions(t *testing.T) {
	t.Parallel()
	base := time.Unix(1000, 0)
	repo := mustRef(t, "acme", "api", "github.com")
	c := seedPRs(t, UserPR{Owner: "acme", Repo: "api", Number: 7, HeadRef: "fix", Host: "github.com"})
	c.Annotate([]PRAnnotationSession{
		{ID: "older", Branch: "fix", Repo: repo, LastActiveAt: base},
		{ID: "newer", Branch: "fix", Repo: repo, LastActiveAt: base.Add(time.Hour)},
	}, nil)

	pr := c.GetAll()[0]
	if pr.SessionIDs[0] != "older" || pr.SessionIDs[1] != "newer" {
		t.Fatalf("SessionIDs = %v, want insertion order [older newer]", pr.SessionIDs)
	}
	if pr.LinkedSessions[0].SessionID != "newer" {
		t.Fatalf("LinkedSessions[0] = %q, want most recently active", pr.LinkedSessions[0].SessionID)
	}
}

func TestPublish_should_ReapplyLatestAnnotationsAndFanOutAnnotated_When_FetchLandsAfterAnnotate(t *testing.T) {
	t.Parallel()
	repo := mustRef(t, "acme", "api", "github.com")
	c := seedPRs(t, UserPR{Owner: "acme", Repo: "api", Number: 7, HeadRef: "fix", Host: "github.com"})
	c.Annotate([]PRAnnotationSession{{ID: "s", Branch: "fix", Repo: repo}}, nil)
	ch := make(chan []UserPR, 1)
	c.Subscribe("t", ch)

	c.publish([]UserPR{{Owner: "acme", Repo: "api", Number: 7, HeadRef: "fix", Host: "github.com", Title: "fresh"}}, nil)

	got := <-ch
	if len(got[0].LinkedSessions) != 1 {
		t.Fatalf("fanned-out PR lost its links: %+v", got[0])
	}
	if snap := c.GetAll()[0]; snap.Title != "fresh" || len(snap.LinkedSessions) != 1 {
		t.Fatalf("snapshot = %+v, want fresh and linked", snap)
	}
}

// Run under -race: Annotate and publish interleave; the last store must always
// carry the newest fetch's data and the latest annotations.
func TestAnnotateAndPublish_should_NeverClobberNewerFetch_When_Interleaved(t *testing.T) {
	t.Parallel()
	repo := mustRef(t, "acme", "api", "github.com")
	c := seedPRs(t, UserPR{Owner: "acme", Repo: "api", Number: 7, HeadRef: "fix", Host: "github.com"})
	sessions := []PRAnnotationSession{{ID: "s", Branch: "fix", Repo: repo}}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); c.Annotate(sessions, nil) }()
		go func() {
			defer wg.Done()
			c.publish([]UserPR{{Owner: "acme", Repo: "api", Number: 7, HeadRef: "fix", Host: "github.com"}}, nil)
		}()
	}
	wg.Wait()
	c.publish([]UserPR{{Owner: "acme", Repo: "api", Number: 7, HeadRef: "fix", Host: "github.com", Title: "newest"}}, nil)
	c.Annotate(sessions, nil)
	if pr := c.GetAll()[0]; pr.Title != "newest" || len(pr.LinkedSessions) != 1 {
		t.Fatalf("final snapshot = %+v, want newest and linked", pr)
	}
}

func TestPublish_should_PullFreshAnnotationsFromSource_When_SourceRegistered(t *testing.T) {
	t.Parallel()
	repo := mustRef(t, "acme", "api", "github.com")
	c := NewUserPRCache()
	c.SetAnnotationSource(func() ([]PRAnnotationSession, []PRAnnotationWorktree) {
		return []PRAnnotationSession{{ID: "live", Branch: "fix", Repo: repo}}, nil
	})
	ch := make(chan []UserPR, 1)
	c.Subscribe("t", ch)

	c.publish([]UserPR{{Owner: "acme", Repo: "api", Number: 7, HeadRef: "fix", Host: "github.com"}}, nil)

	if got := <-ch; len(got[0].LinkedSessions) != 1 || got[0].LinkedSessions[0].SessionID != "live" {
		t.Fatalf("first fan-out not annotated: %+v", got[0])
	}
}
