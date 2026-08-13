package warren_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/pkg/warren"
)

func TestGoroutineGroup_GoAndWait(t *testing.T) {
	g := warren.NewGoroutineGroup(context.Background())
	var count atomic.Int32

	for i := 0; i < 5; i++ {
		g.Go("worker", func(ctx context.Context) {
			count.Add(1)
			<-ctx.Done()
		})
	}

	active := g.Active()
	if active["worker"] != 5 {
		t.Errorf("Active()[\"worker\"] = %d, want 5", active["worker"])
	}

	leaks := g.Wait(time.Second)
	if len(leaks) > 0 {
		t.Errorf("unexpected leaks: %v", leaks)
	}
	if count.Load() != 5 {
		t.Errorf("started %d goroutines, want 5", count.Load())
	}
}

func TestGoroutineGroup_LeakDetected(t *testing.T) {
	g := warren.NewGoroutineGroup(context.Background())
	g.Go("stubborn", func(ctx context.Context) {
		time.Sleep(10 * time.Second) // ignores context
	})

	leaks := g.Wait(50 * time.Millisecond)
	if len(leaks) == 0 {
		t.Fatal("expected leak detection, got none")
	}
	if leaks[0] != "stubborn" {
		t.Errorf("leaked goroutine name = %q, want %q", leaks[0], "stubborn")
	}
}

func TestGoroutineGroup_ActiveNamesAlphabetical(t *testing.T) {
	g := warren.NewGoroutineGroup(context.Background())
	g.Go("zebra", func(ctx context.Context) { <-ctx.Done() })
	g.Go("alpha", func(ctx context.Context) { <-ctx.Done() })
	g.Go("mango", func(ctx context.Context) { <-ctx.Done() })

	// Give goroutines a moment to register
	time.Sleep(10 * time.Millisecond)

	names := g.ActiveNames()
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("names not sorted: %v", names)
		}
	}

	g.Wait(time.Second)
}

func TestGoroutineGroup_CountDecrementsAfterExit(t *testing.T) {
	g := warren.NewGoroutineGroup(context.Background())
	done := make(chan struct{})

	g.Go("counter", func(ctx context.Context) {
		<-done
	})

	time.Sleep(10 * time.Millisecond)
	if g.Active()["counter"] != 1 {
		t.Error("goroutine should be active before done signal")
	}

	close(done)
	time.Sleep(50 * time.Millisecond)

	if g.Active()["counter"] != 0 {
		t.Error("goroutine should not be active after returning")
	}

	g.Stop()
}

func TestGoroutineGroup_ParentContextCancels(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	g := warren.NewGoroutineGroup(parent)
	exited := make(chan struct{})

	g.Go("child", func(ctx context.Context) {
		<-ctx.Done()
		close(exited)
	})

	cancel() // cancel parent — should propagate to g's context

	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("goroutine did not exit after parent context cancel")
	}

	leaks := g.Wait(time.Second)
	if len(leaks) > 0 {
		t.Errorf("unexpected leaks after parent cancel: %v", leaks)
	}
}
