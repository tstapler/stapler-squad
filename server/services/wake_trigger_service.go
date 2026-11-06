package services

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Notification metadata keys the peer-help escalation sets.
const (
	// MetaPeerTitle names the unresponsive answerer; the descriptor builder
	// labels the answer action "Answer for {peer}" with it.
	MetaPeerTitle = "peer_title"
	// MetaGuidanceRequestID keys the descriptors of a notification to its request.
	MetaGuidanceRequestID = metaGuidanceRequestID
)

const (
	// missedListWindow bounds how long a missed trigger stays in the operator list.
	missedListWindow = 7 * 24 * time.Hour
	// peerListWindow bounds how long an ended peer-help request stays listed.
	peerListWindow = 24 * time.Hour
	// reminderExpiryGrace keeps a reminder alive past its fire time so a short
	// outage does not mark it expired.
	reminderExpiryGrace = time.Hour
	reminderLabelRunes  = 200
	operatorLabel       = "operator"
)

// WakeTriggerStore is the storage slice the operator surface reads and writes.
type WakeTriggerStore interface {
	ListWakeTriggersByStatus(ctx context.Context, statuses ...domain.WakeStatus) ([]*session.WakeTriggerData, error)
	ListWakeTriggersByOwner(ctx context.Context, owner string, statuses ...domain.WakeStatus) ([]*session.WakeTriggerData, error)
	GetWakeTrigger(ctx context.Context, id uuid.UUID) (*session.WakeTriggerData, error)
	CancelWakeTrigger(ctx context.Context, id uuid.UUID, now time.Time) (bool, error)
	ListPeerGuidanceRequests(ctx context.Context, since time.Time) ([]*session.GuidanceRequestData, error)
}

// WakeOperatorBackend is the running dispatcher (*wakes.Dispatcher). It is set
// when the dispatcher starts; until then, or in a process that is not the
// leader, cancels fall back to the store and reminders are unavailable.
type WakeOperatorBackend interface {
	Schedule(ctx context.Context, req wakes.ScheduleRequest) (*session.WakeTriggerData, bool, error)
	CancelAny(ctx context.Context, id string) (*session.WakeTriggerData, bool, error)
}

// WakeTriggerDeps are the collaborators of WakeTriggerService.
type WakeTriggerDeps struct {
	Store    WakeTriggerStore
	Owners   wakes.OwnerLookup
	Guidance *GuidanceRequestService
	// Titles names a session for display; nil or "" falls back to the uuid.
	Titles func(sessionUUID string) string
	// Enabled reports whether agent_wake_triggers lets the dispatcher run.
	Enabled func() bool
	// ExpireOverdue ends rows past expires_at as missed; it runs before a list so a
	// stopped dispatcher does not leave expired rows pending. nil skips it.
	ExpireOverdue func(ctx context.Context) error
	Now           func() time.Time
}

// WakeTriggerService backs the WakeTriggerService RPCs and the CANCEL_WAKE /
// CANCEL_PEER_HELP action kinds (it satisfies CancelDelegate).
type WakeTriggerService struct {
	deps    WakeTriggerDeps
	backend atomic.Pointer[WakeOperatorBackend]
}

// NewWakeTriggerService builds the service.
func NewWakeTriggerService(deps WakeTriggerDeps) *WakeTriggerService {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.Enabled == nil {
		deps.Enabled = func() bool { return false }
	}
	return &WakeTriggerService{deps: deps}
}

// SetBackend installs the running dispatcher; nil removes it (shutdown).
func (s *WakeTriggerService) SetBackend(b WakeOperatorBackend) {
	if b == nil {
		s.backend.Store(nil)
		return
	}
	s.backend.Store(&b)
}

func (s *WakeTriggerService) activeBackend() WakeOperatorBackend {
	if p := s.backend.Load(); p != nil {
		return *p
	}
	return nil
}

func (s *WakeTriggerService) title(sessionUUID string) string {
	if s.deps.Titles != nil {
		if t := s.deps.Titles(sessionUUID); t != "" {
			return t
		}
	}
	return sessionUUID
}

// ListWakeTriggers lists live and recently missed triggers of every owner.
//
// +api: wake-trigger:list
func (s *WakeTriggerService) ListWakeTriggers(
	ctx context.Context, _ *connect.Request[sessionv1.ListWakeTriggersRequest],
) (*connect.Response[sessionv1.ListWakeTriggersResponse], error) {
	if s.deps.ExpireOverdue != nil {
		if err := s.deps.ExpireOverdue(ctx); err != nil {
			log.Warn("wake trigger list: expiry sweep failed", "err", err)
		}
	}
	rows, err := s.deps.Store.ListWakeTriggersByStatus(ctx,
		domain.WakeStatusPending, domain.WakeStatusClaimed, domain.WakeStatusNeedsRevive, domain.WakeStatusMissed)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list wake triggers: %w", err))
	}
	cutoff := s.deps.Now().Add(-missedListWindow)
	facts := s.ownerFactsCache(ctx)
	out := &sessionv1.ListWakeTriggersResponse{Enabled: s.deps.Enabled()}
	for _, r := range rows {
		if r.Status == domain.WakeStatusMissed && r.EndedAt != nil && r.EndedAt.Before(cutoff) {
			continue
		}
		out.Triggers = append(out.Triggers, s.infoOf(r, facts(r.OwnerSessionUUID)))
	}
	return connect.NewResponse(out), nil
}

// ownerFactsCache memoizes owner lookups for one request.
func (s *WakeTriggerService) ownerFactsCache(ctx context.Context) func(string) wakes.OwnerFacts {
	seen := map[string]wakes.OwnerFacts{}
	return func(id string) wakes.OwnerFacts {
		if f, ok := seen[id]; ok {
			return f
		}
		var f wakes.OwnerFacts
		if s.deps.Owners != nil {
			f, _ = s.deps.Owners.Owner(ctx, id) // a failed lookup reads as "unknown owner"
		}
		seen[id] = f
		return f
	}
}

func (s *WakeTriggerService) infoOf(r *session.WakeTriggerData, owner wakes.OwnerFacts) *sessionv1.WakeTriggerInfo {
	condition, zone := wakes.Describe(r.Spec, s.title)
	info := &sessionv1.WakeTriggerInfo{
		Id: r.ID, OwnerSessionUuid: r.OwnerSessionUUID, OwnerExists: owner.Exists, OwnerArchived: owner.Archived,
		TriggerType: string(r.Type), Condition: condition, ActionKind: string(r.Action.Kind),
		Status: string(r.Status), EndReason: string(r.EndReason), Message: r.Action.Payload,
		CreatedAt:        timestamppb.New(r.CreatedAt),
		OperatorReminder: r.Action.Kind == domain.WakeActionNotifyOperator,
		CronTimezone:     zone,
	}
	if owner.Exists {
		info.OwnerTitle = s.title(r.OwnerSessionUUID)
	}
	info.NextFireAt = tsOrNil(r.NextFireAt)
	info.DeliveredAt = tsOrNil(r.DeliveredAt)
	info.EndedAt = tsOrNil(r.EndedAt)
	return info
}

func tsOrNil(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

// CancelWakeTrigger cancels one trigger on the operator's behalf.
//
// +api: wake-trigger:cancel
func (s *WakeTriggerService) CancelWakeTrigger(
	ctx context.Context, req *connect.Request[sessionv1.CancelWakeTriggerRequest],
) (*connect.Response[sessionv1.CancelWakeTriggerResponse], error) {
	row, cancelled, err := s.cancelAny(ctx, req.Msg.GetId())
	switch {
	case errors.Is(err, wakes.ErrWakeNotFound):
		return connect.NewResponse(&sessionv1.CancelWakeTriggerResponse{Outcome: sessionv1.WakeCancelOutcome_WAKE_CANCEL_OUTCOME_NOT_FOUND}), nil
	case err != nil:
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	res := &sessionv1.CancelWakeTriggerResponse{Outcome: sessionv1.WakeCancelOutcome_WAKE_CANCEL_OUTCOME_CANCELLED}
	if !cancelled {
		res.Outcome = sessionv1.WakeCancelOutcome_WAKE_CANCEL_OUTCOME_ALREADY_ENDED
		if row.Status == domain.WakeStatusDelivered {
			res.Outcome = sessionv1.WakeCancelOutcome_WAKE_CANCEL_OUTCOME_ALREADY_FIRED
			res.DeliveredAt = tsOrNil(row.DeliveredAt)
		}
	}
	return connect.NewResponse(res), nil
}

// cancelAny goes through the dispatcher when it runs, so its timer and index
// forget the row, and straight to the store otherwise (the kill switch is off
// but Cancel must still work).
func (s *WakeTriggerService) cancelAny(ctx context.Context, id string) (*session.WakeTriggerData, bool, error) {
	if b := s.activeBackend(); b != nil {
		return b.CancelAny(ctx, id)
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, false, wakes.ErrWakeNotFound
	}
	if _, err := s.deps.Store.GetWakeTrigger(ctx, parsed); errors.Is(err, session.ErrNotFound) {
		return nil, false, wakes.ErrWakeNotFound
	} else if err != nil {
		return nil, false, fmt.Errorf("cancel wake: %w", err)
	}
	applied, err := s.deps.Store.CancelWakeTrigger(ctx, parsed, s.deps.Now())
	if err != nil {
		return nil, false, fmt.Errorf("cancel wake: %w", err)
	}
	row, err := s.deps.Store.GetWakeTrigger(ctx, parsed)
	if err != nil {
		return nil, applied, fmt.Errorf("cancel wake: reread: %w", err)
	}
	return row, applied, nil
}

// CreateOperatorReminder schedules a notify_operator trigger about a session.
//
// +api: wake-trigger:remind
func (s *WakeTriggerService) CreateOperatorReminder(
	ctx context.Context, req *connect.Request[sessionv1.CreateOperatorReminderRequest],
) (*connect.Response[sessionv1.CreateOperatorReminderResponse], error) {
	msg := req.Msg
	reply := func(o sessionv1.ReminderOutcome, row *session.WakeTriggerData) *connect.Response[sessionv1.CreateOperatorReminderResponse] {
		res := &sessionv1.CreateOperatorReminderResponse{Outcome: o}
		if row != nil {
			res.Trigger = s.infoOf(row, wakes.OwnerFacts{Exists: true})
		}
		return connect.NewResponse(res)
	}
	if msg.GetSessionUuid() == "" || msg.GetFireAt() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("session_uuid and fire_at are required"))
	}
	fireAt := msg.GetFireAt().AsTime()
	if !fireAt.After(s.deps.Now()) {
		return reply(sessionv1.ReminderOutcome_REMINDER_OUTCOME_PAST_TIME, nil), nil
	}
	b := s.activeBackend()
	if b == nil {
		return reply(sessionv1.ReminderOutcome_REMINDER_OUTCOME_UNAVAILABLE, nil), nil
	}
	action, err := domain.NewWakeAction(domain.WakeActionNotifyOperator, reminderText(msg.GetLabel()))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	expires := fireAt.Add(reminderExpiryGrace)
	row, created, err := b.Schedule(ctx, wakes.ScheduleRequest{
		Owner: msg.GetSessionUuid(), Spec: domain.AtSpec{At: fireAt}, Action: action, ExpiresAt: &expires,
	})
	var capErr *wakes.CapExceededError
	switch {
	case errors.As(err, &capErr):
		return reply(sessionv1.ReminderOutcome_REMINDER_OUTCOME_CAP_EXCEEDED, nil), nil
	case err != nil:
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create reminder: %w", err))
	case !created:
		return reply(sessionv1.ReminderOutcome_REMINDER_OUTCOME_DUPLICATE, row), nil
	}
	return reply(sessionv1.ReminderOutcome_REMINDER_OUTCOME_CREATED, row), nil
}

func reminderText(label string) string {
	if r := []rune(label); len(r) > reminderLabelRunes {
		label = string(r[:reminderLabelRunes])
	}
	if label == "" {
		return "Reminder"
	}
	return label
}

// CancelWake implements CancelDelegate for ACTION_KIND_CANCEL_WAKE.
func (s *WakeTriggerService) CancelWake(ctx context.Context, wakeID string) (ActionResult, error) {
	row, cancelled, err := s.cancelAny(ctx, wakeID)
	switch {
	case errors.Is(err, wakes.ErrWakeNotFound):
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, "not found"), nil
	case err != nil:
		return ActionResult{}, err
	case cancelled:
		return applied(), nil
	}
	detail := "already " + string(row.Status)
	if row.Status == domain.WakeStatusDelivered && row.DeliveredAt != nil {
		detail = "already fired at " + row.DeliveredAt.UTC().Format(time.RFC3339)
	}
	return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, detail), nil
}

// CancelPeerHelp implements CancelDelegate for ACTION_KIND_CANCEL_PEER_HELP.
func (s *WakeTriggerService) CancelPeerHelp(ctx context.Context, requestID string) (ActionResult, error) {
	if s.deps.Guidance == nil {
		return notWired(), nil
	}
	data, wasApplied, err := s.deps.Guidance.CancelPeerHelp(ctx, requestID)
	switch {
	case errors.Is(err, session.ErrNotFound):
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, "not found"), nil
	case errors.Is(err, ErrNotPeerHelp):
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, "not a peer help request"), nil
	case err != nil:
		return ActionResult{}, err
	case wasApplied:
		return applied(), nil
	}
	return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED, s.alreadyEnded(data)), nil
}

func (s *WakeTriggerService) alreadyEnded(d *session.GuidanceRequestData) string {
	if d.Status() == session.GuidanceRequestStatusAnswered {
		return fmt.Sprintf("Already answered by %s: %s", s.answeredBy(d), d.Answer)
	}
	return "Already cancelled"
}

// answeredBy names who answered: the operator, or the peer session.
func (s *WakeTriggerService) answeredBy(d *session.GuidanceRequestData) string {
	if d.AnswererKind == domain.AnswererKindSession {
		return s.title(d.AnswererSessionUUID)
	}
	return operatorLabel
}
