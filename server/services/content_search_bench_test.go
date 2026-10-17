package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Benchmarks the production walker (Story 1.1.3) on the deterministic 50k-file fixture.
// Run in the background, they take a while:
//
//	go test -bench BenchmarkContentSearch -run '^$' -benchtime 5x -benchmem ./server/services &
//
// first_result_ms is the time to the first emitted match, total_ms the whole search,
// cancel_ms the time from cancel() to Search returning. Cold cache: set SSQ_BENCH_COLD=1
// and run as root; each iteration then does `sync; echo 3 > /proc/sys/vm/drop_caches`.
// Without root it logs why it is skipped.

const (
	searchBenchQuery = "TODO"                      // matches most files: first_result is near zero
	benchRareQuery   = "RAREWORD-ONLY-IN-LAST-DIR" // forces a walk to the end of the tree
	benchRareDir     = "zzz_last"                  // sorts after every generated directory
	benchCancelAfter = 5 * time.Millisecond        // inside the first 100 ms
	benchFiles       = 50000
	benchSLOFirstMS  = 2000.0
)

type benchResult struct {
	first, total time.Duration
	matches      int
}

func benchFixture(b *testing.B, opts ...fixtureOption) string {
	b.Helper()
	dir := genFixtureTree(b, benchFiles, opts...) // only benchmarks call this helper, so no Short() guard
	rare := filepath.Join(dir, benchRareDir)
	if err := os.MkdirAll(rare, 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rare, "last.txt"), []byte(benchRareQuery+"\n"), 0o644); err != nil {
		b.Fatal(err)
	}
	return dir
}

func dropCachesIfRequested(b *testing.B) {
	if os.Getenv("SSQ_BENCH_COLD") == "" {
		return
	}
	if err := os.WriteFile("/proc/sys/vm/drop_caches", []byte("3"), 0o200); err != nil {
		b.Logf("cold cache skipped: %v (needs root on Linux)", err)
	}
}

func benchSearchOnce(b *testing.B, dir, query string) benchResult {
	b.Helper()
	r, err := os.OpenRoot(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	var res benchResult
	start := time.Now()
	_, err = NewWalkSearcher().Search(context.Background(), r.FS(), literalOpts(query), func(ms []SearchMatch) error {
		if res.matches == 0 {
			res.first = time.Since(start)
		}
		res.matches += len(ms)
		return nil
	})
	res.total = time.Since(start)
	if err != nil {
		b.Fatal(err)
	}
	return res
}

func runSearchBench(b *testing.B, dir, query string) {
	b.Helper()
	b.ReportAllocs()
	var firstSum, totalSum time.Duration
	for b.Loop() {
		b.StopTimer()
		dropCachesIfRequested(b)
		b.StartTimer()
		res := benchSearchOnce(b, dir, query)
		if res.matches == 0 {
			b.Fatal("query found nothing")
		}
		firstSum += res.first
		totalSum += res.total
	}
	n := float64(b.N)
	first := firstSum.Seconds() * 1000 / n
	b.ReportMetric(first, "first_result_ms")
	b.ReportMetric(totalSum.Seconds()*1000/n, "total_ms")
	if query == benchRareQuery && first >= benchSLOFirstMS {
		b.Fatalf("first_result_ms=%.1f misses the %.0f ms SLO", first, benchSLOFirstMS)
	}
}

func BenchmarkContentSearch(b *testing.B) {
	variants := []struct {
		name string
		opts []fixtureOption
	}{
		{"gitignore1", nil},
		{"gitignore200", []fixtureOption{WithGitignores(200)}},
	}
	for _, v := range variants {
		dir := benchFixture(b, v.opts...)
		b.Run(v.name+"/common", func(b *testing.B) { runSearchBench(b, dir, searchBenchQuery) })
		b.Run(v.name+"/rare", func(b *testing.B) { runSearchBench(b, dir, benchRareQuery) })
	}
}

// BenchmarkContentSearchCancel measures cancel() to return while the walk is still early.
func BenchmarkContentSearchCancel(b *testing.B) {
	for _, v := range []struct {
		name string
		opts []fixtureOption
	}{{"gitignore1", nil}, {"gitignore200", []fixtureOption{WithGitignores(200)}}} {
		dir := benchFixture(b, v.opts...)
		b.Run(v.name, func(b *testing.B) {
			r, err := os.OpenRoot(dir)
			if err != nil {
				b.Fatal(err)
			}
			defer func() { _ = r.Close() }()
			var worst, sum time.Duration
			for b.Loop() {
				ctx, cancel := context.WithCancel(context.Background())
				var cancelledAt time.Time
				timer := time.AfterFunc(benchCancelAfter, func() { cancelledAt = time.Now(); cancel() })
				_, err := NewWalkSearcher().Search(ctx, r.FS(), literalOpts(benchRareQuery), func([]SearchMatch) error { return nil })
				returned := time.Now()
				timer.Stop()
				cancel()
				if err == nil || cancelledAt.IsZero() {
					b.Fatal("search finished before the cancel")
				}
				lat := returned.Sub(cancelledAt)
				sum += lat
				worst = max(worst, lat)
			}
			b.ReportMetric(sum.Seconds()*1000/float64(b.N), "cancel_ms")
			b.ReportMetric(worst.Seconds()*1000, "cancel_worst_ms")
		})
	}
}
