package deliverygate

import (
	"fmt"
	"testing"

	"github.com/tstapler/stapler-squad/pkg/events"
)

// Task 2.3f / T-BF-11: the publish filter runs on the caller goroutine of ~108
// Publish sites, so its cost is measured against an unfiltered Publish, with an
// index of 0 and of 1000 sessions (it lives here rather than in pkg/events,
// which cannot import the gate). The 5% budget is judged from the benchmark
// output in the PR, not asserted. Run in the background per
// docs/reference/benchmarks.md:
//
//	go test -run '^$' -bench 'BenchmarkPublish' -benchmem -count 5 ./server/deliverygate &

func benchEntries(n int) []Entry {
	out := make([]Entry, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, Entry{
			UUID: fmt.Sprintf("uuid-%d", i), Title: fmt.Sprintf("session-%d", i),
			TmuxName: fmt.Sprintf("ssq_session_%d", i), Hidden: i%2 == 0, Kind: KindReview,
		})
	}
	return out
}

func benchGate(sessions int) *Gate {
	flags := &staticFlags{}
	flags.set(true)
	clk := newFakeClock()
	lg, _ := newRecLogger()
	g := NewGate(WithClock(clk.Now), WithLogger(lg), WithFlagLoader(flags.load))
	g.Flags().Reload()
	g.Index().Replace(benchEntries(sessions))
	return g
}

// benchEvents returns the event kinds measured, keyed by name.
func benchEvents() map[string]*events.Event {
	return map[string]*events.Event{
		"hit_hidden_routine": notif("session-0", tTaskComplete, nil), // rejected by the filter
		"hit_hidden_failure": notif("session-0", tError, nil),        // delivered
		"hit_visible":        notif("session-1", tTaskComplete, nil), // delivered
		"miss_unresolved":    notif("not-in-index", tTaskComplete, nil),
		"non_notification":   {Type: events.EventSessionUpdated, SessionID: "session-0"},
	}
}

func BenchmarkPublishNoFilter(b *testing.B) {
	for name, ev := range benchEvents() {
		ev := ev
		b.Run(name, func(b *testing.B) {
			bus := events.NewEventBus(100)
			defer bus.Close()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				e := *ev
				bus.Publish(&e)
			}
		})
	}
}

func BenchmarkPublishWithFilter(b *testing.B) {
	for _, sessions := range []int{0, 1000} {
		for name, ev := range benchEvents() {
			ev := ev
			b.Run(fmt.Sprintf("index_%d/%s", sessions, name), func(b *testing.B) {
				g := benchGate(sessions)
				bus := events.NewEventBus(100)
				defer bus.Close()
				bus.SetPublishFilter(g.PublishFilter())
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					e := *ev
					bus.Publish(&e)
				}
			})
		}
	}
}

// BenchmarkIndexUpsert_1000Sessions records the copy-and-swap cost (T-IX-14).
func BenchmarkIndexUpsert_1000Sessions(b *testing.B) {
	idx := NewVisibilityIndex(newFakeClock().Now)
	idx.Replace(benchEntries(1000))
	e := Entry{UUID: "uuid-new", Title: "new", TmuxName: "ssq_new", Hidden: true, Kind: KindReview}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Upsert(e)
	}
}

// BenchmarkResolve_Hit measures the lock-free read path alone.
func BenchmarkResolve_Hit(b *testing.B) {
	g := benchGate(1000)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			g.Resolver().Resolve("session-500", nil)
		}
	})
}

// BenchmarkFilterOnly isolates the filter from Publish's own cost (ring-buffer
// append and two clock reads), so the filter's absolute cost is visible.
func BenchmarkFilterOnly(b *testing.B) {
	for _, sessions := range []int{0, 1000} {
		for name, ev := range benchEvents() {
			ev := ev
			b.Run(fmt.Sprintf("index_%d/%s", sessions, name), func(b *testing.B) {
				filter := benchGate(sessions).PublishFilter()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					filter(ev)
				}
			})
		}
	}
}
