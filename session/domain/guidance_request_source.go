package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// guidance_request_source.go holds the types that let one GuidanceRequest
// table serve agent guidance, mirrored harness prompts and peer help
// (ADR-003 of notification-actions-and-agent-messaging).

// QuestionSource says who produced a GuidanceRequest. It also selects the
// pending-cap class and the coalescing key.
type QuestionSource string

const (
	// QuestionSourceAgentMCP: raised by an agent through the guidance MCP tools (the legacy path).
	QuestionSourceAgentMCP QuestionSource = "agent_mcp"
	// QuestionSourceHook: mirrored from a harness prompt (a Claude AskUserQuestion hook, or a
	// terminal-detected dialog of a replyable profile).
	QuestionSourceHook QuestionSource = "ask_user_question_hook"
	// QuestionSourcePeer: one session asking another for help.
	QuestionSourcePeer QuestionSource = "peer"
)

// IsValid reports whether s is a known source.
func (s QuestionSource) IsValid() bool {
	switch s {
	case QuestionSourceAgentMCP, QuestionSourceHook, QuestionSourcePeer:
		return true
	}
	return false
}

// AnswererKind says who may answer a GuidanceRequest.
type AnswererKind string

const (
	AnswererKindHuman   AnswererKind = "human"
	AnswererKindSession AnswererKind = "session"
)

// IsValid reports whether k is a known answerer kind.
func (k AnswererKind) IsValid() bool { return k == AnswererKindHuman || k == AnswererKindSession }

// DeliveryRoute is how an answer reaches the asker (ADR-005 ladder). The empty
// value means no route has been chosen yet.
type DeliveryRoute string

const (
	DeliveryRouteHookReply     DeliveryRoute = "hook_reply"
	DeliveryRouteLiveKeystroke DeliveryRoute = "live_keystroke"
	DeliveryRouteResume        DeliveryRoute = "resume"
	DeliveryRouteReadOnly      DeliveryRoute = "read_only"
)

// IsValid reports whether r is a known route; the unset route is not valid.
func (r DeliveryRoute) IsValid() bool {
	switch r {
	case DeliveryRouteHookReply, DeliveryRouteLiveKeystroke, DeliveryRouteResume, DeliveryRouteReadOnly:
		return true
	}
	return false
}

// DeliveryState is the delivery fact of an answer, separate from answered_at:
// answered is not delivered. The empty value means no delivery is tracked.
type DeliveryState string

const (
	DeliveryStatePending       DeliveryState = "pending"
	DeliveryStateSent          DeliveryState = "sent"
	DeliveryStateAcknowledged  DeliveryState = "acknowledged"
	DeliveryStateFailed        DeliveryState = "delivery_failed"
	DeliveryStateSavedForLater DeliveryState = "saved_for_later"
	DeliveryStateStale         DeliveryState = "stale"
	// DeliveryStateAbortedPartial: part of a multi-question answer was typed, then
	// the dialog changed. The row stays unanswered, so it stays in Needs you.
	DeliveryStateAbortedPartial DeliveryState = "aborted_partial"
)

// IsValid reports whether s is a known state; the untracked state is not valid.
func (s DeliveryState) IsValid() bool {
	_, ok := deliveryTransitions[s]
	return ok && s != ""
}

// deliveryTransitions is the single transition table. The empty state is the
// start: a read-only or stopped-session row is born saved_for_later, stale or failed.
var deliveryTransitions = map[DeliveryState][]DeliveryState{
	"":                          {DeliveryStatePending, DeliveryStateSavedForLater, DeliveryStateStale, DeliveryStateFailed},
	DeliveryStatePending:        {DeliveryStateSent, DeliveryStateFailed, DeliveryStateSavedForLater, DeliveryStateStale, DeliveryStateAbortedPartial},
	DeliveryStateSent:           {DeliveryStateAcknowledged, DeliveryStateFailed, DeliveryStateStale},
	DeliveryStateSavedForLater:  {DeliveryStatePending, DeliveryStateStale},
	DeliveryStateFailed:         {DeliveryStatePending, DeliveryStateStale},
	DeliveryStateAcknowledged:   nil,
	DeliveryStateStale:          nil,
	DeliveryStateAbortedPartial: nil,
}

// CanTransitionTo reports whether next may follow s.
func (s DeliveryState) CanTransitionTo(next DeliveryState) bool {
	return slices.Contains(deliveryTransitions[s], next)
}

// PredecessorsOf lists the states from which next is reachable, for a guarded
// conditional UPDATE. The untracked state is the empty string.
func PredecessorsOf(next DeliveryState) []DeliveryState {
	var from []DeliveryState
	for s, targets := range deliveryTransitions {
		if slices.Contains(targets, next) {
			from = append(from, s)
		}
	}
	slices.Sort(from)
	return from
}

// ErrInvalidDelivery is returned for a route and state that cannot coexist.
var ErrInvalidDelivery = errors.New("invalid delivery route/state combination")

// ValidateDelivery checks a route/state pair: both known when set, a state only
// with a route, and a read_only route never in a sent or acknowledged state
// (nothing was pushed to a pane).
func ValidateDelivery(route DeliveryRoute, state DeliveryState) error {
	if route != "" && !route.IsValid() {
		return fmt.Errorf("%w: unknown route %q", ErrInvalidDelivery, route)
	}
	if state != "" && !state.IsValid() {
		return fmt.Errorf("%w: unknown state %q", ErrInvalidDelivery, state)
	}
	if state != "" && route == "" {
		return fmt.Errorf("%w: state %q needs a route", ErrInvalidDelivery, state)
	}
	if route == DeliveryRouteReadOnly {
		switch state {
		case DeliveryStateStale, DeliveryStateSavedForLater, DeliveryStateFailed:
		default:
			return fmt.Errorf("%w: read_only route cannot be in state %q", ErrInvalidDelivery, state)
		}
	}
	return nil
}

// HookCallID is the Claude tool_use_id of one hook invocation.
type HookCallID string

// PeerHelpTTL is how long a peer-help row stays open before it expires.
const PeerHelpTTL = 24 * time.Hour

// QuestionPayload limits. Claude's own AskUserQuestion caps are tighter (4
// questions, 2-4 options, short labels); these bound a hostile or malformed
// payload without rejecting a real one.
const (
	MaxPayloadQuestions     = 4
	MaxQuestionOptions      = 8
	MaxQuestionTextRunes    = 2000
	MaxQuestionHeaderRunes  = 64
	MaxOptionLabelRunes     = 200
	MaxOptionDescriptionLen = 1000
	MaxQuestionProfileRunes = 32
)

// ErrInvalidQuestionPayload wraps every QuestionPayload validation failure.
var ErrInvalidQuestionPayload = errors.New("invalid question payload")

// Option is one choice of a Question.
type Option struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Question is one question of a QuestionPayload; its JSON shape is the
// AskUserQuestion tool input's.
type Question struct {
	Text        string   `json:"question"`
	Header      string   `json:"header,omitempty"`
	Options     []Option `json:"options"`
	MultiSelect bool     `json:"multiSelect"`
	// Profile names the non-Claude harness profile that detected this question
	// (questions_json.profile); empty for Claude's hook payload.
	Profile string `json:"profile,omitempty"`
}

// QuestionPayload is the canonical, validated set of 1-4 questions stored in
// questions_json. Holding one proves the limits above were checked.
type QuestionPayload struct{ questions []Question }

// NewQuestionPayload validates qs and returns a payload that owns a copy.
func NewQuestionPayload(qs []Question) (QuestionPayload, error) {
	if len(qs) < 1 || len(qs) > MaxPayloadQuestions {
		return QuestionPayload{}, fmt.Errorf("%w: need 1-%d questions, got %d", ErrInvalidQuestionPayload, MaxPayloadQuestions, len(qs))
	}
	owned := make([]Question, len(qs))
	for i, q := range qs {
		if err := validateQuestion(q); err != nil {
			return QuestionPayload{}, fmt.Errorf("question %d: %w", i+1, err)
		}
		q.Options = slices.Clone(q.Options)
		owned[i] = q
	}
	return QuestionPayload{questions: owned}, nil
}

func validateQuestion(q Question) error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidQuestionPayload, fmt.Sprintf(format, args...))
	}
	switch {
	case strings.TrimSpace(q.Text) == "":
		return bad("question text is empty")
	case utf8.RuneCountInString(q.Text) > MaxQuestionTextRunes:
		return bad("question text over %d characters", MaxQuestionTextRunes)
	case utf8.RuneCountInString(q.Header) > MaxQuestionHeaderRunes:
		return bad("header over %d characters", MaxQuestionHeaderRunes)
	case utf8.RuneCountInString(q.Profile) > MaxQuestionProfileRunes:
		return bad("profile over %d characters", MaxQuestionProfileRunes)
	case len(q.Options) < 1 || len(q.Options) > MaxQuestionOptions:
		return bad("need 1-%d options, got %d", MaxQuestionOptions, len(q.Options))
	}
	for i, o := range q.Options {
		switch {
		case strings.TrimSpace(o.Label) == "":
			return bad("option %d has an empty label", i+1)
		case utf8.RuneCountInString(o.Label) > MaxOptionLabelRunes:
			return bad("option %d label over %d characters", i+1, MaxOptionLabelRunes)
		case utf8.RuneCountInString(o.Description) > MaxOptionDescriptionLen:
			return bad("option %d description over %d characters", i+1, MaxOptionDescriptionLen)
		}
	}
	return nil
}

// ParseQuestionPayload parses questions_json: a JSON array of questions.
func ParseQuestionPayload(data []byte) (QuestionPayload, error) {
	var qs []Question
	if err := json.Unmarshal(data, &qs); err != nil {
		return QuestionPayload{}, fmt.Errorf("%w: %v", ErrInvalidQuestionPayload, err)
	}
	return NewQuestionPayload(qs)
}

// IsZero reports whether p holds no questions (the unset payload).
func (p QuestionPayload) IsZero() bool { return len(p.questions) == 0 }

// Questions returns a copy of the questions.
func (p QuestionPayload) Questions() []Question {
	out := make([]Question, len(p.questions))
	for i, q := range p.questions {
		q.Options = slices.Clone(q.Options)
		out[i] = q
	}
	return out
}

// JSON is the canonical serialization stored in questions_json; equal
// payloads serialize to equal strings, which the coalescing fallback relies on.
func (p QuestionPayload) JSON() (string, error) {
	if p.IsZero() {
		return "", nil
	}
	b, err := json.Marshal(p.questions)
	if err != nil {
		return "", fmt.Errorf("marshal question payload: %w", err)
	}
	return string(b), nil
}

// MarshalJSON emits the questions array so a payload embeds in other JSON.
func (p QuestionPayload) MarshalJSON() ([]byte, error) { return json.Marshal(p.Questions()) }

// UnmarshalJSON validates while parsing.
func (p *QuestionPayload) UnmarshalJSON(data []byte) error {
	parsed, err := ParseQuestionPayload(data)
	if err != nil {
		return err
	}
	*p = parsed
	return nil
}

// firstQuestionFields derives what old readers see: question 1's text, a
// multiple-choice type and the option labels as a JSON array.
func (p QuestionPayload) firstQuestionFields() (text string, qtype QuestionType, options string, err error) {
	q := p.questions[0]
	labels := make([]string, len(q.Options))
	for i, o := range q.Options {
		labels[i] = o.Label
	}
	b, err := json.Marshal(labels)
	if err != nil {
		return "", "", "", fmt.Errorf("marshal option labels: %w", err)
	}
	return q.Text, QuestionTypeMultipleChoice, string(b), nil
}

// ErrInvalidGuidanceRequest wraps every NewGuidanceRequest cross-field failure.
var ErrInvalidGuidanceRequest = errors.New("invalid guidance request")

// GuidanceRequestParams is the raw input to NewGuidanceRequest.
type GuidanceRequestParams struct {
	Scope        RequestScope
	ItemID       string // set only for backlog-item scope
	SessionUUID  string // set only for session scope; the scope key
	QuestionText string // derived from Payload when empty
	QuestionType QuestionType
	Options      string // JSON []string; derived from Payload when empty
	Source       QuestionSource
	// AskerSessionUUID is the waiting session; required for hook and peer sources.
	AskerSessionUUID    string
	AnswererKind        AnswererKind
	AnswererSessionUUID string
	MultiSelect         bool // question 1's flag when a Payload is set
	Payload             QuestionPayload
	HookCallID          HookCallID
	DeliveryRoute       DeliveryRoute
	DeliveryState       DeliveryState
}

// GuidanceRequest is a validated request. Its only constructor is
// NewGuidanceRequest, so a repository that accepts one never sees an illegal
// field combination.
type GuidanceRequest struct{ p GuidanceRequestParams }

// Params returns a copy of the validated fields, defaults filled in.
func (g GuidanceRequest) Params() GuidanceRequestParams { return g.p }

// NewGuidanceRequest applies the cross-field rules of ADR-003 rule 3b and fills
// defaults (source agent_mcp, answerer human):
//   - source peer implies answerer_kind session; a session answerer needs its UUID
//     and only a session answerer may carry one.
//   - hook and peer rows are session-scoped with asker_session_uuid == the scope
//     key; a hook row carries a QuestionPayload; hook_call_id is not for agent_mcp.
//   - a session-scoped row with an asker names itself as the asker.
//   - route and state pass ValidateDelivery (read_only is never sent).
func NewGuidanceRequest(p GuidanceRequestParams) (GuidanceRequest, error) {
	if p.Source == "" {
		p.Source = QuestionSourceAgentMCP
	}
	if p.AnswererKind == "" {
		p.AnswererKind = AnswererKindHuman
	}
	if !p.Payload.IsZero() {
		text, qtype, options, err := p.Payload.firstQuestionFields()
		if err != nil {
			return GuidanceRequest{}, err
		}
		if p.QuestionText == "" {
			p.QuestionText, p.Options = text, options
		}
		if p.QuestionType == "" {
			p.QuestionType = qtype
		}
		p.MultiSelect = p.Payload.questions[0].MultiSelect
	}
	if err := validateGuidanceParams(p); err != nil {
		return GuidanceRequest{}, fmt.Errorf("%w: %w", ErrInvalidGuidanceRequest, err)
	}
	return GuidanceRequest{p: p}, nil
}

func validateGuidanceParams(p GuidanceRequestParams) error {
	if err := validateGuidanceEnums(p); err != nil {
		return err
	}
	if err := validateGuidanceAnswerer(p); err != nil {
		return err
	}
	if err := validateGuidanceSource(p); err != nil {
		return err
	}
	return ValidateDelivery(p.DeliveryRoute, p.DeliveryState)
}

func validateGuidanceEnums(p GuidanceRequestParams) error {
	switch {
	case !p.Scope.IsValid():
		return fmt.Errorf("unknown scope %q", p.Scope)
	case !p.QuestionType.IsValid():
		return fmt.Errorf("unknown question type %q", p.QuestionType)
	case !p.Source.IsValid():
		return fmt.Errorf("unknown source %q", p.Source)
	case !p.AnswererKind.IsValid():
		return fmt.Errorf("unknown answerer kind %q", p.AnswererKind)
	case strings.TrimSpace(p.QuestionText) == "":
		return errors.New("question text is empty")
	}
	return nil
}

func validateGuidanceAnswerer(p GuidanceRequestParams) error {
	switch {
	case p.Source == QuestionSourcePeer && p.AnswererKind != AnswererKindSession:
		return errors.New("a peer request needs answerer_kind=session")
	case p.AnswererKind == AnswererKindSession && p.AnswererSessionUUID == "":
		return errors.New("answerer_kind=session needs answerer_session_uuid")
	case p.AnswererKind != AnswererKindSession && p.AnswererSessionUUID != "":
		return errors.New("answerer_session_uuid is only for answerer_kind=session")
	}
	return nil
}

func validateGuidanceSource(p GuidanceRequestParams) error {
	if p.Scope == RequestScopeSession && p.AskerSessionUUID != "" && p.AskerSessionUUID != p.SessionUUID {
		return errors.New("for scope=session the asker must be the scope key")
	}
	if p.Source == QuestionSourceAgentMCP {
		if p.HookCallID != "" {
			return errors.New("hook_call_id is not for source=agent_mcp")
		}
		return nil
	}
	switch {
	case p.Scope != RequestScopeSession:
		return fmt.Errorf("source=%s needs scope=session", p.Source)
	case p.AskerSessionUUID == "":
		return fmt.Errorf("source=%s needs asker_session_uuid", p.Source)
	case p.Source == QuestionSourceHook && p.Payload.IsZero():
		return errors.New("source=ask_user_question_hook needs a question payload")
	}
	return nil
}
