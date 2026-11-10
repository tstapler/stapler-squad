package services

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type inputLogLine struct {
	msg  string
	args []any
}

// inputTestRig is a metrics instance with an injectable clock and a captured logger.
type inputTestRig struct {
	m     *terminalInputMetrics
	mu    sync.Mutex
	now   time.Time
	lines []inputLogLine
}

func newInputTestRig() *inputTestRig {
	r := &inputTestRig{now: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)}
	r.m = newTerminalInputMetrics(func() time.Time { r.mu.Lock(); defer r.mu.Unlock(); return r.now },
		func(msg string, args ...any) {
			r.mu.Lock()
			r.lines = append(r.lines, inputLogLine{msg, args})
			r.mu.Unlock()
		})
	return r
}

func (r *inputTestRig) advance(d time.Duration) { r.mu.Lock(); r.now = r.now.Add(d); r.mu.Unlock() }

func (r *inputTestRig) logged(msg string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, l := range r.lines {
		if strings.Contains(l.msg, msg) {
			n++
		}
	}
	return n
}

func TestClassifyInput_ShouldReturnTextEscapeOther_When_LsCRArrowUpCtrlC(t *testing.T) {
	t.Parallel()
	assert.Equal(t, InputClassText, classifyInput([]byte("ls\r")))
	assert.Equal(t, InputClassText, classifyInput([]byte("café 世界\t\n\x7f")))
	assert.Equal(t, InputClassEscape, classifyInput([]byte("\x1b[A")))
	assert.Equal(t, InputClassOther, classifyInput([]byte{0x03}))
	assert.Equal(t, InputClassOther, classifyInput([]byte{0xff, 0xfe}))
	assert.Equal(t, InputClassOther, classifyInput(nil))
}

func TestClassifyInput_ShouldScanAtMost64Bytes_When_LongFrameWithLateControlByte(t *testing.T) {
	t.Parallel()
	late := append([]byte(strings.Repeat("a", terminalInputClassifyLimit)), 0x03)
	assert.Equal(t, InputClassText, classifyInput(late), "a control byte past the scan limit is not examined")
	early := append([]byte(strings.Repeat("a", terminalInputClassifyLimit-1)), 0x03)
	assert.Equal(t, InputClassOther, classifyInput(early))
	// A multi-byte rune cut by the limit must not demote the frame.
	split := []byte(strings.Repeat("a", terminalInputClassifyLimit-1) + "é" + "tail")
	assert.Equal(t, InputClassText, classifyInput(split))
}

func TestRecordTerminalInput_ShouldIncrementReceivedForwardedAndBytes_When_RemoteTextFrameOnCM(t *testing.T) {
	t.Parallel()
	r := newInputTestRig()
	src := r.m.source(PathCM, ListenerRemote, "s-cm")
	data := []byte("hello")

	r.m.record(src, data, StageReceived)
	r.m.record(src, data, StageForwarded)

	rf, rb := r.m.count(InputClassText, ListenerRemote, PathCM, StageReceived)
	ff, fb := r.m.count(InputClassText, ListenerRemote, PathCM, StageForwarded)
	assert.EqualValues(t, 1, rf)
	assert.EqualValues(t, 5, rb)
	assert.EqualValues(t, 1, ff)
	assert.EqualValues(t, 5, fb)
	lf, _ := r.m.count(InputClassText, ListenerLocal, PathCM, StageReceived)
	assert.Zero(t, lf, "other listener untouched")
}

func TestRecordTerminalInput_ShouldAddNothingToBytes_When_EmptyFrame(t *testing.T) {
	t.Parallel()
	r := newInputTestRig()
	r.m.record(r.m.source(PathShell, ListenerLocal, "s-empty"), nil, StageReceived)
	frames, bytes := r.m.count(InputClassOther, ListenerLocal, PathShell, StageReceived)
	assert.EqualValues(t, 1, frames)
	assert.Zero(t, bytes)
}

func TestRecordTerminalInput_ShouldBoundListenerLabel_When_UnknownValue(t *testing.T) {
	t.Parallel()
	r := newInputTestRig()
	r.m.record(r.m.source(PathStream, "attacker-text", "s"), []byte("x"), StageReceived)
	frames, _ := r.m.count(InputClassText, "anything-else", PathStream, StageReceived)
	assert.EqualValues(t, 1, frames, "unknown listeners share one bounded label value")
}

func TestRecordTerminalInput_ShouldUseOnlyKindListenerPathStageLabels_When_Recorded(t *testing.T) {
	t.Parallel()
	RecordTerminalInput(NewInputSource(PathCapture, ListenerRemote, "labels-check"), []byte("x"), StageForwarded)
	for _, name := range []string{"terminal_input_frames_total", "terminal_input_bytes_total"} {
		m := collectMetric(t, name)
		require.NotNil(t, m, name)
		sum, ok := m.Data.(metricdata.Sum[int64])
		require.True(t, ok)
		require.NotEmpty(t, sum.DataPoints)
		for _, dp := range sum.DataPoints {
			keys := map[string]bool{}
			for _, kv := range dp.Attributes.ToSlice() {
				keys[string(kv.Key)] = true
			}
			assert.Equal(t, map[string]bool{"kind": true, "listener": true, "path": true, "stage": true}, keys, name)
		}
	}
}

func TestTally_ShouldEvictOldestAndHoldSize256_When_300SessionsRecorded(t *testing.T) {
	t.Parallel()
	r := newInputTestRig()
	for i := 0; i < 300; i++ {
		r.m.record(r.m.source(PathCM, ListenerLocal, fmt.Sprintf("sess-%03d", i)), []byte("x"), StageReceived)
	}
	assert.Equal(t, 256, r.m.tally.size())
	ids := map[string]bool{}
	for _, row := range r.m.tally.snapshot() {
		ids[row.SessionID] = true
	}
	assert.False(t, ids["sess-000"], "oldest evicted")
	assert.False(t, ids["sess-043"])
	assert.True(t, ids["sess-044"])
	assert.True(t, ids["sess-299"])
}

func TestTally_ShouldCountPerSessionKindAndPath_When_ReceivedAndForwarded(t *testing.T) {
	t.Parallel()
	r := newInputTestRig()
	src := r.m.source(PathCM, ListenerLocal, "s-tally")
	r.m.record(src, []byte("ab"), StageReceived)
	r.m.record(src, []byte("ab"), StageForwarded)
	r.m.record(src, []byte("\x1b[A"), StageReceived)

	rows := r.m.tally.snapshot()
	require.Len(t, rows, 1)
	row := rows[0]
	assert.EqualValues(t, 2, row.Frames)
	assert.EqualValues(t, 5, row.Bytes)
	assert.Equal(t, map[string]int64{"text": 1, "escape": 1}, row.ByKind)
	assert.Equal(t, TerminalInputPathCounts{Received: 2, Forwarded: 1}, row.ByPath["cm"])
	assert.Equal(t, r.now.UTC(), row.LastAt)
}

func TestInputLog_ShouldEmitOneFirstInputLineAndZeroSummaries_When_100FramesIn10s(t *testing.T) {
	t.Parallel()
	r := newInputTestRig()
	src := r.m.source(PathCM, ListenerRemote, "s-log")
	for i := 0; i < 100; i++ {
		r.m.record(src, []byte("k"), StageReceived)
		r.advance(100 * time.Millisecond)
	}
	assert.Equal(t, 1, r.logged("first frame"))
	assert.Equal(t, 0, r.logged("summary"))
}

func TestInputLog_ShouldEmitOneSummary_When_61sElapsed(t *testing.T) {
	t.Parallel()
	r := newInputTestRig()
	src := r.m.source(PathCM, ListenerRemote, "s-log2")
	for i := 0; i < 100; i++ {
		r.m.record(src, []byte("k"), StageReceived)
	}
	r.advance(61 * time.Second)
	r.m.record(src, []byte("k"), StageReceived)
	r.m.record(src, []byte("k"), StageReceived)
	assert.Equal(t, 1, r.logged("first frame"))
	assert.Equal(t, 1, r.logged("summary"), "at most one summary per minute")
}

func TestInputLog_ShouldLogFirstInputPerStream_When_TwoStreamsOnOneSession(t *testing.T) {
	t.Parallel()
	r := newInputTestRig()
	r.m.record(r.m.source(PathCM, ListenerLocal, "same"), []byte("a"), StageReceived)
	r.m.record(r.m.source(PathShell, ListenerLocal, "same"), []byte("a"), StageReceived)
	assert.Equal(t, 2, r.logged("first frame"))
}

func TestInputLogging_ShouldContainNoPayload_When_FirstInputAndSummaryEmitted(t *testing.T) {
	t.Parallel()
	const secret = "hunter2-secret"
	r := newInputTestRig()
	src := r.m.source(PathCM, ListenerRemote, "s-secret")
	r.m.record(src, []byte(secret), StageReceived)
	r.advance(2 * time.Minute)
	r.m.record(src, []byte(secret), StageReceived)
	r.m.record(src, []byte(secret), StageForwarded)

	r.mu.Lock()
	defer r.mu.Unlock()
	require.Len(t, r.lines, 2)
	for _, l := range r.lines {
		assert.NotContains(t, l.msg, secret)
		var keys []string
		for i := 0; i < len(l.args); i += 2 {
			keys = append(keys, l.args[i].(string))
			assert.NotContains(t, fmt.Sprint(l.args[i+1]), secret)
		}
		assert.Equal(t, []string{"session", "path", "listener", "frames", "bytes"}, keys)
	}
	assert.NotContains(t, RenderTerminalInputText(r.m.tally.snapshot()), secret)
}

func TestRecordTerminalInput_ShouldRecordNothing_When_SourceIsNil(t *testing.T) {
	t.Parallel()
	assert.NotPanics(t, func() { RecordTerminalInput(nil, []byte("x"), StageReceived) })
}
