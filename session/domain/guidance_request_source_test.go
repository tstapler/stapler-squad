package domain

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func oneQuestion() []Question {
	return []Question{{Text: "Which cache?", Header: "Cache", Options: []Option{{Label: "Redis", Description: "in-memory"}, {Label: "None"}}}}
}

func mustPayload(t *testing.T, qs []Question) QuestionPayload {
	t.Helper()
	p, err := NewQuestionPayload(qs)
	require.NoError(t, err)
	return p
}

func TestSumTypes_IsValid(t *testing.T) {
	assert.True(t, QuestionSourceAgentMCP.IsValid())
	assert.True(t, QuestionSourceHook.IsValid())
	assert.True(t, QuestionSourcePeer.IsValid())
	assert.False(t, QuestionSource("terminal_detect").IsValid(), "there is no terminal_detect source")
	assert.False(t, QuestionSource("").IsValid())

	assert.True(t, AnswererKindHuman.IsValid())
	assert.True(t, AnswererKindSession.IsValid())
	assert.False(t, AnswererKind("bot").IsValid())

	for _, r := range []DeliveryRoute{DeliveryRouteHookReply, DeliveryRouteLiveKeystroke, DeliveryRouteResume, DeliveryRouteReadOnly} {
		assert.True(t, r.IsValid(), r)
	}
	assert.False(t, DeliveryRoute("").IsValid())
	assert.False(t, DeliveryRoute("carrier_pigeon").IsValid())

	for _, s := range []DeliveryState{DeliveryStatePending, DeliveryStateSent, DeliveryStateAcknowledged, DeliveryStateFailed, DeliveryStateSavedForLater, DeliveryStateStale} {
		assert.True(t, s.IsValid(), s)
	}
	assert.False(t, DeliveryState("").IsValid())
	assert.False(t, DeliveryState("delivered").IsValid())
}

func TestDeliveryState_CanTransitionTo(t *testing.T) {
	tests := []struct {
		from, to DeliveryState
		want     bool
	}{
		{"", DeliveryStatePending, true},
		{"", DeliveryStateSent, false},
		{"", DeliveryStateSavedForLater, true},
		{DeliveryStatePending, DeliveryStateSent, true},
		{DeliveryStatePending, DeliveryStateAcknowledged, false},
		{DeliveryStateSent, DeliveryStateAcknowledged, true},
		{DeliveryStateSent, DeliveryStateFailed, true},
		{DeliveryStateSent, DeliveryStatePending, false},
		{DeliveryStateFailed, DeliveryStatePending, true},
		{DeliveryStateFailed, DeliveryStateSent, false},
		{DeliveryStateSavedForLater, DeliveryStatePending, true},
		{DeliveryStateAcknowledged, DeliveryStateSent, false},
		{DeliveryStateAcknowledged, DeliveryStateStale, false},
		{DeliveryStateStale, DeliveryStatePending, false},
		{DeliveryStateSent, DeliveryStateSent, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			assert.Equal(t, tt.want, tt.from.CanTransitionTo(tt.to))
		})
	}
}

func TestPredecessorsOf_MatchesTheTable(t *testing.T) {
	assert.Equal(t, []DeliveryState{DeliveryStatePending}, PredecessorsOf(DeliveryStateSent))
	assert.Equal(t, []DeliveryState{DeliveryStateSent}, PredecessorsOf(DeliveryStateAcknowledged))
	assert.Equal(t, []DeliveryState{"", DeliveryStateFailed, DeliveryStateSavedForLater}, PredecessorsOf(DeliveryStatePending))
	for next := range deliveryTransitions {
		for _, from := range PredecessorsOf(next) {
			assert.True(t, from.CanTransitionTo(next), "%q -> %q", from, next)
		}
	}
}

func TestValidateDelivery(t *testing.T) {
	tests := []struct {
		name    string
		route   DeliveryRoute
		state   DeliveryState
		wantErr bool
	}{
		{"untracked", "", "", false},
		{"route chosen, nothing sent yet", DeliveryRouteLiveKeystroke, "", false},
		{"keystroke sent", DeliveryRouteLiveKeystroke, DeliveryStateSent, false},
		{"read_only saved", DeliveryRouteReadOnly, DeliveryStateSavedForLater, false},
		{"read_only stale", DeliveryRouteReadOnly, DeliveryStateStale, false},
		{"read_only failed", DeliveryRouteReadOnly, DeliveryStateFailed, false},
		{"read_only sent", DeliveryRouteReadOnly, DeliveryStateSent, true},
		{"read_only acknowledged", DeliveryRouteReadOnly, DeliveryStateAcknowledged, true},
		{"read_only pending", DeliveryRouteReadOnly, DeliveryStatePending, true},
		{"read_only without state", DeliveryRouteReadOnly, "", true},
		{"state without route", "", DeliveryStateSent, true},
		{"unknown route", "pigeon", DeliveryStateSent, true},
		{"unknown state", DeliveryRouteResume, "delivered", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDelivery(tt.route, tt.state)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrInvalidDelivery)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewQuestionPayload(t *testing.T) {
	longLabel := strings.Repeat("x", MaxOptionLabelRunes+1)
	opts := func(n int) []Option {
		o := make([]Option, n)
		for i := range o {
			o[i] = Option{Label: "opt"}
		}
		return o
	}
	tests := []struct {
		name    string
		qs      []Question
		wantErr bool
	}{
		{"one question", oneQuestion(), false},
		{"four questions", []Question{oneQuestion()[0], oneQuestion()[0], oneQuestion()[0], oneQuestion()[0]}, false},
		{"none", nil, true},
		{"five questions", []Question{oneQuestion()[0], oneQuestion()[0], oneQuestion()[0], oneQuestion()[0], oneQuestion()[0]}, true},
		{"empty text", []Question{{Text: " ", Options: opts(2)}}, true},
		{"no options", []Question{{Text: "q", Options: nil}}, true},
		{"too many options", []Question{{Text: "q", Options: opts(MaxQuestionOptions + 1)}}, true},
		{"max options", []Question{{Text: "q", Options: opts(MaxQuestionOptions)}}, false},
		{"empty label", []Question{{Text: "q", Options: []Option{{Label: ""}}}}, true},
		{"long label", []Question{{Text: "q", Options: []Option{{Label: longLabel}}}}, true},
		{"long header", []Question{{Text: "q", Header: strings.Repeat("h", MaxQuestionHeaderRunes+1), Options: opts(2)}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewQuestionPayload(tt.qs)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrInvalidQuestionPayload)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestQuestionPayload_ParsesTheRealHookShapeAndOwnsItsData(t *testing.T) {
	raw := `[{"question":"Which cache?","header":"Cache","options":[{"label":"Redis","description":"d"},{"label":"None","description":""}],"multiSelect":false},
	{"question":"Which letters?","header":"Letters","options":[{"label":"A","description":"Option A"}],"multiSelect":true}]`
	p, err := ParseQuestionPayload([]byte(raw))
	require.NoError(t, err)
	qs := p.Questions()
	require.Len(t, qs, 2)
	assert.True(t, qs[1].MultiSelect)
	assert.Equal(t, "A", qs[1].Options[0].Label)

	qs[0].Options[0].Label = "mutated"
	assert.Equal(t, "Redis", p.Questions()[0].Options[0].Label, "Questions returns a copy")

	_, err = ParseQuestionPayload([]byte(`{"questions": 3}`))
	assert.ErrorIs(t, err, ErrInvalidQuestionPayload)
	_, err = ParseQuestionPayload([]byte(`[]`))
	assert.ErrorIs(t, err, ErrInvalidQuestionPayload)
}

func TestQuestionPayload_JSONIsCanonicalAndRoundTrips(t *testing.T) {
	a, b := mustPayload(t, oneQuestion()), mustPayload(t, oneQuestion())
	ja, err := a.JSON()
	require.NoError(t, err)
	jb, err := b.JSON()
	require.NoError(t, err)
	assert.Equal(t, ja, jb)

	back, err := ParseQuestionPayload([]byte(ja))
	require.NoError(t, err)
	jBack, err := back.JSON()
	require.NoError(t, err)
	assert.Equal(t, ja, jBack)

	var zero QuestionPayload
	empty, err := zero.JSON()
	require.NoError(t, err)
	assert.Empty(t, empty)

	var embedded struct{ P QuestionPayload }
	require.NoError(t, json.Unmarshal([]byte(`{"P":`+ja+`}`), &embedded))
	assert.False(t, embedded.P.IsZero())
	assert.Error(t, json.Unmarshal([]byte(`{"P":[]}`), &embedded))
}

func hookParams(t *testing.T) GuidanceRequestParams {
	t.Helper()
	return GuidanceRequestParams{
		Scope: RequestScopeSession, SessionUUID: "asker-1", Source: QuestionSourceHook, AskerSessionUUID: "asker-1",
		Payload: mustPayload(t, oneQuestion()), HookCallID: "toolu_01",
	}
}

func peerParams() GuidanceRequestParams {
	return GuidanceRequestParams{
		Scope: RequestScopeSession, SessionUUID: "asker-1", Source: QuestionSourcePeer, AskerSessionUUID: "asker-1",
		AnswererKind: AnswererKindSession, AnswererSessionUUID: "peer-2", QuestionText: "How do I run the linter?",
		QuestionType: QuestionTypeShortAnswer,
	}
}

func TestNewGuidanceRequest_FillsDefaultsAndDerivesQuestionOneForOldReaders(t *testing.T) {
	legacy, err := NewGuidanceRequest(GuidanceRequestParams{
		Scope: RequestScopeBacklogItem, ItemID: "i", QuestionText: "Merge?", QuestionType: QuestionTypeYesNo,
	})
	require.NoError(t, err)
	assert.Equal(t, QuestionSourceAgentMCP, legacy.Params().Source)
	assert.Equal(t, AnswererKindHuman, legacy.Params().AnswererKind)

	hook, err := NewGuidanceRequest(hookParams(t))
	require.NoError(t, err)
	got := hook.Params()
	assert.Equal(t, "Which cache?", got.QuestionText)
	assert.Equal(t, QuestionTypeMultipleChoice, got.QuestionType)
	assert.JSONEq(t, `["Redis","None"]`, got.Options)
}

func TestNewGuidanceRequest_RejectsIllegalCombinations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(p *GuidanceRequestParams)
		base   func(t *testing.T) GuidanceRequestParams
	}{
		{"peer with a human answerer", func(p *GuidanceRequestParams) { p.AnswererKind = AnswererKindHuman; p.AnswererSessionUUID = "" }, func(*testing.T) GuidanceRequestParams { return peerParams() }},
		{"session answerer without a uuid", func(p *GuidanceRequestParams) { p.AnswererSessionUUID = "" }, func(*testing.T) GuidanceRequestParams { return peerParams() }},
		{"human answerer with a session uuid", func(p *GuidanceRequestParams) { p.AnswererSessionUUID = "x" }, hookParams},
		{"read_only route that was sent", func(p *GuidanceRequestParams) {
			p.DeliveryRoute = DeliveryRouteReadOnly
			p.DeliveryState = DeliveryStateSent
		}, hookParams},
		{"state without route", func(p *GuidanceRequestParams) { p.DeliveryState = DeliveryStatePending }, hookParams},
		{"asker is not the scope key", func(p *GuidanceRequestParams) { p.AskerSessionUUID = "someone-else" }, hookParams},
		{"hook without a payload", func(p *GuidanceRequestParams) {
			p.Payload = QuestionPayload{}
			p.QuestionText = "q"
			p.QuestionType = QuestionTypeYesNo
		}, hookParams},
		{"hook without an asker", func(p *GuidanceRequestParams) { p.AskerSessionUUID = "" }, hookParams},
		{"hook in backlog-item scope", func(p *GuidanceRequestParams) { p.Scope = RequestScopeBacklogItem; p.ItemID = "i" }, hookParams},
		{"agent_mcp with a hook call id", func(p *GuidanceRequestParams) { p.Source = QuestionSourceAgentMCP }, hookParams},
		{"unknown source", func(p *GuidanceRequestParams) { p.Source = "terminal_detect" }, hookParams},
		{"unknown scope", func(p *GuidanceRequestParams) { p.Scope = "galaxy" }, hookParams},
		{"empty question", func(p *GuidanceRequestParams) { p.QuestionText = " " }, func(*testing.T) GuidanceRequestParams { return peerParams() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.base(t)
			tt.mutate(&p)
			_, err := NewGuidanceRequest(p)
			assert.ErrorIs(t, err, ErrInvalidGuidanceRequest)
		})
	}
}

func TestNewGuidanceRequest_AcceptsLegalMirrorsAndPeers(t *testing.T) {
	p := hookParams(t)
	p.DeliveryRoute, p.DeliveryState = DeliveryRouteLiveKeystroke, DeliveryStatePending
	_, err := NewGuidanceRequest(p)
	require.NoError(t, err)

	q := peerParams()
	q.DeliveryRoute, q.DeliveryState = DeliveryRouteReadOnly, DeliveryStateSavedForLater
	_, err = NewGuidanceRequest(q)
	require.NoError(t, err)
}
