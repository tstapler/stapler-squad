package prwatch

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeBacklogLinks scripts Pending: UUIDs in pending are pr_pending, UUIDs in
// failed fail their lookup, err fails the whole call.
type fakeBacklogLinks struct {
	pending map[string]bool
	failed  map[string]bool
	err     error
	asked   [][]string
}

func (f *fakeBacklogLinks) Pending(_ context.Context, uuids []string) (map[string]bool, map[string]bool, error) {
	f.asked = append(f.asked, append([]string(nil), uuids...))
	return f.pending, f.failed, f.err
}

func okFacts(uuid string) SessionFacts {
	return SessionFacts{UUID: uuid, Title: "t-" + uuid, Owner: "acme", Repo: "widget", PR: 7}
}

func compute(t *testing.T, links BacklogLinks, ignore []string, fs ...SessionFacts) map[string]SessionRef {
	t.Helper()
	refs := ComputeWatchSet(context.Background(), WatchSetInput{Sessions: fs, IgnoreRepos: ignore, Links: links, Now: time.Unix(100, 0)})
	out := map[string]SessionRef{}
	for _, r := range refs {
		out[r.UUID] = r
	}
	return out
}

func TestWatchSet_should_ExcludeSession_When_NewestLinkedItemIsPRPending(t *testing.T) {
	links := &fakeBacklogLinks{pending: map[string]bool{"a": true}}
	got := compute(t, links, nil, okFacts("a"), okFacts("b"))
	if got["a"].SkipReason != SkipBacklogPRPending {
		t.Fatalf("a skip = %q", got["a"].SkipReason)
	}
	if got["b"].SkipReason != "" || got["b"].Repo != "acme/widget" || got["b"].PR != 7 {
		t.Fatalf("b = %+v", got["b"])
	}
	if len(links.asked) != 1 {
		t.Fatalf("want one batched lookup, got %d", len(links.asked))
	}
}

func TestWatchSet_should_UnsubscribeAndNeverSteer_When_SessionBecomesLinkedToPRPendingItemAfterSubscribe(t *testing.T) {
	// The loop unsubscribes any session whose ref carries a SkipReason; this
	// asserts the watch set flips a previously watched session to skipped.
	links := &fakeBacklogLinks{}
	if r := compute(t, links, nil, okFacts("a"))["a"]; r.SkipReason != "" {
		t.Fatalf("first tick skip = %q", r.SkipReason)
	}
	links.pending = map[string]bool{"a": true}
	if r := compute(t, links, nil, okFacts("a"))["a"]; r.SkipReason != SkipBacklogPRPending {
		t.Fatalf("second tick skip = %q", r.SkipReason)
	}
}

func TestWatchSet_should_IncludeSession_When_LinkedItemIsInReviewOrInProgress(t *testing.T) {
	// Only pr_pending is reported by the port; review and in_progress links
	// come back as "not pending" and must be watched.
	links := &fakeBacklogLinks{pending: map[string]bool{}}
	got := compute(t, links, nil, okFacts("review"), okFacts("progress"))
	for u, r := range got {
		if r.SkipReason != "" {
			t.Fatalf("%s skipped: %q", u, r.SkipReason)
		}
	}
}

func TestWatchSet_should_IncludeSession_When_NewestLinkedItemIsDoneArchivedOrCustomStage(t *testing.T) {
	links := &fakeBacklogLinks{pending: map[string]bool{"x": false}}
	got := compute(t, links, nil, okFacts("done"), okFacts("archived"), okFacts("custom"))
	for u, r := range got {
		if r.SkipReason != "" {
			t.Fatalf("%s skipped: %q", u, r.SkipReason)
		}
	}
}

func TestWatchSet_should_ExcludeAndReportBacklogLookupFailed_When_ItemStatusLookupErrors(t *testing.T) {
	perSession := compute(t, &fakeBacklogLinks{failed: map[string]bool{"a": true}}, nil, okFacts("a"), okFacts("b"))
	if perSession["a"].SkipReason != SkipBacklogLookupFailed || perSession["b"].SkipReason != "" {
		t.Fatalf("per-session: a=%q b=%q", perSession["a"].SkipReason, perSession["b"].SkipReason)
	}
	whole := compute(t, &fakeBacklogLinks{err: errors.New("db down")}, nil, okFacts("a"), okFacts("b"))
	if whole["a"].SkipReason != SkipBacklogLookupFailed || whole["b"].SkipReason != SkipBacklogLookupFailed {
		t.Fatalf("whole-call failure must fail closed: %+v", whole)
	}
}

func TestWatchSet_should_ExcludeSession_When_OwnerRepoOrNumberMissingOrHostIsNotGitHubComOrForkOrHidden(t *testing.T) {
	mk := func(uuid string, mut func(*SessionFacts)) SessionFacts {
		f := okFacts(uuid)
		mut(&f)
		return f
	}
	got := compute(t, &fakeBacklogLinks{}, nil,
		mk("nopr", func(f *SessionFacts) { f.PR = 0 }),
		mk("noowner", func(f *SessionFacts) { f.Owner = "" }),
		mk("badrepo", func(f *SessionFacts) { f.Repo = "a/b" }),
		mk("ghe", func(f *SessionFacts) { f.Host = "ghe.example.com" }),
		mk("fork", func(f *SessionFacts) { f.Fork = true }),
		mk("hidden", func(f *SessionFacts) { f.Hidden = true }),
		mk("gone", func(f *SessionFacts) { f.Lifecycle = LifeGone }),
		mk("ok", func(f *SessionFacts) {}),
	)
	want := map[string]SkipReason{
		"nopr": SkipNoPR, "noowner": SkipNoOwnerRepo, "badrepo": SkipNoOwnerRepo, "ghe": SkipGHEHost,
		"fork": SkipFork, "hidden": SkipHidden, "gone": SkipNotLive, "ok": "",
	}
	for u, w := range want {
		if got[u].SkipReason != w {
			t.Errorf("%s: skip %q, want %q", u, got[u].SkipReason, w)
		}
	}
	if got["gone"].State != StateGone {
		t.Errorf("gone state = %q", got["gone"].State)
	}
}

func TestWatchSet_should_ExcludeAndReportIgnoreRepos_When_RepoMatchesCaseInsensitively(t *testing.T) {
	f := okFacts("a")
	f.Owner, f.Repo = "acme-corp", "widget-api"
	other := okFacts("b")
	links := &fakeBacklogLinks{}
	got := compute(t, links, []string{"Acme-Corp/Widget-API"}, f, other)
	if got["a"].SkipReason != SkipIgnoreRepos || got["b"].SkipReason != "" {
		t.Fatalf("a=%q b=%q", got["a"].SkipReason, got["b"].SkipReason)
	}
	if len(links.asked) != 1 || len(links.asked[0]) != 1 || links.asked[0][0] != "b" {
		t.Fatalf("backlog must only be asked about surviving sessions, got %v", links.asked)
	}
}

func TestWatchSet_should_KeepSuspendedSessionsWatched_When_PausedHibernatedOrCrashed(t *testing.T) {
	f := okFacts("a")
	f.Lifecycle = LifeSuspended
	r := compute(t, &fakeBacklogLinks{}, nil, f)["a"]
	if r.State != StateSuspended || r.SkipReason != "" {
		t.Fatalf("%+v", r)
	}
}

func TestWatchSet_should_SetResumedAt_When_SuspendedSessionBecomesLive(t *testing.T) {
	tr := NewResumeTracker()
	run := func(l Lifecycle, at int64) SessionRef {
		f := okFacts("a")
		f.Lifecycle = l
		return ComputeWatchSet(context.Background(), WatchSetInput{Sessions: []SessionFacts{f}, Tracker: tr, Now: time.Unix(at, 0)})[0]
	}
	if r := run(LifeLive, 1); !r.ResumedAt.IsZero() {
		t.Fatalf("live at startup must not be resumed: %v", r.ResumedAt)
	}
	run(LifeSuspended, 2)
	if r := run(LifeTransitional, 3); !r.ResumedAt.IsZero() {
		t.Fatalf("restoring is not yet resumed: %v", r.ResumedAt)
	}
	if r := run(LifeLive, 4); !r.ResumedAt.Equal(time.Unix(4, 0)) {
		t.Fatalf("ResumedAt = %v", r.ResumedAt)
	}
	if r := run(LifeLive, 5); !r.ResumedAt.Equal(time.Unix(4, 0)) {
		t.Fatalf("ResumedAt must stay at the resume time: %v", r.ResumedAt)
	}
	run(LifeSuspended, 6)
	if r := run(LifeSuspended, 7); !r.ResumedAt.IsZero() {
		t.Fatalf("suspended again clears ResumedAt: %v", r.ResumedAt)
	}
}

func TestWatchSet_should_StartSuspendedSessionUnresumed_When_FirstSeenSuspendedThenLive(t *testing.T) {
	tr := NewResumeTracker()
	f := okFacts("a")
	f.Lifecycle = LifeSuspended
	ComputeWatchSet(context.Background(), WatchSetInput{Sessions: []SessionFacts{f}, Tracker: tr})
	f.Lifecycle = LifeLive
	r := ComputeWatchSet(context.Background(), WatchSetInput{Sessions: []SessionFacts{f}, Tracker: tr, Now: time.Unix(9, 0)})[0]
	if r.ResumedAt.IsZero() {
		t.Fatal("suspended at first sight then live is a resume")
	}
}

func TestWatchSet_should_SanitizeTitle_When_ItHasControlCharsNewlinesOrIsLong(t *testing.T) {
	got := SanitizeTitle("  fix\x1b[31m\nthe\tbug‮  now ")
	if got != "fix[31m the bug now" {
		t.Fatalf("got %q", got)
	}
	long := SanitizeTitle(strings.Repeat("é", 200))
	if n := len([]rune(long)); n != maxTitleRunes {
		t.Fatalf("len %d", n)
	}
	if !strings.HasSuffix(long, "…") {
		t.Fatalf("no ellipsis: %q", long)
	}
}
