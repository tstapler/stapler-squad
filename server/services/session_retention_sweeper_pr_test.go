package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/config"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// fakeSweepPRSource wraps the real storage and lets a test replace or break any
// of the three bulk reads the sweeper makes once per cycle.
type fakeSweepPRSource struct {
	real     sweepPRSource
	views    func(map[string][]domain.PRView) (map[string][]domain.PRView, error)
	itemIDs  func(map[string][]string) (map[string][]string, error)
	guards   func(map[string]session.ItemGuardView) (map[string]session.ItemGuardView, error)
	viewCall int
}

func (f *fakeSweepPRSource) ListSessionPRViewsByUUIDs(ctx context.Context, uuids []string) (map[string][]domain.PRView, error) {
	f.viewCall++
	got, err := f.real.ListSessionPRViewsByUUIDs(ctx, uuids)
	if err != nil || f.views == nil {
		return got, err
	}
	return f.views(got)
}

func (f *fakeSweepPRSource) ListItemIDsBySessionUUIDs(ctx context.Context, uuids []string) (map[string][]string, error) {
	got, err := f.real.ListItemIDsBySessionUUIDs(ctx, uuids)
	if err != nil || f.itemIDs == nil {
		return got, err
	}
	return f.itemIDs(got)
}

func (f *fakeSweepPRSource) ListItemGuardViewsByItemIDs(ctx context.Context, ids []string) (map[string]session.ItemGuardView, error) {
	got, err := f.real.ListItemGuardViewsByItemIDs(ctx, ids)
	if err != nil || f.guards == nil {
		return got, err
	}
	return f.guards(got)
}

func newPRSweeper(fix *forkTestFixture) (*SessionRetentionSweeper, *fakeSweepPRSource) {
	sweeper := NewSessionRetentionSweeper(fix.storage, config.DefaultConfig(), fix.svc)
	fake := &fakeSweepPRSource{real: sweeper.prSource}
	sweeper.prSource = fake
	// Never reach GitHub from a test: closed PRs stay closed unless a test says otherwise.
	sweeper.reverifyClosed = func(context.Context, domain.PRRef) (bool, error) { return false, nil }
	return sweeper, fake
}

func oldArchive() time.Time { return time.Now().AddDate(0, 0, -20) }

// addArchivedWithUUID adds an archived session past retention with a fixed uuid.
func addArchivedWithUUID(t *testing.T, fix *forkTestFixture, title, uuid string, opts ...func(*session.Instance)) {
	t.Helper()
	addArchivedInstance(t, fix, title, oldArchive(), append([]func(*session.Instance){func(i *session.Instance) { i.UUID = uuid }}, opts...)...)
}

// linkPR links number to the session. State rows are shared by PR identity, so
// each test uses PR numbers no other test session in the same fixture uses.
func linkPR(t *testing.T, fix *forkTestFixture, uuid string, number int, state domain.PRState, terminal bool) {
	t.Helper()
	linkPRKeepingMirror(t, fix, uuid, number, state, terminal, session.PRMirror{})
}

func linkPRKeepingMirror(t *testing.T, fix *forkTestFixture, uuid string, number int, state domain.PRState, terminal bool, mirror session.PRMirror) {
	t.Helper()
	ref, err := domain.NewPRRef("", "acme", "app", number)
	require.NoError(t, err)
	_, err = fix.storage.AttachSessionPR(context.Background(), uuid, ref,
		session.PRLink{Role: domain.PRRolePrimary, Source: domain.AttachSourceManual, Initial: session.PRInitial{State: state, Terminal: terminal}},
		mirror)
	require.NoError(t, err)
}

func TestRetentionSweeper_ShouldNotDelete_WhenOneLinkedPROpen(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addArchivedWithUUID(t, fix, "mixed", "u-mixed")
	linkPR(t, fix, "u-mixed", 11, domain.PRStateMerged, true)
	linkPR(t, fix, "u-mixed", 12, domain.PRStateOpen, false)
	addArchivedWithUUID(t, fix, "both-terminal", "u-done")
	linkPR(t, fix, "u-done", 21, domain.PRStateMerged, true)
	linkPR(t, fix, "u-done", 22, domain.PRStateClosed, true)

	sweeper, _ := newPRSweeper(fix)
	sweeper.sweep(context.Background())

	assert.True(t, hasStoredTitle(t, fix, "mixed"), "one open linked PR keeps the session")
	assert.False(t, hasStoredTitle(t, fix, "both-terminal"), "all-terminal links release the session")
}

func TestRetentionSweeper_ShouldFailClosed_WhenLegacyNumberAndNoLinkRows(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addArchivedWithUUID(t, fix, "flag-false", "u-f", func(i *session.Instance) { i.GitHubPRNumber, i.GitHubPRStatusTerminal = 9, false })
	addArchivedWithUUID(t, fix, "flag-true", "u-t", func(i *session.Instance) { i.GitHubPRNumber, i.GitHubPRStatusTerminal = 9, true })

	sweeper, _ := newPRSweeper(fix)
	sweeper.sweep(context.Background())

	assert.True(t, hasStoredTitle(t, fix, "flag-false"), "legacy number, flag false, no rows: not eligible")
	assert.False(t, hasStoredTitle(t, fix, "flag-true"), "eligible only through the persisted legacy flag")
}

func TestRetentionSweeper_ShouldFailClosed_WhenUnknownOrUnverifiedTerminal(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addArchivedWithUUID(t, fix, "unknown", "u-unk")
	linkPR(t, fix, "u-unk", 12, domain.PRStateUnknown, false)
	addArchivedWithUUID(t, fix, "unverified", "u-unv")
	linkPR(t, fix, "u-unv", 13, domain.PRStateUnknown, true)
	addArchivedWithUUID(t, fix, "exhausted", "u-exh")
	linkPR(t, fix, "u-exh", 14, domain.PRStateUnknown, true)

	sweeper, _ := newPRSweeper(fix)
	sweeper.sweep(context.Background())
	assert.True(t, hasStoredTitle(t, fix, "unknown"))
	assert.True(t, hasStoredTitle(t, fix, "unverified"), "a stale legacy flag must not make an unpolled PR look safe")
	assert.True(t, hasStoredTitle(t, fix, "exhausted"), "not exhausted yet")

	start := time.Now().Add(-2 * time.Hour)
	sweeper.prSource.(*fakeSweepPRSource).views = func(m map[string][]domain.PRView) (map[string][]domain.PRView, error) {
		for _, v := range m["u-exh"] {
			v.PollFailCount, v.FirstFailAt, v.LastPolledAt = 5, start, start.Add(time.Hour)
			m["u-exh"] = []domain.PRView{v}
		}
		return m, nil
	}
	sweeper.sweep(context.Background())
	assert.False(t, hasStoredTitle(t, fix, "exhausted"), "released once verification is exhausted")
	assert.True(t, hasStoredTitle(t, fix, "unknown"))
	assert.True(t, hasStoredTitle(t, fix, "unverified"))
}

func TestRetentionSweeper_ShouldFailClosed_WhenItemDirectPROpen(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addArchivedWithUUID(t, fix, "worker", "u-w")
	linkPR(t, fix, "u-w", 11, domain.PRStateMerged, true)
	sweeper, fake := newPRSweeper(fix)
	fake.itemIDs = func(map[string][]string) (map[string][]string, error) {
		return map[string][]string{"u-w": {"item-x", "item-y"}}, nil
	}
	merged := domain.PRView{State: domain.PRStateMerged, StoredTerminal: true}
	open := domain.PRView{State: domain.PRStateOpen}
	fake.guards = func(map[string]session.ItemGuardView) (map[string]session.ItemGuardView, error) {
		return map[string]session.ItemGuardView{
			"item-x": {Direct: []domain.PRView{merged}},
			"item-y": {Direct: []domain.PRView{open}},
		}, nil
	}
	sweeper.sweep(context.Background())
	assert.True(t, hasStoredTitle(t, fix, "worker"), "every item of the session is checked, an open item-direct PR holds it")

	fake.guards = func(map[string]session.ItemGuardView) (map[string]session.ItemGuardView, error) {
		return map[string]session.ItemGuardView{
			"item-x": {Direct: []domain.PRView{merged}},
			"item-y": {Direct: []domain.PRView{merged}},
		}, nil
	}
	sweeper.sweep(context.Background())
	assert.False(t, hasStoredTitle(t, fix, "worker"), "item-direct PRs merged: eligible")
}

func TestRetentionSweeper_ShouldFailClosed_WhenLinkOrItemLookupErrors(t *testing.T) {
	t.Parallel()
	boom := errors.New("injected")
	cases := map[string]func(*fakeSweepPRSource){
		"links": func(f *fakeSweepPRSource) {
			f.views = func(map[string][]domain.PRView) (map[string][]domain.PRView, error) { return nil, boom }
		},
		"itemIDs": func(f *fakeSweepPRSource) {
			f.itemIDs = func(map[string][]string) (map[string][]string, error) { return nil, boom }
		},
		"guards": func(f *fakeSweepPRSource) {
			f.itemIDs = func(map[string][]string) (map[string][]string, error) { return map[string][]string{"u-e": {"i"}}, nil }
			f.guards = func(map[string]session.ItemGuardView) (map[string]session.ItemGuardView, error) { return nil, boom }
		},
		"missing guard": func(f *fakeSweepPRSource) {
			f.itemIDs = func(map[string][]string) (map[string][]string, error) { return map[string][]string{"u-e": {"i"}}, nil }
			f.guards = func(map[string]session.ItemGuardView) (map[string]session.ItemGuardView, error) {
				return map[string]session.ItemGuardView{}, nil
			}
		},
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fix := setupForkTestFixture(t)
			defer fix.cleanup()
			addArchivedWithUUID(t, fix, "err-session", "u-e")
			sweeper, fake := newPRSweeper(fix)
			breakIt(fake)
			sweeper.sweep(context.Background())
			assert.True(t, hasStoredTitle(t, fix, "err-session"), "a failed lookup must never delete")
		})
	}
}

func TestRetentionSweeper_ShouldFailClosed_WhenRepresentationMismatch(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	legacy := func(terminal bool) func(*session.Instance) {
		return func(i *session.Instance) {
			i.GitHubPRNumber, i.GitHubOwner, i.GitHubRepo, i.GitHubPRStatusTerminal = 32, "acme", "app", terminal
		}
	}
	addArchivedWithUUID(t, fix, "unrepresented", "u-m", legacy(false))
	linkPRKeepingMirror(t, fix, "u-m", 31, domain.PRStateMerged, true, session.PRMirror{Number: 32})
	addArchivedWithUUID(t, fix, "unrepresented-safe", "u-ms", legacy(true))
	linkPRKeepingMirror(t, fix, "u-ms", 31, domain.PRStateMerged, true, session.PRMirror{Number: 32, Terminal: true})
	addArchivedWithUUID(t, fix, "bad-item-ref", "u-b")

	sweeper, fake := newPRSweeper(fix)
	fake.itemIDs = func(m map[string][]string) (map[string][]string, error) {
		m["u-b"] = []string{"item-bad"}
		return m, nil
	}
	fake.guards = func(map[string]session.ItemGuardView) (map[string]session.ItemGuardView, error) {
		return map[string]session.ItemGuardView{"item-bad": {LegacyUnrepresented: true}}, nil
	}
	sweeper.sweep(context.Background())

	assert.True(t, hasStoredTitle(t, fix, "unrepresented"), "links {#31 merged} do not carry legacy #32, flag false")
	assert.False(t, hasStoredTitle(t, fix, "unrepresented-safe"), "legacy #32 flag true: safe via LegacyOnlyPRSafe")
	assert.True(t, hasStoredTitle(t, fix, "bad-item-ref"), "an item with an unrepresented legacy number holds its sessions")
}

func TestRetentionSweeper_ShouldFailClosed_WhenAllScopesOff(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addArchivedWithUUID(t, fix, "open", "u-o")
	linkPR(t, fix, "u-o", 12, domain.PRStateOpen, false)

	// DefaultConfig leaves every session_multi_pr scope off; the sweeper reads none.
	sweeper, _ := newPRSweeper(fix)
	sweeper.sweep(context.Background())
	assert.True(t, hasStoredTitle(t, fix, "open"))
}

func TestRetentionSweeper_ShouldBeEligible_WhenNoLinksAndNoLegacyNumber(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addArchivedWithUUID(t, fix, "plain", "u-p")

	sweeper, _ := newPRSweeper(fix)
	sweeper.sweep(context.Background())
	assert.False(t, hasStoredTitle(t, fix, "plain"))
}

func TestRetentionSweeper_ShouldIssueBoundedQueries_WhenManySessions(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	for i := 0; i < 5; i++ {
		addArchivedWithUUID(t, fix, fmt.Sprintf("s%d", i), fmt.Sprintf("u-%d", i))
	}
	sweeper, fake := newPRSweeper(fix)
	sweeper.sweep(context.Background())
	assert.Equal(t, 1+5, fake.viewCall, "one bulk link read per cycle plus one single-uuid re-check per session about to be deleted; none for skipped sessions")
}

func TestRetentionSweeper_ShouldSurfaceItemPRHold_OnlyAfterMaxHold_AndNeverRelease(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addArchivedWithUUID(t, fix, "held", "u-held")
	sweeper, fake := newPRSweeper(fix)
	base := time.Now()
	sweeper.now = func() time.Time { return base }
	fake.itemIDs = func(map[string][]string) (map[string][]string, error) {
		return map[string][]string{"u-held": {"item-h"}}, nil
	}
	attached := base.Add(-30 * time.Minute)
	fake.guards = func(map[string]session.ItemGuardView) (map[string]session.ItemGuardView, error) {
		open := domain.PRView{State: domain.PRStateOpen, AttachedAt: attached}
		return map[string]session.ItemGuardView{"item-h": {Direct: []domain.PRView{open}}}, nil
	}
	globalBefore := ItemPRHoldOverMaxCount()

	sweeper.sweep(context.Background())
	assert.Zero(t, sweeper.holds.reportedTotal, "held 30m: below the 1h line")
	assert.Empty(t, sweeper.OverMaxItemPRHolds())

	base = base.Add(2 * time.Hour)
	sweeper.sweep(context.Background())
	sweeper.sweep(context.Background())
	assert.Equal(t, 1, sweeper.holds.reportedTotal, "counted once per (item, window), not per cycle")
	assert.GreaterOrEqual(t, ItemPRHoldOverMaxCount(), globalBefore+1)
	require.Len(t, sweeper.OverMaxItemPRHolds(), 1)
	assert.Equal(t, "item-h", sweeper.OverMaxItemPRHolds()[0].ItemID)
	assert.True(t, hasStoredTitle(t, fix, "held"), "fail-closed forever: surfacing never releases the session")
}

func TestRetentionSweeper_ShouldNotDelete_WhenPRLinkAppearsAfterCycleLoad(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addArchivedWithUUID(t, fix, "raced", "u-race")
	sweeper, fake := newPRSweeper(fix)
	calls := 0
	fake.views = func(m map[string][]domain.PRView) (map[string][]domain.PRView, error) {
		calls++
		if calls > 1 { // a report_pr_created lands after the cycle prefilter
			m["u-race"] = []domain.PRView{{State: domain.PRStateOpen}}
		}
		return m, nil
	}

	sweeper.sweep(context.Background())

	assert.True(t, hasStoredTitle(t, fix, "raced"), "evidence re-read right before delete: the new open PR holds the session")
	assert.GreaterOrEqual(t, calls, 2, "the per-session re-check ran")
}

func TestRetentionSweeper_ShouldKeepSession_WhenClosedPRWasReopenedOrCheckFails(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addArchivedWithUUID(t, fix, "reopened", "u-reo")
	linkPR(t, fix, "u-reo", 61, domain.PRStateClosed, true)
	addArchivedWithUUID(t, fix, "errored", "u-err")
	linkPR(t, fix, "u-err", 62, domain.PRStateClosed, true)
	addArchivedWithUUID(t, fix, "stays-closed", "u-ok")
	linkPR(t, fix, "u-ok", 63, domain.PRStateClosed, true)

	sweeper, _ := newPRSweeper(fix)
	sweeper.reverifyClosed = func(_ context.Context, ref domain.PRRef) (bool, error) {
		switch ref.Number() {
		case 61:
			return true, nil
		case 62:
			return false, errors.New("github unreachable")
		}
		return false, nil
	}
	sweeper.sweep(context.Background())

	assert.True(t, hasStoredTitle(t, fix, "reopened"), "a PR reopened on GitHub holds the session")
	assert.True(t, hasStoredTitle(t, fix, "errored"), "a failed re-verify fails closed")
	assert.False(t, hasStoredTitle(t, fix, "stays-closed"), "a PR GitHub still reports closed releases the session")
}

func TestRetentionSweeper_ShouldNotDelete_WhenSessionUnarchivedAfterCycleLoad(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addArchivedWithUUID(t, fix, "revived", "u-rev")
	sweeper, _ := newPRSweeper(fix)
	sweeper.reread = func(string) (*domain.InstanceData, error) {
		return &domain.InstanceData{UUID: "u-rev"}, nil // ArchivedAt cleared since the cycle load
	}
	sweeper.sweep(context.Background())
	assert.True(t, hasStoredTitle(t, fix, "revived"))
}

func TestRetentionSweeper_ShouldSurfaceSessionPRHold_OnceWhenOverMax(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addArchivedWithUUID(t, fix, "held", "u-held")
	linkPR(t, fix, "u-held", 71, domain.PRStateOpen, false)
	sweeper, _ := newPRSweeper(fix)
	sweeper.now = func() time.Time { return time.Now().Add(48 * time.Hour) }

	sweeper.sweep(context.Background())
	afterFirst := sweeper.sessionHolds.reportedTotal
	sweeper.sweep(context.Background())

	snap := sweeper.sessionHolds.snapshot()
	keys := make([]string, 0, len(snap))
	for _, h := range snap {
		keys = append(keys, h.ItemID)
	}
	assert.Contains(t, keys, "u-held", "the over-max session hold is surfaced")
	assert.Equal(t, afterFirst, sweeper.sessionHolds.reportedTotal, "reported once per hold window, not every sweep")
	assert.Positive(t, SessionPRHoldOverMaxCount())
	assert.True(t, hasStoredTitle(t, fix, "held"))
}

func TestClassifyReverifyError_ShouldSortOutcomes(t *testing.T) {
	t.Parallel()
	for err, want := range map[error]reverifyOutcome{
		githubpkg.ErrNotAuthenticated:                                      reverifyUnverifiable,
		fmt.Errorf("%w: gone", githubpkg.ErrGitHubRefNotFound):             reverifyUnverifiable,
		errors.New("GitHub API: unauthorized (401)"):                       reverifyUnverifiable,
		errors.New("GitHub API: forbidden (403)"):                          reverifyUnverifiable,
		errors.New("GitHub API: primary rate limit exhausted (403)"):       reverifyTransient,
		errors.New("GitHub API: secondary rate limit (403)"):               reverifyTransient,
		errors.New("GitHub API: rate limited (429)"):                       reverifyTransient,
		errors.New("conditional PR request failed: dial tcp: i/o timeout"): reverifyTransient,
		errors.New("HTTP 502 bad gateway"):                                 reverifyTransient,
	} {
		assert.Equal(t, want, classifyReverifyError(err), err.Error())
	}
}

func TestRetentionSweeper_ShouldReleaseSession_OnlyAfterClosedPRStaysUnverifiable(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addArchivedWithUUID(t, fix, "no-token", "u-nt")
	linkPR(t, fix, "u-nt", 81, domain.PRStateClosed, true)
	addArchivedWithUUID(t, fix, "rate-limited", "u-rl")
	linkPR(t, fix, "u-rl", 82, domain.PRStateClosed, true)

	sweeper, _ := newPRSweeper(fix)
	clock := time.Now()
	sweeper.now = func() time.Time { return clock }
	sweeper.reverifyClosed = func(_ context.Context, ref domain.PRRef) (bool, error) {
		if ref.Number() == 81 {
			return false, githubpkg.ErrNotAuthenticated
		}
		return false, errors.New("GitHub API: rate limited (429)")
	}
	before := ClosedPRUnverifiableCount()
	for i := 0; i < domain.VerifyAttemptLimit; i++ {
		assert.True(t, hasStoredTitle(t, fix, "no-token"), "held on sweep %d", i)
		sweeper.sweep(context.Background())
		clock = clock.Add(reverifyUnverifiableSpan/2 + time.Minute)
	}
	assert.False(t, hasStoredTitle(t, fix, "no-token"), "released once unverifiable for the count and span")
	assert.True(t, hasStoredTitle(t, fix, "rate-limited"), "a transient failure never releases")
	assert.GreaterOrEqual(t, ClosedPRUnverifiableCount()-before, int64(domain.VerifyAttemptLimit))
}

func TestClosedPRReverifier_ShouldBoundChecksPerSweep(t *testing.T) {
	t.Parallel()
	var r closedPRReverifier
	now := time.Now()
	r.startSweep(now)
	for i := 0; i < reverifyMaxChecksPerSweep; i++ {
		require.True(t, r.take(now))
	}
	assert.False(t, r.take(now), "check budget")
	r.startSweep(now)
	assert.True(t, r.take(now))
	assert.False(t, r.take(now.Add(reverifyBudgetPerSweep+time.Second)), "time budget")
}
