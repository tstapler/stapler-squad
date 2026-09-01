package services

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
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

// TestArchiveSessionByUUID_SetsStatusStopped covers the CAS variant used by
// callers (e.g. backlog lifecycle) that archive unconditionally by UUID.
func TestArchiveSessionByUUID_SetsStatusStopped(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()

	inst := &session.Instance{
		Title:     "archive-by-uuid",
		UUID:      "test-uuid-1",
		Path:      "/tmp/test",
		Status:    session.Paused,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, fix.storage.AddInstance(inst))
	loaded, err := fix.storage.LoadInstances()
	require.NoError(t, err)
	var liveInst *session.Instance
	for _, li := range loaded {
		if li.Title == "archive-by-uuid" {
			liveInst = li
		}
	}
	require.NotNil(t, liveInst)
	addInstanceToPoller(fix.poller, liveInst)

	err = fix.svc.ArchiveSessionByUUID(context.Background(), "test-uuid-1")
	require.NoError(t, err)

	snap := liveInst.Snapshot()
	assert.NotNil(t, snap.ArchivedAt, "expected ArchivedAt to be set")
	assert.Equal(t, session.Stopped, snap.Status, "expected Status to transition to Stopped when archiving by UUID")
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
