package services

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// FlagAuditPolicy says how a flag's flips are audited. Tightening flips never
// wait on the sink; loosening flips are audited durably before they persist.
type FlagAuditPolicy struct {
	// Loosening reports whether setting the flag to enabled reduces a protection.
	Loosening func(enabled bool) bool
	// EscapeHatch lets a loosening flip persist on the fallback log record alone
	// when the sink is down (the flag is the escape hatch for a sink fault).
	EscapeHatch bool
}

// flagAudit is the audit state of one UpdateFeatureFlag call.
type flagAudit struct {
	sink     *AuditSink
	base     AuditLine
	previous bool
	seq      int64
	outcome  string
	enabled  bool
}

func flagRequestFields(ctx context.Context, peer string, header http.Header) AuditLine {
	l := AuditLine{PeerAddr: peer, UserAgent: header.Get("User-Agent")}
	if rec, ok := RequestRecordFrom(ctx); ok {
		l.Listener, l.Host, l.Origin, l.AuthMode = rec.Listener, rec.Host, rec.Origin, rec.AuthMode
	}
	return l
}

func boolRef(b bool) *bool { return &b }

// begin writes the durable `requested` line of a loosening flip before the
// update mutex is taken (bounded, so a stalled fsync cannot queue the kill
// switch behind it). It returns ErrAuditFailed or ErrAuditTimeout when the flip
// must be refused.
func (f *FeatureFlagService) auditBegin(ctx context.Context, name string, enabled bool, fields AuditLine) (*flagAudit, error) {
	policy, audited := f.auditPolicies[name]
	if f.audit == nil || !audited {
		return nil, nil
	}
	a := &flagAudit{sink: f.audit, enabled: enabled, outcome: flagOutcomePersistFailed}
	a.base = fields
	a.base.Kind, a.base.Flag, a.base.Scope, a.base.ChangeID = auditKindFlagChg, name, "global", uuid.NewString()
	if policy.Loosening == nil || !policy.Loosening(enabled) {
		return a, nil
	}
	req := a.base
	req.Phase, req.New = auditPhaseRequest, boolRef(enabled)
	if err := f.audit.AppendBounded(ctx, req); err != nil {
		if policy.EscapeHatch {
			f.audit.fallback(req, err)
			return a, nil
		}
		return nil, err
	}
	return a, nil
}

// finish enqueues the `result` line. It is deferred before the update mutex is
// taken, so it runs after the unlock and never lengthens the critical section.
func (a *flagAudit) finish() {
	if a == nil {
		return
	}
	l := a.base
	l.Phase, l.Outcome, l.Seq = auditPhaseResult, a.outcome, a.seq
	l.Previous, l.New = boolRef(a.previous), boolRef(a.enabled)
	a.sink.Enqueue(l)
}

// Close joins the audit sink's drain goroutine (a no-op without one).
func (f *FeatureFlagService) Close() {
	if f.audit != nil {
		f.audit.Close()
	}
}
