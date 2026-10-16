package deliverygate

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/pkg/events"
)

// T-BF-05 / T-IX-08: publishes complete while other goroutines hold unrelated
// locks and Upsert is called under a held "Instance.mu" stand-in. A 5s watchdog
// turns a deadlock into a failure.
func TestGate_ShouldCompletePublishes_WhenRQPInstanceAndPMLocksHeld(t *testing.T) {
	t.Parallel()
	g, _, _, _ := newTestGate(true, hiddenReview, visibleSess)
	bus := events.NewEventBus(1024)
	defer bus.Close()
	bus.SetPublishFilter(g.PublishFilter())

	var rqpMu, instMu, pmMu sync.Mutex
	release := make(chan struct{})
	var held sync.WaitGroup
	for _, mu := range []*sync.Mutex{&rqpMu, &instMu, &pmMu} {
		held.Add(1)
		mu := mu
		go func() {
			mu.Lock()
			held.Done()
			<-release
			mu.Unlock()
		}()
	}
	held.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for p := 0; p < 8; p++ {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			for i := 0; i < 200 && ctx.Err() == nil; i++ {
				bus.Publish(notif("review:abc", tTaskComplete, nil))
				bus.Publish(notif("my-work", tTaskComplete, nil))
				g.Resolver().Resolve("never", nil)
			}
		}(p)
	}
	wg.Add(1)
	go func() { // Upsert while holding the Instance.mu stand-in's sibling lock
		defer wg.Done()
		for i := 0; i < 100 && ctx.Err() == nil; i++ {
			g.Index().Upsert(Entry{UUID: "u-new", Title: "new", Hidden: i%2 == 0, Kind: KindOther})
			g.Index().Remove("u-new")
		}
	}()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("publishes did not complete within 5s: filter blocked on a held lock")
	}
	close(release)
}
