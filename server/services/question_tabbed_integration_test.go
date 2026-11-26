package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session/domain"
)

// The measured Spike S1c dialog as a hook sends it: Q1 single (Redis/Memcached/None), Q2 multi (A/B/C).
const tabbedAskBody = `{"tool_name":"AskUserQuestion","cwd":"/work","tool_input":{"questions":[` +
	`{"question":"Which cache?","header":"Cache","options":[{"label":"Redis","description":"r"},{"label":"Memcached","description":"m"},{"label":"None","description":"n"}],"multiSelect":false},` +
	`{"question":"Which letters?","header":"Letters","options":[{"label":"A","description":"a"},{"label":"B","description":"b"},{"label":"C","description":"c"}],"multiSelect":true}]}}`

func TestParseAskUserQuestion_should_FlagOnlyTheMeasuredTabbedShape(t *testing.T) {
	t.Parallel()
	q := func(header string, multi bool, labels ...string) map[string]interface{} {
		opts := make([]interface{}, len(labels))
		for i, l := range labels {
			opts[i] = map[string]interface{}{"label": l, "description": "d"}
		}
		return map[string]interface{}{"question": "Q " + header, "header": header, "options": opts, "multiSelect": multi}
	}
	ask := func(qs ...map[string]interface{}) questionkit.AskedQuestion {
		raw := make([]interface{}, len(qs))
		for i, x := range qs {
			raw[i] = x
		}
		return questionkit.ParseAskUserQuestion(map[string]interface{}{"questions": raw})
	}
	tests := []struct {
		name string
		got  questionkit.AskedQuestion
		want bool
	}{
		{"single then multi is the measured shape", ask(q("Cache", false, "Redis", "None"), q("Letters", true, "A", "B")), true},
		{"multi first was not measured", ask(q("Letters", true, "A", "B"), q("Cache", false, "Redis")), false},
		{"two single-selects were not measured", ask(q("Cache", false, "Redis"), q("Other", false, "A")), false},
		{"three questions were not measured", ask(q("Cache", false, "Redis"), q("Letters", true, "A"), q("More", true, "B")), false},
		{"a question without a header cannot be matched on the tab bar", ask(q("Cache", false, "Redis"), q("", true, "A")), false},
		{"a lone single-select stays on the digit path", ask(q("Cache", false, "Redis", "None")), false},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, tc.got.Tabbed, tc.name)
		assert.False(t, tc.got.Replyable && tc.got.Tabbed, "a tabbed ask never takes the single-digit route: %s", tc.name)
	}
}

func TestRegisterQuestion_should_MirrorATabbedAskAsALiveKeystrokeRowWithoutThePendingStore(t *testing.T) {
	q := newQuestionHarness(t)
	storage := svctest.CreateTestStorage(t)
	q.h.SetGuidanceMirror(newRealStoreMirror(storage, q.store, &mirrorCounts{}))

	md := q.post(tabbedAskBody, map[string]string{hookProofHeader: q.proofs.HeaderValue("sess-uuid")}).NotificationMetadata

	id := md[questionkit.MetaGuidanceRequestID]
	require.NotEmpty(t, id)
	assert.Equal(t, id, md[questionkit.MetaQuestionID], "the row id is the question id the card addresses")
	assert.Equal(t, string(questionkit.QuestionShapeMulti), md[metaQuestionShape])
	assert.Zero(t, q.store.Len(), "the digit store never holds a tabbed dialog")
	row := guidanceRowOf(t, storage, id)
	assert.Equal(t, domain.DeliveryRouteLiveKeystroke, row.DeliveryRoute)
	qs, ok := questionkit.TabbedFromRow(row)
	require.True(t, ok)
	assert.Equal(t, []string{"A", "B", "C"}, qs[1].Labels)
	_, single := questionkit.DialogFromRow(row)
	assert.False(t, single, "DialogMatch cannot describe a tabbed dialog")
}

func TestRegisterQuestion_should_KeepATabbedAskViewOnly_WhenTheSenderIsUnverified(t *testing.T) {
	q := newQuestionHarness(t)
	storage := svctest.CreateTestStorage(t)
	q.h.SetGuidanceMirror(newRealStoreMirror(storage, q.store, &mirrorCounts{}))

	md := q.post(tabbedAskBody, map[string]string{"X-CS-Session-ID": "other-uuid"}).NotificationMetadata

	assert.NotContains(t, md, questionkit.MetaQuestionID)
	row := guidanceRowOf(t, storage, md[questionkit.MetaGuidanceRequestID])
	assert.Empty(t, row.DeliveryRoute)
	_, ok := questionkit.TabbedFromRow(row)
	assert.False(t, ok)
}

func tabbedRowOf(t *testing.T) *domain.GuidanceRequestData {
	t.Helper()
	env := newRouterEnv(t)
	inst, _ := env.addTabbed("tabbed-row", nil)
	return env.row(env.twoQuestionRow(inst, false))
}

func TestJudgeRow_should_MatchATabbedRowByItsTabBarNotByDialogMatch(t *testing.T) {
	row := tabbedRowOf(t)
	frames := tabbedFrames(t)

	for _, i := range []int{0, 1, 4, 7, 8} { // question 1, question 2, toggled, submit focused, review
		assert.Equal(t, questionkit.VerdictMatch, questionkit.JudgeRow(row, frames[i], nil), "frame %d", i)
	}
	assert.Equal(t, questionkit.VerdictGone, questionkit.JudgeRow(row, frames[9], nil), "the composer is back")
	assert.Equal(t, questionkit.VerdictReplaced, questionkit.JudgeRow(row, replyDialogFixture(t, "question_single_select_3opts.txt"), nil), "another dialog is up")
	assert.Equal(t, questionkit.VerdictIndeterminate, questionkit.JudgeRow(row, frames[0], assert.AnError))
}

func TestHeadersMatch_should_RejectATabbedFrameOfAnotherQuestionSet(t *testing.T) {
	qs, ok := questionkit.TabbedFromRow(tabbedRowOf(t))
	require.True(t, ok)
	own := questionkit.TabbedHeadersMatch(qs)
	frame := replyDialogFixture(t, "claude_auq_twoq_step1_single_select.txt")
	assert.True(t, own(frame))
	qs[1].Header = "Colours"
	assert.False(t, questionkit.TabbedHeadersMatch(qs)(frame))
	assert.False(t, own(replyDialogFixture(t, "question_single_select_3opts.txt")))
}

func TestSequenceQuestions_should_RefuseARowThatIsNotAReplyableHookRow(t *testing.T) {
	env := newRouterEnv(t)
	inst, _ := env.addTabbed("agent-twoq", tabbedFrames(t))
	_, ok := env.router.sequenceQuestions(context.Background(), env.twoQuestionRow(inst, false))
	assert.True(t, ok)

	row := *env.row(env.twoQuestionRow(inst, false))
	row.Source = domain.QuestionSourceAgentMCP
	_, ok = questionkit.TabbedFromRow(&row)
	assert.False(t, ok, "an agent_mcp row has no dialog on the pane to type into")

	row = *env.row(env.twoQuestionRow(inst, false))
	row.DeliveryRoute = ""
	_, ok = questionkit.TabbedFromRow(&row)
	assert.False(t, ok, "a view-only hook row is never typed into")
}

func TestAckDetector_should_JudgeATabbedAnswerByItsTabBar_When_theDialogStaysOrLeaves(t *testing.T) {
	qs, ok := questionkit.TabbedFromRow(tabbedRowOf(t))
	require.True(t, ok)
	frames := tabbedFrames(t)
	sentTabbed := func(e *ackEnv) {
		e.det.Sent(questionkit.SentDelivery{RowID: e.rowID, SessionUUID: "s1", Route: domain.DeliveryRouteLiveKeystroke, Matches: questionkit.TabbedHeadersMatch(qs)})
	}

	stuck := newAckEnv(t)
	stuck.capture = frames[8] // the review screen never closed
	sentTabbed(stuck)
	stuck.tick(ackDecideAfter)
	assert.Equal(t, []domain.DeliveryState{domain.DeliveryStateFailed}, stuck.store.states(), "own tabbed frame still up is delivery_failed(still_showing)")

	gone := newAckEnv(t)
	gone.capture = frames[9]
	sentTabbed(gone)
	gone.tick(ackDecideAfter)
	assert.Equal(t, []domain.DeliveryState{domain.DeliveryStateAcknowledged}, gone.store.states())
}

func TestAckDetector_should_RestoreATabbedSentRowWithItsTabBarMatcher(t *testing.T) {
	e := newAckEnv(t)
	row := tabbedRowOf(t)
	e.store.sent = []*domain.GuidanceRequestData{row}
	e.capture = tabbedFrames(t)[8]

	e.det.Restore(context.Background())
	e.tick(ackDecideAfter)

	assert.Equal(t, []domain.DeliveryState{domain.DeliveryStateFailed}, e.store.states())
}
