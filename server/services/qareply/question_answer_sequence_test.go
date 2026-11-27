package qareply

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/server/services/questionkit"
	"github.com/tstapler/stapler-squad/session/domain"
)

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
