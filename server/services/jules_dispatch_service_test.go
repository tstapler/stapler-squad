package services

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	gogit "github.com/go-git/go-git/v5"
	gogitconfig "github.com/go-git/go-git/v5/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/jules"
	"github.com/tstapler/stapler-squad/session"
)

// newTestJulesRepoWithRemote creates a real (empty) git repository at a
// fresh temp directory with an "origin" remote pointing at remoteURL, so
// resolveJulesOwnerRepo(dir) resolves exactly as it would against a real
// worktree — mirroring session.newTestRepoWithRemote
// (session/backlog_lifecycle_pr_trigger_test.go), which this package cannot
// import directly (unexported, different package).
func newTestJulesRepoWithRemote(t *testing.T, remoteURL string) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)
	_, err = repo.CreateRemote(&gogitconfig.RemoteConfig{Name: "origin", URLs: []string{remoteURL}})
	require.NoError(t, err)
	return dir
}

// fakeSourceLister is a fake jules.sourceLister (unexported in package
// jules, satisfied structurally) so jules.NewJulesSourceRegistry can be
// exercised without a real HTTP round trip.
type fakeSourceLister struct {
	sources []jules.JulesSource
	err     error
}

func (f *fakeSourceLister) ListSources(_ context.Context) ([]jules.JulesSource, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sources, nil
}

// newTestJulesSourceRegistry returns a *jules.JulesSourceRegistry that
// resolves "tstapler/stapler-squad" to "sources/github-tstapler-stapler-squad" —
// matching newTestJulesRepoWithRemote's default remote below.
func newTestJulesSourceRegistry() *jules.JulesSourceRegistry {
	return jules.NewJulesSourceRegistry(&fakeSourceLister{
		sources: []jules.JulesSource{{Name: "sources/github-tstapler-stapler-squad", ID: "src-1"}},
	})
}

// fakeJulesSessionCreator is a fake julesSessionCreator with a call counter
// and injectable error/delay/result, plus an optional beforeCreate hook so a
// test can inspect storage state mid-call (e.g. the reservation row's
// pending SessionUUID prefix, before CreateSession returns and
// DispatchToJules swaps it for the real name).
type fakeJulesSessionCreator struct {
	mu           sync.Mutex
	calls        int
	delay        time.Duration
	err          error
	resultName   jules.JulesSessionName
	beforeCreate func()
}

func (f *fakeJulesSessionCreator) CreateSession(_ context.Context, _ jules.CreateSessionRequest) (*jules.JulesSession, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.beforeCreate != nil {
		f.beforeCreate()
	}
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.err != nil {
		return nil, f.err
	}
	name := f.resultName
	if name == "" {
		name = "sessions/xyz"
	}
	return &jules.JulesSession{Name: name, State: jules.JulesStateQueued}, nil
}

func (f *fakeJulesSessionCreator) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeJulesTransitionCall records one transitionWithGuard invocation for
// assertion.
type fakeJulesTransitionCall struct {
	itemID                string
	to                    session.BacklogStatus
	triggeredBy           string
	hasUnresolvedBlockers bool
}

// fakeJulesTransitionGuard is a fake julesTransitionGuard: it records every
// transitionWithGuard call and lets a test control hasUnresolvedBlockers and
// the transition outcome, without needing a real *BacklogService.
type fakeJulesTransitionGuard struct {
	mu              sync.Mutex
	hasBlockers     bool
	hasBlockersErr  error
	transitionErr   error
	transitionCalls []fakeJulesTransitionCall
}

func (f *fakeJulesTransitionGuard) hasUnresolvedBlockers(_ context.Context, _ string) (bool, error) {
	return f.hasBlockers, f.hasBlockersErr
}

func (f *fakeJulesTransitionGuard) transitionWithGuard(_ context.Context, item *session.BacklogItemData, to session.BacklogStatus, _ *session.BacklogItemPrecondition, triggeredBy string, hasUnresolvedBlockers bool) (*session.BacklogItemData, error) {
	f.mu.Lock()
	f.transitionCalls = append(f.transitionCalls, fakeJulesTransitionCall{
		itemID: item.ID, to: to, triggeredBy: triggeredBy, hasUnresolvedBlockers: hasUnresolvedBlockers,
	})
	f.mu.Unlock()
	if f.transitionErr != nil {
		return nil, f.transitionErr
	}
	item.Status = string(to)
	return item, nil
}

func (f *fakeJulesTransitionGuard) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.transitionCalls)
}

// newTestJulesConfig returns a *config.Config with Jules enabled,
// repoPath pre-acknowledged, and generous (but non-default) spend caps —
// the "everything else is satisfied" baseline most guard tests start from.
func newTestJulesConfig(repoPath string) *config.Config {
	return &config.Config{
		Jules: config.JulesConfig{
			Enabled:                    true,
			EgressAcknowledgedRepos:    []string{repoPath},
			MaxConcurrentJulesSessions: 2,
			MaxJulesSessionsPerDay:     15,
		},
	}
}

// createTestJulesReadyItem creates a ready backlog item backed by a real git
// repo (remote "https://github.com/tstapler/stapler-squad.git") so
// resolveJulesOwnerRepo resolves it to tstapler/stapler-squad, matching
// newTestJulesSourceRegistry's registered source.
func createTestJulesReadyItem(t *testing.T, ctx context.Context, storage *session.Storage, title string) *session.BacklogItemData {
	t.Helper()
	repoPath := newTestJulesRepoWithRemote(t, "https://github.com/tstapler/stapler-squad.git")
	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:    title,
		Status:   string(session.BacklogStatusReady),
		RepoPath: repoPath,
	})
	require.NoError(t, err)
	return item
}

func TestJulesDispatchService_DispatchToJules_should_ReserveCreateConfirmAndTransition_When_ClientSucceeds(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()

	item := createTestJulesReadyItem(t, ctx, storage, "dispatch success item")
	cfg := newTestJulesConfig(item.RepoPath)
	guard := &fakeJulesTransitionGuard{hasBlockers: false}
	client := &fakeJulesSessionCreator{}
	client.beforeCreate = func() {
		sessions, err := storage.ListItemSessions(ctx, item.ID)
		require.NoError(t, err)
		require.Len(t, sessions, 1, "reservation row must exist before the billed CreateSession call")
		assert.Equal(t, session.SessionRoleJulesWork, sessions[0].Role)
		assert.True(t, strings.HasPrefix(sessions[0].SessionUUID, julesPendingUUIDPrefix),
			"reservation SessionUUID must carry the pending prefix before CreateSession returns, got %q", sessions[0].SessionUUID)
	}
	svc := NewJulesDispatchService(storage, guard, client, newTestJulesSourceRegistry(), func() *config.Config { return cfg })

	req, err := NewJulesDispatchRequest(item.ID, "backlog/item-1", "Fix the flaky poller test")
	require.NoError(t, err)

	name, err := svc.DispatchToJules(ctx, item, req)
	require.NoError(t, err)
	assert.Equal(t, jules.JulesSessionName("sessions/xyz"), name)
	assert.Equal(t, 1, client.callCount(), "CreateSession must be called exactly once")

	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, session.SessionRoleJulesWork, sessions[0].Role)
	assert.Equal(t, "jules-sessions/xyz", sessions[0].SessionUUID)

	require.Equal(t, 1, guard.callCount())
	call := guard.transitionCalls[0]
	assert.Equal(t, item.ID, call.itemID)
	assert.Equal(t, session.BacklogStatusInProgress, call.to)
	assert.Equal(t, session.TriggeredByUser, call.triggeredBy)
	assert.False(t, call.hasUnresolvedBlockers, "must be sourced from the real hasUnresolvedBlockers check, not a hardcoded literal")
}

func TestJulesDispatchService_DispatchToJules_should_ReturnErrJulesDispatchInFlight_When_PersistedOpenSessionFoundAfterFirstCallReturned(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()

	item := createTestJulesReadyItem(t, ctx, storage, "persisted duplicate guard item")
	cfg := newTestJulesConfig(item.RepoPath)
	registry := newTestJulesSourceRegistry()
	guard := &fakeJulesTransitionGuard{}

	client1 := &fakeJulesSessionCreator{}
	svc1 := NewJulesDispatchService(storage, guard, client1, registry, func() *config.Config { return cfg })
	req, err := NewJulesDispatchRequest(item.ID, "backlog/item-1", "First dispatch")
	require.NoError(t, err)

	_, err = svc1.DispatchToJules(ctx, item, req)
	require.NoError(t, err, "first call must succeed and its mutex must already be released on return")

	// A second, independently-constructed service instance — proves the
	// guard is the persisted row, not svc1's in-process mutex.
	client2 := &fakeJulesSessionCreator{}
	svc2 := NewJulesDispatchService(storage, guard, client2, registry, func() *config.Config { return cfg })

	_, err = svc2.DispatchToJules(ctx, item, req)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrJulesDispatchInFlight)
	assert.Equal(t, 0, client2.callCount(), "the second call must never reach CreateSession")
}

func TestJulesDispatchService_DispatchToJules_should_CollapseConcurrentDoubleClicksToOneCreateViaMutex_When_TwoGoroutinesRaceBeforePersistedRowExists(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()

	item := createTestJulesReadyItem(t, ctx, storage, "mutex race item")
	cfg := newTestJulesConfig(item.RepoPath)
	guard := &fakeJulesTransitionGuard{}
	client := &fakeJulesSessionCreator{delay: 50 * time.Millisecond}
	svc := NewJulesDispatchService(storage, guard, client, newTestJulesSourceRegistry(), func() *config.Config { return cfg })

	req, err := NewJulesDispatchRequest(item.ID, "backlog/item-1", "Racing dispatch")
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	for i := range 2 {
		go func(idx int) {
			defer wg.Done()
			_, callErr := svc.DispatchToJules(ctx, item, req)
			errs[idx] = callErr
		}(i)
	}
	wg.Wait()

	successCount, inFlightCount := 0, 0
	for _, callErr := range errs {
		switch {
		case callErr == nil:
			successCount++
		case assert.ErrorIs(t, callErr, ErrJulesDispatchInFlight):
			inFlightCount++
		}
	}
	assert.Equal(t, 1, successCount, "exactly one goroutine must succeed")
	assert.Equal(t, 1, inFlightCount, "the other must be rejected by the mutex, not silently swallowed")
	assert.Equal(t, 1, client.callCount(), "CreateSession must be called exactly once")
}

func TestJulesDispatchService_DispatchToJules_should_RejectWithErrUnresolvedBlockers_When_ItemHasUnresolvedBlocker(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()

	item := createTestJulesReadyItem(t, ctx, storage, "blocker gate item")
	cfg := newTestJulesConfig(item.RepoPath)
	guard := &fakeJulesTransitionGuard{hasBlockers: true}
	client := &fakeJulesSessionCreator{}
	svc := NewJulesDispatchService(storage, guard, client, newTestJulesSourceRegistry(), func() *config.Config { return cfg })

	req, err := NewJulesDispatchRequest(item.ID, "backlog/item-1", "Blocked dispatch")
	require.NoError(t, err)

	_, err = svc.DispatchToJules(ctx, item, req)
	require.Error(t, err)
	assert.ErrorIs(t, err, session.ErrUnresolvedBlockers)
	assert.Equal(t, 0, client.callCount())

	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	assert.Empty(t, sessions, "no reservation row may be created before the blocker gate")
}

func TestJulesDispatchService_DispatchToJules_should_EndReservationWithDispatchFailedReason_When_CreateSessionFails(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()

	item := createTestJulesReadyItem(t, ctx, storage, "create failure item")
	cfg := newTestJulesConfig(item.RepoPath)
	guard := &fakeJulesTransitionGuard{}
	client := &fakeJulesSessionCreator{err: jules.ErrJulesTransient}
	svc := NewJulesDispatchService(storage, guard, client, newTestJulesSourceRegistry(), func() *config.Config { return cfg })

	req, err := NewJulesDispatchRequest(item.ID, "backlog/item-1", "Failing dispatch")
	require.NoError(t, err)

	_, err = svc.DispatchToJules(ctx, item, req)
	require.Error(t, err)
	assert.ErrorIs(t, err, jules.ErrJulesTransient)

	sessions, err := storage.ListItemSessions(ctx, item.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.NotNil(t, sessions[0].EndedAt, "the reservation must be ended, not left as an orphan claim")
	assert.Equal(t, "dispatch_failed", sessions[0].EndReason)

	refreshed, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusReady), refreshed.Status, "item must stay in ready")

	notes, err := storage.ListProgressNotesForItem(ctx, item.ID)
	require.NoError(t, err)
	require.NotEmpty(t, notes)
	assert.Contains(t, notes[len(notes)-1].Note, "Jules dispatch failed", "failure must be visible, not silent")
}

func TestJulesDispatchService_DispatchToJules_should_RejectWithConcurrencyCapMessage_When_OpenSessionsAtCeiling(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()

	// Two open jules_work sessions on other items, at the (default) ceiling.
	for i := 0; i < 2; i++ {
		other, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
			Title:  "other item",
			Status: string(session.BacklogStatusInProgress),
		})
		require.NoError(t, err)
		_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
			ItemID:      other.ID,
			SessionUUID: "jules-sessions/other-" + other.ID,
			SessionRole: session.SessionRoleJulesWork,
		})
		require.NoError(t, err)
	}

	item := createTestJulesReadyItem(t, ctx, storage, "concurrency cap item")
	cfg := newTestJulesConfig(item.RepoPath)
	cfg.Jules.MaxConcurrentJulesSessions = 2
	guard := &fakeJulesTransitionGuard{}
	client := &fakeJulesSessionCreator{}
	svc := NewJulesDispatchService(storage, guard, client, newTestJulesSourceRegistry(), func() *config.Config { return cfg })

	req, err := NewJulesDispatchRequest(item.ID, "backlog/item-1", "Over the cap")
	require.NoError(t, err)

	_, err = svc.DispatchToJules(ctx, item, req)
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "2 Jules sessions are already running (limit 2)")
	assert.Equal(t, 0, client.callCount())
}

// TestJulesDispatchService_DispatchToJules_should_AllowExactlyOneSuccess_When_TwoDifferentItemsRaceAtConcurrencyCeiling
// is the regression test for the cross-item spend-guard TOCTOU race
// (Bug 2): checkSpendGuards's open-session count and the reservation write
// are only serialized per-item by itemMutex, so two *different* items
// dispatched concurrently near MaxConcurrentJulesSessions could each
// independently observe a stale under-limit count and both proceed to
// CreateSession, overshooting the ceiling. Models
// TestJulesDispatchService_DispatchToJules_should_CollapseConcurrentDoubleClicksToOneCreateViaMutex_When_TwoGoroutinesRaceBeforePersistedRowExists
// above, but with two distinct item IDs sharing one JulesDispatchService
// instance (so itemMutex alone cannot serialize them) instead of one item
// dispatched twice.
func TestJulesDispatchService_DispatchToJules_should_AllowExactlyOneSuccess_When_TwoDifferentItemsRaceAtConcurrencyCeiling(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()

	item1 := createTestJulesReadyItem(t, ctx, storage, "cross-item race item 1")
	item2 := createTestJulesReadyItem(t, ctx, storage, "cross-item race item 2")
	cfg := newTestJulesConfig(item1.RepoPath)
	cfg.Jules.EgressAcknowledgedRepos = append(cfg.Jules.EgressAcknowledgedRepos, item2.RepoPath)
	cfg.Jules.MaxConcurrentJulesSessions = 1
	guard := &fakeJulesTransitionGuard{}
	client := &fakeJulesSessionCreator{delay: 50 * time.Millisecond}
	svc := NewJulesDispatchService(storage, guard, client, newTestJulesSourceRegistry(), func() *config.Config { return cfg })

	req1, err := NewJulesDispatchRequest(item1.ID, "backlog/item-1", "Racing dispatch for item 1")
	require.NoError(t, err)
	req2, err := NewJulesDispatchRequest(item2.ID, "backlog/item-2", "Racing dispatch for item 2")
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, callErr := svc.DispatchToJules(ctx, item1, req1)
		errs[0] = callErr
	}()
	go func() {
		defer wg.Done()
		_, callErr := svc.DispatchToJules(ctx, item2, req2)
		errs[1] = callErr
	}()
	wg.Wait()

	successCount, capRejectedCount := 0, 0
	for _, callErr := range errs {
		switch {
		case callErr == nil:
			successCount++
		case assert.ErrorIs(t, callErr, errJulesConcurrencyCapReached):
			capRejectedCount++
		}
	}
	assert.Equal(t, 1, successCount, "exactly one of the two different items must succeed")
	assert.Equal(t, 1, capRejectedCount, "the other must be rejected by the concurrency cap, not silently allowed through")
	assert.Equal(t, 1, client.callCount(), "CreateSession must be called exactly once — the ceiling must never be overshot")
}

func TestJulesDispatchService_DispatchToJules_should_RejectWithDailyCapMessage_When_TwentyFourHourCountAtLimit(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()

	// 15 confirmed, ended jules_work sessions created (and completed) in the
	// trailing 24h — zero currently open, but the daily cap still trips.
	const dailyLimit = 15
	for i := 0; i < dailyLimit; i++ {
		other, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
			Title:  "completed jules item",
			Status: string(session.BacklogStatusDone),
		})
		require.NoError(t, err)
		is, err := storage.CreateItemSession(ctx, session.ItemSessionData{
			ItemID:      other.ID,
			SessionUUID: "jules-sessions/done-" + other.ID,
			SessionRole: session.SessionRoleJulesWork,
		})
		require.NoError(t, err)
		require.NoError(t, storage.UpdateItemSessionEnded(ctx, is.ID, time.Now()))
	}

	item := createTestJulesReadyItem(t, ctx, storage, "daily cap item")
	cfg := newTestJulesConfig(item.RepoPath)
	cfg.Jules.MaxJulesSessionsPerDay = dailyLimit
	guard := &fakeJulesTransitionGuard{}
	client := &fakeJulesSessionCreator{}
	svc := NewJulesDispatchService(storage, guard, client, newTestJulesSourceRegistry(), func() *config.Config { return cfg })

	req, err := NewJulesDispatchRequest(item.ID, "backlog/item-1", "Over the daily cap")
	require.NoError(t, err)

	_, err = svc.DispatchToJules(ctx, item, req)
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "limit 15")
	assert.Equal(t, 0, client.callCount())
}

func TestJulesDispatchService_DispatchToJules_should_RejectNamingRepo_When_EgressNotAcknowledged(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()

	item := createTestJulesReadyItem(t, ctx, storage, "unacknowledged repo item")
	cfg := &config.Config{Jules: config.JulesConfig{Enabled: true}} // EgressAcknowledgedRepos left empty
	guard := &fakeJulesTransitionGuard{}
	client := &fakeJulesSessionCreator{}
	svc := NewJulesDispatchService(storage, guard, client, newTestJulesSourceRegistry(), func() *config.Config { return cfg })

	req, err := NewJulesDispatchRequest(item.ID, "backlog/item-1", "Needs consent")
	require.NoError(t, err)

	_, err = svc.DispatchToJules(ctx, item, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tstapler/stapler-squad")
	assert.Contains(t, err.Error(), "Google's cloud VM")
	assert.ErrorIs(t, err, ErrJulesEgressNotAcknowledged,
		"classifyJulesDispatchError (backlog_service_jules.go) keys off this sentinel, not the message text")
	assert.Equal(t, 0, client.callCount())
}

func TestJulesDispatchService_DispatchToJules_should_ProceedWithoutReconfirmation_When_RepoAlreadyAcknowledged(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()

	item := createTestJulesReadyItem(t, ctx, storage, "already acknowledged item")
	// EgressAcknowledgedRepos pre-populated exactly as ConfirmEgressConsent
	// (Story 2.4.2) would have written it — never via the request itself.
	cfg := newTestJulesConfig(item.RepoPath)
	guard := &fakeJulesTransitionGuard{}
	client := &fakeJulesSessionCreator{}
	svc := NewJulesDispatchService(storage, guard, client, newTestJulesSourceRegistry(), func() *config.Config { return cfg })

	req, err := NewJulesDispatchRequest(item.ID, "backlog/item-1", "Already consented")
	require.NoError(t, err)

	_, err = svc.DispatchToJules(ctx, item, req)
	require.NoError(t, err)
	assert.Equal(t, 1, client.callCount())
}

func TestJulesDispatchService_DispatchToJules_should_ReturnErrJulesNotConfigured_When_EnabledFalse(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()

	item := createTestJulesReadyItem(t, ctx, storage, "disabled feature item")
	cfg := newTestJulesConfig(item.RepoPath)
	cfg.Jules.Enabled = false // valid key (assumed resolvable) and acknowledged repo, but disabled
	guard := &fakeJulesTransitionGuard{}
	client := &fakeJulesSessionCreator{}
	svc := NewJulesDispatchService(storage, guard, client, newTestJulesSourceRegistry(), func() *config.Config { return cfg })

	req, err := NewJulesDispatchRequest(item.ID, "backlog/item-1", "Disabled")
	require.NoError(t, err)

	_, err = svc.DispatchToJules(ctx, item, req)
	require.Error(t, err)
	assert.ErrorIs(t, err, jules.ErrJulesNotConfigured)
	assert.Equal(t, 0, client.callCount())
}

func TestJulesDispatchService_DispatchToJules_should_LeaveEgressAcknowledgedReposUnchanged_When_CalledTwentyTimesForUnacknowledgedRepo(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()

	item := createTestJulesReadyItem(t, ctx, storage, "no self-grant item")
	cfg := &config.Config{Jules: config.JulesConfig{Enabled: true}} // EgressAcknowledgedRepos left empty
	guard := &fakeJulesTransitionGuard{}
	client := &fakeJulesSessionCreator{}
	svc := NewJulesDispatchService(storage, guard, client, newTestJulesSourceRegistry(), func() *config.Config { return cfg })

	req, err := NewJulesDispatchRequest(item.ID, "backlog/item-1", "Repeated unacknowledged attempts")
	require.NoError(t, err)

	for i := 0; i < 20; i++ {
		_, err := svc.DispatchToJules(ctx, item, req)
		require.Error(t, err)
		assert.Empty(t, cfg.Jules.EgressAcknowledgedRepos, "call %d must not have written to EgressAcknowledgedRepos", i)
	}
	assert.Equal(t, 0, client.callCount())
}

// TestJulesDispatchService_DispatchToJules_should_ObserveConfigWriteOnVeryNextCall_When_ConfirmEgressConsentRunsBetweenDispatches
// is the regression test for the frozen-*config.Config-pointer bug: a
// JulesDispatchService built with cfgFn = config.LoadConfig (mirroring
// server/dependencies.go's real wiring) must observe a config.json write
// made by JulesConfigService.ConfirmEgressConsent on its very next
// DispatchToJules call, in the same process, with no restart. Before the
// fix, JulesDispatchService held a *config.Config snapshotted once at
// construction time, so this second dispatch would still fail with
// ErrJulesEgressNotAcknowledged even after ConfirmEgressConsent succeeded.
func TestJulesDispatchService_DispatchToJules_should_ObserveConfigWriteOnVeryNextCall_When_ConfirmEgressConsentRunsBetweenDispatches(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	storage := createTestStorage(t)
	ctx := t.Context()

	item := createTestJulesReadyItem(t, ctx, storage, "live config reload item")
	require.NoError(t, config.SaveConfig(&config.Config{Jules: config.JulesConfig{
		Enabled:                    true,
		MaxConcurrentJulesSessions: 2,
		MaxJulesSessionsPerDay:     15,
		// EgressAcknowledgedRepos deliberately left empty — confirmed mid-test
		// via the real ConfirmEgressConsent RPC, not pre-populated here.
	}}))

	guard := &fakeJulesTransitionGuard{}
	client := &fakeJulesSessionCreator{}
	// cfgFn is config.LoadConfig itself — the exact production wiring
	// (server/dependencies.go) — not a closure over a fixed *config.Config,
	// so this test cannot pass by accident the way a fake in-memory cfgFn
	// could.
	dispatchSvc := NewJulesDispatchService(storage, guard, client, newTestJulesSourceRegistry(), config.LoadConfig)
	req, err := NewJulesDispatchRequest(item.ID, "backlog/item-1", "Needs consent")
	require.NoError(t, err)

	_, err = dispatchSvc.DispatchToJules(ctx, item, req)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrJulesEgressNotAcknowledged, "repo must not be acknowledged yet")
	assert.Equal(t, 0, client.callCount())

	configSvc := NewJulesConfigService(nil, nil, nil)
	_, err = configSvc.ConfirmEgressConsent(ctx, connect.NewRequest(&sessionv1.ConfirmEgressConsentRequest{RepoPath: item.RepoPath}))
	require.NoError(t, err)

	name, err := dispatchSvc.DispatchToJules(ctx, item, req)
	require.NoError(t, err, "the very next dispatch call, in the same process, must see ConfirmEgressConsent's write with no restart")
	assert.Equal(t, jules.JulesSessionName("sessions/xyz"), name)
	assert.Equal(t, 1, client.callCount())
}

// TestJulesDispatchService_checkEgressConsent_should_HaveNoConsentParameter_When_SignatureInspected
// is a compile-level signature check (Task 2.2.3b): checkEgressConsent takes
// only a *session.BacklogItemData and returns only an error — no
// boolean/EgressAcknowledged-shaped parameter a future edit could
// reintroduce and have DispatchToJules silently trust.
func TestJulesDispatchService_checkEgressConsent_should_HaveNoConsentParameter_When_SignatureInspected(t *testing.T) {
	t.Parallel()
	svc := &JulesDispatchService{}
	methodType := reflect.TypeOf(svc.checkEgressConsent)

	require.Equal(t, 1, methodType.NumIn(), "checkEgressConsent must take exactly one parameter")
	assert.Equal(t, reflect.TypeOf(&session.BacklogItemData{}), methodType.In(0),
		"checkEgressConsent's only parameter must be *session.BacklogItemData, never a caller-supplied consent flag")

	require.Equal(t, 1, methodType.NumOut(), "checkEgressConsent must return exactly one value")
	assert.Equal(t, reflect.TypeOf((*error)(nil)).Elem(), methodType.Out(0))
}

// --- Epic 2.4.3: BacklogService.DispatchToJules RPC-level tests ---

// createTestJulesReadyItemForRPC is createTestJulesReadyItem plus
// SkipPlanning: true — the RPC-level tests below exercise the real
// *BacklogService.transitionWithGuard (not fakeJulesTransitionGuard), which
// additionally enforces "plan approved or skip_planning" before allowing the
// in_progress transition; that gate is orthogonal to what Epic 2.4.3 tests,
// so it's satisfied here rather than exercised.
func createTestJulesReadyItemForRPC(t *testing.T, ctx context.Context, storage *session.Storage, title string) *session.BacklogItemData {
	t.Helper()
	repoPath := newTestJulesRepoWithRemote(t, "https://github.com/tstapler/stapler-squad.git")
	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:        title,
		Status:       string(session.BacklogStatusReady),
		RepoPath:     repoPath,
		SkipPlanning: true,
	})
	require.NoError(t, err)
	return item
}

// newTestBacklogServiceWithJules builds a *BacklogService wired exactly like
// production (server/dependencies.go, Task 2.4.4a): the service itself is
// passed as JulesDispatchService's transitionGuard, since *BacklogService
// satisfies julesTransitionGuard structurally.
func newTestBacklogServiceWithJules(storage *session.Storage, cfg *config.Config, client julesSessionCreator, sources *jules.JulesSourceRegistry) *BacklogService {
	svc := NewBacklogService(storage, nil, cfg, nil, nil, nil)
	dispatchSvc := NewJulesDispatchService(storage, svc, client, sources, func() *config.Config { return cfg })
	svc.SetJulesDispatcher(dispatchSvc)
	return svc
}

func TestBacklogService_DispatchToJules_should_ReturnItemSessionWithJulesWorkRole_When_RequestValid(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()

	item := createTestJulesReadyItemForRPC(t, ctx, storage, "rpc dispatch success item")
	cfg := newTestJulesConfig(item.RepoPath)
	svc := newTestBacklogServiceWithJules(storage, cfg, &fakeJulesSessionCreator{}, newTestJulesSourceRegistry())

	resp, err := svc.DispatchToJules(ctx, connect.NewRequest(&sessionv1.DispatchToJulesRequest{
		ItemId: item.ID,
		Branch: "backlog/item-1",
		Prompt: "Fix the flaky poller test",
	}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.ItemSession)
	assert.Equal(t, string(session.SessionRoleJulesWork), resp.Msg.ItemSession.SessionRole)
	assert.True(t, strings.HasPrefix(resp.Msg.ItemSession.SessionUuid, "jules-sessions/"),
		"session_uuid must start with jules-sessions/, got %q", resp.Msg.ItemSession.SessionUuid)

	refreshed, err := storage.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(session.BacklogStatusInProgress), refreshed.Status)
}

func TestBacklogService_DispatchToJules_should_ReturnInvalidArgument_When_BranchEmpty(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()

	item := createTestJulesReadyItem(t, ctx, storage, "rpc missing branch item")
	cfg := newTestJulesConfig(item.RepoPath)
	svc := newTestBacklogServiceWithJules(storage, cfg, &fakeJulesSessionCreator{}, newTestJulesSourceRegistry())

	_, err := svc.DispatchToJules(ctx, connect.NewRequest(&sessionv1.DispatchToJulesRequest{
		ItemId: item.ID,
		Branch: "",
		Prompt: "Fix the flaky poller test",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "pushed to GitHub")
}

func TestBacklogService_DispatchToJules_should_ReturnFailedPrecondition_When_ConcurrencyCeilingReached(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()

	item1 := createTestJulesReadyItemForRPC(t, ctx, storage, "rpc concurrency cap item 1")
	item2 := createTestJulesReadyItemForRPC(t, ctx, storage, "rpc concurrency cap item 2")
	cfg := newTestJulesConfig(item1.RepoPath)
	cfg.Jules.EgressAcknowledgedRepos = append(cfg.Jules.EgressAcknowledgedRepos, item2.RepoPath)
	cfg.Jules.MaxConcurrentJulesSessions = 1
	svc := newTestBacklogServiceWithJules(storage, cfg, &fakeJulesSessionCreator{}, newTestJulesSourceRegistry())

	_, err := svc.DispatchToJules(ctx, connect.NewRequest(&sessionv1.DispatchToJulesRequest{
		ItemId: item1.ID,
		Branch: "backlog/item-1",
		Prompt: "First dispatch fills the one slot",
	}))
	require.NoError(t, err, "first dispatch must succeed and consume the only slot")

	_, err = svc.DispatchToJules(ctx, connect.NewRequest(&sessionv1.DispatchToJulesRequest{
		ItemId: item2.ID,
		Branch: "backlog/item-2",
		Prompt: "Second dispatch must be rejected by the cap",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestBacklogService_DispatchToJules_should_ReturnFailedPreconditionPointingAtSettings_When_JulesDisabled(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()

	item := createTestJulesReadyItem(t, ctx, storage, "rpc disabled item")
	cfg := newTestJulesConfig(item.RepoPath)
	cfg.Jules.Enabled = false
	svc := newTestBacklogServiceWithJules(storage, cfg, &fakeJulesSessionCreator{}, newTestJulesSourceRegistry())

	_, err := svc.DispatchToJules(ctx, connect.NewRequest(&sessionv1.DispatchToJulesRequest{
		ItemId: item.ID,
		Branch: "backlog/item-1",
		Prompt: "Should be rejected because Jules is disabled",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "Settings")
	assert.Contains(t, err.Error(), "Jules")
}

func TestBacklogService_DispatchToJules_should_ReturnFailedPreconditionDirectingToDispatchDialog_When_RepoNotAcknowledged(t *testing.T) {
	t.Parallel()
	storage := createTestStorage(t)
	ctx := t.Context()

	item := createTestJulesReadyItem(t, ctx, storage, "rpc unacknowledged repo item")
	cfg := &config.Config{Jules: config.JulesConfig{
		Enabled:                    true,
		MaxConcurrentJulesSessions: 2,
		MaxJulesSessionsPerDay:     15,
		// EgressAcknowledgedRepos deliberately left empty — item.RepoPath was
		// never confirmed via ConfirmEgressConsent.
	}}
	svc := newTestBacklogServiceWithJules(storage, cfg, &fakeJulesSessionCreator{}, newTestJulesSourceRegistry())

	_, err := svc.DispatchToJules(ctx, connect.NewRequest(&sessionv1.DispatchToJulesRequest{
		ItemId: item.ID,
		Branch: "backlog/item-1",
		Prompt: "Should be rejected — repo not acknowledged for cloud egress",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "dispatch dialog")
}
