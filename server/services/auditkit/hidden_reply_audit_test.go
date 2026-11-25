package auditkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/fsops/memfs"
)

const testAuditFile = "/cfg/audit/" + AuditFileName

var errDiskFull = errors.New("no space left on device")

type stepClock struct {
	mu sync.Mutex
	t  time.Time
}

func newStepClock() *stepClock {
	return &stepClock{t: time.Date(2026, 1, 2, 3, 4, 5, 123456789, time.UTC)}
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *stepClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func jsonLines(b []byte) [][]byte {
	return bytes.Split(bytes.TrimSpace(b), []byte("\n"))
}

func cfgDir() (string, error) { return "/cfg", nil }

// newTestSink builds a sink on an in-memory fs with a stepping clock and a captured log.
func newTestSink(t *testing.T, mfs *memfs.MemFS, opts ...AuditSinkOption) (*AuditSink, *stepClock, *logCapture) {
	t.Helper()
	clk, lg := newStepClock(), &logCapture{}
	s := NewAuditSink(cfgDir, append([]AuditSinkOption{WithAuditFS(mfs), WithAuditClock(clk.Now), WithAuditLogger(slog.New(lg))}, opts...)...)
	t.Cleanup(s.Close)
	return s, clk, lg
}

func readLines(t *testing.T, mfs *memfs.MemFS) []AuditLine {
	t.Helper()
	raw, ok := mfs.Get(testAuditFile)
	if !ok || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var out []AuditLine
	for _, l := range jsonLines(raw) {
		var al AuditLine
		require.NoError(t, json.Unmarshal(l, &al), string(l))
		out = append(out, al)
	}
	return out
}

func TestAuditSink_ShouldCreateDirTakeBootSeqAndFsyncBeforeReturn_WhenAppending(t *testing.T) {
	t.Parallel()
	mfs, clk := memfs.New(), newStepClock()
	open := func() *AuditSink {
		return NewAuditSink(cfgDir, WithAuditFS(mfs), WithAuditClock(clk.Now))
	}
	s1 := open()
	defer s1.Close()
	require.NoError(t, s1.Append(AuditLine{Kind: "flag_change", Flag: "x"}))
	raw, ok := mfs.Get(testAuditFile)
	require.True(t, ok)
	var l AuditLine
	require.NoError(t, json.Unmarshal(jsonLines(raw)[0], &l))
	assert.Equal(t, "1", l.BootSeq)
	assert.NotEmpty(t, l.BootID)
	assert.Equal(t, auditFileMode, int(mfs.Modes[testAuditFile]))

	// Two process starts order by boot_seq even when the clock steps backward.
	clk.Advance(-24 * time.Hour)
	s2 := open()
	defer s2.Close()
	require.NoError(t, s2.Append(AuditLine{Kind: "flag_change", Flag: "x"}))
	all := jsonLines(func() []byte { b, _ := mfs.Get(testAuditFile); return b }())
	var second AuditLine
	require.NoError(t, json.Unmarshal(all[1], &second))
	assert.Equal(t, "2", second.BootSeq)
	assert.NotEqual(t, l.BootID, second.BootID)
	assert.True(t, second.BootTS < l.BootTS, "boot_ts is a label only and did step backward")
}

func TestAuditSink_ShouldRotateAt5MiBKeepingThreeFilesAndRefuseWithZeroWrites_WhenRotationFails(t *testing.T) {
	t.Parallel()
	mfs, clk := memfs.New(), newStepClock()
	s := NewAuditSink(cfgDir, WithAuditFS(mfs), WithAuditClock(clk.Now))
	defer s.Close()
	big := AuditLine{Kind: "flag_change", UserAgent: strings.Repeat("x", 2<<20)}
	for i := 0; i < 8; i++ {
		require.NoError(t, s.Append(big))
	}
	assert.Len(t, mfs.Names(testAuditFile), 3, "live file plus two rotated")

	before, _ := mfs.Get(testAuditFile)
	mfs.FailRename = errDiskFull
	var wrote error
	for i := 0; i < 3 && wrote == nil; i++ { // enough 2 MiB lines to force a rotation
		wrote = s.Append(big)
	}
	require.ErrorIs(t, wrote, ErrAuditFailed)
	after, _ := mfs.Get(testAuditFile)
	assert.GreaterOrEqual(t, len(after), len(before))
	assert.LessOrEqual(t, len(after), len(before)+2*(2<<20)+1024, "the refused append wrote nothing")
}

func TestAuditSink_ShouldShiftRotatedFilesOldestFirst_WhenLiveFileIsFull(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	s, _, _ := newTestSink(t, mfs)
	live := testAuditFile
	mfs.Put(live, bytes.Repeat([]byte("L"), auditRotateBytes-10))
	mfs.Put(live+".1", []byte("one"))
	mfs.Put(live+".2", []byte("two"))

	require.NoError(t, s.Append(AuditLine{Kind: "flag_change", Flag: "next"}))

	one, _ := mfs.Get(live + ".1")
	two, _ := mfs.Get(live + ".2")
	assert.Len(t, one, auditRotateBytes-10, "the full live file becomes .1")
	assert.Equal(t, "one", string(two), "the old .1 becomes .2 and the old .2 is overwritten")
	assert.Len(t, readLines(t, mfs), 1, "the new live file holds only the new line")
	assert.Len(t, mfs.Names(live), 3)
}

func TestAuditSink_ShouldKeepAppendingToLiveFile_WhenItFitsUnderTheRotateBound(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	s, _, _ := newTestSink(t, mfs)
	mfs.Put(testAuditFile, []byte("{\"kind\":\"old\"}\n"))

	require.NoError(t, s.Append(AuditLine{Kind: "flag_change"}))

	assert.Len(t, mfs.Names(testAuditFile), 1, "no rotation below the bound")
	raw, _ := mfs.Get(testAuditFile)
	assert.Len(t, jsonLines(raw), 2)
}

func TestAuditSink_ShouldResumeBootSeqFromWallClock_WhenCounterIsCorrupt(t *testing.T) {
	t.Parallel()
	for _, corrupt := range []string{"garbage", "-5"} {
		mfs := memfs.New()
		mfs.Put("/cfg/audit/"+auditBootSeqName, []byte(corrupt))
		s, clk, lg := newTestSink(t, mfs)

		require.NoError(t, s.Append(AuditLine{Kind: "flag_change"}))

		want := clk.Now().Unix() + 1
		got := readLines(t, mfs)
		require.Len(t, got, 1)
		assert.Equal(t, want, mustInt(t, got[0].BootSeq), corrupt)
		assert.Equal(t, 1, lg.count("WARN audit boot_seq unreadable, resuming from wall clock"), corrupt)
	}
}

func mustInt(t *testing.T, s string) int64 {
	t.Helper()
	n, err := strconv.ParseInt(s, 10, 64)
	require.NoError(t, err)
	return n
}

func TestAuditSink_ShouldReportErrAuditFailedWithCause_WhenOpenOrWriteFails(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		setup func(*memfs.MemFS) func() (string, error)
		cause string
	}{
		{"mkdir", func(m *memfs.MemFS) func() (string, error) { m.FailMkdir = errors.New("mkdir boom"); return cfgDir }, "mkdir boom"},
		{"dir resolution", func(*memfs.MemFS) func() (string, error) {
			return func() (string, error) { return "", errors.New("no dir") }
		}, "no dir"},
		{"boot_seq write", func(m *memfs.MemFS) func() (string, error) { m.SetFailWrite(errDiskFull); return cfgDir }, "boot_seq: " + errDiskFull.Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mfs := memfs.New()
			s := NewAuditSink(tc.setup(mfs), WithAuditFS(mfs))
			defer s.Close()
			err := s.Append(AuditLine{Kind: "flag_change"})
			require.ErrorIs(t, err, ErrAuditFailed)
			assert.Equal(t, "audit append failed: "+tc.cause, err.Error())
			_, written := mfs.Get(testAuditFile)
			assert.False(t, written, "nothing is written when the sink cannot open")
		})
	}
}

func TestAuditSink_ShouldReportErrAuditFailed_WhenLineWriteFailsAfterOpen(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	s, _, _ := newTestSink(t, mfs)
	require.NoError(t, s.Append(AuditLine{Kind: "flag_change"}))
	mfs.SetFailWrite(errDiskFull)

	err := s.Append(AuditLine{Kind: "flag_change"})

	require.ErrorIs(t, err, ErrAuditFailed)
	assert.Len(t, readLines(t, mfs), 1)
}

func TestAuditSink_ShouldStampEveryLineWithBootIdentityAndRFC3339NanoUTC_WhenAppending(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	s, clk, _ := newTestSink(t, mfs)

	require.NoError(t, s.Append(AuditLine{Kind: "flag_change", TS: "caller-supplied", BootID: "forged", BootSeq: "forged"}))
	clk.Advance(time.Second)
	require.NoError(t, s.Append(AuditLine{Kind: "flag_change"}))

	got := readLines(t, mfs)
	require.Len(t, got, 2)
	assert.Equal(t, "2026-01-02T03:04:05.123456789Z", got[0].TS, "the sink overwrites a caller-supplied ts")
	assert.Equal(t, "2026-01-02T03:04:06.123456789Z", got[1].TS)
	assert.Equal(t, "2026-01-02T03:04:05.123456789Z", got[0].BootTS)
	assert.Equal(t, got[0].BootID, got[1].BootID)
	assert.NotEqual(t, "forged", got[0].BootID)
	assert.Equal(t, "1", got[0].BootSeq)
	assert.Len(t, got[0].BootID, 16)
}

func TestAuditSink_ShouldOmitEmptyOptionalFields_WhenLineIsMinimal(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	s, _, _ := newTestSink(t, mfs)

	require.NoError(t, s.Append(AuditLine{Kind: AuditKindFlagChg, Phase: AuditPhaseRequest}))

	raw, _ := mfs.Get(testAuditFile)
	var m map[string]any
	require.NoError(t, json.Unmarshal(jsonLines(raw)[0], &m))
	assert.ElementsMatch(t, []string{"ts", "kind", "phase", "boot_id", "boot_ts", "boot_seq"}, keys(m))
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestAuditSink_ShouldPinWireLiterals_WhenKindsPhasesAndOutcomesAreWritten(t *testing.T) {
	t.Parallel()
	got := map[string]string{
		"file": AuditFileName, "flag_change": AuditKindFlagChg, "reply": AuditKindReply,
		"backlog_steer": AuditKindBacklogSteer, "guard_bypass": AuditKindGuardBypass, "prune": AuditKindPrune,
		"notification_action": AuditKindNotificationAction, "requested": AuditPhaseRequest, "result": AuditPhaseResult,
		"applied": FlagOutcomeApplied, "aborted_persist_failed": FlagOutcomePersistFailed,
		"aborted_controller_failed": FlagOutcomeControllerFailed, "aborted_rolled_back": FlagOutcomeRolledBack,
		"flag_change_queued": AuditModeQueued, "flag_change_fallback_log": AuditModeFallbackLog, "steer_fallback_log": auditModeSteerLog,
	}
	for want, have := range got {
		if want == "file" {
			assert.Equal(t, "hidden-session-replies.jsonl", have)
			continue
		}
		assert.Equal(t, want, have)
	}
}

func TestAuditSink_ShouldReturnErrAuditTimeout_WhenFsyncStallsPastTheBound(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	mfs.SyncStall = make(chan struct{})
	timer := make(chan time.Time, 1)
	timer <- time.Time{}
	var bound time.Duration
	s, _, _ := newTestSink(t, mfs, WithAuditAfter(func(d time.Duration) <-chan time.Time { bound = d; return timer }))
	defer close(mfs.SyncStall)

	err := s.AppendBounded(context.Background(), AuditLine{Kind: "flag_change"})

	require.ErrorIs(t, err, ErrAuditTimeout)
	assert.Equal(t, 2*time.Second, bound)
}

func TestAuditSink_ShouldReturnContextError_WhenCallerCancelsBeforeFsyncFinishes(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	mfs.SyncStall = make(chan struct{})
	s, _, _ := newTestSink(t, mfs, WithAuditAfter(func(time.Duration) <-chan time.Time { return nil }))
	defer close(mfs.SyncStall)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.AppendBounded(ctx, AuditLine{Kind: "flag_change"})

	require.ErrorIs(t, err, context.Canceled)
}

func TestAuditSink_ShouldReturnAppendResult_WhenAppendBoundedFinishesInTime(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	s, _, _ := newTestSink(t, mfs, WithAuditAfter(func(time.Duration) <-chan time.Time { return nil }))

	require.NoError(t, s.AppendBounded(context.Background(), AuditLine{Kind: "flag_change", Flag: "ok"}))

	mfs.SetFailWrite(errDiskFull)
	require.ErrorIs(t, s.AppendBounded(context.Background(), AuditLine{Kind: "flag_change"}), ErrAuditFailed)
}

func TestAuditSink_ShouldDrainQueuedLinesInOrderBeforeCloseReturns_WhenEnqueued(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	var mu sync.Mutex
	var modes []string
	s, _, _ := newTestSink(t, mfs, WithAuditDegradedCounter(func(m string) { mu.Lock(); modes = append(modes, m); mu.Unlock() }))
	for i := int64(1); i <= 20; i++ {
		s.Enqueue(AuditLine{Kind: "flag_change", Seq: i})
	}

	s.Close()

	got := readLines(t, mfs)
	require.Len(t, got, 20)
	for i, l := range got {
		assert.Equal(t, int64(i+1), l.Seq)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, m := range modes {
		assert.Equal(t, AuditModeQueued, m)
	}
}

func TestAuditSink_ShouldFallBackToLogWithSteerFieldsOrFlagFields_WhenSinkIsClosed(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	var mu sync.Mutex
	var modes []string
	s, _, lg := newTestSink(t, mfs, WithAuditDegradedCounter(func(m string) { mu.Lock(); modes = append(modes, m); mu.Unlock() }))
	require.NoError(t, s.Append(AuditLine{Kind: "flag_change"}))
	s.Close()
	yes := true

	s.Enqueue(AuditLine{Kind: AuditKindBacklogSteer, SessionUUID: "su", ItemID: "it", PeerLoopback: &yes})
	s.Enqueue(AuditLine{Kind: AuditKindGuardBypass, SessionUUID: "su2"})
	s.Enqueue(AuditLine{Kind: AuditKindFlagChg, Flag: "f", Previous: &yes})
	s.Enqueue(AuditLine{Kind: AuditKindReply, Flag: "g"})

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{auditModeSteerLog, auditModeSteerLog, AuditModeFallbackLog, AuditModeFallbackLog}, modes)
	require.Len(t, lg.recs, 4)
	assert.Contains(t, lg.recs[0], "WARN steer_audit_degraded cause=sink closed kind=backlog_steer")
	assert.Contains(t, lg.recs[0], "session_uuid=su")
	assert.Contains(t, lg.recs[0], "peer_loopback=true proxied=unknown")
	assert.Contains(t, lg.recs[1], "kind=guard_bypass")
	assert.Contains(t, lg.recs[2], "WARN flag_change_audit_degraded cause=sink closed")
	assert.Contains(t, lg.recs[2], "flag=f scope= previous=true new=unknown")
	assert.Contains(t, lg.recs[3], "WARN flag_change_audit_degraded")
	assert.Len(t, readLines(t, mfs), 1, "a closed sink writes nothing more to the file")
}

func TestAuditSink_ShouldFallBackToLogWithCause_WhenQueuedLineCannotBeWritten(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	mfs.FailMkdir = errDiskFull
	s, _, lg := newTestSink(t, mfs)

	s.Enqueue(AuditLine{Kind: AuditKindFlagChg, Flag: "f"})
	s.Close()

	require.Len(t, lg.recs, 1)
	assert.Contains(t, lg.recs[0], "WARN flag_change_audit_degraded cause="+errDiskFull.Error())
}

func TestAuditSink_ShouldBeIdempotentAndFlushFile_WhenClosedTwice(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	s, _, _ := newTestSink(t, mfs)
	require.NoError(t, s.Append(AuditLine{Kind: "flag_change"}))

	s.Close()
	s.Close()

	assert.Len(t, readLines(t, mfs), 1)
}

func TestAuditSink_ShouldDefaultToTheRealClockAndOSFS_WhenNoOptionsAreGiven(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := NewAuditSink(func() (string, error) { return dir, nil })
	defer s.Close()

	require.NoError(t, s.Append(AuditLine{Kind: "flag_change", Flag: "real"}))

	ts, err := time.Parse(time.RFC3339Nano, s.bootTS.Format(time.RFC3339Nano))
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), ts, time.Minute)
}

func TestAuditSink_ShouldNotTouchTheFileSystem_WhenNeverUsed(t *testing.T) {
	t.Parallel()
	mfs := memfs.New()
	called := false
	s := NewAuditSink(func() (string, error) { called = true; return "/cfg", nil }, WithAuditFS(mfs))
	s.Close()

	assert.False(t, called, "the directory resolves on first use only")
	assert.Empty(t, mfs.Names("/"))
}

type logCapture struct {
	mu   sync.Mutex
	recs []string
}

func (l *logCapture) Enabled(context.Context, slog.Level) bool { return true }
func (l *logCapture) WithAttrs([]slog.Attr) slog.Handler       { return l }
func (l *logCapture) WithGroup(string) slog.Handler            { return l }

func (l *logCapture) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Level.String() + " " + r.Message)
	r.Attrs(func(a slog.Attr) bool {
		b.WriteString(" " + a.Key + "=" + a.Value.String())
		return true
	})
	l.mu.Lock()
	l.recs = append(l.recs, b.String())
	l.mu.Unlock()
	return nil
}

func (l *logCapture) count(prefix string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, r := range l.recs {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}
