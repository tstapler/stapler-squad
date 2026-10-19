package wakes

import (
	"context"
	"fmt"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// maxAnswerDetailRunes bounds the answer text echoed into a wake.
const maxAnswerDetailRunes = 1000

// registerGuidanceSource installs the guidance_answered predicate. There is no
// bus matcher: services.AnswerListener calls OnGuidanceAnswered instead.
func (d *Dispatcher) registerGuidanceSource() {
	d.RegisterPredicate(domain.WakeTriggerGuidanceAnswered, func(ctx context.Context, spec domain.WakeSpec) (bool, error) {
		s, ok := spec.(domain.GuidanceAnsweredSpec)
		if !ok || d.cfg.Guidance == nil {
			return false, nil
		}
		f, err := d.cfg.Guidance.Guidance(ctx, s.GuidanceRequestID)
		if err != nil {
			return false, fmt.Errorf("guidance predicate %s: %w", s.GuidanceRequestID, err)
		}
		return f.Exists && f.Answered, nil
	})
}

// OnGuidanceAnswered is the services.AnswerListener body: it fires the pending
// guidance_answered triggers on id. A restart before delivery is covered by
// reconcile re-checking the row's answered state.
func (d *Dispatcher) OnGuidanceAnswered(ctx context.Context, id string) {
	select {
	case <-d.ready:
	default:
		return // startup reconcile will find the answered row
	}
	d.fireCell(ctx, EventMatch{Type: domain.WakeTriggerGuidanceAnswered, Key: id, Check: true}, "")
}

// guidanceDetail says who answered, so the asker can tell a peer's answer from
// the operator's (ADR-007 rule 4).
func (d *Dispatcher) guidanceDetail(ctx context.Context, id string) string {
	if d.cfg.Guidance == nil {
		return ""
	}
	f, err := d.cfg.Guidance.Guidance(ctx, id)
	if err != nil || !f.Exists || !f.Answered {
		if err != nil {
			log.Warn("wake dispatcher: guidance detail unavailable", "guidance_request_id", id, errKey, err)
		}
		return ""
	}
	answer := truncateRunes(f.Answer, maxAnswerDetailRunes)
	if f.AnswererKind == string(domain.AnswererKindSession) {
		return fmt.Sprintf("help request %s answered by session %s: %s", id, f.AnswererSessionUUID, answer)
	}
	return fmt.Sprintf("guidance request %s answered by the operator: %s", id, answer)
}

// withGuidanceAnswer returns row with the answer's provenance appended when
// row waits on a guidance request; every other row is returned unchanged.
func (d *Dispatcher) withGuidanceAnswer(ctx context.Context, row *session.WakeTriggerData) *session.WakeTriggerData {
	spec, ok := row.Spec.(domain.GuidanceAnsweredSpec)
	if !ok {
		return row
	}
	detail := d.guidanceDetail(ctx, spec.GuidanceRequestID)
	if detail == "" {
		return row
	}
	out := *row
	withEventDetail(&out, detail)
	return &out
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}
