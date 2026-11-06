package session

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/flow"
)

// Live-size checks. Set SSQ_FLOW_PERF_DB to a `sqlite3 .backup` copy of a
// workspace DB (never the live file); the test copies it into a temp dir first
// because the writer rounds insert rows. Run by hand:
//
//	SSQ_FLOW_PERF_DB=/tmp/flowperf.db go test ./session -run TestFlow.*BackupCopy -v -count=1
const perfDBEnv = "SSQ_FLOW_PERF_DB"

const (
	writeBudget  = 100 * time.Millisecond
	perfRounds   = 5
	perfWriteGap = 2 * time.Millisecond
)

func openPerfRepo(t *testing.T) *EntRepository {
	t.Helper()
	src := os.Getenv(perfDBEnv)
	if src == "" {
		t.Skipf("set %s to a sqlite3 .backup copy of a workspace DB", perfDBEnv)
	}
	dst := filepath.Join(t.TempDir(), "sessions.db")
	in, err := os.Open(src)
	require.NoError(t, err)
	defer in.Close()
	out, err := os.Create(dst)
	require.NoError(t, err)
	_, err = io.Copy(out, in)
	require.NoError(t, err)
	require.NoError(t, out.Close())
	repo, err := NewEntRepository(WithDatabasePath(dst))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func perfAggregator(r *EntFlowReader) *flow.Aggregator {
	return flow.NewAggregator(flow.Sources{Events: r, Verdicts: r, Stuck: r, Items: r}, flow.SystemClock{})
}

func timedSummary(t *testing.T, r *EntFlowReader, spec flow.WindowSpec) (time.Duration, *flow.FlowSummary) {
	t.Helper()
	start := time.Now()
	s, err := perfAggregator(r).Summary(context.Background(), flow.FlowQuery{Window: spec})
	require.NoError(t, err)
	return time.Since(start), s
}

func TestFlowSummary_should_ReportColdAndWarmTimings_When_BackupCopyOfLiveDb(t *testing.T) {
	repo := openPerfRepo(t)
	r := NewEntFlowReader(repo.GetEntClient())
	for _, tc := range []struct {
		name string
		spec flow.WindowSpec
	}{{"30-day", flow.WindowSpec{Days: 30}}, {"all-time", flow.WindowSpec{AllTime: true}}} {
		cold, s := timedSummary(t, r, tc.spec)
		warm, _ := timedSummary(t, r, tc.spec)
		t.Logf("%s: cold=%dms warm=%dms rows=%d slow_path=%v", tc.name, cold.Milliseconds(), warm.Milliseconds(), s.Meta.RowCounts.Events, s.Meta.SlowPath)
		for _, sec := range []flow.SectionMeta{s.Tiles.Meta, s.Funnel.Meta, s.Review.Meta, s.Stuck.Meta} {
			require.NotEqual(t, flow.SectionFailed, sec.Status, sec.Error)
		}
		if tc.spec.Days == 30 {
			require.Less(t, cold, time.Second, "30-day SLO")
		}
	}
}

// writerLoop inserts status events until stop closes, returning each insert's latency.
func writerLoop(t *testing.T, c *ent.Client, item uuid.UUID, stop <-chan struct{}) []time.Duration {
	t.Helper()
	var lat []time.Duration
	for i := 0; ; i++ {
		select {
		case <-stop:
			return lat
		default:
		}
		start := time.Now()
		_, err := c.BacklogStatusEvent.Create().SetItemID(item).SetFromStatus("ready").SetToStatus("in_progress").
			SetTriggeredBy("agent").SetCreatedAt(time.Now().UTC()).Save(context.Background())
		if err != nil {
			t.Errorf("a concurrent write must not fail (SQLITE_BUSY): %v", err)
			return lat
		}
		lat = append(lat, time.Since(start))
		time.Sleep(perfWriteGap)
	}
}

func p99(d []time.Duration) time.Duration {
	s := slices.Clone(d)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[min(len(s)-1, len(s)*99/100)]
}

func median(d []time.Duration) time.Duration {
	s := slices.Clone(d)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[len(s)/2]
}

// round runs the writer loop while work executes and returns the writer's latencies.
func round(t *testing.T, c *ent.Client, item uuid.UUID, work func()) []time.Duration {
	t.Helper()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var lat []time.Duration
	wg.Add(1)
	go func() { defer wg.Done(); lat = writerLoop(t, c, item, stop) }()
	work()
	close(stop)
	wg.Wait()
	return lat
}

func TestFlowAllTime_should_KeepConcurrentWritesUnder100ms_When_BackupCopyOfLiveDb(t *testing.T) {
	repo := openPerfRepo(t)
	c := repo.GetEntClient()
	item := c.BacklogItem.Query().FirstX(context.Background()).ID
	r := NewEntFlowReader(c)

	var withReader, withoutReader []time.Duration
	var readerMs []time.Duration
	for i := 0; i < perfRounds; i++ {
		var took time.Duration
		lat := round(t, c, item, func() { took, _ = timedSummary(t, r, flow.WindowSpec{AllTime: true}) })
		withReader = append(withReader, p99(lat))
		readerMs = append(readerMs, took)
		require.Less(t, slices.Max(lat), writeBudget, "round %d: a write exceeded the budget during the all-time read", i)

		lat = round(t, c, item, func() { time.Sleep(took) })
		withoutReader = append(withoutReader, p99(lat))
	}
	t.Logf("writer p99 median over %d alternating rounds: with all-time read=%v, without=%v (all-time read median %v)",
		perfRounds, median(withReader), median(withoutReader), median(readerMs))
	t.Logf("per-round p99 with reader=%v without=%v", withReader, withoutReader)
}
