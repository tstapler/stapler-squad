package services

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/testutil"
)

// TestArchiveSession_SetsStatusStopped is a regression test for the bug where
// ArchiveSession set ArchivedAt but left Status at whatever it was before
// archiving (e.g. Active), leaving the two out of sync. The retention sweep (and
// anything else gated on Status) depends on an archived session also being Stopped.
func TestArchiveSession_SetsStatusStopped(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	addPausedSession(t, fix, "archive-me")

	req := connect.NewRequest(&sessionv1.ArchiveSessionRequest{SessionId: "archive-me"})
	resp, err := fix.svc.ArchiveSession(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	inst := fix.poller.FindInstance("archive-me")
	require.NotNil(t, inst, "expected instance still resolvable by title after archiving")

	snap := inst.Snapshot()
	assert.NotNil(t, snap.ArchivedAt, "expected ArchivedAt to be set")
	assert.Equal(t, session.Stopped, snap.Status, "expected Status to transition to Stopped when archiving")
}

// addLivePausedInstance persists a Paused instance under storage/uuid/title and wires it
// into fix.poller, returning the loaded (storage-backed) *session.Instance — the
// add+load+find+addInstanceToPoller boilerplate shared by the ArchiveSessionByUUID tests
// below, which all need a live, storage-backed instance to archive.
func addLivePausedInstance(t *testing.T, fix *forkTestFixture, uuid, title string) *session.Instance {
	t.Helper()
	require.NoError(t, fix.storage.AddInstance(&session.Instance{
		Title:     title,
		UUID:      uuid,
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}))
	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	var liveInst *session.Instance
	for _, li := range loaded {
		if li.Title == title {
			liveInst = li
		}
	}
	require.NotNil(t, liveInst)
	addInstanceToPoller(fix.poller, liveInst)
	return liveInst
}

// TestArchiveSessionByUUID_SetsStatusStopped covers the CAS variant used by
// callers (e.g. backlog lifecycle) that archive unconditionally by UUID.
func TestArchiveSessionByUUID_SetsStatusStopped(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	liveInst := addLivePausedInstance(t, fix, "test-uuid-1", "archive-by-uuid")

	err := fix.svc.ArchiveSessionByUUID(context.Background(), "test-uuid-1")
	require.NoError(t, err)

	snap := liveInst.Snapshot()
	assert.NotNil(t, snap.ArchivedAt, "expected ArchivedAt to be set")
	assert.Equal(t, session.Stopped, snap.Status, "expected Status to transition to Stopped when archiving by UUID")
}

// TestArchiveSessionByUUID_PublishesSessionArchivedEvent verifies ArchiveSessionByUUID
// publishes EventSessionArchived — the event ReactiveQueueManager relies on to evict a
// stale review-queue entry for the archived session (see review_queue_manager.go's
// EventSessionDeleted/EventSessionArchived case). Regression for archived/superseded
// sessions surfacing a permanent "no activity" notification because nothing told the
// queue the session was gone.
func TestArchiveSessionByUUID_PublishesSessionArchivedEvent(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	addLivePausedInstance(t, fix, "test-uuid-publishes-event", "archive-publishes-event")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eventCh, subID := fix.bus.Subscribe(ctx)
	defer fix.bus.Unsubscribe(subID)

	err := fix.svc.ArchiveSessionByUUID(context.Background(), "test-uuid-publishes-event")
	require.NoError(t, err)

	var got *events.Event
	require.NoError(t, testutil.WaitForCondition(func() bool {
		select {
		case e := <-eventCh:
			if e.Type == events.EventSessionArchived {
				got = e
				return true
			}
			return false
		default:
			return false
		}
	}, testutil.FastWaitConfig()), "expected EventSessionArchived to be published")
	assert.Equal(t, "test-uuid-publishes-event", got.SessionID)
}

// TestArchiveSessionByUUID_should_useStorageFallback_When_SessionNotInLivePoller is the
// regression test for the fix where ArchiveSessionByUUID silently no-op'd for any session
// not resident in ReviewQueuePoller.instances (e.g. after a server restart) — the
// done-transition hook and the periodic archive_terminal_sessions safety-net sweep both
// called it correctly, but ArchivedAt never got set because FindLiveInstance came back
// nil. This session is persisted in storage but deliberately never added to fix.poller.
func TestArchiveSessionByUUID_should_useStorageFallback_When_SessionNotInLivePoller(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	inst := &session.Instance{
		Title:     "not-in-poller",
		UUID:      "test-uuid-not-in-poller",
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, fix.storage.AddInstance(inst))
	require.Nil(t, fix.poller.FindInstance("test-uuid-not-in-poller"), "precondition: session must not be in the live poller")

	err := fix.svc.ArchiveSessionByUUID(context.Background(), "test-uuid-not-in-poller")
	require.NoError(t, err)

	data, err := fix.storage.FindInstanceDataByID("test-uuid-not-in-poller")
	require.NoError(t, err)
	assert.NotNil(t, data.ArchivedAt, "expected ArchivedAt to be set via the storage fallback")
	assert.Equal(t, session.Stopped, data.Status, "expected Status to transition to Stopped via the storage fallback")
}

// TestArchiveSessionByUUID_should_returnNilWithoutPanicking_When_ConcStorageIsNil covers
// the fake-InstanceStore-in-tests degrade path: concStorage is only ever nil when the
// SessionService is constructed with an InstanceStore that isn't a *session.Storage (see
// NewSessionServiceWithSearchEngine's type assertion). The storage fallback must not be
// attempted in that case — no nil-pointer dereference on concStorage.ArchiveInstanceDataByID.
func TestArchiveSessionByUUID_should_returnNilWithoutPanicking_When_ConcStorageIsNil(t *testing.T) {
	t.Parallel()

	queue := session.NewReviewQueue()
	statusMgr := session.NewInstanceStatusManager()
	poller := session.NewReviewQueuePoller(queue, statusMgr, nil)

	// Construct SessionService directly (not via NewSessionService) so this test only
	// exercises ArchiveSessionByUUID's concStorage nil-guard, not the rest of the
	// constructor's unrelated wiring (sub-services that assume a fully-formed service).
	svc := &SessionService{
		storage:           &fakeInstanceStore{},
		reviewQueuePoller: poller,
	}

	err := svc.ArchiveSessionByUUID(context.Background(), "not-tracked-anywhere")
	assert.NoError(t, err)
}
