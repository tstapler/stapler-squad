package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

type fakeSessionEnds struct {
	titles map[string]string // title -> uuid
	ended  map[string]bool   // uuid -> ended
}

func (f fakeSessionEnds) ResolveUUID(id string) string {
	if u, ok := f.titles[id]; ok {
		return u
	}
	return id
}
func (f fakeSessionEnds) Ended(u string) bool { return f.ended[u] }

const reaperPeerSession = "7c1e0000-0000-4000-8000-0000000000bb"

type reaperFixture struct {
	storage *session.Storage
	mirror  *GuidanceHookMirror
	reaper  *GuidanceSessionReaper
	counts  *mirrorCounts
	hookID  string
	peerID  string
	agentID string
}

func newReaperFixture(t *testing.T, ends fakeSessionEnds) *reaperFixture {
	t.Helper()
	storage := createTestStorage(t)
	counts := &mirrorCounts{}
	mirror := newRealStoreMirror(storage, NewPendingQuestionStore(nil), counts)
	f := &reaperFixture{storage: storage, mirror: mirror, counts: counts}
	var ok bool
	f.hookID, ok = mirror.Mirror(mirrorReq(t, "toolu_1", "Which color?", true))
	require.True(t, ok)

	ctx := context.Background()
	peer, err := domain.NewGuidanceRequest(domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: mirrorSession, Source: domain.QuestionSourcePeer,
		AskerSessionUUID: mirrorSession, AnswererKind: domain.AnswererKindSession, AnswererSessionUUID: reaperPeerSession,
		QuestionText: "Can you check the schema?", QuestionType: domain.QuestionTypeShortAnswer,
	})
	require.NoError(t, err)
	peerRow, err := storage.CreateMirroredGuidanceRequest(ctx, peer, 0)
	require.NoError(t, err)
	f.peerID = peerRow.ID

	agent, err := storage.CreateGuidanceRequest(ctx, session.CreateGuidanceRequestInput{
		Scope: domain.RequestScopeSession, SessionUUID: mirrorSession, QuestionText: "Deploy now?", QuestionType: domain.QuestionTypeYesNo,
	})
	require.NoError(t, err)
	f.agentID = agent.ID

	f.reaper = NewGuidanceSessionReaper(storage, ends, mirror, counts.onCleared)
	return f
}

func (f *reaperFixture) cancelled(t *testing.T, id string) bool {
	t.Helper()
	row, err := f.storage.GetGuidanceRequest(context.Background(), uuid.MustParse(id))
	require.NoError(t, err)
	return row.CancelledAt != nil
}

func TestReaper_should_CancelHookAndPeerRowsOnly_WhenSessionIsArchived(t *testing.T) {
	t.Parallel()
	f := newReaperFixture(t, fakeSessionEnds{})
	f.reaper.HandleEvent(context.Background(), events.NewSessionArchivedEvent(mirrorSession))

	assert.True(t, f.cancelled(t, f.hookID))
	assert.True(t, f.cancelled(t, f.peerID))
	assert.False(t, f.cancelled(t, f.agentID), "agent_mcp guidance follows Story 3.4, never the reaper")
	row, err := f.storage.GetGuidanceRequest(context.Background(), uuid.MustParse(f.hookID))
	require.NoError(t, err)
	assert.Equal(t, domain.CancelReasonSessionEnded, row.CancelReason)
	assert.Len(t, f.counts.cleared, 2)
}

func TestReaper_should_ResolveATitleToTheUUID_BeforeMatching(t *testing.T) {
	t.Parallel()
	f := newReaperFixture(t, fakeSessionEnds{titles: map[string]string{"my-session": mirrorSession}})
	f.reaper.HandleEvent(context.Background(), events.NewSessionDeletedEvent("my-session"))
	assert.True(t, f.cancelled(t, f.hookID))
	assert.False(t, f.cancelled(t, f.agentID))
}

func TestReaper_should_IgnoreOtherSessionsAndOtherEvents(t *testing.T) {
	t.Parallel()
	f := newReaperFixture(t, fakeSessionEnds{})
	f.reaper.HandleEvent(context.Background(), events.NewSessionDeletedEvent("7c1e0000-0000-4000-8000-0000000000ff"))
	f.reaper.HandleEvent(context.Background(), events.NewUserInteractionEvent(mirrorSession, "x", "y"))
	f.reaper.HandleEvent(context.Background(), nil)
	assert.False(t, f.cancelled(t, f.hookID))
	assert.False(t, f.cancelled(t, f.peerID))
}

func TestReaper_should_CancelPeerRowAddressedToTheEndedSession(t *testing.T) {
	t.Parallel()
	f := newReaperFixture(t, fakeSessionEnds{})
	f.reaper.HandleEvent(context.Background(), events.NewSessionDeletedEvent(reaperPeerSession))
	assert.True(t, f.cancelled(t, f.peerID))
	assert.False(t, f.cancelled(t, f.hookID), "the asker is still alive")
}

func TestReaper_StartupPass_should_CancelRowsOfSessionsThatEndedDuringDowntime(t *testing.T) {
	t.Parallel()
	f := newReaperFixture(t, fakeSessionEnds{ended: map[string]bool{mirrorSession: true}})
	f.reaper.StartupPass(context.Background())
	assert.True(t, f.cancelled(t, f.hookID))
	assert.True(t, f.cancelled(t, f.peerID))
	assert.False(t, f.cancelled(t, f.agentID))

	live := newReaperFixture(t, fakeSessionEnds{})
	live.reaper.StartupPass(context.Background())
	assert.False(t, live.cancelled(t, live.hookID), "a live session keeps its rows")
}

func TestReaper_Run_should_RehydrateAfterTheStartupPass_AndStopWhenTheBusCloses(t *testing.T) {
	t.Parallel()
	f := newReaperFixture(t, fakeSessionEnds{})
	bus := events.NewEventBus(8)
	done := make(chan struct{})
	go func() { f.reaper.Run(context.Background(), bus); close(done) }()

	require.Eventually(t, func() bool {
		_, ok := f.mirror.pending.Peek(QuestionKey{SessionUUID: mirrorSession, QuestionID: f.hookID})
		return ok
	},
		5*time.Second, 5*time.Millisecond, "the open replyable row is rehydrated on start")
	bus.Publish(events.NewSessionArchivedEvent(mirrorSession))
	require.Eventually(t, func() bool { return f.cancelled(t, f.hookID) }, 5*time.Second, 5*time.Millisecond)
	_, stillCached := f.mirror.pending.Peek(QuestionKey{SessionUUID: mirrorSession, QuestionID: f.hookID})
	assert.False(t, stillCached)

	bus.Close()
	<-done
}
