package queue_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/queue"
)

// makeItem returns a ReviewItem suitable for benchmarking.
func makeItem(id string) *queue.ReviewItem {
	return &queue.ReviewItem{
		SessionID:    id,
		SessionName:  id,
		Reason:       queue.ReasonTaskComplete,
		Priority:     queue.PriorityLow,
		DetectedAt:   time.Now(),
		LastActivity: time.Now(),
	}
}

// BenchmarkReviewQueue_ConcurrentReads exercises the sync.RWMutex read path
// under goroutine concurrency. Run with -race to verify no data races.
// A regression to sync.Mutex would show ~linear slowdown with GOMAXPROCS.
func BenchmarkReviewQueue_ConcurrentReads(b *testing.B) {
	rq := queue.NewReviewQueue()
	for i := range 100 {
		rq.Add(makeItem(fmt.Sprintf("session-%d", i)))
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = rq.Has("session-42")
			_ = rq.Count()
		}
	})
}

// BenchmarkReviewQueue_Add benchmarks the write path for comparison.
func BenchmarkReviewQueue_Add(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		rq := queue.NewReviewQueue()
		rq.Add(makeItem("s1"))
	}
}
