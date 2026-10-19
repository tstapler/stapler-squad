package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

var wakeNow = time.Date(2026, 10, 19, 13, 10, 0, 0, time.UTC)

type wakeStubs struct{}

func (wakeStubs) Owner(context.Context, string) (wakes.OwnerFacts, error) {
	return wakes.OwnerFacts{Exists: true, Live: true, WorkspaceKey: "repo"}, nil
}
func (wakeStubs) Admit(context.Context) (bool, error) { return true, nil }
func (wakeStubs) Notify(wakes.Notification)           {}
func (wakeStubs) IsReadyForSteer(string) bool         { return false }
func (wakeStubs) WriteWake(context.Context, string, string) (wakes.WriteOutcome, error) {
	return wakes.WriteNotAttempted, nil
}

// stubOwners serves live owners unless a test overrides one.
type stubOwners map[string]wakes.OwnerFacts

func (o stubOwners) Owner(_ context.Context, id string) (wakes.OwnerFacts, error) {
	if f, ok := o[id]; ok {
		return f, nil
	}
	return wakes.OwnerFacts{Exists: true, Live: true, WorkspaceKey: "repo"}, nil
}

type wakeRig struct {
	owners  stubOwners
	storage *session.Storage
	svc     *WakeTriggerService
	d       *wakes.Dispatcher
	guidSvc *GuidanceRequestService
	enabled bool
}

func newWakeRig(t *testing.T, withDispatcher bool, limits wakes.LimitSettings) *wakeRig {
	t.Helper()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	r := &wakeRig{storage: storage, enabled: true, owners: stubOwners{}}
	r.guidSvc = NewGuidanceRequestService(storage, nil, nil)
	r.svc = NewWakeTriggerService(WakeTriggerDeps{
		Store: storage, Owners: r.owners, Guidance: r.guidSvc,
		Titles:  func(id string) string { return "title-" + id },
		Enabled: func() bool { return r.enabled },
		Now:     func() time.Time { return wakeNow },
	})
	if withDispatcher {
		r.d = wakes.NewDispatcher(wakes.Config{
			Store: storage, Clock: wakes.NewFakeClock(wakeNow), Steerer: wakeStubs{}, Admission: wakeStubs{},
			Owners: wakeStubs{}, Notifier: wakeStubs{}, Limits: limits,
		})
		storage.SetWakeChangeHook(r.d.ChangeHook())
		r.svc.SetBackend(r.d)
	}
	return r
}

func (r *wakeRig) atWake(t *testing.T, owner string, at time.Time, kind domain.WakeActionKind) *session.WakeTriggerData {
	t.Helper()
	action, err := domain.NewWakeAction(kind, "check back on "+owner)
	require.NoError(t, err)
	expires := at.Add(time.Hour)
	row, _, err := r.storage.CreateWakeTrigger(context.Background(), session.CreateWakeTriggerInput{
		OwnerSessionUUID: owner, Spec: domain.AtSpec{At: at}, Action: action, NextFireAt: &at, ExpiresAt: &expires,
	})
	require.NoError(t, err)
	return row
}

func (r *wakeRig) list(t *testing.T) *sessionv1.ListWakeTriggersResponse {
	t.Helper()
	res, err := r.svc.ListWakeTriggers(context.Background(), connect.NewRequest(&sessionv1.ListWakeTriggersRequest{}))
	require.NoError(t, err)
	return res.Msg
}

func TestWakeTriggerService_ListWakeTriggers_should_ListOwnerPlainLanguageConditionAndNextFire_When_TwoPending(t *testing.T) {
	r := newWakeRig(t, false, wakes.LimitSettings{})
	r.atWake(t, "owner-a", wakeNow.Add(25*time.Minute), domain.WakeActionSteerText)
	ciAction, err := domain.NewWakeAction(domain.WakeActionSteerText, "ci")
	require.NoError(t, err)
	_, _, err = r.storage.CreateWakeTrigger(context.Background(), session.CreateWakeTriggerInput{
		OwnerSessionUUID: "owner-b", Spec: domain.CIEventSpec{Repo: "o/r", Number: 901, Kind: "success"}, Action: ciAction,
	})
	require.NoError(t, err)

	got := r.list(t)

	require.Len(t, got.Triggers, 2)
	assert.True(t, got.Enabled)
	byOwner := map[string]*sessionv1.WakeTriggerInfo{}
	for _, tr := range got.Triggers {
		byOwner[tr.OwnerSessionUuid] = tr
	}
	assert.Equal(t, "title-owner-a", byOwner["owner-a"].OwnerTitle)
	assert.Equal(t, wakeNow.Add(25*time.Minute), byOwner["owner-a"].NextFireAt.AsTime())
	assert.Equal(t, "when PR #901 (o/r) CI passes", byOwner["owner-b"].Condition)
	assert.Nil(t, byOwner["owner-b"].NextFireAt, "an event trigger has no fire time")
	assert.Equal(t, "pending", byOwner["owner-b"].Status)
}

func TestWakeTriggerService_ListWakeTriggers_should_ShowMissedRowsWithReason_And_ReportPaused_When_FlagOff(t *testing.T) {
	r := newWakeRig(t, false, wakes.LimitSettings{})
	missed := r.atWake(t, "owner-a", wakeNow.Add(time.Minute), domain.WakeActionSteerText)
	parsed := mustParseUUID(t, missed.ID)
	ok, err := r.storage.MarkWakeMissed(context.Background(), parsed, domain.WakeReasonExpired, wakeNow)
	require.NoError(t, err)
	require.True(t, ok)
	delivered := r.atWake(t, "owner-b", wakeNow.Add(2*time.Minute), domain.WakeActionSteerText)
	_, err = r.storage.CancelWakeTrigger(context.Background(), mustParseUUID(t, delivered.ID), wakeNow)
	require.NoError(t, err)
	r.enabled = false

	got := r.list(t)

	require.Len(t, got.Triggers, 1, "cancelled rows leave the list")
	assert.Equal(t, "missed", got.Triggers[0].Status)
	assert.Equal(t, "expired", got.Triggers[0].EndReason)
	assert.False(t, got.Enabled)
}

func TestWakeTriggerService_CancelWakeTrigger_should_CancelThenReportAlreadyEnded(t *testing.T) {
	for _, withDispatcher := range []bool{true, false} {
		r := newWakeRig(t, withDispatcher, wakes.LimitSettings{})
		row := r.atWake(t, "owner-a", wakeNow.Add(time.Hour), domain.WakeActionSteerText)

		first := cancelRPC(t, r, row.ID)
		second := cancelRPC(t, r, row.ID)

		assert.Equal(t, sessionv1.WakeCancelOutcome_WAKE_CANCEL_OUTCOME_CANCELLED, first.Outcome, "dispatcher=%v", withDispatcher)
		assert.Equal(t, sessionv1.WakeCancelOutcome_WAKE_CANCEL_OUTCOME_ALREADY_ENDED, second.Outcome)
		assert.Empty(t, r.list(t).Triggers)
		assert.Equal(t, sessionv1.WakeCancelOutcome_WAKE_CANCEL_OUTCOME_NOT_FOUND, cancelRPC(t, r, "not-a-uuid").Outcome)
	}
}

func TestWakeTriggerService_CancelWakeTrigger_should_ReportAlreadyFiredWithTime_When_DeliveredFirst(t *testing.T) {
	r := newWakeRig(t, true, wakes.LimitSettings{})
	row := r.atWake(t, "owner-a", wakeNow.Add(time.Minute), domain.WakeActionSteerText)
	id := mustParseUUID(t, row.ID)
	_, err := r.storage.ClaimWakeTrigger(context.Background(), id, wakeNow.Add(time.Minute))
	require.NoError(t, err)
	fired := wakeNow.Add(2 * time.Minute)
	_, err = r.storage.MarkWakeDelivered(context.Background(), id, fired, fired)
	require.NoError(t, err)

	res := cancelRPC(t, r, row.ID)

	assert.Equal(t, sessionv1.WakeCancelOutcome_WAKE_CANCEL_OUTCOME_ALREADY_FIRED, res.Outcome)
	assert.True(t, fired.Equal(res.DeliveredAt.AsTime()))
}

func cancelRPC(t *testing.T, r *wakeRig, id string) *sessionv1.CancelWakeTriggerResponse {
	t.Helper()
	res, err := r.svc.CancelWakeTrigger(context.Background(), connect.NewRequest(&sessionv1.CancelWakeTriggerRequest{Id: id}))
	require.NoError(t, err)
	return res.Msg
}

func remind(t *testing.T, r *wakeRig, at time.Time) *sessionv1.CreateOperatorReminderResponse {
	t.Helper()
	res, err := r.svc.CreateOperatorReminder(context.Background(), connect.NewRequest(&sessionv1.CreateOperatorReminderRequest{
		SessionUuid: "review-1", FireAt: timestamppb.New(at), Label: "Review finished",
	}))
	require.NoError(t, err)
	return res.Msg
}

func TestWakeTriggerService_CreateOperatorReminder_should_CreateNotifyOperatorTrigger_And_DedupADuplicateTap(t *testing.T) {
	r := newWakeRig(t, true, wakes.LimitSettings{})
	at := wakeNow.Add(time.Hour)

	first := remind(t, r, at)
	again := remind(t, r, at)

	assert.Equal(t, sessionv1.ReminderOutcome_REMINDER_OUTCOME_CREATED, first.Outcome)
	assert.True(t, first.Trigger.OperatorReminder)
	assert.Equal(t, "notify_operator", first.Trigger.ActionKind)
	assert.Equal(t, sessionv1.ReminderOutcome_REMINDER_OUTCOME_DUPLICATE, again.Outcome)
	assert.Equal(t, first.Trigger.Id, again.Trigger.Id)
	assert.Len(t, r.list(t).Triggers, 1, "a duplicate tap leaves a single trigger")
}

func TestWakeTriggerService_CreateOperatorReminder_should_AcceptTomorrowBeyondTheDefaultExpiryHorizon(t *testing.T) {
	r := newWakeRig(t, true, wakes.LimitSettings{})
	assert.Equal(t, sessionv1.ReminderOutcome_REMINDER_OUTCOME_CREATED, remind(t, r, wakeNow.Add(40*time.Hour)).Outcome)
}

func TestWakeTriggerService_CreateOperatorReminder_should_RejectPastTimeCapAndUnavailable(t *testing.T) {
	r := newWakeRig(t, true, wakes.LimitSettings{PendingPerOwner: 1})
	assert.Equal(t, sessionv1.ReminderOutcome_REMINDER_OUTCOME_PAST_TIME, remind(t, r, wakeNow.Add(-time.Minute)).Outcome)
	assert.Equal(t, sessionv1.ReminderOutcome_REMINDER_OUTCOME_CREATED, remind(t, r, wakeNow.Add(time.Hour)).Outcome)
	assert.Equal(t, sessionv1.ReminderOutcome_REMINDER_OUTCOME_CAP_EXCEEDED, remind(t, r, wakeNow.Add(2*time.Hour)).Outcome)

	off := newWakeRig(t, false, wakes.LimitSettings{})
	assert.Equal(t, sessionv1.ReminderOutcome_REMINDER_OUTCOME_UNAVAILABLE, remind(t, off, wakeNow.Add(time.Hour)).Outcome)
}

func TestWakeTriggerService_CancelWake_should_MapToActionOutcomes(t *testing.T) {
	r := newWakeRig(t, true, wakes.LimitSettings{})
	row := r.atWake(t, "owner-a", wakeNow.Add(time.Hour), domain.WakeActionSteerText)

	first, err := r.svc.CancelWake(context.Background(), row.ID)
	require.NoError(t, err)
	second, err := r.svc.CancelWake(context.Background(), row.ID)
	require.NoError(t, err)

	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED, first.Outcome)
	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, second.Outcome)
}

func peerRequest(t *testing.T, r *wakeRig, asker, answerer string) *session.GuidanceRequestData {
	t.Helper()
	req, err := domain.NewGuidanceRequest(domain.GuidanceRequestParams{
		Scope: domain.RequestScopeSession, SessionUUID: asker, QuestionText: "Which approach?", QuestionType: domain.QuestionTypeShortAnswer,
		Source: domain.QuestionSourcePeer, AskerSessionUUID: asker,
		AnswererKind: domain.AnswererKindSession, AnswererSessionUUID: answerer,
	})
	require.NoError(t, err)
	row, err := r.storage.CreateMirroredGuidanceRequest(context.Background(), req, 10)
	require.NoError(t, err)
	return row
}

func peerList(t *testing.T, r *wakeRig) []*sessionv1.PeerHelpInfo {
	t.Helper()
	res, err := r.svc.ListPeerHelpRequests(context.Background(), connect.NewRequest(&sessionv1.ListPeerHelpRequestsRequest{}))
	require.NoError(t, err)
	return res.Msg.Requests
}

func TestWakeTriggerService_CancelPeerHelp_should_CancelRowAndWakeTheAsker(t *testing.T) {
	r := newWakeRig(t, false, wakes.LimitSettings{})
	var mu sync.Mutex
	var woken []string
	r.guidSvc.SetAnswerListener(func(_ context.Context, id string) { mu.Lock(); woken = append(woken, id); mu.Unlock() })
	row := peerRequest(t, r, "asker", "peer")

	res, err := r.svc.CancelPeerHelp(context.Background(), row.ID)
	require.NoError(t, err)
	again, err := r.svc.CancelPeerHelp(context.Background(), row.ID)
	require.NoError(t, err)

	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED, res.Outcome)
	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, again.Outcome)
	assert.Equal(t, "Already cancelled", again.Detail)
	mu.Lock()
	assert.Equal(t, []string{row.ID}, woken, "the asker's guidance_answered wake fires once")
	mu.Unlock()
	infos := peerList(t, r)
	require.Len(t, infos, 1)
	assert.Equal(t, "cancelled", infos[0].State)
	assert.Equal(t, "cancelled by the operator", infos[0].StateReason)
}

func TestWakeTriggerService_CancelPeerHelp_should_ReportAlreadyAnsweredBySession_When_PeerAnsweredFirst(t *testing.T) {
	r := newWakeRig(t, false, wakes.LimitSettings{})
	row := peerRequest(t, r, "asker", "peer")
	_, applied, err := r.guidSvc.AnswerAs(context.Background(), row.ID, "use option B", SessionAnswerer("peer"))
	require.NoError(t, err)
	require.True(t, applied)

	res, err := r.svc.CancelPeerHelp(context.Background(), row.ID)

	require.NoError(t, err)
	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, res.Outcome)
	assert.Equal(t, "Already answered by title-peer: use option B", res.Detail)
}

func TestWakeTriggerService_CancelPeerHelp_should_RefuseANonPeerRequest(t *testing.T) {
	r := newWakeRig(t, false, wakes.LimitSettings{})
	row, err := r.storage.CreateGuidanceRequest(context.Background(), session.CreateGuidanceRequestInput{
		Scope: domain.RequestScopeStandalone, QuestionText: "ok?", QuestionType: domain.QuestionTypeShortAnswer,
	})
	require.NoError(t, err)

	res, err := r.svc.CancelPeerHelp(context.Background(), row.ID)

	require.NoError(t, err)
	assert.Equal(t, sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, res.Outcome)
}

func TestWakeTriggerService_ListPeerHelpRequests_should_DeriveWaitingAnsweredAndOnBehalfOf(t *testing.T) {
	r := newWakeRig(t, false, wakes.LimitSettings{})
	waiting := peerRequest(t, r, "asker", "peer")
	answered := peerRequest(t, r, "asker2", "peer2")
	_, applied, err := r.guidSvc.AnswerAs(context.Background(), answered.ID, "go with B", HumanAnswerer())
	require.NoError(t, err)
	require.True(t, applied)

	byID := map[string]*sessionv1.PeerHelpInfo{}
	for _, p := range peerList(t, r) {
		byID[p.Id] = p
	}

	assert.Equal(t, "waiting", byID[waiting.ID].State)
	assert.Equal(t, "title-asker", byID[waiting.ID].AskerTitle)
	assert.False(t, byID[waiting.ID].Escalated)
	assert.Equal(t, "answered", byID[answered.ID].State)
	assert.Equal(t, "operator", byID[answered.ID].AnsweredBy)
	assert.Equal(t, "title-peer2", byID[answered.ID].OnBehalfOf, "the operator answered in the peer's place")
}

func TestGuidanceDescriptors_should_OfferCancelRequestWithConfirmAndPeerLabel_When_PeerHelpIsOpen(t *testing.T) {
	b := &DescriptorBuilder{
		sources: DescriptorSources{Guidance: &fakeGuidance{open: map[string]OpenGuidance{"g1": {PeerHelp: true}}}},
		flag:    func(string) bool { return true },
	}
	rec := &sessionv1.NotificationHistoryRecord{Metadata: map[string]string{metaGuidanceRequestID: "g1", MetaPeerTitle: "diagnose:abc"}}

	b.Annotate(context.Background(), []*sessionv1.NotificationHistoryRecord{rec}, ListenerClassLocal)

	kinds := make([]sessionv1.ActionKind, 0, len(rec.Actions))
	for _, a := range rec.Actions {
		kinds = append(kinds, a.Kind)
	}
	assert.Equal(t, []sessionv1.ActionKind{
		sessionv1.ActionKind_ACTION_KIND_ANSWER_GUIDANCE, sessionv1.ActionKind_ACTION_KIND_CANCEL_PEER_HELP, sessionv1.ActionKind_ACTION_KIND_OPEN,
	}, kinds)
	assert.Equal(t, "Answer for diagnose:abc", rec.Actions[0].Label)
	assert.True(t, rec.Actions[1].RequiresConfirm)
	assert.Equal(t, sessionv1.ActionTier_ACTION_TIER_A, rec.Actions[1].Tier)
}

func mustParseUUID(t *testing.T, id string) uuid.UUID {
	t.Helper()
	parsed, err := uuid.Parse(id)
	require.NoError(t, err)
	return parsed
}

func TestWakeTriggerService_ListPeerHelpRequests_should_ShowNotSentPlainReason_And_StalledForAStoppedPeer(t *testing.T) {
	r := newWakeRig(t, false, wakes.LimitSettings{})
	deleted := peerRequest(t, r, "asker", "gone")
	archived := peerRequest(t, r, "asker", "archived")
	stopped := peerRequest(t, r, "asker", "stopped")
	r.owners["gone"] = wakes.OwnerFacts{}
	r.owners["archived"] = wakes.OwnerFacts{Exists: true, Archived: true}
	r.owners["stopped"] = wakes.OwnerFacts{Exists: true, WorkspaceKey: "repo"}

	byID := map[string]*sessionv1.PeerHelpInfo{}
	for _, p := range peerList(t, r) {
		byID[p.Id] = p
	}

	assert.Equal(t, "not_sent", byID[deleted.ID].State)
	assert.Equal(t, "peer no longer exists", byID[deleted.ID].StateReason)
	assert.Equal(t, "not_sent", byID[archived.ID].State)
	assert.Equal(t, "peer is archived", byID[archived.ID].StateReason)
	assert.Equal(t, "waiting", byID[stopped.ID].State, "a stopped peer is stalled, not undeliverable")
	assert.Equal(t, "Peer is stopped. Not delivered unless resumed.", byID[stopped.ID].StateReason)
}
