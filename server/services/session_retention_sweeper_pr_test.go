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
	"github.com/tstapler/stapler-squad/session"
)

// fakeSweepPRSource wraps the real storage and lets a test replace or break any
// of the three bulk reads the sweeper makes once per cycle.
type fakeSweepPRSource struct {
	real     sweepPRSource
	views    func(map[string][]session.PRView) (map[string][]session.PRView, error)
	itemIDs  func(map[string][]string) (map[string][]string, error)
	guards   func(map[string]session.ItemGuardView) (map[string]session.ItemGuardView, error)
	viewCall int
}

func (f *fakeSweepPRSource) ListSessionPRViewsByUUIDs(ctx context.Context, uuids []string) (map[string][]session.PRView, error) {
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
func linkPR(t *testing.T, fix *forkTestFixture, uuid string, number int, state session.PRState, terminal bool) {
	t.Helper()
	linkPRKeepingMirror(t, fix, uuid, number, state, terminal, session.PRMirror{})
}

func linkPRKeepingMirror(t *testing.T, fix *forkTestFixture, uuid string, number int, state session.PRState, terminal bool, mirror session.PRMirror) {
	t.Helper()
	ref, err := session.NewPRRef("", "acme", "app", number)
	require.NoError(t, err)
	_, err = fix.storage.AttachSessionPR(context.Background(), uuid, ref,
		session.PRLink{Role: session.PRRolePrimary, Source: session.AttachSourceManual, Initial: session.PRInitial{State: state, Terminal: terminal}},
		mirror)
	require.NoError(t, err)
}

func TestRetentionSweeper_ShouldNotDelete_WhenOneLinkedPROpen(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	addArchivedWithUUID(t, fix, "mixed", "u-mixed")
	linkPR(t, fix, "u-mixed", 11, session.PRStateMerged, true)
	linkPR(t, fix, "u-mixed", 12, session.PRStateOpen, false)
	addArchivedWithUUID(t, fix, "both-terminal", "u-done")
	linkPR(t, fix, "u-done", 21, session.PRStateMerged, true)
	linkPR(t, fix, "u-done", 22, session.PRStateClosed, true)

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
	linkPR(t, fix, "u-unk", 12, session.PRStateUnknown, false)
	addArchivedWithUUID(t, fix, "unverified", "u-unv")
	linkPR(t, fix, "u-unv", 13, session.PRStateUnknown, true)
	addArchivedWithUUID(t, fix, "exhausted", "u-exh")
	linkPR(t, fix, "u-exh", 14, session.PRStateUnknown, true)

	sweeper, _ := newPRSweeper(fix)
	sweeper.sweep(context.Background())
	assert.True(t, hasStoredTitle(t, fix, "unknown"))
	assert.True(t, hasStoredTitle(t, fix, "unverified"), "a stale legacy flag must not make an unpolled PR look safe")
	assert.True(t, hasStoredTitle(t, fix, "exhausted"), "not exhausted yet")

	start := time.Now().Add(-2 * time.Hour)
	sweeper.prSource.(*fakeSweepPRSource).views = func(m map[string][]session.PRView) (map[string][]session.PRView, error) {
		for _, v := range m["u-exh"] {
			v.PollFailCount, v.FirstFailAt, v.LastPolledAt = 5, start, start.Add(time.Hour)
			m["u-exh"] = []session.PRView{v}
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
	linkPR(t, fix, "u-w", 11, session.PRStateMerged, true)
	sweeper, fake := newPRSweeper(fix)
	fake.itemIDs = func(map[string][]string) (map[string][]string, error) {
		return map[string][]string{"u-w": {"item-x", "item-y"}}, nil
	}
	merged := session.PRView{State: session.PRStateMerged, StoredTerminal: true}
	open := session.PRView{State: session.PRStateOpen}
	fake.guards = func(map[string]session.ItemGuardView) (map[string]session.ItemGuardView, error) {
		return map[string]session.ItemGuardView{
			"item-x": {Direct: []session.PRView{merged}},
			"item-y": {Direct: []session.PRView{open}},
		}, nil
	}
	sweeper.sweep(context.Background())
	assert.True(t, hasStoredTitle(t, fix, "worker"), "every item of the session is checked, an open item-direct PR holds it")

	fake.guards = func(map[string]session.ItemGuardView) (map[string]session.ItemGuardView, error) {
		return map[string]session.ItemGuardView{
			"item-x": {Direct: []session.PRView{merged}},
			"item-y": {Direct: []session.PRView{merged}},
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
			f.views = func(map[string][]session.PRView) (map[string][]session.PRView, error) { return nil, boom }
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
	linkPRKeepingMirror(t, fix, "u-m", 31, session.PRStateMerged, true, session.PRMirror{Number: 32})
	addArchivedWithUUID(t, fix, "unrepresented-safe", "u-ms", legacy(true))
	linkPRKeepingMirror(t, fix, "u-ms", 31, session.PRStateMerged, true, session.PRMirror{Number: 32, Terminal: true})
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
	linkPR(t, fix, "u-o", 12, session.PRStateOpen, false)

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
		open := session.PRView{State: session.PRStateOpen, AttachedAt: attached}
		return map[string]session.ItemGuardView{"item-h": {Direct: []session.PRView{open}}}, nil
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
	fake.views = func(m map[string][]session.PRView) (map[string][]session.PRView, error) {
		calls++
		if calls > 1 { // a report_pr_created lands after the cycle prefilter
			m["u-race"] = []session.PRView{{State: session.PRStateOpen}}
		}
		return m, nil
	}

	sweeper.sweep(context.Background())

	assert.True(t, hasStoredTitle(t, fix, "raced"), "evidence re-read right before delete: the new open PR holds the session")
	assert.GreaterOrEqual(t, calls, 2, "the per-session re-check ran")
}
