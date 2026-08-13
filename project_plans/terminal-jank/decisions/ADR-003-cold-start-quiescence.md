# ADR-003: Quiescence Detector Over Fixed Sleep for Cold-Start Timing

**Status**: Accepted
**Date**: 2026-04-09
**Deciders**: Tyler Stapler
**Relates to**: Terminal Jank Elimination (Story 2)

## Context

The cold-start path in `streamViaControlMode()` performs a +/-1 nudge resize to force Claude's TUI to redraw, then waits a fixed `time.Sleep(200 * time.Millisecond)` before calling `tmux capture-pane`. This fixed sleep is a worst-case guess that is wrong in both directions:

- **Idle sessions** (shell prompt, no active process): redraw completes in <10ms, but the server waits 200ms anyway.
- **Active sessions** (Claude streaming output): the 200ms may not be enough for a complex TUI redraw, resulting in a partial or stale capture.

The 200ms sleep is the single largest contributor to cold-start latency, which totals 500-1200ms. With the terminal pool (ADR-001), cold starts only affect first-view sessions and evicted sessions, but they still need to be fast to avoid a visible delay.

Three timing strategies were evaluated:

1. **Fixed sleep (current)**: `time.Sleep(200ms)` after resize. Simple, racy in both directions.
2. **Output quiescence detector**: Monitor the control mode `%output` event stream after resize. Wait until no output arrives for 50ms (quiet window), with a 500ms hard cap. Adaptive to actual TUI redraw speed.
3. **PTY byte counting**: After resize, monitor the raw PTY byte stream and wait until N bytes have been received. Fragile: byte count depends on terminal dimensions, content complexity, and encoding.

## Decision

We chose Option 2: output quiescence detector with a 50ms quiet window and 500ms hard cap.

```go
func waitForQuiescence(updates <-chan struct{}, timeout time.Duration, quietFor time.Duration) {
    deadline := time.After(timeout)
    quiet := time.NewTimer(quietFor)
    defer quiet.Stop()
    for {
        select {
        case _, ok := <-updates:
            if !ok { return }
            if !quiet.Stop() {
                select {
                case <-quiet.C:
                default:
                }
            }
            quiet.Reset(quietFor)
        case <-quiet.C:
            return
        case <-deadline:
            return
        }
    }
}
```

The control mode `%output` events are the natural signal: tmux fires `%output` for every write from the pane process. When the TUI finishes redrawing after SIGWINCH, output stops flowing. A 50ms quiet window is long enough to distinguish "redraw complete" from "brief pause between render frames" (Claude's TUI typically completes redraws in a single burst).

This is paired with a per-session snapshot cache (Task 2.2) that avoids running `capture-pane` entirely when the session has had no output since the last capture.

## Consequences

### Positive

- Idle sessions cold-start in ~50ms (one quiet window) instead of 250ms. This is a 5x improvement for the common case of switching to a session where Claude is waiting for input.
- Active sessions get a correct snapshot: the detector waits until the TUI actually finishes redrawing rather than guessing.
- The 500ms hard cap prevents pathological cases (continuous streaming output) from blocking the connection indefinitely.
- Combined with the snapshot cache, unchanged sessions cold-start in ~0ms (cached content served directly).

### Negative

- The quiescence detector introduces a dependency on the control mode `%output` event channel. If control mode is not active for a session (e.g., external sessions, non-tmux sessions), the detector must fall back to a fixed timeout.
- There is a residual race window: the `%output` events flow through Go channels and may lag behind the actual PTY writes by the Go scheduler quantum (~1-10ms). This means the quiet window could fire slightly before the last bytes have been written to the pane buffer, causing `capture-pane` to miss the final line. In practice, this is unlikely because `capture-pane` reads from tmux's internal buffer, not the PTY, and tmux processes `%output` only after writing to its buffer.
- The detector consumes one goroutine per cold-start connection during the quiescence wait. This goroutine exits after the wait completes (~50-500ms).

### Neutral

- The snapshot cache adds ~1KB of memory per session (cached capture-pane output). This is negligible.
- The `dirty` flag on the snapshot cache is set on every `%output` event. For high-output sessions, this means the cache is almost always dirty. The cache is most beneficial for idle sessions, which is the common case for session switching.

## Patterns Applied

- **Event-Driven Readiness**: Replace timing guesses with signal-based coordination. The quiescence detector responds to actual system behavior (output flow) rather than estimated durations.
- **Circuit Breaker / Hard Cap**: The 500ms deadline prevents the detector from blocking indefinitely on pathological input (continuous streaming). This follows the "fail fast with bounded wait" pattern from Michael Nygard's "Release It!"
- **Cache with Invalidation**: The snapshot cache follows the standard write-through invalidation pattern: cached data is served directly; writes (output events) invalidate the cache; the next read triggers a refresh.
