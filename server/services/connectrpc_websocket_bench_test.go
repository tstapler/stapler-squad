package services

// Benchmarks for the terminal-jank elimination changes:
//   - waitForQuiescence: replaces the hard-coded 250ms sleep on cold connect
//   - snapshotCache:     caches tmux capture-pane output, invalidated on %output
//
// NOTE: This file is in package services alongside session_service.go, which has
// a pre-existing interface-compliance error (var _ SessionServiceHandler = ...).
// These benchmarks will compile and run once that pre-existing error is resolved.
//
// Run with:
//   go test -bench=BenchmarkWaitForQuiescence ./server/services/ -benchmem -benchtime=3s
//   go test -bench=BenchmarkSnapshot         ./server/services/ -benchmem -benchtime=3s

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
)

// ---------------------------------------------------------------------------
// waitForQuiescence benchmarks
//
// waitForQuiescence replaces the fixed 250 ms sleep that previously preceded
// every tmux capture-pane call on cold connect.  The two cases below model
// the two production scenarios:
//
//   ImmediatelyIdle  – session has no active output; quiet timer fires after
//                      quietFor with zero channel reads.  This was previously
//                      always charged the full 250 ms; now it costs ≈ quietFor.
//
//   BurstThenQuiet   – Claude TUI is repainting after SIGWINCH; N update
//                      events arrive, each resetting the quiet timer, then the
//                      channel drains and the timer fires.  The total wait is
//                      proportional to actual redraw duration, not a fixed cap.
// ---------------------------------------------------------------------------

// BenchmarkWaitForQuiescence_ImmediatelyIdle measures the idle-session fast
// path: the channel receives no events so waitForQuiescence returns after
// exactly quietFor.  Use a short quietFor so the bench iteration count is high
// enough for stable numbers.
func BenchmarkWaitForQuiescence_ImmediatelyIdle(b *testing.B) {
	const quietFor = 5 * time.Millisecond
	const timeout = 500 * time.Millisecond

	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ch := make(chan struct{}, 16)
		waitForQuiescence(ch, timeout, quietFor)
	}
}

// BenchmarkWaitForQuiescence_BurstThenQuiet models an active Claude TUI
// session: numUpdates events are pre-loaded so they arrive at nanosecond
// intervals (simulating rapid repaint), then the channel drains and
// waitForQuiescence returns after quietFor.
func BenchmarkWaitForQuiescence_BurstThenQuiet(b *testing.B) {
	const quietFor = 5 * time.Millisecond
	const timeout = 500 * time.Millisecond
	const numUpdates = 8 // typical TUI emits 3–8 redraws after SIGWINCH

	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ch := make(chan struct{}, 16)
		for j := 0; j < numUpdates; j++ {
			ch <- struct{}{}
		}
		waitForQuiescence(ch, timeout, quietFor)
	}
}

// ---------------------------------------------------------------------------
// snapshotCache benchmarks
//
// The snapshot cache stores the last tmux capture-pane result per session.
// It is invalidated (dirty = true) whenever a %output event arrives from
// tmux control mode.  On connect, getOrRefreshSnapshot returns the cached
// value if clean or calls the capture function if dirty.
//
// Hot paths:
//   Hit  – most connects after the first; captureFn is never called.
//   Miss – first connect, or after new terminal output.
//   Dirty – called in the streaming goroutine on every %output event.
// ---------------------------------------------------------------------------

// BenchmarkSnapshotCacheHit measures getOrRefreshSnapshot when the cache is
// warm and clean.  captureFn must never be invoked; the benchmark verifies
// this post-run.
func BenchmarkSnapshotCacheHit(b *testing.B) {
	h := &ConnectRPCWebSocketHandler{
		snapshotCache: xsync.NewMapOf[string, sessionSnapshot](),
	}
	cachedContent := strings.Repeat("x", 4096) // ~4 KB: typical terminal screen
	h.snapshotCache.Store("bench-session", sessionSnapshot{
		content:    cachedContent,
		capturedAt: time.Now(),
		dirty:      false,
	})

	captureCallCount := 0
	captureFn := func() (string, error) {
		captureCallCount++
		return "fresh", nil
	}

	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		content, err := h.getOrRefreshSnapshot("bench-session", captureFn)
		if err != nil || len(content) == 0 {
			b.Fatalf("unexpected result: err=%v content_len=%d", err, len(content))
		}
	}

	b.StopTimer()
	if captureCallCount > 0 {
		b.Fatalf("cache hit path invoked captureFn %d times (expected 0)", captureCallCount)
	}
}

// BenchmarkSnapshotCacheMiss measures getOrRefreshSnapshot when the snapshot
// is dirty.  captureFn is a fast stub to isolate cache-layer overhead from
// the real tmux exec cost (which is 20–80 ms and dominates in production).
func BenchmarkSnapshotCacheMiss(b *testing.B) {
	h := &ConnectRPCWebSocketHandler{
		snapshotCache: xsync.NewMapOf[string, sessionSnapshot](),
	}
	freshContent := strings.Repeat("x", 4096)
	captureFn := func() (string, error) {
		return freshContent, nil
	}

	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Force a miss on every iteration by marking dirty before the call.
		h.snapshotCache.Store("bench-session", sessionSnapshot{dirty: true})

		content, err := h.getOrRefreshSnapshot("bench-session", captureFn)
		if err != nil || len(content) == 0 {
			b.Fatalf("unexpected result: err=%v content_len=%d", err, len(content))
		}
	}
}

// BenchmarkMarkSnapshotDirty measures the hot path called on every tmux
// %output event in the streaming goroutine.  Each active session receives
// many events per second during Claude TUI repaints, so this must be cheap.
func BenchmarkMarkSnapshotDirty(b *testing.B) {
	h := &ConnectRPCWebSocketHandler{
		snapshotCache: xsync.NewMapOf[string, sessionSnapshot](),
	}
	h.snapshotCache.Store("bench-session", sessionSnapshot{
		content:    strings.Repeat("x", 4096),
		capturedAt: time.Now(),
		dirty:      false,
	})

	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		h.markSnapshotDirty("bench-session")
	}
}

// BenchmarkMarkSnapshotDirty_Concurrent measures markSnapshotDirty under
// concurrent load from multiple goroutines, each representing a different
// active session streaming output simultaneously.
func BenchmarkMarkSnapshotDirty_Concurrent(b *testing.B) {
	const numSessions = 8 // typical pool size
	h := &ConnectRPCWebSocketHandler{
		snapshotCache: xsync.NewMapOf[string, sessionSnapshot](),
	}
	for i := 0; i < numSessions; i++ {
		sessionID := fmt.Sprintf("bench-session-%02d", i)
		h.snapshotCache.Store(sessionID, sessionSnapshot{
			content:    strings.Repeat("x", 4096),
			capturedAt: time.Now(),
			dirty:      false,
		})
	}

	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			sessionID := fmt.Sprintf("bench-session-%02d", i%numSessions)
			h.markSnapshotDirty(sessionID)
			i++
		}
	})
}
