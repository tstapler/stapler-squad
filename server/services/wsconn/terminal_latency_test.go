package wsconn

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/internal/termlat"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

func latencyTotal(stage string, l termlat.Labels) uint64 {
	for _, s := range termlat.Default.Report() {
		if s.Stage == stage && s.Backend == l.Backend && s.Listener == l.Listener {
			return s.Total
		}
	}
	return 0
}

func TestStreamLatency_should_RecordEchoOnlyForTheFirstOutputAfterAKeystroke(t *testing.T) {
	t.Parallel()
	var sl streamLatency
	sl.setBackend(session.BackendNative)
	sl.setListener(services.ListenerRemote)
	l := sl.labels()
	assert.Equal(t, termlat.Labels{Backend: "native", Listener: "remote"}, l)
	before := latencyTotal(termlat.StageEchoServer, l)

	now := time.Now()
	sl.noteOutputWritten(now) // output with no keystroke pending is not an echo
	assert.Equal(t, before, latencyTotal(termlat.StageEchoServer, l))

	sl.noteInput(now)
	sl.noteOutputWritten(now.Add(12 * time.Millisecond))
	sl.noteOutputWritten(now.Add(20 * time.Millisecond)) // second output: the keystroke was already matched
	assert.Equal(t, before+1, latencyTotal(termlat.StageEchoServer, l))
}

func TestStreamLatency_should_IgnoreOutputTooLongAfterTheKeystroke(t *testing.T) {
	t.Parallel()
	var sl streamLatency
	sl.setBackend(session.BackendTymux)
	sl.setListener(services.ListenerLocal)
	before := latencyTotal(termlat.StageEchoServer, sl.labels())

	now := time.Now()
	sl.noteInput(now)
	sl.noteOutputWritten(now.Add(maxEchoGap + time.Second))
	assert.Equal(t, before, latencyTotal(termlat.StageEchoServer, sl.labels()))
}

func TestKeystrokeDelivery_should_BuildASlowCallTreeOnlyAboveTheThreshold(t *testing.T) {
	t.Parallel()
	start := time.Unix(1_700_000_000, 0)
	k := keystrokeDelivery{logPrefix: "[x]"}
	labels := termlat.Labels{Backend: "tymux", Listener: "local"}

	_, slow := k.slowCall(labels, "sess", start, start.Add(time.Millisecond), start.Add(10*time.Millisecond))
	assert.False(t, slow)

	c, slow := k.slowCall(labels, "sess", start, start.Add(5*time.Millisecond), start.Add(900*time.Millisecond))
	assert.True(t, slow)
	assert.Equal(t, "terminal.input", c.Name)
	assert.Equal(t, 900*time.Millisecond, c.Duration)
	require.Len(t, c.Stages, 2)
	assert.Equal(t, "terminal.input.bookkeep", c.Stages[0].Name)
	assert.Equal(t, "terminal.input.send", c.Stages[1].Name)
	assert.Equal(t, start.Add(5*time.Millisecond), c.Stages[1].Start)
}

func TestStreamLatency_should_DefaultAnUnsetBackendToTmux(t *testing.T) {
	t.Parallel()
	var sl streamLatency
	sl.setBackend("")
	assert.Equal(t, "tmux", sl.labels().Backend)
}
