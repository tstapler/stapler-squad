package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
)

// notification_action_service.go is the single InvokeNotificationAction
// dispatcher (ADR-002 decision 3): validate, tier policy, flags, idempotency,
// then a closed switch over ActionKind that delegates to the existing handlers
// through consumer-side interfaces. It re-implements none of their checks.

const (
	// actionClientHeader is sent by the web UI on every action call. It labels the
	// audit line; it is a hint, not authentication (a same-user process can send it).
	actionClientHeader = "X-Stapler-Client"
	actionClientWebUI  = "web-ui"

	errUnknownKind         = "unknown action_kind"
	maxIdempotencyKeyBytes = 128
	maxAnswerBytes         = 4096

	// Audit and metric label values.
	channelWebLocal  = "web_local"
	channelWebRemote = "web_remote"
	channelUnknown   = "unknown"
	callerOperatorUI = "operator_ui"
	callerUnknown    = "unknown"
)

// ActionResult is what a delegate reports. Expected conditions are outcomes;
// only an infrastructure fault is a returned error.
type ActionResult struct {
	Outcome     sessionv1.ActionOutcome
	StaleReason string
	RetryAfter  int32
	Detail      string
}

func applied() ActionResult {
	return ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED}
}

func outcome(o sessionv1.ActionOutcome, detail string) ActionResult {
	return ActionResult{Outcome: o, Detail: detail}
}

// ApprovalResolver resolves a pending approval.
type ApprovalResolver interface {
	ResolveApproval(ctx context.Context, d ApprovalChoice) (ActionResult, error)
}

// ApprovalChoice is an operator decision on one approval.
type ApprovalChoice struct {
	ApprovalID string
	Allow      bool
	Reason     string
	// Override acknowledges a CI-red block ("Approve anyway").
	Override bool
}

// QuestionReplier answers a pending AskUserQuestion. RouteFor tells the
// dispatcher where the write goes so it can pick the tier and flag gate.
type QuestionReplier interface {
	RouteFor(sessionID string) (route answerRoute, known bool)
	ReplyToQuestion(ctx context.Context, a QuestionAnswer) (ActionResult, error)
}

// QuestionAnswer is one option choice for one pending question.
type QuestionAnswer struct {
	SessionID  string
	QuestionID string
	// OptionLabel is the label of the chosen option.
	OptionLabel string
	// ReplyID is the caller's idempotency key.
	ReplyID string
	// Confirm is the operator's "Retry anyway": re-verify the dialog and write
	// again although an earlier digit may already have landed.
	Confirm bool
}

// GuidanceAnswerer answers a durable guidance request.
type GuidanceAnswerer interface {
	AnswerGuidance(ctx context.Context, requestID, answer string) (ActionResult, error)
}

// BacklogDecider applies one backlog decision choice.
type BacklogDecider interface {
	Decide(ctx context.Context, d BacklogDecision) (ActionResult, error)
}

// BacklogDecision is an operator choice on a backlog item.
type BacklogDecision struct {
	ItemID  string
	Choice  string
	Reason  string
	Confirm bool
}

// CancelDelegate cancels a pending wake or an open peer-help request (Epic 4
// implements it; both are row updates, Tier A).
type CancelDelegate interface {
	CancelWake(ctx context.Context, wakeID string) (ActionResult, error)
	CancelPeerHelp(ctx context.Context, requestID string) (ActionResult, error)
}

// NotificationResolver retires the history rows of a decided target. The
// dispatcher calls it once the entity has changed, so a card never outlives its
// decision in another tab.
type NotificationResolver interface {
	ResolveItemNotifications(itemID string)
	ResolveQuestionNotifications(questionID string)
	ResolveApprovalNotifications(approvalID string)
}

// actionFlags resolves the flag decision points the dispatcher consults.
type actionFlags interface {
	// Inline resolves a notification_inline_actions decision point.
	Inline(decisionPoint string) bool
	// Answer resolves the three-flag ANSWER_QUESTION composition for route.
	Answer(route answerRoute) bool
}

// configActionFlags resolves against the live config and the compiled spike
// evidence. Tests inject evidence to stand in for a recorded spike result.
type configActionFlags struct{ ev flagEvidence }

func (f configActionFlags) Inline(dp string) bool {
	return resolveEffectiveFlag(config.LoadConfig(), f.ev, notificationInlineActionsFlagName, dp)
}

func (f configActionFlags) Answer(route answerRoute) bool {
	return resolveAnswerQuestion(config.LoadConfig(), f.ev, route)
}

// actionAudit appends audit lines; *AuditSink satisfies it.
type actionAudit interface {
	Append(AuditLine) error
	AppendBounded(ctx context.Context, l AuditLine) error
}

// ActionCall is one inbound request with the facts the dispatcher needs from
// the transport.
type ActionCall struct {
	Msg    *sessionv1.InvokeNotificationActionRequest
	Peer   string
	Header http.Header
}

// NotificationActionDispatcher serves InvokeNotificationAction.
type NotificationActionDispatcher struct {
	approvals ApprovalResolver
	questions QuestionReplier
	guidance  GuidanceAnswerer
	backlog   BacklogDecider
	cancels   CancelDelegate
	resolver  NotificationResolver
	flags     actionFlags
	audit     func() actionAudit
	metrics   ActionMetrics
	cache     *actionIdempotency
	now       func() time.Time
}

// ActionDelegates are the consumer-side collaborators of the dispatcher. A nil
// delegate makes its kinds report FLAG_OFF-free INVALID ("not wired").
type ActionDelegates struct {
	Approvals ApprovalResolver
	Questions QuestionReplier
	Guidance  GuidanceAnswerer
	Backlog   BacklogDecider
	Cancels   CancelDelegate
	Resolver  NotificationResolver
}

// NewNotificationActionDispatcher builds a dispatcher reading the live flags.
// audit may return nil (the audit trail is then unavailable and Tier B refuses).
func NewNotificationActionDispatcher(d ActionDelegates, audit func() actionAudit) *NotificationActionDispatcher {
	return &NotificationActionDispatcher{
		approvals: d.Approvals, questions: d.Questions, guidance: d.Guidance, backlog: d.Backlog, cancels: d.Cancels, resolver: d.Resolver,
		flags: configActionFlags{ev: compiledFlagEvidence}, audit: audit, metrics: defaultActionMetrics(),
		cache: newActionIdempotency(actionCacheMax), now: time.Now,
	}
}

// Invoke runs one action. The returned error is only for infrastructure faults.
func (d *NotificationActionDispatcher) Invoke(ctx context.Context, call ActionCall) (*sessionv1.InvokeNotificationActionResponse, error) {
	start := d.now()
	msg := call.Msg
	caller := callerOf(ctx, call.Header)

	res, err := d.run(ctx, call, caller)
	d.record(msg, caller, res, err, d.now().Sub(start))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("action failed"))
	}
	return &sessionv1.InvokeNotificationActionResponse{
		Outcome: res.Outcome, StaleReason: res.StaleReason, RetryAfterSeconds: res.RetryAfter, Detail: res.Detail,
	}, nil
}

// callerInfo labels who made the call, for audit and metrics.
type callerInfo struct {
	channel  string
	kind     string
	listener ListenerClass
}

func callerOf(ctx context.Context, h http.Header) callerInfo {
	lc := ListenerClassFrom(ctx)
	if h.Get(actionClientHeader) != actionClientWebUI {
		return callerInfo{channel: channelUnknown, kind: callerUnknown, listener: lc}
	}
	ch := channelWebLocal
	if lc == ListenerClassRemote {
		ch = channelWebRemote
	}
	return callerInfo{channel: ch, kind: callerOperatorUI, listener: lc}
}

func (d *NotificationActionDispatcher) run(ctx context.Context, call ActionCall, caller callerInfo) (ActionResult, error) {
	msg := call.Msg
	if bad := validateAction(msg); bad != "" {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, bad), nil
	}
	if msg.GetActionKind() == sessionv1.ActionKind_ACTION_KIND_OPEN {
		return applied(), nil // OPEN is navigation: always on, no side effect
	}
	route, res, stop := d.resolveRoute(msg)
	if stop {
		return res, nil
	}
	tier := tierOf(msg.GetActionKind(), route)
	if res, stop = d.tierGate(ctx, call, caller, tier); stop {
		return res, nil
	}
	if res, stop = d.flagGate(msg.GetActionKind(), route, caller); stop {
		return res, nil
	}
	flight, role := d.cache.begin(msg.GetIdempotencyKey(), payloadHash(msg))
	switch role {
	case flightMismatch:
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, "idempotency_key was already used for a different action"), nil
	case flightFollower:
		return flight.wait(ctx)
	}
	res, err := d.execute(ctx, call, caller, tier)
	if err != nil || !cacheable(res.Outcome) {
		d.cache.forget(flight, res)
		return res, err
	}
	d.cache.settle(flight, res)
	return res, nil
}

// cacheable reports whether a retry with the same key must see the same result.
// Outcomes that are safe or useful to retry are not replayed.
func cacheable(o sessionv1.ActionOutcome) bool {
	switch o {
	case sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED,
		sessionv1.ActionOutcome_ACTION_OUTCOME_ALREADY_RESOLVED,
		sessionv1.ActionOutcome_ACTION_OUTCOME_STALE,
		sessionv1.ActionOutcome_ACTION_OUTCOME_SEND_INDETERMINATE:
		return true
	default:
		return false
	}
}

func validateAction(m *sessionv1.InvokeNotificationActionRequest) string {
	switch {
	case m == nil:
		return "request is required"
	case m.GetIdempotencyKey() == "" || len(m.GetIdempotencyKey()) > maxIdempotencyKeyBytes:
		return "idempotency_key is required (at most 128 bytes)"
	case len(m.GetParams().GetAnswer()) > maxAnswerBytes:
		return "answer is too long"
	}
	if _, known := actionPolicies[m.GetActionKind()]; !known {
		return errUnknownKind
	}
	if m.GetActionKind() != sessionv1.ActionKind_ACTION_KIND_OPEN && m.GetTargetKey() == "" {
		return "target_key is required"
	}
	return ""
}

// resolveRoute finds the answer route of an ANSWER_QUESTION; other kinds have none.
func (d *NotificationActionDispatcher) resolveRoute(m *sessionv1.InvokeNotificationActionRequest) (answerRoute, ActionResult, bool) {
	if m.GetActionKind() != sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION {
		return routeDurableRecord, ActionResult{}, false
	}
	if d.questions == nil {
		return 0, outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_FLAG_OFF, flagOffMessage), true
	}
	if m.GetSessionId() == "" {
		return 0, outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, "session_id is required"), true
	}
	route, known := d.questions.RouteFor(m.GetSessionId())
	if !known {
		return 0, ActionResult{Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_STALE, StaleReason: "session_ended"}, true
	}
	return route, ActionResult{}, false
}

// tierOf is the tier of kind: the table row, except an ANSWER_QUESTION whose
// route is a durable record (Tier A).
func tierOf(kind sessionv1.ActionKind, route answerRoute) sessionv1.ActionTier {
	if kind == sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION {
		return answerTier(route)
	}
	return actionPolicies[kind].Tier
}

// tierGate refuses Tier B where it may not run, before any flag or write. The
// remote listener refuses it unless channel:remote:tierB is effective (S6 gate);
// the local listener re-runs the in-handler verdict on the real peer, because
// the delegate below sees only this process as its caller.
func (d *NotificationActionDispatcher) tierGate(ctx context.Context, call ActionCall, caller callerInfo, tier sessionv1.ActionTier) (ActionResult, bool) {
	if tier != sessionv1.ActionTier_ACTION_TIER_B {
		return ActionResult{}, false
	}
	if caller.listener == ListenerClassRemote {
		if d.flags.Inline(scopeChannelTierB) {
			return ActionResult{}, false
		}
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_FORBIDDEN, unavailableDesktopOnly), true
	}
	if _, err := EvaluateLocalWrite(ctx, call.Peer, LocalWriteLocalCaller); err != nil {
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_FORBIDDEN, "local caller required"), true
	}
	return ActionResult{}, false
}

// flagGate applies the flag matrix: the action's scope (ANSWER_QUESTION composes
// its route gate) and, on the remote listener, channel:remote.
func (d *NotificationActionDispatcher) flagGate(kind sessionv1.ActionKind, route answerRoute, caller callerInfo) (ActionResult, bool) {
	on := d.flags.Inline(scopePrefixAction + kindName(kind))
	if kind == sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION {
		on = d.flags.Answer(route)
	}
	if on && caller.listener == ListenerClassRemote {
		on = d.flags.Inline(scopeChannelRemote)
	}
	if on {
		return ActionResult{}, false
	}
	return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_FLAG_OFF, flagOffMessage), true
}

func payloadHash(m *sessionv1.InvokeNotificationActionRequest) string {
	p := m.GetParams()
	h := sha256.New()
	for _, f := range []string{
		m.GetActionKind().String(), m.GetTargetKeyType().String(), m.GetTargetKey(), m.GetSessionId(),
		p.GetAnswer(), p.GetChoice(), p.GetReason(), boolField(p.GetConfirm()),
	} {
		h.Write([]byte(f))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func boolField(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// execute audits and runs the delegate for the closed set of kinds.
func (d *NotificationActionDispatcher) execute(ctx context.Context, call ActionCall, caller callerInfo, tier sessionv1.ActionTier) (ActionResult, error) {
	line := d.auditLine(call.Msg, caller)
	if tier == sessionv1.ActionTier_ACTION_TIER_B {
		if err := d.appendRequest(ctx, line); err != nil {
			return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_NOT_SENT, "audit log unavailable, nothing sent"), nil
		}
	}
	res, err := d.delegate(ctx, call.Msg)
	d.appendResult(line, res)
	return res, err
}

func (d *NotificationActionDispatcher) delegate(ctx context.Context, m *sessionv1.InvokeNotificationActionRequest) (ActionResult, error) {
	p := m.GetParams()
	switch m.GetActionKind() {
	case sessionv1.ActionKind_ACTION_KIND_APPROVE, sessionv1.ActionKind_ACTION_KIND_DENY:
		if d.approvals == nil {
			return notWired(), nil
		}
		res, err := d.approvals.ResolveApproval(ctx, ApprovalChoice{
			ApprovalID: m.GetTargetKey(), Allow: m.GetActionKind() == sessionv1.ActionKind_ACTION_KIND_APPROVE,
			Reason: p.GetReason(), Override: p.GetConfirm(),
		})
		if err == nil && res.Outcome == sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED && d.resolver != nil {
			d.resolver.ResolveApprovalNotifications(m.GetTargetKey())
		}
		return res, err
	case sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION:
		res, err := d.questions.ReplyToQuestion(ctx, QuestionAnswer{
			SessionID: m.GetSessionId(), QuestionID: m.GetTargetKey(), OptionLabel: p.GetAnswer(), ReplyID: m.GetIdempotencyKey(), Confirm: p.GetConfirm(),
		})
		if err == nil && res.Outcome == sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED && d.resolver != nil {
			d.resolver.ResolveQuestionNotifications(m.GetTargetKey())
		}
		return res, err
	case sessionv1.ActionKind_ACTION_KIND_ANSWER_GUIDANCE:
		if d.guidance == nil {
			return notWired(), nil
		}
		return d.guidance.AnswerGuidance(ctx, m.GetTargetKey(), p.GetAnswer())
	case sessionv1.ActionKind_ACTION_KIND_BACKLOG_DECISION:
		return d.decide(ctx, m)
	case sessionv1.ActionKind_ACTION_KIND_CANCEL_WAKE:
		if d.cancels == nil {
			return notWired(), nil
		}
		return d.cancels.CancelWake(ctx, m.GetTargetKey())
	case sessionv1.ActionKind_ACTION_KIND_CANCEL_PEER_HELP:
		if d.cancels == nil {
			return notWired(), nil
		}
		return d.cancels.CancelPeerHelp(ctx, m.GetTargetKey())
	case sessionv1.ActionKind_ACTION_KIND_OPEN, sessionv1.ActionKind_ACTION_KIND_UNSPECIFIED:
		return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, errUnknownKind), nil
	}
	return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, errUnknownKind), nil
}

func notWired() ActionResult {
	return outcome(sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID, "this action is not available on this server")
}

// decide applies a backlog choice, then retires the item's decision rows once
// the entity has changed (the entity is acted on first, ADR-002 decision 4).
func (d *NotificationActionDispatcher) decide(ctx context.Context, m *sessionv1.InvokeNotificationActionRequest) (ActionResult, error) {
	if d.backlog == nil {
		return notWired(), nil
	}
	p := m.GetParams()
	res, err := d.backlog.Decide(ctx, BacklogDecision{
		ItemID: m.GetTargetKey(), Choice: p.GetChoice(), Reason: p.GetReason(), Confirm: p.GetConfirm(),
	})
	if err == nil && res.Outcome == sessionv1.ActionOutcome_ACTION_OUTCOME_APPLIED && d.resolver != nil {
		d.resolver.ResolveItemNotifications(m.GetTargetKey())
	}
	return res, err
}

func (d *NotificationActionDispatcher) auditLine(m *sessionv1.InvokeNotificationActionRequest, c callerInfo) AuditLine {
	return AuditLine{
		Kind: auditKindNotificationAction, Channel: c.channel, CallerKind: c.kind, Listener: string(c.listener),
		ActionKind: kindName(m.GetActionKind()), TargetKey: m.GetTargetKey(), ReplyID: m.GetIdempotencyKey(),
	}
}

func (d *NotificationActionDispatcher) sink() actionAudit {
	if d.audit == nil {
		return nil
	}
	return d.audit()
}

func (d *NotificationActionDispatcher) appendRequest(ctx context.Context, l AuditLine) error {
	l.Phase = auditPhaseRequest
	s := d.sink()
	if s == nil {
		return ErrAuditFailed
	}
	return s.AppendBounded(ctx, l)
}

// appendResult cannot undo the action, so a fault logs.
func (d *NotificationActionDispatcher) appendResult(l AuditLine, res ActionResult) {
	d.appendResultLine(l, res.Outcome)
}

func (d *NotificationActionDispatcher) appendResultLine(l AuditLine, o sessionv1.ActionOutcome) {
	l.Phase, l.Outcome = auditPhaseResult, outcomeLabel(o)
	s := d.sink()
	if s == nil {
		log.Warn("[NotificationAction] no audit sink; result line not written", "action", l.ActionKind, "outcome", l.Outcome)
		return
	}
	if err := s.Append(l); err != nil {
		log.Warn("[NotificationAction] result audit line not written", "action", l.ActionKind, "outcome", l.Outcome, "err", err)
	}
}

// record counts the call and audits outcomes decided before execute (FORBIDDEN,
// FLAG_OFF, INVALID), so every invocation leaves one queryable line.
func (d *NotificationActionDispatcher) record(m *sessionv1.InvokeNotificationActionRequest, c callerInfo, res ActionResult, err error, elapsed time.Duration) {
	label := outcomeLabel(res.Outcome)
	if err != nil {
		label = "error"
	}
	d.metrics.Record(ActionSample{Kind: strings.ToLower(kindName(m.GetActionKind())), Outcome: label, Channel: c.channel, Elapsed: elapsed})
	if preExecution(res.Outcome) {
		d.appendResultLine(d.auditLine(m, c), res.Outcome)
	}
}

// preExecution reports outcomes that never reach execute, which writes its own line.
func preExecution(o sessionv1.ActionOutcome) bool {
	switch o {
	case sessionv1.ActionOutcome_ACTION_OUTCOME_FORBIDDEN,
		sessionv1.ActionOutcome_ACTION_OUTCOME_FLAG_OFF,
		sessionv1.ActionOutcome_ACTION_OUTCOME_INVALID:
		return true
	default:
		return false
	}
}

func outcomeLabel(o sessionv1.ActionOutcome) string {
	return strings.ToLower(strings.TrimPrefix(o.String(), "ACTION_OUTCOME_"))
}

// InvokeNotificationAction applies one inline notification action.
// +api: notification:invoke-action
// +api: InvokeNotificationAction
func (s *SessionService) InvokeNotificationAction(
	ctx context.Context,
	req *connect.Request[sessionv1.InvokeNotificationActionRequest],
) (*connect.Response[sessionv1.InvokeNotificationActionResponse], error) {
	if s.actionDispatcher == nil {
		return connect.NewResponse(&sessionv1.InvokeNotificationActionResponse{
			Outcome: sessionv1.ActionOutcome_ACTION_OUTCOME_FLAG_OFF, Detail: flagOffMessage,
		}), nil
	}
	resp, err := s.actionDispatcher.Invoke(ctx, ActionCall{Msg: req.Msg, Peer: req.Peer().Addr, Header: req.Header()})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// SetNotificationActionDispatcher wires the dispatcher behind the RPC.
func (s *SessionService) SetNotificationActionDispatcher(d *NotificationActionDispatcher) {
	s.actionDispatcher = d
}
