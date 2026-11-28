package access

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestNewInputSourceFromContext_ShouldCountUnderTheStampedListener_WhenARecordIsOnTheContext(t *testing.T) {
	t.Parallel()
	var got context.Context
	h := WithRequestRecord(ListenerRemote, true)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r.Context() }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, ListenerRemote, NewInputSourceFromContext(got, PathStream, "ctx-remote").ListenerName())
	assert.Equal(t, "unknown", NewInputSourceFromContext(context.Background(), PathStream, "ctx-none").ListenerName(),
		"no stamped record is counted under the bounded unknown label")
}

var reportRuns atomic.Int64

func TestTerminalInputReport_ShouldListRecordedSessions_WhenInputWasRecordedOnTheDefaultMetrics(t *testing.T) {
	t.Parallel()
	// The default metrics live as long as the process, so each run (-count=N) needs its own row.
	session := fmt.Sprintf("report-session-%d", reportRuns.Add(1))
	RecordTerminalInput(NewInputSource(PathCapture, ListenerLocal, session), []byte("x"), StageReceived)

	var row *TerminalInputSession
	for _, r := range TerminalInputReport() {
		if r.SessionID == session {
			row = &r
		}
	}
	require.NotNil(t, row)
	assert.EqualValues(t, 1, row.Frames)
	assert.Equal(t, TerminalInputPathCounts{Received: 1}, row.ByPath["capture"])
}

func TestSessionNudgeGuard_ShouldUseTheInjectedClock_WhenBuiltWithClockOrSetClock(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_800_000_000, 0)
	clock := func() time.Time { return now }

	var set SessionNudgeGuard
	set.SetClock(clock)
	for name, g := range map[string]*SessionNudgeGuard{"constructor": NewSessionNudgeGuardWithClock(clock), "setter": &set} {
		release, out := g.TryBegin("s", "sig")
		require.Equal(t, GuardOK, out, name)
		release(true)
		_, out = g.TryBegin("s", "sig")
		assert.Equal(t, GuardDuplicate, out, name)

		now = now.Add(nudgeDuplicateWindow + time.Second)
		release, out = g.TryBegin("s", "sig")
		assert.Equal(t, GuardOK, out, name+": the injected clock moved past the duplicate window")
		release(true)
		now = time.Unix(1_800_000_000, 0)
	}
}

func TestSessionNudgeGuard_ShouldExpireTheWindowExactlyAtItsEdge_WhenTheClockReachesIt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		success bool
		window  time.Duration
		inside  GuardOutcome
	}{
		{"delivered: 60s duplicate window", true, nudgeDuplicateWindow, GuardDuplicate},
		{"failed: 10s cooldown", false, nudgeFailureCooldown, GuardCoolingDown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, 60*time.Second, nudgeDuplicateWindow, "the documented duplicate window")
			assert.Equal(t, 10*time.Second, nudgeFailureCooldown, "the documented failure cooldown")
			start := time.Unix(1_800_000_000, 0)
			now := start
			g := NewSessionNudgeGuardWithClock(func() time.Time { return now })
			release, out := g.TryBegin("s", "sig")
			require.Equal(t, GuardOK, out)
			release(tc.success)

			now = start.Add(tc.window - time.Nanosecond)
			_, out = g.TryBegin("s", "sig")
			assert.Equal(t, tc.inside, out, "one nanosecond inside the window")

			now = start.Add(tc.window)
			release, out = g.TryBegin("s", "sig")
			assert.Equal(t, GuardOK, out, "at the edge the window is over")
			release(true)
		})
	}
}

func TestClassifyInput_ShouldTreatC1ControlsAsOtherAndNoBreakSpaceAsText_WhenTheRuneIsMultiByte(t *testing.T) {
	t.Parallel()
	assert.Equal(t, InputClassText, classifyInput([]byte(" ")), "U+00A0 is the first printable Latin-1 rune")
	assert.Equal(t, InputClassOther, classifyInput([]byte("\u009f")), "U+009F is a C1 control")
	assert.Equal(t, InputClassText, classifyInput([]byte{0x7f}), "DEL is accepted as text input")
}

func TestRecordTerminalInput_ShouldCountNoBytes_WhenTheFrameIsEmpty(t *testing.T) {
	// Not parallel: it reads the process-wide metric reader.
	RecordTerminalInput(NewInputSource(PathCapture, "no-such-listener", "empty-frame"), nil, StageForwarded)

	const attrs = "kind=other listener=unknown path=capture stage=forwarded"
	for _, tc := range []struct {
		name    string
		wantAny bool
	}{{"terminal_input_frames_total", true}, {"terminal_input_bytes_total", false}} {
		m := collectInputMetric(t, tc.name)
		found := false
		if m != nil {
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			for _, dp := range sum.DataPoints {
				got := ""
				for _, kv := range dp.Attributes.ToSlice() {
					got += string(kv.Key) + "=" + kv.Value.AsString() + " "
				}
				if strings.TrimSpace(got) == attrs {
					found = true
				}
			}
		}
		assert.Equal(t, tc.wantAny, found, tc.name)
	}
}

// installFakeTmux points TMUX_BIN at a script that logs its argv and exits with
// the status in FAKE_TMUX_EXIT; it returns the log path.
func installFakeTmux(t *testing.T, exit string) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-tmux")
	body := "#!/bin/sh\necho \"$@\" >> \"$FAKE_TMUX_LOG\"\nexit $FAKE_TMUX_EXIT\n"
	require.NoError(t, os.WriteFile(script, []byte(body), 0o755))
	logPath := filepath.Join(dir, "argv.log")
	t.Setenv("TMUX_BIN", script)
	t.Setenv("FAKE_TMUX_LOG", logPath)
	t.Setenv("FAKE_TMUX_EXIT", exit)
	return logPath
}

func TestSendInputToTmuxWithRetry_ShouldRetryTwiceWithBackoff_WhenTmuxKeepsFailing(t *testing.T) {
	logPath := installFakeTmux(t, "1")

	start := time.Now()
	err := sendInputToTmuxWithRetry("ssq-seams", "ssq_a", []byte("k"))
	elapsed := time.Since(start)

	require.Error(t, err)
	raw, readErr := os.ReadFile(logPath)
	require.NoError(t, readErr)
	assert.Equal(t, 3, strings.Count(string(raw), "send-keys -t ssq_a -H 6b\n"), "one attempt plus %d retries", sendInputToTmuxInputRetries)
	assert.GreaterOrEqual(t, elapsed, 2*sendInputToTmuxRetryBackoff, "each retry waits the backoff, including the first")
}

func TestSendInputToTmuxWithRetry_ShouldStopAtTheFirstSuccess_WhenTmuxAnswers(t *testing.T) {
	logPath := installFakeTmux(t, "0")

	require.NoError(t, sendInputToTmuxWithRetry("ssq-seams", "ssq_a", []byte("k")))
	raw, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(raw), "\n"))
}

func TestSessionNudgeGuard_ShouldRecordNothing_WhenAClaimIsAbandoned(t *testing.T) {
	t.Parallel()
	g := NewSessionNudgeGuardWithClock(func() time.Time { return time.Unix(1_800_000_000, 0) })
	_, out := g.TryBegin("s", "sig")
	require.Equal(t, GuardOK, out)
	_, out = g.TryBegin("s", "sig")
	assert.Equal(t, GuardBusy, out, "the claim is in flight")

	g.Abandon("s")
	release, out := g.TryBegin("s", "sig")
	assert.Equal(t, GuardOK, out, "an abandoned claim neither blocks nor records a duplicate window")
	release(false)
}
