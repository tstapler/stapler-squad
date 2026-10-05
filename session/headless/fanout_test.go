package headless

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assistantLine(id string, toolNames ...string) string {
	content := `{"type":"text","text":"working"}`
	for _, n := range toolNames {
		content += fmt.Sprintf(`,{"type":"tool_use","id":"tu_%s_%s","name":%q,"input":{}}`, id, n, n)
	}
	return fmt.Sprintf(`{"type":"assistant","message":{"id":%q,"content":[%s]}}`, id, content)
}

func TestFanoutCounter_CountsDistinctTurnsAndSubagents(t *testing.T) {
	t.Parallel()
	c := newFanoutCounter(FanoutLimits{})

	for _, line := range []string{
		`{"type":"system","subtype":"init"}`,
		assistantLine("m1", "Agent", "Read"),
		assistantLine("m1", "Agent"), // same message id split across lines: one turn, but a second launch block
		assistantLine("m2", "Task"),  // legacy subagent tool name
		assistantLine("m3", "Bash"),
		`{"type":"user","message":{"content":[{"type":"tool_result"}]}}`,
		`not json but mentions "assistant"`,
	} {
		assert.Nil(t, c.observe(line))
	}
	assert.Equal(t, 3, c.turns)
	assert.Equal(t, 3, c.subagents)
}

func TestFanoutCounter_ExceededPerLimit(t *testing.T) {
	t.Parallel()
	turns := newFanoutCounter(FanoutLimits{MaxTurns: 2})
	assert.Nil(t, turns.observe(assistantLine("a")))
	assert.Nil(t, turns.observe(assistantLine("b")))
	tripped := turns.observe(assistantLine("c"))
	require.NotNil(t, tripped)
	assert.ErrorIs(t, tripped, ErrFanoutCeilingExceeded)
	assert.Equal(t, 3, tripped.Turns)

	exact := newFanoutCounter(FanoutLimits{MaxTurns: 2})
	assert.Nil(t, exact.observe(assistantLine("a")))
	assert.Nil(t, exact.observe(assistantLine("b")), "exactly MaxTurns must not trip")

	subs := newFanoutCounter(FanoutLimits{MaxSubagents: 1})
	assert.Nil(t, subs.observe(assistantLine("a", "Agent")))
	err := subs.observe(assistantLine("b", "Agent"))
	require.NotNil(t, err)
	assert.ErrorIs(t, err, ErrFanoutCeilingExceeded)
	assert.Equal(t, 2, err.Subagents)
}

// replay882Lines reproduces the shape of the #882 incident (1,094 turns, 262
// subagent launches in one call) followed by a normal terminal result.
func replay882Lines() []string {
	var lines []string
	subagents := 0
	for i := 0; i < 1094; i++ {
		if subagents < 262 && i%4 == 0 {
			subagents++
			lines = append(lines, assistantLine(fmt.Sprintf("m%d", i), "Agent"))
		} else {
			lines = append(lines, assistantLine(fmt.Sprintf("m%d", i), "Monitor"))
		}
	}
	return append(lines, firstCallJSON("sess-882", "done"))
}

// TestPool_FirstCall_FanoutCeiling_StopsIncidentShapeBeforeBudget replays the
// #882 stream with the sdd-triage defaults and asserts the call is killed with
// the distinct ceiling error long before the 3h triageCallBudget.
func TestPool_FirstCall_FanoutCeiling_StopsIncidentShapeBeforeBudget(t *testing.T) {
	t.Parallel()
	stopped := make(chan struct{})
	pool := NewPoolWithRunner(PoolConfig{}, &idlingLinesRunner{lines: replay882Lines(), stopped: stopped})
	pool.fanout = FanoutLimits{MaxTurns: 600, MaxSubagents: 120}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour) // triageCallBudget
	defer cancel()

	partial, err := pool.CallBlocking(ctx, "f1", "sys", "prompt", CallOptions{}, DiscardCost)

	require.Error(t, err)
	assert.Contains(t, partial, `"type":"assistant"`, "partial transcript must survive the abort for failure capture")
	assert.ErrorIs(t, err, ErrFanoutCeilingExceeded)
	assert.NotErrorIs(t, err, context.DeadlineExceeded)
	var ceiling *FanoutCeilingError
	require.ErrorAs(t, err, &ceiling)
	assert.Equal(t, 121, ceiling.Subagents, "stops at the first launch past MaxSubagents")
	assert.Less(t, ceiling.Turns, 1094, "stops before the incident's turn count")
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("expected the subprocess to be stopped")
	}
}

// TestPool_FirstCall_NoFanoutLimits_Unaffected pins AC4: with no ceiling
// configured (default triage), the same stream completes normally.
func TestPool_FirstCall_NoFanoutLimits_Unaffected(t *testing.T) {
	t.Parallel()
	pool := NewPoolWithRunner(PoolConfig{}, &idlingLinesRunner{lines: replay882Lines()})

	result, err := pool.CallBlocking(context.Background(), "f1", "sys", "prompt", CallOptions{}, DiscardCost)
	require.NoError(t, err)
	assert.Equal(t, "done", result)
}
