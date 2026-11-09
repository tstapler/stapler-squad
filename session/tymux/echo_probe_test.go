package tymux

import (
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
	assert.Equal(t, uint64(2), transit.Total, "transit is measured on every stamped frame")
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
