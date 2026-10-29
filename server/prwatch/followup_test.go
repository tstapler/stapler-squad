package prwatch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func followupLines(l *fakeLog) []LogLine {
	var out []LogLine
	for _, ln := range l.Lines() {
		if ln.Msg == "prwatch.steer_followup" {
			out = append(out, ln)
		}
	}
	return out
}

func kvMap(kv []any) map[string]any {
	m := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

func TestFollowup_should_LogAgentTurnObservedTrue_When_SessionLeavesIdleWithinWindow(t *testing.T) {
	sess, clock, log := &fakeSessions{busy: map[string]bool{}}, newFakeClock(), &fakeLog{}
	f := NewFollowupObserver(sess, clock, log, 120*time.Second)
	f.Track("s1", "o/r", 7)

	for tick := 1; tick <= 12; tick++ { // 10s ticks across the 120s window
		clock.Advance(10 * time.Second)
		if tick == 3 {
			sess.busy["s1"] = true
		}
		if tick == 4 {
			sess.busy["s1"] = false
		}
		f.Tick()
	}
	lines := followupLines(log)
	require.Len(t, lines, 1)
	assert.Equal(t, map[string]any{"session": "s1", "repo": "o/r", "pr": 7, "agent_turn_observed": true}, kvMap(lines[0].KV))
}

func TestFollowup_should_LogAgentTurnObservedFalse_When_SessionStaysIdleThroughWindow(t *testing.T) {
	sess, clock, log := &fakeSessions{busy: map[string]bool{}}, newFakeClock(), &fakeLog{}
	f := NewFollowupObserver(sess, clock, log, 120*time.Second)
	f.Track("s1", "o/r", 7)
	clock.Advance(60 * time.Second)
	f.Track("s1", "o/r", 7) // second steer inside the window: separate observation

	for tick := 1; tick <= 20; tick++ {
		clock.Advance(10 * time.Second)
		f.Tick()
	}
	lines := followupLines(log)
	require.Len(t, lines, 2, "each steer gets its own line")
	for _, ln := range lines {
		assert.Equal(t, false, kvMap(ln.KV)["agent_turn_observed"])
	}
	f.Tick()
	assert.Len(t, followupLines(log), 2, "a finished window logs once")
}

func TestFollowup_should_NotLogBeforeWindowEndsOrCarryActorOrText_When_Ticked(t *testing.T) {
	sess, clock, log := &fakeSessions{busy: map[string]bool{"s1": true}}, newFakeClock(), &fakeLog{}
	f := NewFollowupObserver(sess, clock, log, 120*time.Second)
	f.Track("s1", "o/r", 7)
	clock.Advance(30 * time.Second)
	f.Tick()
	assert.Empty(t, followupLines(log))
	clock.Advance(90 * time.Second)
	f.Tick()
	lines := followupLines(log)
	require.Len(t, lines, 1)
	assert.ElementsMatch(t, []any{"session", "repo", "pr", "agent_turn_observed"},
		[]any{lines[0].KV[0], lines[0].KV[2], lines[0].KV[4], lines[0].KV[6]})
}
