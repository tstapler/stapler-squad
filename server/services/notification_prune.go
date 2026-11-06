package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/notifications"
)

// Prune audit outcomes and count keys (kind=prune lines carry counts only).
const (
	pruneOutcomeApplied      = "applied"
	pruneOutcomeFailed       = "failed"
	pruneCountKeptUnread     = "kept_unread_actionable"
	pruneCountUndeterminable = "undeterminable"
	pruneCountVisible        = "visible"
)

// SetAuditSink wires the sink a prune apply appends its kind=prune line to.
func (ns *NotificationService) SetAuditSink(sink *AuditSink) { ns.auditSink = sink }

func (ns *NotificationService) pruneClock() time.Time {
	if ns.pruneNow != nil {
		return ns.pruneNow()
	}
	return time.Now()
}

// PruneHiddenSessionNotifications removes stored notification rows of hidden
// sessions on the operator's command (plan Story 2.7). It is a dry run unless
// apply is set, keeps rows whose session cannot be classified, keeps unread
// pending decisions unless include_unread_actionable is set, refuses while the
// visibility index is unseeded, and writes a timestamped backup and an audit
// line before it deletes. It is in the LocalWriteGuard set (whole procedure)
// and has no MCP tool.
func (ns *NotificationService) PruneHiddenSessionNotifications(
	ctx context.Context,
	req *connect.Request[sessionv1.PruneHiddenSessionNotificationsRequest],
) (*connect.Response[sessionv1.PruneHiddenSessionNotificationsResponse], error) {
	if ns.notificationStore == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("notification history is not configured"))
	}
	if ns.deliveryGate == nil || !ns.deliveryGate.Index().Seeded() {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("the session visibility index is not seeded yet; retry once the server has finished starting"))
	}

	msg := req.Msg
	facts := requestFactsOf(ctx, req)
	resolver := ns.deliveryGate.Resolver()
	classify := func(r *notifications.NotificationRecord) notifications.PruneDecision {
		return pruneDecision(resolver.ClassifyStored(r.SessionID, r.Metadata), r.SessionID)
	}

	var line AuditLine
	requested := false // the durable requested line is written
	opts := notifications.PruneOptions{
		Apply:                   msg.GetApply(),
		IncludeUnreadActionable: msg.GetIncludeUnreadActionable(),
		Now:                     ns.pruneClock(),
	}
	if opts.Apply {
		opts.BeforeApply = func(p notifications.PrunePlan) error {
			line = pruneAuditLine(facts, p, opts.IncludeUnreadActionable)
			line.Phase = auditPhaseRequest
			if err := ns.appendPruneRequest(ctx, line); err != nil {
				return err
			}
			requested = true
			return nil
		}
	}

	plan, err := ns.notificationStore.PruneByPredicate(classify, opts)
	if err != nil {
		if requested {
			ns.appendPruneResult(line, pruneOutcomeFailed)
		}
		log.Error("[NotificationHistory] prune failed; nothing further deleted", "apply", opts.Apply, "err", err)
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if plan.Applied {
		ns.appendPruneResult(line, pruneOutcomeApplied)
	}
	logPrune(plan, opts, facts)
	return connect.NewResponse(pruneResponse(plan)), nil
}

func (ns *NotificationService) appendPruneRequest(ctx context.Context, line AuditLine) error {
	if ns.auditSink == nil {
		return fmt.Errorf("%w: no audit sink configured", ErrAuditFailed)
	}
	return ns.auditSink.AppendBounded(ctx, line)
}

// appendPruneResult records the outcome after the delete; a fault here cannot
// undo anything, so it only logs.
func (ns *NotificationService) appendPruneResult(line AuditLine, outcome string) {
	line.Phase, line.Outcome = auditPhaseResult, outcome
	if ns.auditSink == nil {
		return
	}
	if err := ns.auditSink.Append(line); err != nil {
		log.Warn("[NotificationHistory] prune result audit line not written", "change_id", line.ChangeID, "outcome", outcome, "err", err)
	}
}

func pruneDecision(res deliverygate.RowResolution, rawID string) notifications.PruneDecision {
	d := notifications.PruneDecision{Form: string(res.Form), ByAlias: res.ByAlias}
	switch res.Class {
	case deliverygate.RowVisible:
		d.Visible = true
	case deliverygate.RowHidden:
		d.Hidden = true
		title := res.Title
		if title == "" {
			title = rawID // tombstones carry no title
		}
		d.Group = title + " [" + string(res.Kind) + "]"
	}
	return d
}

// pruneAuditLine builds the kind=prune line: request facts, counts and flags,
// no row content, title or session id.
func pruneAuditLine(f steerRequestFacts, p notifications.PrunePlan, includeUnread bool) AuditLine {
	matched := len(p.Remove)
	counts := p.ReasonCounts()
	counts[pruneCountKeptUnread] = p.KeptUnreadActionable
	counts[pruneCountUndeterminable] = p.Undeterminable
	counts[pruneCountVisible] = p.Visible
	return AuditLine{
		Kind: auditKindPrune, ChangeID: uuid.NewString(),
		MatchedCount: &matched, IncludeUnreadActionable: boolRef(includeUnread), Counts: counts,
		Listener: f.refusal.Listener, PeerAddr: f.refusal.Peer, Host: f.refusal.Host, Origin: f.refusal.Origin,
		UserAgent: f.userAgent, AuthMode: f.refusal.AuthMode,
		PeerLoopback: boolRef(f.refusal.PeerLoopback), Proxied: boolRef(f.refusal.Proxied),
	}
}

func pruneResponse(p notifications.PrunePlan) *sessionv1.PruneHiddenSessionNotificationsResponse {
	return &sessionv1.PruneHiddenSessionNotificationsResponse{
		Applied:         p.Applied,
		NotificationIds: p.IDs(),
		// #nosec G115 -- counts are local notification-store row counts, far below int32 range.
		KeptUnreadActionable: int32(p.KeptUnreadActionable),
		// #nosec G115 -- as above.
		Undeterminable: int32(p.Undeterminable),
		BackupPath:     p.BackupPath,
	}
}

// logPrune logs the removed (or would-be-removed) count and ids at INFO, grouped
// by resolved session and kind, with the caller's remote address and listener.
func logPrune(p notifications.PrunePlan, opts notifications.PruneOptions, f steerRequestFacts) {
	msg := "[NotificationHistory] prune dry run"
	if p.Applied {
		msg = "[NotificationHistory] prune applied"
	}
	log.Info(msg, "count", len(p.Remove), "ids", p.IDs(), "kept_unread_actionable", p.KeptUnreadActionable,
		"undeterminable", p.Undeterminable, "include_unread_actionable", opts.IncludeUnreadActionable,
		"backup", p.BackupPath, "peer_addr", f.refusal.Peer, "listener", f.refusal.Listener)
	groups := map[string][]string{}
	for _, r := range p.Remove {
		key := fmt.Sprintf("%s id_form=%s reason=%s", r.Group, r.Form, r.Reason)
		if r.ByAlias {
			key += " matched_by_alias"
		}
		groups[key] = append(groups[key], r.ID)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		log.Info("[NotificationHistory] prune group", "group", k, "count", len(groups[k]), "ids", groups[k])
	}
}
