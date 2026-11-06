package services

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/telemetry"
)

// notification_quality_metrics.go holds the paired quality metrics of Epic 5
// (requirements.md): reopen_after_action_total, the Unconfirmed counter, and
// the ScheduleWakeup receipt counter behind the Epic 4 cost outcome.

const (
	metricReopenAfterAction    = "reopen_after_action_total"
	metricGuidanceUnconfirmed  = "guidance_answer_unconfirmed_total"
	metricScheduleWakeupCalls  = "agent_schedule_wakeup_calls_total"
	scheduleWakeupTool         = "ScheduleWakeup"
	scheduleWakeupReceiptEvent = "schedule_wakeup_receipt"
	roleUnlinked               = "unlinked"
	roleUnknown                = "unknown"

	// reopenWindow is how soon after an applied action an operator open of the
	// same session counts as a possible wrong answer.
	reopenWindow = 60 * time.Second
)

// counterNamed resolves the instrument on every use (the SDK returns the same
// one per meter), so a test that swaps the meter provider is never stuck with a
// counter bound to an earlier one.
func counterNamed(name, desc string) func() metric.Int64Counter {
	return func() metric.Int64Counter {
		c, err := telemetry.GetMeter().Int64Counter(name, metric.WithDescription(desc))
		if err != nil {
			log.Error("services: failed to register "+name, "error", err)
			return nil
		}
		return c
	}
}

func addOne(c metric.Int64Counter, attrs ...attribute.KeyValue) {
	if c != nil {
		c.Add(context.Background(), 1, metric.WithAttributes(attrs...))
	}
}

var (
	reopenCounter = counterNamed(metricReopenAfterAction,
		"Operator opened a session's terminal within 60s of an applied inline answer to it")
	answerUnconfirmedCounter = counterNamed(metricGuidanceUnconfirmed,
		"Keystroke answers that entered the Unconfirmed sub-state (indeterminate write or no acknowledgement at 60s)")
	scheduleWakeupCounter = counterNamed(metricScheduleWakeupCalls,
		"ScheduleWakeup PostToolUse receipts by backlog role")
)

type pendingReopen struct {
	kind string
	at   time.Time
}

// reopenTracker counts an applied ANSWER_QUESTION followed by the operator
// opening that session's terminal within reopenWindow. Only answers carry a
// session id, so APPROVE, DENY and BACKLOG_DECISION are not tracked. A nil
// tracker ignores every call.
type reopenTracker struct {
	mu      sync.Mutex
	now     func() time.Time
	pending map[string]pendingReopen
}

func newReopenTracker(now func() time.Time) *reopenTracker {
	if now == nil {
		now = time.Now
	}
	return &reopenTracker{now: now, pending: map[string]pendingReopen{}}
}

var defaultReopenTracker = newReopenTracker(nil)

func (t *reopenTracker) noteApplied(m *sessionv1.InvokeNotificationActionRequest, res ActionResult) {
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
func (t *reopenTracker) attached(ids ...string) {
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
		addOne(reopenCounter(), attribute.String("kind", kind))
	}
}

// recordAnswerUnconfirmed counts one answer entering Unconfirmed; the other
// delivery states are already in guidance_answer_delivery_total{route,state}.
func recordAnswerUnconfirmed() { addOne(answerUnconfirmedCounter()) }

// scheduleWakeupRoles resolves a session's backlog role for the receipt counter.
type scheduleWakeupRoles struct{ lookup itemSessionLookup }

func (r scheduleWakeupRoles) roleOf(ctx context.Context, sessionUUID string) string {
	if r.lookup == nil {
		return roleUnlinked
	}
	row, err := r.lookup.GetItemSessionBySessionUUID(ctx, sessionUUID)
	switch {
	case errors.Is(err, session.ErrNotFound):
		return roleUnlinked
	case err != nil:
		return roleUnknown
	case row.Role == "":
		return roleUnlinked
	}
	return row.Role
}

// observeScheduleWakeup counts one ScheduleWakeup receipt and writes the
// structured line the wake-baseline script reads.
func observeScheduleWakeup(ctx context.Context, roles scheduleWakeupRoles, sessionUUID string) {
	role := roles.roleOf(ctx, sessionUUID)
	addOne(scheduleWakeupCounter(), attribute.String("role", role))
	log.Info(scheduleWakeupReceiptEvent, "session", sessionUUID, "role", role)
}

// noteOperatorOpen is called when a web terminal stream attaches to an agent
// session's pane.
func noteOperatorOpen(inst *session.Instance) {
	defaultReopenTracker.attached(inst.GetStableID(), inst.GetTitle())
}
