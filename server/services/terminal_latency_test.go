package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/internal/termlat"
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
	sl.setListener(ListenerRemote)
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
	sl.setListener(ListenerLocal)
	before := latencyTotal(termlat.StageEchoServer, sl.labels())

	now := time.Now()
	sl.noteInput(now)
	sl.noteOutputWritten(now.Add(maxEchoGap + time.Second))
	assert.Equal(t, before, latencyTotal(termlat.StageEchoServer, sl.labels()))
}

func TestStreamLatency_should_DefaultAnUnsetBackendToTmux(t *testing.T) {
	t.Parallel()
	var sl streamLatency
	sl.setBackend("")
	assert.Equal(t, "tmux", sl.labels().Backend)
}
