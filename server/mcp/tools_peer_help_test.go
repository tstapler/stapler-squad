package mcp

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func askArgs(peer string) map[string]any {
	return map[string]any{"peer": peer, "question": "How do I run the linter?"}
}

func (r *toolRig) openPeerRows(asker string) []*session.GuidanceRequestData {
	r.t.Helper()
	rows, err := r.storage.ListOpenGuidanceRequestsByAsker(context.Background(), asker, domain.QuestionSourcePeer)
	require.NoError(r.t, err)
	return rows
}

func TestRequestHelp_should_CreatePeerRow_ArmAskerWake_And_PromptThePeer(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	r.owners.facts["A"], r.owners.facts["B"] = liveOwner(), liveOwner()

	m := r.call(as("A"), toolRequestHelp, askArgs("B"))

	require.True(t, m["success"].(bool), "%v", m)
	rows := r.openPeerRows("A")
	require.Len(t, rows, 1)
	row := rows[0]
	assert.Equal(t, "A", row.SessionUUID, "scope_key is the asker")
	assert.Equal(t, domain.QuestionSourcePeer, row.Source)
	assert.Equal(t, "B", row.AnswererSessionUUID)
	assert.Equal(t, row.ID, m["request_id"])
	assert.Equal(t, "sent", m["delivery"])

	calls := r.backend.scheduleCalls()
	require.Len(t, calls, 2)
	assert.Equal(t, "A", calls[0].Owner)
	assert.Equal(t, domain.GuidanceAnsweredSpec{GuidanceRequestID: row.ID}, calls[0].Spec, "the asker's implicit answer wake")
	assert.Equal(t, "B", calls[1].Owner)
	assert.Contains(t, calls[1].Action.Payload, "from session A")
	assert.Contains(t, calls[1].Action.Payload, row.ID)
}

func TestRequestHelp_should_Reject_And_CreateNoRow_When_PeerCannotBeAsked(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		peer       string
		facts      wakes.OwnerFacts
		wantReason string
	}{
		{"self", "A", liveOwner(), rejectSelfTarget},
		{"unknown", "B", wakes.OwnerFacts{}, rejectPeerNotFound},
		{"archived", "B", wakes.OwnerFacts{Exists: true, Archived: true, WorkspaceKey: workspaceKeyA}, rejectPeerArchived},
		{"one-shot", "B", wakes.OwnerFacts{Exists: true, OneShot: true, WorkspaceKey: workspaceKeyA}, rejectPeerOneShot},
		{"item terminal", "B", wakes.OwnerFacts{Exists: true, ItemTerminal: true, WorkspaceKey: workspaceKeyA}, rejectPeerItemDone},
		{"other workspace", "B", wakes.OwnerFacts{Exists: true, WorkspaceKey: "github.com/o/other"}, rejectCrossWorkspace},
		{"workspace unknown", "B", wakes.OwnerFacts{Exists: true}, rejectCrossWorkspace},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := newToolRig(t)
			r.owners.facts["A"], r.owners.facts["B"] = liveOwner(), tt.facts

			m := r.call(as("A"), toolRequestHelp, askArgs(tt.peer))

			assert.Equal(t, CodeRejected, resultCode(m))
			assert.Equal(t, tt.wantReason, resultErrField(m, "reason"))
			assert.Empty(t, r.openPeerRows("A"), "no row")
			assert.Empty(t, r.backend.scheduleCalls(), "no wake")
			assert.EqualValues(t, 1, r.metrics.Value(wakes.MetricPeerHelp, "rejected"))
		})
	}
}

func TestRequestHelp_should_ReturnCapExceeded_When_AskerHasTooManyOpenRequests(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	r.owners.facts["A"], r.owners.facts["B"] = liveOwner(), liveOwner()
	limit := r.backend.limits.PeerOpenPerAsker

	for i := range limit {
		m := r.call(as("A"), toolRequestHelp, map[string]any{"peer": "B", "question": "question " + string(rune('a'+i))})
		require.True(t, m["success"].(bool), "%v", m)
	}
	m := r.call(as("A"), toolRequestHelp, map[string]any{"peer": "B", "question": "one too many"})

	assert.Equal(t, CodeCapExceeded, resultCode(m))
	assert.Equal(t, wakes.LimitPeerOpenPerAsker, resultErrField(m, "limit"))
	assert.Len(t, r.openPeerRows("A"), limit)
}

func TestRequestHelp_should_WithdrawTheRow_When_TheAnswerWakeIsRefused(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	r.owners.facts["A"], r.owners.facts["B"] = liveOwner(), liveOwner()
	r.backend.scheduleErr = func(int, wakes.ScheduleRequest) error {
		return &wakes.CapExceededError{Limit: wakes.LimitPendingPerOwner}
	}

	m := r.call(as("A"), toolRequestHelp, askArgs("B"))

	assert.Equal(t, CodeCapExceeded, resultCode(m))
	assert.Empty(t, r.openPeerRows("A"), "the request was withdrawn, not left dangling")
}

func TestRequestHelp_should_StillSucceed_And_ReportNotSent_When_PeerPromptCannotBeQueued(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	r.owners.facts["A"], r.owners.facts["B"] = liveOwner(), liveOwner()
	r.backend.scheduleErr = func(call int, _ wakes.ScheduleRequest) error {
		if call == 2 {
			return &wakes.CapExceededError{Limit: wakes.LimitPendingPerOwner}
		}
		return nil
	}

	m := r.call(as("A"), toolRequestHelp, askArgs("B"))

	require.True(t, m["success"].(bool), "%v", m)
	assert.Equal(t, "not_sent", m["delivery"])
	assert.Len(t, r.openPeerRows("A"), 1, "the request stands for the operator to see")
	assert.EqualValues(t, 1, r.metrics.Value(wakes.MetricPeerHelp, "not_sent"))
}

func TestRequestHelp_should_AliasCreateGuidanceRequest_When_PeerIsHuman(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)

	m := r.call(as("A"), toolRequestHelp, askArgs("human"))

	require.True(t, m["success"].(bool), "%v", m)
	rows, _, _, err := r.storage.ListPendingGuidanceRequests(context.Background(), domain.RequestScopeSession, "A", 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, domain.QuestionSourceAgentMCP, rows[0].Source, "the existing store and source, no new entity")
	calls := r.backend.scheduleCalls()
	require.Len(t, calls, 1, "the asker's answer wake only; there is no peer to prompt")
	assert.Equal(t, "A", calls[0].Owner)
}

func TestRequestHelp_should_ReturnFlagOff_When_ToolOrAnswerTriggerIsOff(t *testing.T) {
	t.Parallel()
	for _, scopes := range []map[string]bool{{"tool:request_help": false}, {"trigger:guidance_answered": false}} {
		r := newToolRig(t)
		r.cfg.FeatureFlagScopes = map[string]map[string]bool{"agent_wake_triggers": scopes}
		r.owners.facts["A"], r.owners.facts["B"] = liveOwner(), liveOwner()

		m := r.call(as("A"), toolRequestHelp, askArgs("B"))

		assert.Equal(t, CodeFlagOff, resultCode(m), scopes)
		assert.Empty(t, r.openPeerRows("A"))
	}
}

// askAndGetID has A ask B and returns the request id.
func askAndGetID(t *testing.T, r *toolRig) string {
	t.Helper()
	r.owners.facts["A"], r.owners.facts["B"] = liveOwner(), liveOwner()
	m := r.call(as("A"), toolRequestHelp, askArgs("B"))
	require.True(t, m["success"].(bool), "%v", m)
	return m["request_id"].(string)
}

func TestAnswerHelp_should_AcceptOnlyTheNamedAnswerer(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	id := askAndGetID(t, r)
	rowID := uuid.MustParse(id)

	forged := r.call(as("C"), toolAnswerHelp, map[string]any{"request_id": id, "answer": "use approach 2"})
	assert.Equal(t, CodeForbidden, resultCode(forged))
	byAsker := r.call(as("A"), toolAnswerHelp, map[string]any{"request_id": id, "answer": "self-answer"})
	assert.Equal(t, CodeForbidden, resultCode(byAsker), "the asker cannot answer its own request")
	row, err := r.storage.GetGuidanceRequest(context.Background(), rowID)
	require.NoError(t, err)
	assert.Equal(t, session.GuidanceRequestStatusPending, row.Status(), "the request is unchanged")
	assert.Empty(t, r.answeredIDs())

	ok := r.call(as("B"), toolAnswerHelp, map[string]any{"request_id": id, "answer": "use approach 2"})
	require.True(t, ok["success"].(bool), "%v", ok)
	assert.Equal(t, true, ok["applied"])
	row, err = r.storage.GetGuidanceRequest(context.Background(), rowID)
	require.NoError(t, err)
	assert.Equal(t, "use approach 2", row.Answer)
	assert.Equal(t, "B", row.AnswererSessionUUID)
	assert.Equal(t, []string{id}, r.answeredIDs(), "the answer listener fires the asker's wake")
	assert.EqualValues(t, 1, r.metrics.Value(wakes.MetricPeerHelp, "answered"))

	again := r.call(as("B"), toolAnswerHelp, map[string]any{"request_id": id, "answer": "changed my mind"})
	assert.Equal(t, false, again["applied"])
	assert.Equal(t, "use approach 2", again["answer"], "an answered request is not overwritten")
}

func TestAnswerHelp_should_NotAnswerRequestsAddressedToTheHuman(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	human := r.call(as("A"), toolRequestHelp, askArgs("human"))
	require.True(t, human["success"].(bool))

	m := r.call(as("B"), toolAnswerHelp, map[string]any{"request_id": human["request_id"], "answer": "I know"})

	assert.Equal(t, CodeForbidden, resultCode(m))
}

func TestAnswerHelp_should_ReportNotFound_And_ValidateArguments(t *testing.T) {
	t.Parallel()
	r := newToolRig(t)
	missing := r.call(as("B"), toolAnswerHelp, map[string]any{"request_id": uuid.NewString(), "answer": "x"})
	assert.Equal(t, ErrItemNotFound, resultCode(missing))
	empty := r.call(as("B"), toolAnswerHelp, map[string]any{"request_id": "x"})
	assert.Equal(t, ErrInvalidArgument, resultCode(empty))
}
