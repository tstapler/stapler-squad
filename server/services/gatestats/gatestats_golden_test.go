package gatestats

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/services/fsops/memfs"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/gatestats.golden")

// Transcript recorded from the pre-move code (server/services at origin/main
// 0830eb23e): store bytes and status, quarantine and fault paths, writer tick
// and foreign-writer handoff, response proto text and liveness outcomes.
func TestGatestatsGolden_ShouldMatchTranscriptRecordedFromPreMoveCode(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	goldenStore(t, &b)
	goldenQuarantine(t, &b)
	goldenForeign(t, &b)
	goldenWriter(t, &b)
	goldenResponse(&b)
	goldenLiveness(&b)

	path := filepath.Join("testdata", "gatestats.golden")
	if *updateGolden {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, []byte(b.String()), 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), b.String())
}

func dumpFS(b *strings.Builder, mfs *memfs.MemFS) {
	names := mfs.Names(testStatsDir)
	sort.Strings(names)
	for _, n := range names {
		data, _ := mfs.Get(n)
		fmt.Fprintf(b, "  file %s: %s\n", n, data)
	}
}

func goldenStore(t *testing.T, b *strings.Builder) {
	mfs, clk := memfs.New(), newGateTestClock()
	store, warns := newTestStore(t, mfs, clk, 100)
	fmt.Fprintf(b, "== store\nstatus0=%+v\n", store.Status())
	fmt.Fprintf(b, "save=%v status=%+v\n", store.Save(sampleStats(clk)), store.Status())
	dumpFS(b, mfs)
	p, ok := store.Load()
	fmt.Fprintf(b, "load ok=%v pid=%d writtenAt=%s version=%d buckets=%d status=%+v\n",
		ok, p.WriterPID, p.WrittenAt.Format(time.RFC3339Nano), p.Version, len(p.Buckets), store.Status())

	mfs.FailRename = errDiskFull
	clk.Advance(time.Minute)
	err := store.Save(sampleStats(clk))
	fmt.Fprintf(b, "save-rename-fail=%v status=%+v warn=%d\n", err, store.Status(), warns("delivery_gate_stats_write_failed"))
	err = store.Save(sampleStats(clk))
	fmt.Fprintf(b, "save-again=%v warn=%d\n", err, warns("delivery_gate_stats_write_failed"))
	clk.Advance(11 * time.Minute)
	_ = store.Save(sampleStats(clk))
	fmt.Fprintf(b, "after-10m warn=%d\n", warns("delivery_gate_stats_write_failed"))
	dumpFS(b, mfs)
}

func goldenQuarantine(t *testing.T, b *strings.Builder) {
	good := func() ([]byte, *memfs.MemFS, *gateTestClock) {
		mfs, clk := memfs.New(), newGateTestClock()
		s, _ := newTestStore(t, mfs, clk, 1)
		require.NoError(t, s.Save(sampleStats(clk)))
		d, _ := mfs.Get(testStatsDir + "/" + statsFileName)
		return d, mfs, clk
	}
	cases := []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"invalid-json", func([]byte) []byte { return []byte("{nope") }},
		{"bad-version", func(d []byte) []byte { return []byte(strings.Replace(string(d), `"version":1`, `"version":99`, 1)) }},
		{"bad-checksum", func(d []byte) []byte { return []byte(strings.Replace(string(d), `"count":3`, `"count":4`, 1)) }},
		{"oversize", func([]byte) []byte { return make([]byte, statsFileMaxBytes+1) }},
		{"valid", func(d []byte) []byte { return d }},
	}
	fmt.Fprintf(b, "== quarantine\n")
	for _, c := range cases {
		d, mfs, clk := good()
		mfs.Put(testStatsDir+"/"+statsFileName, c.mutate(d))
		mfs.Put(testStatsDir+"/"+statsFileName+".7.tmp", []byte("stale"))
		store, warns := newTestStore(t, mfs, clk, 2)
		_, ok := store.Load()
		fmt.Fprintf(b, "%s ok=%v status=%+v corrupt-warns=%d\n", c.name, ok, store.Status(), warns("delivery_gate_stats_corrupt"))
		for _, n := range mfs.Names(testStatsDir) {
			fmt.Fprintf(b, "  name %s\n", n)
		}
	}
}

func goldenForeign(t *testing.T, b *strings.Builder) {
	fmt.Fprintf(b, "== foreign\n")
	for _, alive := range []bool{true, false} {
		mfs, clk := memfs.New(), newGateTestClock()
		owner, _ := newTestStore(t, mfs, clk, 100)
		require.NoError(t, owner.Save(sampleStats(clk)))
		lg, warns := newLogCapture()
		other := NewFileStatsStore(testStatsDir, WithStatsFS(mfs), WithStatsClock(clk.Now), WithStatsPID(200), WithStatsLogger(lg),
			WithStatsPIDLiveness(func(pid int, _ time.Time) bool { return alive && pid == 100 }))
		fmt.Fprintf(b, "alive=%v foreign=%v save=%v warns=%d\n", alive, other.ForeignWriterActive(), other.Save(sampleStats(clk)), warns("delivery_gate_stats_foreign_writer"))
		clk.Advance(3 * time.Minute)
		fmt.Fprintf(b, "  after-window foreign=%v save=%v\n", other.ForeignWriterActive(), other.Save(sampleStats(clk)))
		dumpFS(b, mfs)
	}
}

func goldenWriter(t *testing.T, b *strings.Builder) {
	fmt.Fprintf(b, "== writer\n")
	mfs, clk := memfs.New(), newGateTestClock()
	w1, s1, _ := newWriterEnv(t, 1, mfs, clk)
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	w1.Start(ctx, tick, &wg)
	fmt.Fprintf(b, "running=%v\n", s1.WriterRunning())
	clk.Advance(10 * time.Minute)
	tick <- clk.Now()
	cancel()
	wg.Wait()
	fmt.Fprintf(b, "running-after=%v\n", s1.WriterRunning())
	w1.FlushFinal()
	dumpFS(b, mfs)

	clk.Advance(6 * time.Minute)
	w2, s2, _ := newWriterEnv(t, 2, mfs, clk)
	ctx2, cancel2 := context.WithCancel(context.Background())
	var wg2 sync.WaitGroup
	w2.Start(ctx2, make(chan time.Time), &wg2)
	cancel2()
	wg2.Wait()
	snap := s2.Snapshot(clk.Now())
	fmt.Fprintf(b, "restart buckets=%d uptime=%d prevZero=%v\n", len(snap.Buckets), snap.Buckets[0].UptimeSeconds, snap.PreviousProcessStart.IsZero())

	// foreign handoff: writer 3 flushes while writer 2's file is fresh, then the window passes.
	w2.flush()
	clk.Advance(time.Minute)
	w3, s3, store3 := newWriterEnv(t, 3, mfs, clk)
	w3.flush()
	fmt.Fprintf(b, "foreign-flag=%v status=%+v\n", w3.foreign, store3.Status())
	clk.Advance(3 * time.Minute)
	w3.flush()
	fmt.Fprintf(b, "after-window foreign-flag=%v running=%v buckets=%d\n", w3.foreign, s3.WriterRunning(), len(s3.Snapshot(clk.Now()).Buckets))
	dumpFS(b, mfs)
}

func goldenResponse(b *strings.Builder) {
	fmt.Fprintf(b, "== response\n")
	clk := newGateTestClock()
	stats := deliverygate.NewStats(clk.Now)
	stats.Merge(sampleStats(clk))
	clk.Advance(30 * time.Minute)
	snap := stats.Snapshot(clk.Now())
	snap.FlagHistory = []deliverygate.FlagChange{
		{Scope: "global", Value: false, At: clk.Now(), Mutation: "SET"},
		{Scope: "kind:review", Value: true, At: clk.Now(), Mutation: "BOGUS"},
	}
	snap.Soak.LastOffFlipAt = clk.Now()
	series := []deliverygate.Series{
		{Name: "suppressed", Labels: map[string]string{"kind": "review", "class": "routine", "z": "1", "a": "2"}, Count: 7},
		{Name: "hidden_delivered", Labels: map[string]string{"kind": "review", "class": "failure"}, Count: 2},
		{Name: "rpc_unversioned", Labels: map[string]string{}, Count: 1},
	}
	full := BuildResponse(StatsView{Snapshot: snap, Series: series, File: deliverygate.StatsFileStatus{Loaded: true, Writable: true}, ExplicitOff: []string{"global", "kind:review"}})
	fmt.Fprintf(b, "%x\n", mustMarshal(full))
	empty := BuildResponse(StatsView{})
	fmt.Fprintf(b, "empty: %x\n", mustMarshal(empty))
}

func mustMarshal(m proto.Message) []byte {
	out, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
	if err != nil {
		panic(err)
	}
	return out
}

func goldenLiveness(b *strings.Builder) {
	fmt.Fprintf(b, "== liveness\n")
	self := os.Getpid()
	now := time.Now()
	fmt.Fprintf(b, "self-now=%v self-future=%v zero=%v neg=%v bogus=%v\n",
		ProcessWriterAlive(self, now), ProcessWriterAlive(self, now.Add(time.Hour)),
		ProcessWriterAlive(0, now), ProcessWriterAlive(-5, now),
		ProcessWriterAlive(1<<22+12345, now))
}

// A process that started after the file was written cannot be its writer; only
// asserted where the OS exposes a start time (Linux, macOS).
func TestProcessWriterAlive_ShouldRejectReusedPid_WhenProcessStartIsKnown(t *testing.T) {
	t.Parallel()
	if _, ok := processStart(os.Getpid()); !ok {
		t.Skip("process start time unavailable on this OS")
	}
	require.False(t, ProcessWriterAlive(os.Getpid(), time.Now().Add(-24*time.Hour)))
}
