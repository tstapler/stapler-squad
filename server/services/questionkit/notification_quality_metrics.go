package questionkit

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/telemetry"
)

// notification_quality_metrics.go holds the paired quality metrics of Epic 5
// (requirements.md): reopen_after_action_total, the Unconfirmed counter, and
// the ScheduleWakeup receipt counter behind the Epic 4 cost outcome.

const (
	metricReopenAfterAction    = "reopen_after_action_total"
	MetricGuidanceUnconfirmed  = "guidance_answer_unconfirmed_total"
	MetricScheduleWakeupCalls  = "agent_schedule_wakeup_calls_total"
	ScheduleWakeupTool         = "ScheduleWakeup"
	scheduleWakeupReceiptEvent = "schedule_wakeup_receipt"
	RoleUnlinked               = "unlinked"
	RoleUnknown                = "unknown"

	// reopenWindow is how soon after an applied action an operator open of the
	// same session counts as a possible wrong answer.
	reopenWindow = 60 * time.Second
)

var (
	reopenCounter = telemetry.CounterNamed(metricReopenAfterAction,
		"Operator opened a session's terminal within 60s of an applied inline answer to it")
	answerUnconfirmedCounter = telemetry.CounterNamed(MetricGuidanceUnconfirmed,
		"Keystroke answers that entered the Unconfirmed sub-state (indeterminate write or no acknowledgement at 60s)")
	scheduleWakeupCounter = telemetry.CounterNamed(MetricScheduleWakeupCalls,
		"ScheduleWakeup PostToolUse receipts by backlog role")
)

type pendingReopen struct {
	kind string
	at   time.Time
}

// ReopenTracker counts an applied ANSWER_QUESTION followed by the operator
// opening that session's terminal within reopenWindow. Only answers carry a
// session id, so APPROVE, DENY and BACKLOG_DECISION are not tracked. A nil
// tracker ignores every call.
type ReopenTracker struct {
	mu      sync.Mutex
	now     func() time.Time
	pending map[string]pendingReopen
}

func NewReopenTracker(now func() time.Time) *ReopenTracker {
	if now == nil {
		now = time.Now
	}
	return &ReopenTracker{now: now, pending: map[string]pendingReopen{}}
}

var DefaultReopenTracker = NewReopenTracker(nil)

func (t *ReopenTracker) NoteApplied(m *sessionv1.InvokeNotificationActionRequest, res ActionResult) {
	if t == nil || res.Outcome != sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED ||
		m.GetActionKind() != sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION || m.GetSessionId() == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	t.pending[m.GetSessionId()] = pendingReopen{kind: "answer_question", at: now}
	for id, p := range t.pending {
		if now.Sub(p.at) > reopenWindow {
			delete(t.pending, id)
		}
	}
}

// attached is called with every identifier of a session whose terminal the
// operator just opened (stable id and title); it counts at most once per action.
func (t *ReopenTracker) Attached(ids ...string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	var hits []string
	for _, id := range ids {
		if p, ok := t.pending[id]; ok {
			delete(t.pending, id)
			if t.now().Sub(p.at) <= reopenWindow {
				hits = append(hits, p.kind)
			}
		}
	}
	t.mu.Unlock()
	for _, kind := range hits {
		telemetry.AddOne(reopenCounter(), attribute.String("kind", kind))
	}
}

// RecordAnswerUnconfirmed counts one answer entering Unconfirmed; the other
// delivery states are already in guidance_answer_delivery_total{route,state}.
func RecordAnswerUnconfirmed() { telemetry.AddOne(answerUnconfirmedCounter()) }

// ScheduleWakeupRoles resolves a session's backlog role for the receipt counter.
type ScheduleWakeupRoles struct{ Lookup ItemSessionLookup }

func (r ScheduleWakeupRoles) RoleOf(ctx context.Context, sessionUUID string) string {
	if r.Lookup == nil {
		return RoleUnlinked
	}
	row, err := r.Lookup.GetItemSessionBySessionUUID(ctx, sessionUUID)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return RoleUnlinked
	case err != nil:
		return RoleUnknown
	case row.Role == "":
		return RoleUnlinked
	}
	return row.Role
}

// ObserveScheduleWakeup counts one ScheduleWakeup receipt and writes the
// structured line the wake-baseline script reads.
func ObserveScheduleWakeup(ctx context.Context, roles ScheduleWakeupRoles, sessionUUID string) {
	role := roles.RoleOf(ctx, sessionUUID)
	telemetry.AddOne(scheduleWakeupCounter(), attribute.String("role", role))
	log.Info(scheduleWakeupReceiptEvent, "session", sessionUUID, "role", role)
}

// noteOperatorOpen is called when a web terminal stream attaches to an agent
// session's pane.
func NoteOperatorOpen(inst *instance.Instance) {
	DefaultReopenTracker.Attached(inst.GetStableID(), inst.GetTitle())
}
