package services

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/server/deliverygate"
)

// gateTestClock is a mutex-guarded manual clock for the stats tests.
type gateTestClock struct {
	mu sync.Mutex
	t  time.Time
}

func newGateTestClock() *gateTestClock {
	return &gateTestClock{t: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
}

func (c *gateTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *gateTestClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// gateMsgCapture is an injected slog handler (never the global default, BUG-087).
type gateMsgCapture struct {
	mu   *sync.Mutex
	msgs *[]string
}

func newLogCapture() (*slog.Logger, func(msg string) int) {
	h := gateMsgCapture{mu: &sync.Mutex{}, msgs: &[]string{}}
	return slog.New(h), func(msg string) int {
		h.mu.Lock()
		defer h.mu.Unlock()
		n := 0
		for _, m := range *h.msgs {
			if m == msg {
				n++
			}
		}
		return n
	}
}

func (gateMsgCapture) Enabled(context.Context, slog.Level) bool { return true }
func (h gateMsgCapture) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	*h.msgs = append(*h.msgs, r.Message)
	h.mu.Unlock()
	return nil
}
func (h gateMsgCapture) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h gateMsgCapture) WithGroup(string) slog.Handler      { return h }

const testStatsDir = "/cfg"

func newTestStore(t *testing.T, mfs *memFS, clk *gateTestClock, pid int) (*FileStatsStore, func(string) int) {
	t.Helper()
	lg, count := newLogCapture()
	return NewFileStatsStore(testStatsDir, WithStatsFS(mfs), WithStatsClock(clk.Now), WithStatsPID(pid), WithStatsLogger(lg)), count
}

func sampleStats(clk *gateTestClock) deliverygate.PersistedStats {
	return deliverygate.PersistedStats{
		Version: deliverygate.StatsFileVersion, ProcessStart: clk.Now(),
		Buckets: []deliverygate.PersistedBucket{{
			HourStart: clk.Now().Truncate(time.Hour), UptimeMs: 1800_000, GateOnMs: 1800_000, RoutineWhileOn: 3,
			Counters: []deliverygate.PersistedCounter{{Counter: "suppressed", Kind: "review", Class: "routine", Count: 3}},
		}},
	}
}

func TestStatsFile_ShouldWriteAtomicallyAndKeepPreviousFile_WhenCrashBetweenTempWriteAndRename(t *testing.T) {
	t.Parallel()
	mfs, clk := newMemFS(), newGateTestClock()
	store, _ := newTestStore(t, mfs, clk, 100)
	require.NoError(t, store.Save(sampleStats(clk)))
	before, ok := mfs.get(testStatsDir + "/" + statsFileName)
	require.True(t, ok)

	mfs.failRename = errDiskFull
	next := sampleStats(clk)
	next.Buckets[0].RoutineWhileOn = 99
	require.Error(t, store.Save(next))

	after, _ := mfs.get(testStatsDir + "/" + statsFileName)
	assert.Equal(t, string(before), string(after), "previous file must be intact")
	assert.Empty(t, mfs.names(testStatsDir+"/"+statsFileName+"."), "failed write must not leave a temp file")
	assert.False(t, store.Status().Writable)
	assert.Equal(t, os.FileMode(0o600), mfs.modes[testStatsDir+"/"+statsFileName+".100.tmp"]|0o600)
}

func TestStatsFile_ShouldStartEmptyQuarantineCorruptFileAndWarnOnce_WhenInvalidJsonUnknownVersionOrBadChecksum(t *testing.T) {
	t.Parallel()
	good := func(clk *gateTestClock) []byte {
		mfs := newMemFS()
		store, _ := newTestStore(t, mfs, clk, 1)
		require.NoError(t, store.Save(sampleStats(clk)))
		b, _ := mfs.get(testStatsDir + "/" + statsFileName)
		return b
	}
	mutate := func(b []byte, f func(map[string]any)) []byte {
		var m map[string]any
		require.NoError(t, json.Unmarshal(b, &m))
		f(m)
		out, err := json.Marshal(m)
		require.NoError(t, err)
		return out
	}
	cases := map[string]func(*gateTestClock) []byte{
		"invalid json":     func(*gateTestClock) []byte { return []byte("{not json") },
		"unknown version":  func(c *gateTestClock) []byte { return mutate(good(c), func(m map[string]any) { m["version"] = 99 }) },
		"bad checksum":     func(c *gateTestClock) []byte { return mutate(good(c), func(m map[string]any) { m["writer_pid"] = 77 }) },
		"oversize (1 MiB)": func(*gateTestClock) []byte { return []byte(strings.Repeat("x", statsFileMaxBytes+1)) },
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mfs, clk := newMemFS(), newGateTestClock()
			mfs.put(testStatsDir+"/"+statsFileName, mk(clk))
			mfs.put(testStatsDir+"/"+statsFileName+".corrupt-20200101T000000Z", []byte("old"))
			store, warns := newTestStore(t, mfs, clk, 2)

			_, ok := store.Load()
			assert.False(t, ok)
			assert.Equal(t, 1, warns("delivery_gate_stats_corrupt"))
			assert.True(t, store.Status().Quarantined)
			_, still := mfs.get(testStatsDir + "/" + statsFileName)
			assert.False(t, still, "corrupt file must be moved aside")
			assert.Len(t, mfs.names(testStatsDir+"/"+statsFileName+".corrupt-"), 1, "at most one quarantined file is kept")
		})
	}
}

func TestStatsFile_ShouldRoundTripAndLoadStatus_WhenValidFileExists(t *testing.T) {
	t.Parallel()
	mfs, clk := newMemFS(), newGateTestClock()
	store, _ := newTestStore(t, mfs, clk, 1)
	require.NoError(t, store.Save(sampleStats(clk)))
	assert.True(t, store.Status().Writable)

	again, _ := newTestStore(t, mfs, clk, 2)
	p, ok := again.Load()
	require.True(t, ok)
	assert.Equal(t, 1, p.WriterPID)
	assert.Equal(t, int64(3), p.Buckets[0].RoutineWhileOn)
	assert.True(t, again.Status().Loaded)
}

func TestStatsStore_ShouldNeverBlockPanicOrFailStartupAndWarnOncePer10Minutes_WhenDirUnwritableOrDiskFull(t *testing.T) {
	t.Parallel()
	mfs, clk := newMemFS(), newGateTestClock()
	mfs.failWrite = errDiskFull
	store, warns := newTestStore(t, mfs, clk, 1)
	mfs.put(testStatsDir+"/"+statsFileName+".5.tmp", []byte("stale"))

	_, ok := store.Load() // removes the stale temp, no file: starts empty
	assert.False(t, ok)
	assert.Empty(t, mfs.names(testStatsDir+"/"+statsFileName+"."))

	for i := 0; i < 5; i++ {
		require.Error(t, store.Save(sampleStats(clk)))
		clk.Advance(time.Minute)
	}
	assert.Equal(t, 1, warns("delivery_gate_stats_write_failed"), "one WARN inside the 10-minute window")
	clk.Advance(10 * time.Minute)
	require.Error(t, store.Save(sampleStats(clk)))
	assert.Equal(t, 2, warns("delivery_gate_stats_write_failed"))
	assert.False(t, store.Status().Writable)
}

func TestStatsStore_ShouldUsePidSpecificTempNameDefinedChecksumAndForeignWriterRule_WhenTwoProcessesShareTheDirectory(t *testing.T) {
	t.Parallel()
	mfs, clk := newMemFS(), newGateTestClock()
	a, _ := newTestStore(t, mfs, clk, 100)
	b, bWarns := newTestStore(t, mfs, clk, 200)
	assert.Equal(t, testStatsDir+"/"+statsFileName+".100.tmp", a.tmpPath())
	assert.Equal(t, testStatsDir+"/"+statsFileName+".200.tmp", b.tmpPath())

	require.NoError(t, a.Save(sampleStats(clk)))
	raw, _ := mfs.get(testStatsDir + "/" + statsFileName)
	var p deliverygate.PersistedStats
	require.NoError(t, json.Unmarshal(raw, &p))
	want, err := statsChecksum(p)
	require.NoError(t, err)
	assert.Equal(t, want, p.Checksum)

	require.ErrorIs(t, b.Save(sampleStats(clk)), errForeignWriter)
	assert.Equal(t, 1, bWarns("delivery_gate_stats_foreign_writer"))
	after, _ := mfs.get(testStatsDir + "/" + statsFileName)
	assert.Equal(t, string(raw), string(after), "the foreign-owned file is left alone")

	clk.Advance(3 * time.Minute) // writer pid 100 is gone: last write older than 2 minutes
	require.NoError(t, b.Save(sampleStats(clk)))
}

func TestStatsStore_ShouldDefaultToNoopStoreWithNoDiskOrGoroutine_WhenUnitTestsBuildAGate(t *testing.T) {
	defer goleak.VerifyNone(t)
	var s deliverygate.StatsStore = deliverygate.NoopStatsStore{}
	_, ok := s.Load()
	assert.False(t, ok)
	assert.NoError(t, s.Save(deliverygate.PersistedStats{}))
	assert.False(t, s.Status().Writable)
}

func newWriterEnv(t *testing.T, pid int, mfs *memFS, clk *gateTestClock) (*StatsWriter, *deliverygate.Stats, *FileStatsStore) {
	t.Helper()
	store, _ := newTestStore(t, mfs, clk, pid)
	stats := deliverygate.NewStats(clk.Now)
	lg, _ := newLogCapture()
	return NewStatsWriter(stats, store, clk.Now, func(time.Duration) <-chan time.Time { return nil }, lg), stats, store
}

func TestStatsWriter_ShouldReloadBucketsAfterRestartWhileUptimeStartsAtZero_WhenStatsFilePersisted(t *testing.T) {
	t.Parallel()
	mfs, clk := newMemFS(), newGateTestClock()
	w1, s1, _ := newWriterEnv(t, 1, mfs, clk)
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	w1.Start(ctx, tick, &wg)
	assert.True(t, s1.WriterRunning())
	clk.Advance(10 * time.Minute)
	tick <- clk.Now()
	cancel()
	wg.Wait()
	assert.False(t, s1.WriterRunning())
	w1.FlushFinal()

	clk.Advance(5 * time.Minute)
	w2, s2, _ := newWriterEnv(t, 2, mfs, clk)
	clk.Advance(time.Minute) // keep the first writer outside the foreign-writer window
	ctx2, cancel2 := context.WithCancel(context.Background())
	var wg2 sync.WaitGroup
	w2.Start(ctx2, make(chan time.Time), &wg2)
	cancel2()
	wg2.Wait()

	snap := s2.Snapshot(clk.Now())
	require.Len(t, snap.Buckets, 1)
	// 600s reloaded from the previous window plus 60s this process has been up.
	assert.Equal(t, int32(660), snap.Buckets[0].UptimeSeconds, "previous window's uptime is reloaded")
	assert.False(t, snap.PreviousProcessStart.IsZero())
}

func TestShutdownFlush_ShouldReturnWithinStatsFlushTimeout_WhenFsyncStallsDuringTheDeferredFinalFlush(t *testing.T) {
	t.Parallel()
	mfs, clk := newMemFS(), newGateTestClock()
	mfs.syncStall = make(chan struct{})
	mfs.syncEntered = make(chan struct{}, 1)
	store, _ := newTestStore(t, mfs, clk, 1)
	stats := deliverygate.NewStats(clk.Now)
	timeout := make(chan time.Time, 1)
	lg, warns := newLogCapture()
	w := NewStatsWriter(stats, store, clk.Now, func(d time.Duration) <-chan time.Time {
		assert.Equal(t, StatsFlushTimeout, d)
		return timeout
	}, lg)

	returned := make(chan struct{})
	go func() { w.FlushFinal(); close(returned) }()
	<-mfs.syncEntered
	timeout <- clk.Now() // the bound fires while the fsync is still stalled
	<-returned
	assert.Equal(t, 1, warns("delivery_gate_stats_final_flush_timed_out"))
	close(mfs.syncStall) // release the abandoned write so it can finish

	// The abandoned write leaves at most a stale temp; the next start removes it.
	store2, _ := newTestStore(t, mfs, clk, 1)
	store2.Load()
	assert.Empty(t, mfs.names(testStatsDir+"/"+statsFileName+".1.tmp"))
}

func TestStatsStore_ShouldSerializeSaveAndFlush_WhenWriterTickAndFinalFlushRace(t *testing.T) {
	t.Parallel()
	mfs, clk := newMemFS(), newGateTestClock()
	w, stats, store := newWriterEnv(t, 1, mfs, clk)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.FlushFinal()
		}()
	}
	wg.Wait()
	_ = stats
	p, ok := store.Load()
	require.True(t, ok, "a valid file results")
	assert.Equal(t, 1, p.WriterPID)
}

func TestStatsStore_ShouldMergeBucketsByHourStart_WhenForeignWriterExitsAndProcessResumes(t *testing.T) {
	t.Parallel()
	mfs, clk := newMemFS(), newGateTestClock()
	foreign, _ := newTestStore(t, mfs, clk, 900)
	fp := sampleStats(clk)
	fp.Buckets[0].UptimeMs = 3000_000
	fp.Buckets[0].RoutineWhileOn = 2
	require.NoError(t, foreign.Save(fp))

	w, stats, _ := newWriterEnv(t, 1, mfs, clk)
	stats.Merge(deliverygate.PersistedStats{Buckets: []deliverygate.PersistedBucket{{
		HourStart: clk.Now().Truncate(time.Hour), UptimeMs: 1800_000, RoutineWhileOn: 5,
	}}})
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	w.Start(ctx, tick, &wg) // Start loads the file (the foreign one) into memory too
	cancel()
	wg.Wait()

	// Within the foreign window a flush keeps stats in memory and writes nothing.
	w.flush()
	raw, _ := mfs.get(testStatsDir + "/" + statsFileName)
	var p deliverygate.PersistedStats
	require.NoError(t, json.Unmarshal(raw, &p))
	assert.Equal(t, 900, p.WriterPID)

	clk.Advance(3 * time.Minute)
	w.flush() // foreign is gone: merge on resume, then write as pid 1
	raw, _ = mfs.get(testStatsDir + "/" + statsFileName)
	require.NoError(t, json.Unmarshal(raw, &p))
	assert.Equal(t, 1, p.WriterPID)
	assert.LessOrEqual(t, p.Buckets[0].UptimeMs, int64(3600_000), "seconds are clamped to one hour")
}
