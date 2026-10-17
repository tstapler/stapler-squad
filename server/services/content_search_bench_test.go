package services

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

// Throwaway prototype of the SearchContent scanner core (Story 0.1.2). Run with:
//
//	go test -bench BenchmarkContentSearch -run '^$' -benchtime 5x ./server/services &
//
// Cold cache: set SSQ_BENCH_COLD=1 and run as root; each iteration then does
// `sync; echo 3 > /proc/sys/vm/drop_caches`. Without root it prints why it is skipped.

const searchBenchQuery = "TODO"

const binarySniffBytes = 512

type searchMetrics struct {
	hits  int64
	first time.Duration
}

func ignoredBy(dir string) (dirs map[string]bool, suffixes []string) {
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		return nil, nil
	}
	for _, l := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasSuffix(l, "/"):
			if dirs == nil {
				dirs = map[string]bool{}
			}
			dirs[strings.TrimSuffix(l, "/")] = true
		case strings.HasPrefix(l, "*."):
			suffixes = append(suffixes, l[1:])
		}
	}
	return dirs, suffixes
}

type ignoreFrame struct {
	dirs     map[string]bool
	suffixes []string
}

// walkCandidates feeds non-ignored file paths to emit, honoring nested .gitignore files.
func walkCandidates(root string, emit func(path string)) error {
	var walk func(dir string, stack []ignoreFrame) error
	walk = func(dir string, stack []ignoreFrame) error {
		d, s := ignoredBy(dir)
		if d != nil || s != nil {
			stack = append(stack[:len(stack):len(stack)], ignoreFrame{d, s})
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			name := e.Name()
			if name == ".git" {
				continue
			}
			skip := false
			for _, f := range stack {
				if e.IsDir() && f.dirs[name] {
					skip = true
				}
				for _, suf := range f.suffixes {
					if strings.HasSuffix(name, suf) {
						skip = true
					}
				}
			}
			if skip {
				continue
			}
			if e.IsDir() {
				if err := walk(filepath.Join(dir, name), stack); err != nil {
					return err
				}
				continue
			}
			emit(filepath.Join(dir, name))
		}
		return nil
	}
	return walk(root, nil)
}

func scanFile(path string, re *regexp.Regexp, m *searchMetrics, mu *sync.Mutex, start time.Time) {
	data, err := os.ReadFile(path)
	if err != nil || bytes.IndexByte(data[:min(len(data), binarySniffBytes)], 0) >= 0 {
		return
	}
	if !re.Match(data) {
		return
	}
	atomic.AddInt64(&m.hits, 1)
	mu.Lock()
	if m.first == 0 {
		m.first = time.Since(start)
	}
	mu.Unlock()
}

func searchSingleThread(root string) searchMetrics {
	re := regexp.MustCompile(regexp.QuoteMeta(searchBenchQuery))
	var m searchMetrics
	var mu sync.Mutex
	start := time.Now()
	_ = walkCandidates(root, func(p string) { scanFile(p, re, &m, &mu, start) })
	return m
}

func searchWorkerPool(root string) searchMetrics {
	re := regexp.MustCompile(regexp.QuoteMeta(searchBenchQuery))
	var m searchMetrics
	var mu sync.Mutex
	start := time.Now()
	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(runtime.GOMAXPROCS(0))
	_ = walkCandidates(root, func(p string) {
		g.Go(func() error {
			scanFile(p, re, &m, &mu, start)
			return nil
		})
	})
	_ = g.Wait()
	return m
}

func dropCachesIfRequested(b *testing.B) {
	if os.Getenv("SSQ_BENCH_COLD") == "" {
		return
	}
	if err := os.WriteFile("/proc/sys/vm/drop_caches", []byte("3"), 0o200); err != nil {
		b.Logf("cold cache skipped: %v (needs root on Linux)", err)
	}
}

func runSearchBench(b *testing.B, root string, search func(string) searchMetrics) {
	b.Helper()
	var firstSum, totalSum time.Duration
	for b.Loop() {
		b.StopTimer()
		dropCachesIfRequested(b)
		b.StartTimer()
		t0 := time.Now()
		m := search(root)
		totalSum += time.Since(t0)
		firstSum += m.first
		if m.hits == 0 {
			b.Fatal("query found nothing")
		}
	}
	n := float64(b.N)
	b.ReportMetric(firstSum.Seconds()*1000/n, "first_result_ms")
	b.ReportMetric(totalSum.Seconds()*1000/n, "total_ms")
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
		root := genFixtureTree(b, 50000, v.opts...)
		b.Run(v.name+"/single", func(b *testing.B) { runSearchBench(b, root, searchSingleThread) })
		b.Run(v.name+"/pool", func(b *testing.B) { runSearchBench(b, root, searchWorkerPool) })
	}
}
