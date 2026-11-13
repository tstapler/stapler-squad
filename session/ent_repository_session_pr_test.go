package session

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/pullrequeststate"
)

var (
	prRepoT0 = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	// prRepoNow is the injected apply-time clock: later than every observed_at the
	// tests use except where a test sets Now itself to exercise the clamp.
	prRepoNow = prRepoT0.Add(72 * time.Hour)
)

func newPRRepo(t *testing.T) *EntRepository {
	t.Helper()
	return NewTestEntRepository(t)
}

func upsertLink(t *testing.T, repo *EntRepository, owner PROwner, ref domain.PRRef, src domain.AttachSource) PRLinkResult {
	t.Helper()
	res, err := repo.UpsertSessionPR(context.Background(), owner, ref, PRLink{Role: domain.PRRolePrimary, Source: src})
	require.NoError(t, err)
	return res
}

func countStates(t *testing.T, repo *EntRepository) int {
	t.Helper()
	n, err := repo.Client.PullRequestState.Query().Where(pullrequeststate.NumberGT(0)).Count(context.Background())
	require.NoError(t, err)
	return n
}

func countLinks(t *testing.T, repo *EntRepository) int {
	t.Helper()
	n, err := repo.Client.SessionPR.Query().Where().Count(context.Background())
	require.NoError(t, err)
	return n
}

// ---- statement counting --------------------------------------------------------

type countingDriver struct {
	dialect.Driver
	n     *atomic.Int64
	match func(query string) bool // nil counts every statement
}

func (d countingDriver) count(q string) {
	if d.match == nil || d.match(q) {
		d.n.Add(1)
	}
}

func (d countingDriver) Exec(ctx context.Context, q string, args, v any) error {
	d.count(q)
	return d.Driver.Exec(ctx, q, args, v)
}

func (d countingDriver) Query(ctx context.Context, q string, args, v any) error {
	d.count(q)
	return d.Driver.Query(ctx, q, args, v)
}

func (d countingDriver) Tx(ctx context.Context) (dialect.Tx, error) {
	tx, err := d.Driver.Tx(ctx)
	if err != nil {
		return nil, err
	}
	return countingTx{Tx: tx, c: d}, nil
}

type countingTx struct {
	dialect.Tx
	c countingDriver
}

func (t countingTx) Exec(ctx context.Context, q string, args, v any) error {
	t.c.count(q)
	return t.Tx.Exec(ctx, q, args, v)
}

func (t countingTx) Query(ctx context.Context, q string, args, v any) error {
	t.c.count(q)
	return t.Tx.Query(ctx, q, args, v)
}

// countStatements runs fn with the repository's client swapped for one that
// counts every SQL statement, and returns the count.
func countStatements(t *testing.T, repo *EntRepository, fn func()) int64 {
	t.Helper()
	return countStatementsMatching(t, repo, nil, fn)
}

// countStatementsMatching counts only the statements match accepts (nil: all).
func countStatementsMatching(t *testing.T, repo *EntRepository, match func(string) bool, fn func()) int64 {
	t.Helper()
	var n atomic.Int64
	orig := repo.Client
	repo.Client = ent.NewClient(ent.Driver(countingDriver{Driver: repo.driver, n: &n, match: match}))
	defer func() { repo.Client = orig }()
	fn()
	return n.Load()
}

// ---- upsert / reads --------------------------------------------------------------

func TestUpsertSessionPR_ShouldBeIdempotent_WhenCalledTwice(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ref := mustPRRef(t, "github.com", "acme", "app", 12)

	first := upsertLink(t, repo, SessionPROwner("u1"), ref, domain.AttachSourceManual)
	require.True(t, first.Created)
	before, err := repo.ListBySessionUUIDs(context.Background(), []string{"u1"})
	require.NoError(t, err)

	second := upsertLink(t, repo, SessionPROwner("u1"), ref, domain.AttachSourceManual)
	assert.False(t, second.Created)
	assert.Equal(t, first.StateID, second.StateID)
	after, err := repo.ListBySessionUUIDs(context.Background(), []string{"u1"})
	require.NoError(t, err)

	assert.Equal(t, 1, countStates(t, repo))
	assert.Equal(t, 1, countLinks(t, repo))
	require.Len(t, after["u1"], 1)
	assert.True(t, before["u1"][0].AttachedAt.Equal(after["u1"][0].AttachedAt), "attached_at must not change on the second call")
}

func TestListBySessionUUIDs_ShouldShareOneState_WhenTwoSessionsAttachSamePR(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ref := mustPRRef(t, "", "acme", "app", 12)
	upsertLink(t, repo, SessionPROwner("u1"), ref, domain.AttachSourceManual)
	upsertLink(t, repo, SessionPROwner("u2"), ref, domain.AttachSourceBranchDiscovery)

	var got map[string][]domain.PRView
	stmts := countStatements(t, repo, func() {
		var err error
		got, err = repo.ListBySessionUUIDs(context.Background(), []string{"u1", "u2"})
		require.NoError(t, err)
	})
	assert.LessOrEqual(t, stmts, int64(2), "links then states, 2*ceil(2/500)")
	require.Len(t, got["u1"], 1)
	require.Len(t, got["u2"], 1)
	assert.Equal(t, got["u1"][0].StateID, got["u2"][0].StateID)

	_, err := repo.UpdatePullRequestState(context.Background(), ref, fetchObs(PRSourceTick, domain.PRStateMerged, prRepoT0))
	require.NoError(t, err)
	got, err = repo.ListBySessionUUIDs(context.Background(), []string{"u1", "u2"})
	require.NoError(t, err)
	assert.Equal(t, domain.PRStateMerged, got["u1"][0].State)
	assert.Equal(t, domain.PRStateMerged, got["u2"][0].State)
}

func TestUpsertSessionPR_ShouldKeepOneStateRow_WhenSessionItemAndJulesLinkSamePR(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ref := mustPRRef(t, "", "acme", "app", 12)
	upsertLink(t, repo, SessionPROwner("u1"), ref, domain.AttachSourceBranchDiscovery)
	upsertLink(t, repo, ItemPROwner("item-1"), ref, domain.AttachSourceReportPRCreated)
	upsertLink(t, repo, SessionPROwner("u-jules"), ref, domain.AttachSourceJules)

	assert.Equal(t, 1, countStates(t, repo))
	assert.Equal(t, 3, countLinks(t, repo))
}

func TestListBySessionUUIDs_ShouldStayWithinStatementBudget_WhenManySessions(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	uuids := make([]string, 0, 1001)
	for s := 0; s < 200; s++ {
		uuids = append(uuids, fmt.Sprintf("sess-%03d", s))
		for k := 0; k < 3; k++ {
			role := domain.PRRoleFollowup
			ref := mustPRRef(t, "", "acme", "app", s*3+k+1)
			_, err := repo.UpsertSessionPR(ctx, SessionPROwner(uuids[s]), ref, PRLink{Role: role, Source: domain.AttachSourceBackfill})
			require.NoError(t, err)
		}
	}

	var got map[string][]domain.PRView
	stmts := countStatements(t, repo, func() {
		var err error
		got, err = repo.ListBySessionUUIDs(ctx, uuids)
		require.NoError(t, err)
	})
	assert.Len(t, got, 200)
	assert.LessOrEqual(t, stmts, int64(2), "200 sessions x 3 PRs: 2*ceil(200/500)")

	for i := 200; i < 1001; i++ {
		uuids = append(uuids, fmt.Sprintf("none-%04d", i))
	}
	stmts = countStatements(t, repo, func() {
		_, err := repo.ListBySessionUUIDs(ctx, uuids)
		require.NoError(t, err)
	})
	assert.LessOrEqual(t, stmts, int64(6), "1001 uuids: at most 2*ceil(1001/500)")
}

func TestUpsertSessionPR_ShouldDowngradeToFollowup_WhenSessionAlreadyHasPrimary(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	a := upsertLink(t, repo, SessionPROwner("u1"), mustPRRef(t, "", "acme", "app", 1), domain.AttachSourceManual)
	b := upsertLink(t, repo, SessionPROwner("u1"), mustPRRef(t, "", "acme", "app", 2), domain.AttachSourceManual)
	assert.Equal(t, domain.PRRolePrimary, a.Role)
	assert.Equal(t, domain.PRRoleFollowup, b.Role)
}

func TestUpsertSessionPR_ShouldRejectEleventhLink_WhenCapReached(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	for n := 1; n <= maxSessionPRLinks; n++ {
		_, err := repo.UpsertSessionPR(ctx, SessionPROwner("u1"), mustPRRef(t, "", "acme", "app", n), PRLink{Role: domain.PRRoleFollowup, Source: domain.AttachSourcePushScan})
		require.NoError(t, err)
	}
	_, err := repo.UpsertSessionPR(ctx, SessionPROwner("u1"), mustPRRef(t, "", "acme", "app", 999), PRLink{Source: domain.AttachSourcePushScan})
	assert.ErrorIs(t, err, ErrSessionPRLinkCap)
	// An already-linked PR is still a no-op rather than a cap error.
	res, err := repo.UpsertSessionPR(ctx, SessionPROwner("u1"), mustPRRef(t, "", "acme", "app", 1), PRLink{Source: domain.AttachSourcePushScan})
	require.NoError(t, err)
	assert.False(t, res.Created)
}

func TestUpsertSessionPR_ShouldRejectLink_WhenItemOwnerCapReached(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	for n := 1; n <= maxSessionPRLinks; n++ {
		_, err := repo.UpsertSessionPR(ctx, ItemPROwner("item-1"), mustPRRef(t, "", "acme", "app", n), PRLink{Source: domain.AttachSourceReportPRCreated})
		require.NoError(t, err)
	}
	_, err := repo.UpsertSessionPR(ctx, ItemPROwner("item-1"), mustPRRef(t, "", "acme", "app", 999), PRLink{Source: domain.AttachSourceReportPRCreated})
	assert.ErrorIs(t, err, ErrSessionPRLinkCap)
	_, err = repo.UpsertSessionPR(ctx, ItemPROwner("item-2"), mustPRRef(t, "", "acme", "app", 999), PRLink{Source: domain.AttachSourceReportPRCreated})
	require.NoError(t, err, "the cap is per owner")
}

func TestSetPinned_ShouldClearOtherPins_WhenPinningAnotherPR(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	r1, r2 := mustPRRef(t, "", "acme", "app", 1), mustPRRef(t, "", "acme", "app", 2)
	upsertLink(t, repo, SessionPROwner("u1"), r1, domain.AttachSourceManual)
	upsertLink(t, repo, SessionPROwner("u1"), r2, domain.AttachSourceManual)

	require.NoError(t, repo.SetPinned(ctx, SessionPROwner("u1"), r1, true))
	require.NoError(t, repo.SetPinned(ctx, SessionPROwner("u1"), r2, true))
	got, err := repo.ListBySessionUUIDs(ctx, []string{"u1"})
	require.NoError(t, err)
	pins := map[int]bool{}
	for _, v := range got["u1"] {
		pins[v.Ref.Number()] = v.Pinned
	}
	assert.Equal(t, map[int]bool{1: false, 2: true}, pins)

	require.NoError(t, repo.SetPinned(ctx, SessionPROwner("u1"), r2, false))
	got, _ = repo.ListBySessionUUIDs(ctx, []string{"u1"})
	for _, v := range got["u1"] {
		assert.False(t, v.Pinned)
	}
	assert.ErrorIs(t, repo.SetPinned(ctx, ItemPROwner("i1"), r1, true), ErrInvalidPROwner)
}

func TestOwnerRules_ShouldRejectBadOwnersAndUpperCaseIdentity(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	ref := mustPRRef(t, "", "acme", "app", 1)

	_, err := repo.UpsertSessionPR(ctx, PROwner{}, ref, PRLink{Source: domain.AttachSourceManual})
	assert.ErrorIs(t, err, ErrInvalidPROwner)
	_, err = repo.UpsertSessionPR(ctx, PROwner{SessionUUID: "u", BacklogItemID: "i"}, ref, PRLink{Source: domain.AttachSourceManual})
	assert.ErrorIs(t, err, ErrInvalidPROwner)

	// Database backstops for callers that bypass the repository.
	stateID := upsertLink(t, repo, SessionPROwner("u1"), ref, domain.AttachSourceManual).StateID
	err = repo.Client.SessionPR.Create().SetPullRequestStateID(stateID).SetAttachSource("manual").Exec(ctx)
	assert.True(t, ent.IsConstraintError(err), "both owners empty must violate the CHECK, got %v", err)
	err = repo.Client.SessionPR.Create().SetPullRequestStateID(stateID).SetSessionUUID("a").SetBacklogItemID("b").SetAttachSource("manual").Exec(ctx)
	assert.True(t, ent.IsConstraintError(err), "both owners set must violate the CHECK, got %v", err)
	err = repo.Client.PullRequestState.Create().SetHost("github.com").SetOwner("Acme").SetRepo("app").SetNumber(3).Exec(ctx)
	assert.True(t, ent.IsValidationError(err), "upper-case owner must fail the column validator, got %v", err)
	err = repo.Client.SessionPRTombstone.Create().SetSessionUUID("u").SetHost("github.com").SetOwner("acme").SetRepo("App").SetNumber(3).Exec(ctx)
	assert.True(t, ent.IsValidationError(err), "upper-case tombstone repo must fail the column validator, got %v", err)
}

// ---- tombstones ---------------------------------------------------------------

func TestUpsertSessionPR_ShouldNotRecreateLink_WhenOwnerDetachedThenAutoAttaches(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	ref := mustPRRef(t, "", "acme", "app", 12)
	owner := SessionPROwner("u1")
	upsertLink(t, repo, owner, ref, domain.AttachSourceManual)

	deleted, err := repo.DeleteSessionPR(ctx, owner, ref)
	require.NoError(t, err)
	require.True(t, deleted)
	assert.Equal(t, 0, countLinks(t, repo))

	for _, src := range []domain.AttachSource{domain.AttachSourceBranchDiscovery, domain.AttachSourceBackfill} {
		res := upsertLink(t, repo, owner, ref, src)
		assert.True(t, res.Tombstoned, "%s must not recreate a detached link", src)
		assert.False(t, res.Created)
	}
	assert.Equal(t, 0, countLinks(t, repo))

	require.NoError(t, repo.ClearTombstone(ctx, owner, ref))
	res := upsertLink(t, repo, owner, ref, domain.AttachSourceManual)
	assert.True(t, res.Created)
	assert.Equal(t, 1, countLinks(t, repo))
}

func TestDeleteSessionPR_ShouldAffectOnlyThatOwner_WhenTwoOwnersLinkSamePR(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	ref := mustPRRef(t, "", "acme", "app", 12)
	upsertLink(t, repo, SessionPROwner("u1"), ref, domain.AttachSourceManual)
	upsertLink(t, repo, ItemPROwner("item-1"), ref, domain.AttachSourceManual)

	_, err := repo.DeleteSessionPR(ctx, SessionPROwner("u1"), ref)
	require.NoError(t, err)

	remaining, err := repo.ListByItem(ctx, "item-1")
	require.NoError(t, err)
	assert.Len(t, remaining, 1, "the item's link survives a session detach")
	itemTomb, err := repo.IsTombstoned(ctx, ItemPROwner("item-1"), ref)
	require.NoError(t, err)
	sessTomb, err := repo.IsTombstoned(ctx, SessionPROwner("u1"), ref)
	require.NoError(t, err)
	assert.False(t, itemTomb)
	assert.True(t, sessTomb)

	st, err := repo.ListSessionTombstonesByUUIDs(ctx, []string{"u1", "u2"})
	require.NoError(t, err)
	assert.Equal(t, map[string][]domain.PRRef{"u1": {ref}}, st)
	_, err = repo.DeleteSessionPR(ctx, ItemPROwner("item-1"), ref)
	require.NoError(t, err)
	it, err := repo.ListItemTombstones(ctx, []string{"item-1"})
	require.NoError(t, err)
	assert.Equal(t, map[string][]domain.PRRef{"item-1": {ref}}, it)
}

// ---- UpdatePullRequestState ---------------------------------------------------

func fetchObs(src PRObservationSource, state domain.PRState, at time.Time) PRObservation {
	return PRObservation{Source: src, Kind: PRObservationFetch, State: state, ObservedAt: at, Now: prRepoNow}
}

func failObs(at time.Time, kind PRObservationKind, pollErr string) PRObservation {
	return PRObservation{Source: PRSourceTick, Kind: kind, ObservedAt: at, Now: at.Add(time.Second), PollError: pollErr}
}

// seedState inserts ref with the given initial state and applies no observation.
func seedState(t *testing.T, repo *EntRepository, ref domain.PRRef, state domain.PRState, terminal bool) {
	t.Helper()
	_, err := repo.EnsureState(context.Background(), ref, PRInitial{State: state, Terminal: terminal})
	require.NoError(t, err)
}

func reload(t *testing.T, repo *EntRepository, ref domain.PRRef) *ent.PullRequestState {
	t.Helper()
	row, err := repo.Client.PullRequestState.Query().Where(refPredicate(ref)).Only(context.Background())
	require.NoError(t, err)
	return row
}

// setPolled stamps last_polled_at directly so a test can start from a known clock.
func setPolled(t *testing.T, repo *EntRepository, ref domain.PRRef, at time.Time) {
	t.Helper()
	require.NoError(t, repo.Client.PullRequestState.Update().Where(refPredicate(ref)).SetLastPolledAt(at).Exec(context.Background()))
}

func TestUpdatePullRequestState_ShouldFollowTransitionTable(t *testing.T) {
	t.Parallel()
	type tc struct {
		name     string
		from     domain.PRState
		terminal bool
		obs      PRObservation
		want     domain.PRState
		dropped  PRDropReason
	}
	later := prRepoT0.Add(time.Hour)
	tests := []tc{
		{"open to open", domain.PRStateOpen, false, fetchObs(PRSourceTick, domain.PRStateOpen, later), domain.PRStateOpen, ""},
		{"open to merged", domain.PRStateOpen, false, fetchObs(PRSourceTick, domain.PRStateMerged, later), domain.PRStateMerged, ""},
		{"open to closed", domain.PRStateOpen, false, fetchObs(PRSourceTick, domain.PRStateClosed, later), domain.PRStateClosed, ""},
		{"closed to merged by tick", domain.PRStateClosed, true, fetchObs(PRSourceTick, domain.PRStateMerged, later), domain.PRStateMerged, ""},
		{"closed to merged by reconciler refresh", domain.PRStateClosed, true, fetchObs(PRSourceReconcilerRefresh, domain.PRStateMerged, later), domain.PRStateMerged, ""},
		{"closed to open by tick is sticky", domain.PRStateClosed, true, fetchObs(PRSourceTick, domain.PRStateOpen, later), domain.PRStateClosed, PRDropSticky},
		{"closed to open by reconciler refresh is dropped on source", domain.PRStateClosed, true, fetchObs(PRSourceReconcilerRefresh, domain.PRStateOpen, later), domain.PRStateClosed, PRDropSource},
		{"closed to open by plain webhook is dropped on source", domain.PRStateClosed, true, fetchObs(PRSourceWebhook, domain.PRStateOpen, later), domain.PRStateClosed, PRDropSource},
		{"closed to open by webhook_reopened", domain.PRStateClosed, true, fetchObs(PRSourceWebhookReopened, domain.PRStateOpen, later), domain.PRStateOpen, ""},
		{"closed to open by webhook_reopened at the same instant", domain.PRStateClosed, true, fetchObs(PRSourceWebhookReopened, domain.PRStateOpen, prRepoT0), domain.PRStateClosed, PRDropSticky},
		{"merged to open", domain.PRStateMerged, true, fetchObs(PRSourceWebhookReopened, domain.PRStateOpen, later), domain.PRStateMerged, PRDropSticky},
		{"merged to closed", domain.PRStateMerged, true, fetchObs(PRSourceTick, domain.PRStateClosed, later), domain.PRStateMerged, PRDropSticky},
		{"merged to merged", domain.PRStateMerged, true, fetchObs(PRSourceTick, domain.PRStateMerged, later), domain.PRStateMerged, PRDropSticky},
		{"unknown to open", domain.PRStateUnknown, false, fetchObs(PRSourceTick, domain.PRStateOpen, later), domain.PRStateOpen, ""},
		{"unknown to merged", domain.PRStateUnknown, false, fetchObs(PRSourceTick, domain.PRStateMerged, later), domain.PRStateMerged, ""},
		{"unknown to closed", domain.PRStateUnknown, false, fetchObs(PRSourceTick, domain.PRStateClosed, later), domain.PRStateClosed, ""},
		{"unverified terminal to open", domain.PRStateUnknown, true, fetchObs(PRSourceTick, domain.PRStateOpen, later), domain.PRStateOpen, ""},
		{"unverified terminal to merged", domain.PRStateUnknown, true, fetchObs(PRSourceTick, domain.PRStateMerged, later), domain.PRStateMerged, ""},
		{"older observation on a merged row is stale", domain.PRStateMerged, true, fetchObs(PRSourceTick, domain.PRStateOpen, prRepoT0.Add(-time.Hour)), domain.PRStateMerged, PRDropStale},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := newPRRepo(t)
			ref := mustPRRef(t, "", "acme", "app", i+1)
			seedState(t, repo, ref, tt.from, tt.terminal)
			setPolled(t, repo, ref, prRepoT0)

			res, err := repo.UpdatePullRequestState(context.Background(), ref, tt.obs)
			require.NoError(t, err)
			assert.Equal(t, tt.dropped == "", res.Applied)
			assert.Equal(t, tt.dropped, res.Dropped)
			row := reload(t, repo, ref)
			assert.Equal(t, string(tt.want), row.State)
			assert.Equal(t, tt.want.Terminal(), row.Terminal, "terminal bit follows the stored state after a fetch")
		})
	}
}

func TestUpdatePullRequestState_ShouldEndMerged_WhenFetchesLandInEitherOrder(t *testing.T) {
	t.Parallel()
	t1, t3 := prRepoT0, prRepoT0.Add(2*time.Hour)
	for name, order := range map[string][]PRObservation{
		"older first": {fetchObs(PRSourceTick, domain.PRStateOpen, t1), fetchObs(PRSourceTick, domain.PRStateMerged, t3)},
		"newer first": {fetchObs(PRSourceTick, domain.PRStateMerged, t3), fetchObs(PRSourceTick, domain.PRStateOpen, t1)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repo := newPRRepo(t)
			ref := mustPRRef(t, "", "acme", "app", 5)
			seedState(t, repo, ref, domain.PRStateOpen, false)
			for _, o := range order {
				_, err := repo.UpdatePullRequestState(context.Background(), ref, o)
				require.NoError(t, err)
			}
			assert.Equal(t, "merged", reload(t, repo, ref).State)
		})
	}
}

func TestUpdatePullRequestState_ShouldApply_WhenLastPolledAtIsInTheFuture(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ref := mustPRRef(t, "", "acme", "app", 5)
	seedState(t, repo, ref, domain.PRStateOpen, false)
	setPolled(t, repo, ref, prRepoT0.Add(48*time.Hour)) // clock stepped back since

	obs := fetchObs(PRSourceTick, domain.PRStateMerged, prRepoT0)
	obs.Now = prRepoT0.Add(time.Hour) // last_polled_at (T0+48h) is later than "now"
	res, err := repo.UpdatePullRequestState(context.Background(), ref, obs)
	require.NoError(t, err)
	assert.True(t, res.Applied)
	assert.Equal(t, PRObservationDrops{}, repo.PRObservationDropCounts())
	assert.True(t, reload(t, repo, ref).LastPolledAt.Equal(prRepoT0), "last_polled_at stores the applied observed_at")
}

func TestUpdatePullRequestState_ShouldCountDrops_ByReason(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ref := mustPRRef(t, "", "acme", "app", 5)
	seedState(t, repo, ref, domain.PRStateMerged, true)
	setPolled(t, repo, ref, prRepoT0)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		_, err := repo.UpdatePullRequestState(ctx, ref, fetchObs(PRSourceTick, domain.PRStateOpen, prRepoT0.Add(-time.Hour)))
		require.NoError(t, err)
	}
	_, err := repo.UpdatePullRequestState(ctx, ref, fetchObs(PRSourceTick, domain.PRStateOpen, prRepoT0.Add(time.Hour)))
	require.NoError(t, err)
	assert.Equal(t, PRObservationDrops{Stale: 4, Sticky: 1}, repo.PRObservationDropCounts())
}

func TestUpdatePullRequestState_ShouldMoveOpenToUnknown_OnlyAfterFiveCountedFailuresOverThirtyMinutes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	run := func(t *testing.T, gap time.Duration) *ent.PullRequestState {
		repo := newPRRepo(t)
		ref := mustPRRef(t, "", "acme", "app", 5)
		seedState(t, repo, ref, domain.PRStateOpen, false)
		for i := 0; i < 5; i++ {
			_, err := repo.UpdatePullRequestState(ctx, ref, failObs(prRepoT0.Add(time.Duration(i)*gap), PRObservationCountedFailure, "network"))
			require.NoError(t, err)
		}
		return reload(t, repo, ref)
	}
	t.Run("five failures over 40 minutes", func(t *testing.T) {
		t.Parallel()
		row := run(t, 10*time.Minute)
		assert.Equal(t, "unknown", row.State)
		assert.False(t, row.Terminal)
		assert.Equal(t, 5, row.PollFailCount)
		assert.Equal(t, "network", row.PollError)
	})
	t.Run("five failures within 4 minutes stay open", func(t *testing.T) {
		t.Parallel()
		row := run(t, time.Minute)
		assert.Equal(t, "open", row.State)
		assert.Equal(t, 5, row.PollFailCount)
	})
	t.Run("terminal rows never move to unknown", func(t *testing.T) {
		t.Parallel()
		repo := newPRRepo(t)
		ref := mustPRRef(t, "", "acme", "app", 6)
		seedState(t, repo, ref, domain.PRStateClosed, true)
		res, err := repo.UpdatePullRequestState(ctx, ref, failObs(prRepoT0, PRObservationCountedFailure, "network"))
		require.NoError(t, err)
		assert.Equal(t, PRDropSticky, res.Dropped)
		assert.Equal(t, "closed", reload(t, repo, ref).State)
	})
	t.Run("a successful fetch ends the streak and clears the error", func(t *testing.T) {
		t.Parallel()
		repo := newPRRepo(t)
		ref := mustPRRef(t, "", "acme", "app", 7)
		seedState(t, repo, ref, domain.PRStateOpen, false)
		_, err := repo.UpdatePullRequestState(ctx, ref, failObs(prRepoT0, PRObservationCountedFailure, "auth"))
		require.NoError(t, err)
		_, err = repo.UpdatePullRequestState(ctx, ref, fetchObs(PRSourceTick, domain.PRStateOpen, prRepoT0.Add(time.Minute)))
		require.NoError(t, err)
		row := reload(t, repo, ref)
		assert.Equal(t, 0, row.PollFailCount)
		assert.Nil(t, row.FirstFailAt)
		assert.Equal(t, "", row.PollError)
	})
}

func TestUpdatePullRequestState_ShouldWriteOnlyPollError_WhenErrorNote(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	ref := mustPRRef(t, "", "acme", "app", 5)
	seedState(t, repo, ref, domain.PRStateOpen, false)
	_, err := repo.UpdatePullRequestState(ctx, ref, failObs(prRepoT0, PRObservationCountedFailure, "network"))
	require.NoError(t, err)
	before := reload(t, repo, ref)

	res, err := repo.UpdatePullRequestState(ctx, ref, failObs(prRepoT0.Add(time.Minute), PRObservationErrorNote, "rate_limited"))
	require.NoError(t, err)
	require.True(t, res.Applied)
	after := reload(t, repo, ref)
	assert.Equal(t, "rate_limited", after.PollError)
	assert.Equal(t, before.PollFailCount, after.PollFailCount, "a non-counted failure neither breaks nor extends the streak")
	assert.True(t, before.FirstFailAt.Equal(*after.FirstFailAt))
	assert.True(t, before.LastPolledAt.Equal(*after.LastPolledAt), "and does not move last_polled_at")
}

func TestUpdatePullRequestState_ShouldRejectBadInput(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	ref := mustPRRef(t, "", "acme", "app", 5)
	_, err := repo.UpdatePullRequestState(ctx, domain.PRRef{}, fetchObs(PRSourceTick, domain.PRStateOpen, prRepoT0))
	assert.ErrorIs(t, err, ErrZeroPRRef)
	_, err = repo.UpdatePullRequestState(ctx, ref, fetchObs(PRSourceTick, domain.PRStateOpen, prRepoT0))
	assert.ErrorIs(t, err, domain.ErrNotFound, "no state row yet")
	seedState(t, repo, ref, domain.PRStateOpen, false)
	_, err = repo.UpdatePullRequestState(ctx, ref, fetchObs(PRSourceTick, domain.PRStateUnknown, prRepoT0))
	assert.Error(t, err, "a fetch cannot report unknown")
	_, err = repo.UpdatePullRequestState(ctx, ref, PRObservation{Source: PRSourceTick, Kind: PRObservationFetch, State: domain.PRStateOpen})
	assert.Error(t, err, "ObservedAt is required")
	_, err = repo.UpdatePullRequestState(ctx, ref, failObs(prRepoT0, PRObservationCountedFailure, "bogus"))
	assert.Error(t, err)
}

func TestResetVerifyCounters_ShouldTouchOnlyUnverifiedTerminalRows(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	unverified := mustPRRef(t, "", "acme", "app", 1)
	open := mustPRRef(t, "", "acme", "app", 2)
	seedState(t, repo, unverified, domain.PRStateUnknown, true)
	seedState(t, repo, open, domain.PRStateOpen, false)
	for _, ref := range []domain.PRRef{unverified, open} {
		for i := 0; i < 3; i++ {
			_, err := repo.UpdatePullRequestState(ctx, ref, failObs(prRepoT0.Add(time.Duration(i)*time.Minute), PRObservationCountedFailure, "network"))
			require.NoError(t, err)
		}
	}

	n, err := repo.ResetVerifyCounters(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	u, o := reload(t, repo, unverified), reload(t, repo, open)
	assert.Equal(t, 0, u.PollFailCount)
	assert.Nil(t, u.FirstFailAt)
	assert.Equal(t, "", u.PollError)
	assert.Equal(t, 3, o.PollFailCount, "an open row keeps its counters")
}

func TestListPollableStates_ShouldOrderNonTerminalFirstAndSkipExhaustedAndLinkless(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	repo.MarkPRMaintenanceSucceeded()
	link := func(owner PROwner, n int, state domain.PRState, terminal bool) domain.PRRef {
		ref := mustPRRef(t, "", "acme", "app", n)
		_, err := repo.UpsertSessionPR(ctx, owner, ref, PRLink{Role: domain.PRRoleFollowup, Source: domain.AttachSourceBackfill, Initial: PRInitial{State: state, Terminal: terminal}})
		require.NoError(t, err)
		return ref
	}
	link(SessionPROwner("u1"), 1, domain.PRStateOpen, false)             // pollable
	link(ItemPROwner("item-1"), 2, domain.PRStateUnknown, false)         // item-only, pollable
	link(SessionPROwner("u1"), 3, domain.PRStateMerged, true)            // terminal: skipped
	seeded := link(SessionPROwner("u1"), 4, domain.PRStateUnknown, true) // unverified terminal, not exhausted
	exhausted := link(SessionPROwner("u1"), 5, domain.PRStateUnknown, true)
	seedState(t, repo, mustPRRef(t, "", "acme", "app", 6), domain.PRStateOpen, false) // no link: skipped
	for i := 0; i < 5; i++ {
		_, err := repo.UpdatePullRequestState(ctx, exhausted, failObs(prRepoT0.Add(time.Duration(i)*10*time.Minute), PRObservationCountedFailure, "not_found"))
		require.NoError(t, err)
	}
	_ = seeded

	got, err := repo.ListPollableStates(ctx)
	require.NoError(t, err)
	order := make([]int, 0, len(got))
	for _, v := range got {
		order = append(order, v.Ref.Number())
	}
	require.Len(t, order, 3)
	assert.Equal(t, 4, order[2], "the unverified terminal row comes last")
	assert.ElementsMatch(t, []int{1, 2, 4}, order)
}

func TestVerifyCounters_ShouldReadAsZero_UntilMaintenanceSucceeded(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	ref := mustPRRef(t, "", "acme", "app", 5)
	_, err := repo.UpsertSessionPR(ctx, SessionPROwner("u1"), ref, PRLink{Source: domain.AttachSourceBackfill, Initial: PRInitial{State: domain.PRStateUnknown, Terminal: true}})
	require.NoError(t, err)
	for i := 0; i < 5; i++ {
		_, err := repo.UpdatePullRequestState(ctx, ref, failObs(prRepoT0.Add(time.Duration(i)*10*time.Minute), PRObservationCountedFailure, "not_found"))
		require.NoError(t, err)
	}
	got, _ := repo.ListBySessionUUIDs(ctx, []string{"u1"})
	assert.False(t, got["u1"][0].VerifyExhausted(), "untrusted counters never read as exhausted")
	assert.False(t, got["u1"][0].Terminal())

	repo.MarkPRMaintenanceSucceeded()
	got, _ = repo.ListBySessionUUIDs(ctx, []string{"u1"})
	assert.True(t, got["u1"][0].VerifyExhausted())
	assert.True(t, got["u1"][0].Terminal())
}

func TestEnsureState_ShouldNeverModifyAnExistingRow(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ref := mustPRRef(t, "", "acme", "app", 5)
	first, err := repo.EnsureState(context.Background(), ref, PRInitial{State: domain.PRStateMerged, Terminal: true, URL: "https://github.com/Acme/App/pull/5"})
	require.NoError(t, err)
	second, err := repo.EnsureState(context.Background(), ref, PRInitial{State: domain.PRStateOpen, URL: "other"})
	require.NoError(t, err)
	assert.Equal(t, first, second)
	row := reload(t, repo, ref)
	assert.Equal(t, "merged", row.State)
	assert.True(t, row.Terminal)
	assert.Equal(t, "https://github.com/Acme/App/pull/5", row.URL)
	_, err = repo.EnsureState(context.Background(), domain.PRRef{}, PRInitial{})
	assert.True(t, errors.Is(err, ErrZeroPRRef))
}

// ---- instance <-> storage dual write (Story 2.3) ---------------------------------

// persistedPRSession adds a started session with a uuid to a fresh storage and
// returns both plus the instance.
func persistedPRSession(t *testing.T) (*Storage, *Instance) {
	t.Helper()
	storage, cleanup := createTestStorage(t)
	t.Cleanup(cleanup)
	inst := newTestInstance("pr-session")
	inst.UUID = "uuid-pr-session"
	inst.GitHubOwner, inst.GitHubRepo, inst.Branch = "acme", "app", "feat/x"
	finishInstanceConstruction(inst)
	require.NoError(t, storage.AddInstance(inst))
	return storage, inst
}

func sessionMirror(t *testing.T, storage *Storage, title string) InstanceData {
	t.Helper()
	data, err := storage.Repo.Get(context.Background(), title)
	require.NoError(t, err)
	return *data
}

func TestAttachSessionPR_ShouldReturnSentinels_WhenNoUUIDOrNoRow(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ref := mustPRRef(t, "", "acme", "app", 12)
	_, err := repo.AttachSessionPR(context.Background(), "", ref, PRLink{Source: domain.AttachSourceManual}, PRMirror{})
	assert.ErrorIs(t, err, ErrNoSessionUUID)
	_, err = repo.AttachSessionPR(context.Background(), "no-such-uuid", ref, PRLink{Source: domain.AttachSourceManual}, PRMirror{})
	assert.ErrorIs(t, err, ErrSessionNotPersisted)
	assert.Equal(t, 0, countLinks(t, repo), "a rejected attach must leave no link behind")
	assert.Equal(t, 0, countStates(t, repo))
}

func TestAttachSessionPR_ShouldKeepOneStateAndLink_WhenRefCasingDiffers(t *testing.T) {
	t.Parallel()
	storage, inst := persistedPRSession(t)
	upper, err := domain.NewPRRef("GitHub.com", "Acme", "App", 12)
	require.NoError(t, err)
	lower := mustPRRef(t, "", "acme", "app", 12)
	ctx := context.Background()
	for _, ref := range []domain.PRRef{upper, lower} {
		_, err := storage.AttachSessionPR(ctx, inst.UUID, ref, PRLink{Source: domain.AttachSourceManual}, PRMirror{Number: 12})
		require.NoError(t, err)
	}
	assert.Equal(t, 1, countStates(t, storage.Repo))
	assert.Equal(t, 1, countLinks(t, storage.Repo))
}

func TestAttachSessionPR_ShouldBeAllOrNothing_WhenFailureBeforeMirrorUpdate(t *testing.T) {
	t.Parallel()
	storage, inst := persistedPRSession(t)
	storage.Repo.afterPRLinkHook = func() error { return errors.New("injected failure before mirror update") }

	_, err := storage.AttachSessionPR(context.Background(), inst.UUID, mustPRRef(t, "", "acme", "app", 12),
		PRLink{Source: domain.AttachSourceManual, Initial: PRInitial{State: domain.PRStateOpen}}, PRMirror{Number: 12, URL: "https://github.com/acme/app/pull/12"})
	require.Error(t, err)

	assert.Equal(t, 0, countLinks(t, storage.Repo), "link must roll back")
	assert.Equal(t, 0, countStates(t, storage.Repo), "state row must roll back")
	got := sessionMirror(t, storage, "pr-session")
	assert.Zero(t, got.GitHubPRNumber)
	assert.Empty(t, got.GitHubPRURL)
}

func TestPersistPRMirror_ShouldClearLegacyNumber_WhenLastLinkDetached(t *testing.T) {
	t.Parallel()
	storage, inst := persistedPRSession(t)
	ref := mustPRRef(t, "", "acme", "app", 12)
	_, err := storage.AttachSessionPR(context.Background(), inst.UUID, ref,
		PRLink{Source: domain.AttachSourceManual}, PRMirror{Number: 12, URL: "https://github.com/acme/app/pull/12", Terminal: true})
	require.NoError(t, err)
	require.Equal(t, 12, sessionMirror(t, storage, "pr-session").GitHubPRNumber)

	_, err = storage.Repo.DeleteSessionPR(context.Background(), SessionPROwner(inst.UUID), ref)
	require.NoError(t, err)
	require.NoError(t, storage.PersistPRMirror(inst.UUID, "", 0, false))

	got := sessionMirror(t, storage, "pr-session")
	assert.Zero(t, got.GitHubPRNumber, "the old >0 guard could not clear this")
	assert.Empty(t, got.GitHubPRURL)
	assert.False(t, got.GitHubPRStatusTerminal)
	assert.ErrorIs(t, storage.PersistPRMirror("no-such-uuid", "", 0, false), ErrSessionNotPersisted)
}

func TestLegacyReader_ShouldSeeActivePR_WhenOldSchemaLoadsFlagOnData(t *testing.T) {
	t.Parallel()
	storage, inst := persistedPRSession(t)
	inst.SetPRPersister(storage)
	require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 11), domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen}))
	require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 12), domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen}))

	// An old binary reads only the InstanceData JSON tags, i.e. the session columns.
	rows, err := storage.ListInstanceData()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, 12, rows[0].GitHubPRNumber)
	assert.Equal(t, "https://github.com/acme/app/pull/12", rows[0].GitHubPRURL)
}

func TestAttachPR_ShouldDeferPersist_WhenSessionRowNotYetCreated(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	t.Cleanup(cleanup)
	inst := newTestInstance("deferred-pr")
	inst.UUID = "uuid-deferred"
	inst.GitHubOwner, inst.GitHubRepo = "acme", "app"
	finishInstanceConstruction(inst)
	inst.SetPRPersister(storage)

	require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 12), domain.AttachSourceCreatedFromURL, PRInitial{State: domain.PRStateOpen}))
	assert.True(t, inst.PRPersistPending(), "no session row yet: the write is deferred")
	assert.Equal(t, 12, inst.Snapshot().GitHub.GitHubPRNumber, "the in-memory snapshot already holds the PR")
	assert.Equal(t, 0, countLinks(t, storage.Repo))

	require.NoError(t, storage.AddInstance(inst)) // first full save: state + link + mirror together
	assert.Equal(t, 1, countLinks(t, storage.Repo))
	assert.Equal(t, 1, countStates(t, storage.Repo))
	assert.Equal(t, 12, sessionMirror(t, storage, "deferred-pr").GitHubPRNumber)
}

func TestAttachPR_ShouldRetryOnNextSave_WhenPersistFails(t *testing.T) {
	t.Parallel()
	storage, inst := persistedPRSession(t)
	inst.SetPRPersister(storage)
	failOnce := true
	storage.Repo.afterPRLinkHook = func() error {
		if failOnce {
			failOnce = false
			return errors.New("transient")
		}
		return nil
	}
	require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 12), domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen}))
	require.True(t, inst.PRPersistPending())
	require.Equal(t, 0, countLinks(t, storage.Repo))

	require.NoError(t, storage.SaveInstances([]*Instance{inst}))
	assert.Equal(t, 1, countLinks(t, storage.Repo), "the next save retried the link")
	assert.False(t, inst.PRPersistPending())
}

func TestAttachPR_ShouldAllowLinkOnlyChange_WhenMirrorUnchanged(t *testing.T) {
	t.Parallel()
	storage, inst := persistedPRSession(t)
	inst.SetPRPersister(storage)
	require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 12), domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen}))
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))

	// A second link that does not change the mirror still defeats the unchanged-row skip.
	storage.Repo.afterPRLinkHook = func() error { return errors.New("down") }
	require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 13), domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateMerged, Terminal: true, HeadBranch: "x"}))
	storage.Repo.afterPRLinkHook = nil
	require.Equal(t, 1, countLinks(t, storage.Repo))
	require.NoError(t, storage.SaveInstances([]*Instance{inst}))
	assert.Equal(t, 2, countLinks(t, storage.Repo))
}

// ---- full-save mirror semantics (T2.3.3) -----------------------------------------

func TestFullSave_ShouldClearTerminalFlag_WhenOpenPRAttachedToTerminalSession(t *testing.T) {
	t.Parallel()
	storage, inst := persistedPRSession(t)
	inst.SetPRPersister(nil) // force the full-save path, not the narrow AttachSessionPR write

	require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 11), domain.AttachSourceBranchDiscovery,
		PRInitial{State: domain.PRStateMerged, Terminal: true}))
	require.NoError(t, storage.UpdateInstance(inst))
	got := sessionMirror(t, storage, "pr-session")
	require.True(t, got.GitHubPRStatusTerminal, "all PRs terminal: the flag is persisted true")
	require.Equal(t, 11, got.GitHubPRNumber)

	require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 12), domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen}))
	require.NoError(t, storage.UpdateInstance(inst)) // update path

	rows, err := storage.ListInstanceDataWithWorktree()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.False(t, rows[0].GitHubPRStatusTerminal, "the update path must clear a stale true")
	assert.Equal(t, 12, rows[0].GitHubPRNumber)
	assert.Equal(t, 2, countLinks(t, storage.Repo))

	// Create path: a brand-new row built from the same set carries the cleared flag.
	fresh, cleanup := createTestStorage(t)
	t.Cleanup(cleanup)
	clone := newTestInstance("pr-session-clone")
	clone.UUID, clone.GitHubOwner, clone.GitHubRepo = "uuid-clone", "acme", "app"
	finishInstanceConstruction(clone)
	require.NoError(t, clone.AttachPR(mustPRRef(t, "", "acme", "app", 11), domain.AttachSourceBackfill, PRInitial{State: domain.PRStateMerged, Terminal: true}))
	require.NoError(t, clone.AttachPR(mustPRRef(t, "", "acme", "app", 12), domain.AttachSourceBackfill, PRInitial{State: domain.PRStateOpen}))
	require.NoError(t, fresh.AddInstance(clone))
	assert.False(t, sessionMirror(t, fresh, "pr-session-clone").GitHubPRStatusTerminal)
}

func TestFullSave_ShouldLeaveLegacyColumns_WhenLegacyNumberUnrepresented(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	t.Cleanup(cleanup)
	prLoadFixture(t, storage, "legacy-only", "uuid-legacy-only", "", "", 41)
	loaded, err := storage.LoadInstances()
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	inst := loaded[0]
	want := sessionMirror(t, storage, "legacy-only")

	inst.SetPRPersister(nil) // exercise the full save; the narrow attach write overwrites the mirror by design
	inst.UUID = "uuid-legacy-only"
	inst.GitHubOwner, inst.GitHubRepo = "acme", "app"
	finishInstanceConstruction(inst)
	require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 12), domain.AttachSourceBranchDiscovery, PRInitial{State: domain.PRStateOpen}))
	require.True(t, inst.Snapshot().GitHub.LegacyPRUnrepresented, "an unrelated attach keeps the flag")
	require.NoError(t, storage.UpdateInstance(inst))

	got := sessionMirror(t, storage, "legacy-only")
	assert.Equal(t, want.GitHubPRNumber, got.GitHubPRNumber)
	assert.Equal(t, want.GitHubPRURL, got.GitHubPRURL)
}

func TestHookProcess_ShouldNotChangeLegacyColumns_WhenLinklessSessionSavedViaSsqHooksPath(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "hook-path.db")
	// The ssq-hooks process opens the database exactly like this (cmd/ssq-hooks/main.go loadStorage).
	open := func() *Storage {
		repo, err := NewEntRepository(WithDatabasePath(dbPath))
		require.NoError(t, err)
		storage, err := NewStorageWithRepository(repo)
		require.NoError(t, err)
		return storage
	}
	owner := open()
	prLoadFixture(t, owner, "hook-session", "uuid-hook", "acme", "app", 41)
	want := sessionMirror(t, owner, "hook-session")
	require.NoError(t, owner.Repo.UpdateGitHubPRStatusTerminal(context.Background(), "hook-session", true))
	want.GitHubPRStatusTerminal = true
	owner.Close()

	hook := open() // the hook process: open, read rules, write analytics, close
	loaded, err := hook.LoadInstances()
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	// Even the worst case, a full save of the loaded session, leaves the columns alone.
	require.NoError(t, hook.UpdateInstance(loaded[0]))
	hook.Close()

	check := open()
	defer check.Close()
	got := sessionMirror(t, check, "hook-session")
	assert.Equal(t, want.GitHubPRNumber, got.GitHubPRNumber)
	assert.Equal(t, want.GitHubPRURL, got.GitHubPRURL)
	assert.Equal(t, want.GitHubPRStatusTerminal, got.GitHubPRStatusTerminal)
}

func TestSessionUUIDsByPR_ShouldReturnSessionLinksOnly_WhenItemAndOtherPRLinked(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ref := mustPRRef(t, "", "acme", "app", 12)
	upsertLink(t, repo, SessionPROwner("u2"), ref, domain.AttachSourceManual)
	upsertLink(t, repo, SessionPROwner("u1"), ref, domain.AttachSourceManual)
	upsertLink(t, repo, ItemPROwner("item-1"), ref, domain.AttachSourceManual)
	upsertLink(t, repo, SessionPROwner("u3"), mustPRRef(t, "", "acme", "app", 13), domain.AttachSourceManual)

	got, err := repo.SessionUUIDsByPR(context.Background(), mustPRRef(t, "", "Acme", "App", 12))
	require.NoError(t, err)
	assert.Equal(t, []string{"u1", "u2"}, got)

	none, err := repo.SessionUUIDsByPR(context.Background(), mustPRRef(t, "", "acme", "app", 99))
	require.NoError(t, err)
	assert.Empty(t, none)

	_, err = repo.SessionUUIDsByPR(context.Background(), domain.PRRef{})
	assert.ErrorIs(t, err, ErrZeroPRRef)
}

func TestUpdatePullRequestState_ShouldPersistRateLimitReset_OnNoteAndCountedFailures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, kind := range []PRObservationKind{PRObservationErrorNote, PRObservationCountedFailure} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			repo := newPRRepo(t)
			ref := mustPRRef(t, "", "acme", "app", 5)
			seedState(t, repo, ref, domain.PRStateOpen, false)
			reset := prRepoT0.Add(30 * time.Minute)

			obs := failObs(prRepoT0, kind, "rate_limited")
			obs.RateLimitResetAt = reset
			_, err := repo.UpdatePullRequestState(ctx, ref, obs)
			require.NoError(t, err)
			row := reload(t, repo, ref)
			require.NotNil(t, row.RateLimitResetAt)
			assert.True(t, reset.Equal(*row.RateLimitResetAt), "got %v want %v", row.RateLimitResetAt, reset)

			// A later failure without a reset must not clear it.
			_, err = repo.UpdatePullRequestState(ctx, ref, failObs(prRepoT0.Add(time.Minute), kind, "network"))
			require.NoError(t, err)
			row = reload(t, repo, ref)
			require.NotNil(t, row.RateLimitResetAt)
			assert.True(t, reset.Equal(*row.RateLimitResetAt))
		})
	}
}
