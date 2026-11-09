package tymux

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"

	"github.com/tstapler/stapler-squad/internal/termlat"
)

func statFor(rec *termlat.Recorder, stage string) (termlat.Stats, bool) {
	for _, s := range rec.Report() {
		if s.Stage == stage && s.Backend == "tymux" {
			return s, true
		}
	}
	return termlat.Stats{}, false
}

func probeAt(rec *termlat.Recorder, now time.Time) *echoProbe {
	return &echoProbe{rec: rec, now: func() time.Time { return now }}
}

func TestEchoProbe_should_SplitEchoIntoDaemonAndTransit(t *testing.T) {
	t.Parallel()
	rec := termlat.New(nil)
	base := time.Unix(1_700_000_000, 0)
	p := probeAt(rec, base.Add(30*time.Millisecond))
	p.noteInput(base)

	// The daemon stamped the frame 25ms after the keystroke; squad received it 5ms later.
	p.noteOutput(uint64(base.Add(25 * time.Millisecond).UnixMicro()))

	daemon, ok := statFor(rec, termlat.StageEchoDaemon)
	require.True(t, ok)
	assert.InDelta(t, 25.0, daemon.Max, 0.01)
	transit, ok := statFor(rec, termlat.StageOutputTransit)
	require.True(t, ok)
	assert.InDelta(t, 5.0, transit.Max, 0.01)
}

func TestEchoProbe_should_AttributeOnlyTheFirstOutputAfterAKeystroke(t *testing.T) {
	t.Parallel()
	rec := termlat.New(nil)
	base := time.Unix(1_700_000_000, 0)
	p := probeAt(rec, base.Add(10*time.Millisecond))
	p.noteInput(base)

	stamp := uint64(base.Add(5 * time.Millisecond).UnixMicro())
	p.noteOutput(stamp)
	p.noteOutput(stamp)

	daemon, _ := statFor(rec, termlat.StageEchoDaemon)
	assert.Equal(t, uint64(1), daemon.Total, "the second frame is not an echo of the same keystroke")
	transit, _ := statFor(rec, termlat.StageOutputTransit)
	assert.Equal(t, uint64(1), transit.Total, "transit is measured on the echo frame only, so bulk output costs one atomic load")
}

func TestEchoProbe_should_IgnoreFramesTheDaemonDidNotStamp(t *testing.T) {
	t.Parallel()
	rec := termlat.New(nil)
	base := time.Unix(1_700_000_000, 0)
	p := probeAt(rec, base)
	p.noteInput(base)

	p.noteOutput(0) // daemon without capability output.sent_at

	assert.Empty(t, rec.Report())
	assert.NotZero(t, p.lastInputNs.Load(), "an unstamped frame must not consume the keystroke")
}

func TestEchoProbe_should_LeaveTheKeystrokePendingWhenAStaleFrameArrives(t *testing.T) {
	t.Parallel()
	rec := termlat.New(nil)
	base := time.Unix(1_700_000_000, 0)
	p := probeAt(rec, base.Add(40*time.Millisecond))
	p.noteInput(base)

	// A frame the daemon stamped before the keystroke was already in flight.
	p.noteOutput(uint64(base.Add(-3 * time.Millisecond).UnixMicro()))
	_, recorded := statFor(rec, termlat.StageEchoDaemon)
	require.False(t, recorded, "a frame older than the keystroke is not its echo")

	p.noteOutput(uint64(base.Add(20 * time.Millisecond).UnixMicro()))
	daemon, ok := statFor(rec, termlat.StageEchoDaemon)
	require.True(t, ok, "the real echo must still be attributed")
	assert.InDelta(t, 20.0, daemon.Max, 0.01)
}

func TestSendInputViaControlMode_should_StartTheEchoClockButSendKeysShouldNot(t *testing.T) {
	// sessionWith uses t.Setenv, so this test cannot be parallel.
	sess, _ := sessionWith(t)

	_, err := sess.SendKeys("agent prompt")
	require.NoError(t, err)
	assert.Zero(t, sess.echo.lastInputNs.Load(), "programmatic input is not a user keystroke")

	require.NoError(t, sess.SendInputViaControlMode(context.Background(), []byte("k")))
	assert.NotZero(t, sess.echo.lastInputNs.Load())
}

func TestHandleAttachEvent_should_FeedTheEchoProbeFromLegacyOutput(t *testing.T) {
	t.Parallel()
	rec := termlat.New(nil)
	base := time.Unix(1_700_000_000, 0)
	s := &tymuxGRPCSession{fanout: NewClientFanout(), echo: *probeAt(rec, base.Add(8*time.Millisecond))}
	s.echo.noteInput(base)

	s.handleAttachEvent(&v1.AttachEvent{
		SentAtUnixMicros: uint64(base.Add(4 * time.Millisecond).UnixMicro()),
		Payload:          &v1.AttachEvent_Output{Output: []byte("a")},
	})

	_, ok := statFor(rec, termlat.StageEchoDaemon)
	assert.True(t, ok)
}

func TestEchoProbe_should_DropImplausibleSamples(t *testing.T) {
	t.Parallel()
	base := time.Unix(1_700_000_000, 0)
	cases := map[string]struct {
		recv  time.Time
		stamp time.Time
	}{
		"stamp after receive (clock skew)": {recv: base, stamp: base.Add(time.Second)},
		"echo gap beyond bound":            {recv: base.Add(10 * time.Second), stamp: base.Add(9 * time.Second)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := termlat.New(nil)
			p := probeAt(rec, tc.recv)
			p.noteInput(base.Add(-time.Millisecond))
			p.noteOutput(uint64(tc.stamp.UnixMicro()))
			if tc.stamp.After(tc.recv) {
				_, ok := statFor(rec, termlat.StageOutputTransit)
				assert.False(t, ok, "negative transit")
			}
			_, ok := statFor(rec, termlat.StageEchoDaemon)
			assert.False(t, ok, name)
		})
	}
}

func TestHandleAttachEvent_should_FeedTheEchoProbeFromOutputChunks(t *testing.T) {
	t.Parallel()
	rec := termlat.New(nil)
	base := time.Unix(1_700_000_000, 0)
	s := &tymuxGRPCSession{fanout: NewClientFanout(), echo: *probeAt(rec, base.Add(8*time.Millisecond))}
	s.echo.noteInput(base)

	s.handleAttachEvent(&v1.AttachEvent{
		SentAtUnixMicros: uint64(base.Add(6 * time.Millisecond).UnixMicro()),
		Payload:          &v1.AttachEvent_OutputChunk{OutputChunk: &v1.OutputChunk{Seq: 1, Data: []byte("a")}},
	})

	daemon, ok := statFor(rec, termlat.StageEchoDaemon)
	require.True(t, ok)
	assert.InDelta(t, 6.0, daemon.Max, 0.01)
}
