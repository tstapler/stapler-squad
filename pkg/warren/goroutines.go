package warren

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// GoroutineGroup tracks named background goroutines spawned during application
// startup. It provides:
//
//   - Context propagation: every goroutine receives a context that is cancelled
//     when the group is stopped.
//   - Leak detection: Wait() returns the names of any goroutines that did not
//     exit within the configured timeout.
//   - Multiplicity tracking: multiple goroutines may share a name; the Active()
//     report shows each name once with a count.
//
// Typical use is through [App.Go]. Use GoroutineGroup directly when you need
// goroutine tracking inside a service that is unaware of the App lifecycle.
type GoroutineGroup struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	active map[string]int // name -> running count
	wg     sync.WaitGroup
}

// NewGoroutineGroup creates a GoroutineGroup whose context is derived from
// parent. Cancelling parent also cancels the group's internal context.
func NewGoroutineGroup(parent context.Context) *GoroutineGroup {
	ctx, cancel := context.WithCancel(parent)
	return &GoroutineGroup{
		ctx:    ctx,
		cancel: cancel,
		active: make(map[string]int),
	}
}

// Go spawns a named tracked goroutine. fn receives the group's context; it
// must return when that context is cancelled.
//
// Multiple goroutines may be registered under the same name (e.g. per-session
// pollers). Active() reports the count per name.
func (g *GoroutineGroup) Go(name string, fn func(ctx context.Context)) {
	g.mu.Lock()
	g.active[name]++
	g.wg.Add(1)
	g.mu.Unlock()

	go func() {
		defer func() {
			g.mu.Lock()
			g.active[name]--
			if g.active[name] == 0 {
				delete(g.active, name)
			}
			g.mu.Unlock()
			g.wg.Done()
		}()
		fn(g.ctx)
	}()
}

// Active returns a snapshot of currently running goroutine names and their
// counts, sorted alphabetically.
func (g *GoroutineGroup) Active() map[string]int {
	g.mu.Lock()
	defer g.mu.Unlock()
	snapshot := make(map[string]int, len(g.active))
	for k, v := range g.active {
		snapshot[k] = v
	}
	return snapshot
}

// ActiveNames returns the sorted list of names with at least one running goroutine.
func (g *GoroutineGroup) ActiveNames() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	names := make([]string, 0, len(g.active))
	for name := range g.active {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Context returns the group's context. Goroutines can use this directly, or
// they can use the context passed to Go's fn parameter (which is the same value).
func (g *GoroutineGroup) Context() context.Context {
	return g.ctx
}

// Stop cancels the group context. Goroutines that respect context cancellation
// will begin shutting down.
func (g *GoroutineGroup) Stop() {
	g.cancel()
}

// Wait stops the group and blocks until all goroutines exit or timeout elapses.
// Returns the names of any goroutines still running after the timeout (leaks).
// An empty slice means clean shutdown.
func (g *GoroutineGroup) Wait(timeout time.Duration) []string {
	g.cancel()

	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return g.ActiveNames()
	}
}

// leakReport formats leaked goroutine names for an error message.
func leakReport(names []string, timeout time.Duration) string {
	if len(names) == 0 {
		return ""
	}
	return fmt.Sprintf("goroutine leaks after %s timeout: %v", timeout, names)
}
