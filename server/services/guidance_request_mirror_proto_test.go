package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestGuidanceRequestToProto_should_RoundTripMirrorFields_When_MarshalledAndParsed(t *testing.T) {
	payload, err := domain.NewQuestionPayload([]domain.Question{
		{Text: "Which cache?", Header: "Cache", Options: []domain.Option{{Label: "Redis", Description: "in-memory"}, {Label: "None"}}},
		{Text: "Which letters?", Header: "Letters", MultiSelect: true, Options: []domain.Option{{Label: "A"}, {Label: "B"}}},
	})
	require.NoError(t, err)
	questionsJSON, err := payload.JSON()
	require.NoError(t, err)
	expires := time.Now().Add(domain.PeerHelpTTL).Truncate(time.Millisecond)
	in := &session.GuidanceRequestData{
		ID: "11111111-1111-4111-8111-111111111111", Scope: domain.RequestScopeSession, SessionUUID: "asker",
		QuestionText: "Which cache?", QuestionType: domain.QuestionTypeMultipleChoice, CreatedAt: time.Now(),
		Source: domain.QuestionSourcePeer, AskerSessionUUID: "asker", AnswererKind: domain.AnswererKindSession,
		AnswererSessionUUID: "peer", MultiSelect: true, QuestionsJSON: questionsJSON,
		DeliveryRoute: domain.DeliveryRouteLiveKeystroke, DeliveryState: domain.DeliveryStateSent, DeliveryAttempts: 2,
		HookCallID: "toolu_01", ExpiresAt: &expires,
	}

	wire, err := proto.Marshal(guidanceRequestToProto(in))
	require.NoError(t, err)
	var out sessionv1.GuidanceRequest
	require.NoError(t, proto.Unmarshal(wire, &out))

	assert.Equal(t, "peer", out.Source)
	assert.Equal(t, "asker", out.AskerSessionUuid)
	assert.Equal(t, "session", out.AnswererKind)
	assert.Equal(t, "peer", out.AnswererSessionUuid)
	assert.True(t, out.MultiSelect)
	assert.Equal(t, "live_keystroke", out.DeliveryRoute)
	assert.Equal(t, "sent", out.DeliveryState)
	assert.EqualValues(t, 2, out.DeliveryAttempts)
	assert.Equal(t, "toolu_01", out.HookCallId)
	assert.True(t, expires.Equal(out.ExpiresAt.AsTime()))
	require.Len(t, out.Questions, 2)
	assert.Equal(t, "in-memory", out.Questions[0].Options[0].Description)
	assert.True(t, out.Questions[1].MultiSelect)
	assert.Equal(t, "B", out.Questions[1].Options[1].Label)
}

func TestGuidanceRequestToProto_should_ReportLegacyDefaults_When_RowPredatesMirrorFields(t *testing.T) {
	out := guidanceRequestToProto(&session.GuidanceRequestData{
		ID: "id", Scope: domain.RequestScopeStandalone, QuestionText: "q", QuestionType: domain.QuestionTypeYesNo,
		CreatedAt: time.Now(), Source: domain.QuestionSourceAgentMCP, AnswererKind: domain.AnswererKindHuman,
	})
	assert.Equal(t, "agent_mcp", out.Source)
	assert.Equal(t, "human", out.AnswererKind)
	assert.Empty(t, out.Questions)
	assert.Nil(t, out.ExpiresAt)
}

func TestGuidanceRequestToProto_should_SendNoQuestions_When_QuestionsJSONIsCorrupt(t *testing.T) {
	out := guidanceRequestToProto(&session.GuidanceRequestData{
		ID: "id", Scope: domain.RequestScopeSession, QuestionText: "q", QuestionType: domain.QuestionTypeYesNo,
		CreatedAt: time.Now(), QuestionsJSON: "{not json",
	})
	assert.Empty(t, out.Questions)
	assert.Equal(t, "q", out.QuestionText)
}
