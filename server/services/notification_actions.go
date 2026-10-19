package services

import (
	"context"
	"encoding/json"
	"strings"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
)

// notification_actions.go computes the inline-action descriptors of a
// notification history page (ADR-002) and holds the one tier/availability
// table the builder and the dispatcher share (ADR-004, Story 2.8).

// ListenerClass is which listener served the request.
type ListenerClass string

const (
	ListenerClassLocal  ListenerClass = ListenerLocal
	ListenerClassRemote ListenerClass = ListenerRemote
)

// ListenerClassFrom derives the class from the request record stamped by the
// listener chain. A request nobody stamped is treated as remote (fail closed).
func ListenerClassFrom(ctx context.Context) ListenerClass {
	if rec, ok := RequestRecordFrom(ctx); ok && rec.Listener == ListenerLocal {
		return ListenerClassLocal
	}
	return ListenerClassRemote
}

// unavailableDesktopOnly is the descriptor reason for a Tier B action on the remote listener.
const unavailableDesktopOnly = "desktop_only"

// Notification metadata keys read by the builder.
const (
	metaApprovalID         = "approval_id"
	metaItemID             = "item_id"
	metaDecisionSchema     = "decision_schema"
	boolTrue               = "true"
	backlogStatusReview    = "review"
	backlogStatusPRPending = "pr_pending"
)

// Backlog decision choice values (the stable part of decision_schema).
const (
	choiceAccept = "accept"
	choiceRework = "rework"
	choiceMerge  = "merge"
)

// actionPolicy is the single tier and remote-availability row of an action.
type actionPolicy struct {
	Tier            sessionv1.ActionTier
	RemoteAvailable bool
}

var (
	policyTierA = actionPolicy{Tier: sessionv1.ActionTier_ACTION_TIER_A, RemoteAvailable: true}
	policyTierB = actionPolicy{Tier: sessionv1.ActionTier_ACTION_TIER_B, RemoteAvailable: false}
)

// actionPolicies has exactly one row per ActionKind. ANSWER_QUESTION is listed
// at its live_keystroke tier; a durable-record answer is Tier A (answerTier).
var actionPolicies = map[sessionv1.ActionKind]actionPolicy{
	sessionv1.ActionKind_ACTION_KIND_OPEN:             policyTierA,
	sessionv1.ActionKind_ACTION_KIND_APPROVE:          policyTierA,
	sessionv1.ActionKind_ACTION_KIND_DENY:             policyTierA,
	sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION:  policyTierB,
	sessionv1.ActionKind_ACTION_KIND_BACKLOG_DECISION: policyTierA,
	sessionv1.ActionKind_ACTION_KIND_ANSWER_GUIDANCE:  policyTierA,
	sessionv1.ActionKind_ACTION_KIND_CANCEL_WAKE:      policyTierA,
	sessionv1.ActionKind_ACTION_KIND_CANCEL_PEER_HELP: policyTierA,
}

// Control names the pane controls that are not an ActionKind but share the table.
type Control string

const (
	ControlRemindMe        Control = "remind_me"
	ControlResumeScheduled Control = "resume_scheduled_prompts"
)

// controlPolicies: Remind me creates a wake row (data only); Resume scheduled
// prompts flips agent_wake_triggers, a flag write allowed on the local listener
// only ("Resume from desktop" on the remote one).
var controlPolicies = map[Control]actionPolicy{
	ControlRemindMe:        policyTierA,
	ControlResumeScheduled: {Tier: sessionv1.ActionTier_ACTION_TIER_A, RemoteAvailable: false},
}

// answerTier is the tier of an ANSWER_QUESTION on route: only a live keystroke
// writes into a pane.
func answerTier(route answerRoute) sessionv1.ActionTier {
	switch route {
	case routeLiveKeystrokeHidden, routeLiveKeystrokeVisible:
		return sessionv1.ActionTier_ACTION_TIER_B
	default:
		return sessionv1.ActionTier_ACTION_TIER_A
	}
}

// DescriptorSources are the per-page lookups of the builder. Each is called at
// most once per page, and only when some unresolved record needs it.
type DescriptorSources struct {
	Approvals ApprovalLookup
	Questions QuestionLookup
	Backlog   BacklogDecisionLookup
	Guidance  GuidanceLookup
}

// ApprovalLookup returns the ids of approvals still waiting for a decision.
type ApprovalLookup interface {
	PendingApprovalIDs() map[string]bool
}

// QuestionLookup returns the live registered questions among keys.
type QuestionLookup interface {
	PeekQuestions(keys []QuestionKey) map[QuestionKey]PendingQuestion
}

// BacklogDecisionState is what the builder needs to know about an item.
type BacklogDecisionState struct {
	Status string
}

// BacklogDecisionLookup returns the state of the given backlog items.
type BacklogDecisionLookup interface {
	DecisionStates(ctx context.Context, itemIDs []string) (map[string]BacklogDecisionState, error)
}

// OpenGuidance is an unanswered guidance request.
type OpenGuidance struct {
	Options []string
}

// GuidanceLookup returns the open guidance requests among ids.
type GuidanceLookup interface {
	OpenGuidance(ctx context.Context, ids []string) (map[string]OpenGuidance, error)
}

// DescriptorBuilder assembles descriptors for a history page.
type DescriptorBuilder struct {
	sources DescriptorSources
	// flag resolves a notification_inline_actions decision point; tests inject it.
	flag func(decisionPoint string) bool
}

// NewDescriptorBuilder builds a builder reading the live flags.
func NewDescriptorBuilder(src DescriptorSources) *DescriptorBuilder {
	return &DescriptorBuilder{sources: src, flag: func(dp string) bool {
		return effectiveFlag(notificationInlineActionsFlagName, dp)
	}}
}

// pageKeys are the entity keys of the unresolved records of one page.
type pageKeys struct {
	needApprovals bool
	questions     []QuestionKey
	itemIDs       []string
	guidanceIDs   []string
}

func collectPageKeys(records []*sessionv1.NotificationHistoryRecord) pageKeys {
	var k pageKeys
	seenQ, seenItem, seenG := map[QuestionKey]bool{}, map[string]bool{}, map[string]bool{}
	for _, r := range records {
		if r.GetIsRead() {
			continue
		}
		md := r.GetMetadata()
		if md[metaApprovalID] != "" {
			k.needApprovals = true
		}
		if qk, ok := questionKeyOf(r); ok && !seenQ[qk] {
			seenQ[qk] = true
			k.questions = append(k.questions, qk)
		}
		if id := md[metaItemID]; id != "" && md[metaDecisionSchema] != "" && !seenItem[id] {
			seenItem[id] = true
			k.itemIDs = append(k.itemIDs, id)
		}
		if id := md[metaGuidanceRequestID]; id != "" && !seenG[id] {
			seenG[id] = true
			k.guidanceIDs = append(k.guidanceIDs, id)
		}
	}
	return k
}

func questionKeyOf(r *sessionv1.NotificationHistoryRecord) (QuestionKey, bool) {
	md := r.GetMetadata()
	if md[metaQuestionID] == "" || md[metaReplyUnavailable] == boolTrue || r.GetSessionId() == "" {
		return QuestionKey{}, false
	}
	return QuestionKey{SessionUUID: r.GetSessionId(), QuestionID: md[metaQuestionID]}, true
}

// pageState is the result of the per-page lookups.
type pageState struct {
	approvals map[string]bool
	questions map[QuestionKey]PendingQuestion
	backlog   map[string]BacklogDecisionState
	guidance  map[string]OpenGuidance
}

func (b *DescriptorBuilder) lookup(ctx context.Context, k pageKeys) pageState {
	var st pageState
	if k.needApprovals && b.sources.Approvals != nil {
		st.approvals = b.sources.Approvals.PendingApprovalIDs()
	}
	if len(k.questions) > 0 && b.sources.Questions != nil {
		st.questions = b.sources.Questions.PeekQuestions(k.questions)
	}
	if len(k.itemIDs) > 0 && b.sources.Backlog != nil {
		m, err := b.sources.Backlog.DecisionStates(ctx, k.itemIDs)
		if err != nil {
			log.Warn("[NotificationActions] backlog decision lookup failed; decisions reduce to open", "err", err)
		}
		st.backlog = m
	}
	if len(k.guidanceIDs) > 0 && b.sources.Guidance != nil {
		m, err := b.sources.Guidance.OpenGuidance(ctx, k.guidanceIDs)
		if err != nil {
			log.Warn("[NotificationActions] guidance lookup failed; answers reduce to open", "err", err)
		}
		st.guidance = m
	}
	return st
}

// Annotate sets Actions on every record of one history page. Resolved records
// get only OPEN. Lookups are batched: one call per store per page.
func (b *DescriptorBuilder) Annotate(ctx context.Context, records []*sessionv1.NotificationHistoryRecord, lc ListenerClass) {
	st := b.lookup(ctx, collectPageKeys(records))
	for _, r := range records {
		r.Actions = b.describe(r, st, lc)
	}
}

func (b *DescriptorBuilder) describe(r *sessionv1.NotificationHistoryRecord, st pageState, lc ListenerClass) []*sessionv1.NotificationActionDescriptor {
	open := openDescriptor(r)
	if r.GetIsRead() {
		return []*sessionv1.NotificationActionDescriptor{open}
	}
	var out []*sessionv1.NotificationActionDescriptor
	out = append(out, b.approvalDescriptors(r, st)...)
	out = append(out, b.questionDescriptors(r, st, lc)...)
	out = append(out, b.guidanceDescriptors(r, st)...)
	out = append(out, b.decisionDescriptors(r, st)...)
	out = b.gate(out, lc)
	return append(out, open)
}

// gate drops descriptors whose action flag (or the remote channel flag) is off,
// so a flag-off page reduces to OPEN.
func (b *DescriptorBuilder) gate(in []*sessionv1.NotificationActionDescriptor, lc ListenerClass) []*sessionv1.NotificationActionDescriptor {
	out := in[:0:0]
	for _, d := range in {
		if !b.flag(scopePrefixAction + kindName(d.GetKind())) {
			continue
		}
		if lc == ListenerClassRemote && !b.flag(scopeChannelRemote) {
			continue
		}
		out = append(out, d)
	}
	return out
}

// kindName is the scope name of a kind: APPROVE for ACTION_KIND_APPROVE.
func kindName(k sessionv1.ActionKind) string {
	return strings.TrimPrefix(k.String(), "ACTION_KIND_")
}

func openDescriptor(r *sessionv1.NotificationHistoryRecord) *sessionv1.NotificationActionDescriptor {
	d := &sessionv1.NotificationActionDescriptor{
		Kind: sessionv1.ActionKind_ACTION_KIND_OPEN, Label: "Open",
		Tier: sessionv1.ActionTier_ACTION_TIER_A, Available: true, SessionId: r.GetSessionId(),
	}
	if id := r.GetMetadata()[metaItemID]; id != "" {
		d.TargetKeyType, d.TargetKey = sessionv1.ActionTargetKey_ACTION_TARGET_KEY_ITEM_ID, id
	}
	return d
}

func tierADescriptor(kind sessionv1.ActionKind, label string, kt sessionv1.ActionTargetKey, key string) *sessionv1.NotificationActionDescriptor {
	return &sessionv1.NotificationActionDescriptor{
		Kind: kind, Label: label, Tier: sessionv1.ActionTier_ACTION_TIER_A,
		Available: true, TargetKeyType: kt, TargetKey: key,
	}
}

func (b *DescriptorBuilder) approvalDescriptors(r *sessionv1.NotificationHistoryRecord, st pageState) []*sessionv1.NotificationActionDescriptor {
	id := r.GetMetadata()[metaApprovalID]
	if id == "" || !st.approvals[id] {
		return nil
	}
	kt := sessionv1.ActionTargetKey_ACTION_TARGET_KEY_APPROVAL_ID
	return []*sessionv1.NotificationActionDescriptor{
		tierADescriptor(sessionv1.ActionKind_ACTION_KIND_APPROVE, "Approve", kt, id),
		tierADescriptor(sessionv1.ActionKind_ACTION_KIND_DENY, "Deny", kt, id),
	}
}

func (b *DescriptorBuilder) questionDescriptors(r *sessionv1.NotificationHistoryRecord, st pageState, lc ListenerClass) []*sessionv1.NotificationActionDescriptor {
	key, ok := questionKeyOf(r)
	if !ok {
		return nil
	}
	q, live := st.questions[key]
	if !live {
		return nil
	}
	tier := answerTier(routeLiveKeystrokeHidden)
	d := &sessionv1.NotificationActionDescriptor{
		Kind: sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION, Label: "Answer", Tier: tier, Available: true,
		TargetKeyType: sessionv1.ActionTargetKey_ACTION_TARGET_KEY_QUESTION_ID, TargetKey: key.QuestionID,
		SessionId: key.SessionUUID,
	}
	for _, l := range q.Dialog.Labels {
		d.Options = append(d.Options, &sessionv1.ActionOption{Label: l})
	}
	if lc == ListenerClassRemote && !actionPolicies[sessionv1.ActionKind_ACTION_KIND_ANSWER_QUESTION].RemoteAvailable {
		d.Available, d.UnavailableReason = false, unavailableDesktopOnly
	}
	return []*sessionv1.NotificationActionDescriptor{d}
}

func (b *DescriptorBuilder) guidanceDescriptors(r *sessionv1.NotificationHistoryRecord, st pageState) []*sessionv1.NotificationActionDescriptor {
	id := r.GetMetadata()[metaGuidanceRequestID]
	g, open := st.guidance[id]
	if id == "" || !open {
		return nil
	}
	d := tierADescriptor(sessionv1.ActionKind_ACTION_KIND_ANSWER_GUIDANCE, "Answer",
		sessionv1.ActionTargetKey_ACTION_TARGET_KEY_GUIDANCE_REQUEST_ID, id)
	for _, o := range g.Options {
		d.Options = append(d.Options, &sessionv1.ActionOption{Label: o})
	}
	return []*sessionv1.NotificationActionDescriptor{d}
}

// DecisionSchema is the stable shape of the decision_schema metadata.
type DecisionSchema struct {
	Choices []DecisionChoice `json:"choices"`
}

// DecisionChoice is one choice with the existing RPC it maps to.
type DecisionChoice struct {
	Value string `json:"value"`
	Label string `json:"label"`
	RPC   string `json:"rpc"`
}

// backlogChoices maps each decision choice to the existing RPC that performs it
// (confirmed against proto/session/v1/backlog.proto and session.proto). A choice
// with no row here has no RPC and is emitted as OPEN only.
var backlogChoices = map[string]DecisionChoice{
	choiceAccept: {Value: choiceAccept, Label: "Mark done", RPC: "TransitionBacklogItemStatus"},
	choiceRework: {Value: choiceRework, Label: "Send back", RPC: "TransitionBacklogItemStatus"},
	choiceMerge:  {Value: choiceMerge, Label: "Merge", RPC: "MergePR"},
}

// DefaultDecisionSchema is the schema of an item awaiting accept, rework or merge.
func DefaultDecisionSchema() DecisionSchema {
	return DecisionSchema{Choices: []DecisionChoice{backlogChoices[choiceAccept], backlogChoices[choiceRework], backlogChoices[choiceMerge]}}
}

// EncodeDecisionSchema is the metadata value of decision_schema.
func EncodeDecisionSchema(s DecisionSchema) string {
	b, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	return string(b)
}

// choicesFor lists the schema choices valid for the item's current status.
func choicesFor(status string, schema DecisionSchema) []DecisionChoice {
	allowed := allowedChoices(status)
	var out []DecisionChoice
	for _, c := range schema.Choices {
		if known, ok := backlogChoices[c.Value]; ok && allowed[c.Value] {
			out = append(out, known)
		}
	}
	return out
}

func allowedChoices(status string) map[string]bool {
	switch status {
	case backlogStatusReview:
		return map[string]bool{choiceAccept: true, choiceRework: true, choiceMerge: true}
	case backlogStatusPRPending:
		return map[string]bool{choiceMerge: true}
	default:
		return nil
	}
}

func (b *DescriptorBuilder) decisionDescriptors(r *sessionv1.NotificationHistoryRecord, st pageState) []*sessionv1.NotificationActionDescriptor {
	md := r.GetMetadata()
	id, raw := md[metaItemID], md[metaDecisionSchema]
	state, known := st.backlog[id]
	if id == "" || raw == "" || !known {
		return nil
	}
	var schema DecisionSchema
	if err := json.Unmarshal([]byte(raw), &schema); err != nil {
		return nil
	}
	choices := choicesFor(state.Status, schema)
	if len(choices) == 0 {
		return nil
	}
	d := tierADescriptor(sessionv1.ActionKind_ACTION_KIND_BACKLOG_DECISION, "Decide",
		sessionv1.ActionTargetKey_ACTION_TARGET_KEY_ITEM_ID, id)
	for _, c := range choices {
		d.Options = append(d.Options, &sessionv1.ActionOption{Value: c.Value, Label: c.Label})
	}
	return []*sessionv1.NotificationActionDescriptor{d}
}
