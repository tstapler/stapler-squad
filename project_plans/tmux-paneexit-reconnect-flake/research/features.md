# Research: Features / Edge Cases — fast-recheck for pane-exit detection

## 1. Existing "detect state change independent of a slow retry loop" patterns

The canonical idiom in this codebase for a decoupled background poll is
`StartZombieReaper` (`session/tmux/zombie_reaper.go:20-32`):

```go
func StartZombieReaper(ctx context.Context, interval time.Duration, logFn func(string, ...any)) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if n := reapZombieChildren(); n > 0 { ... }
			}
		}
	}()
}
```

Shape to follow: `time.NewTicker` + `defer ticker.Stop()` + `select { ctx.Done() / ticker.C }`,
launched as its own goroutine, taking a `context.Context` for cancellation (never a raw
`time.Sleep` loop). `session/hibernation_sweeper.go:178` uses the identical ticker+ctx.Done
shape for its sweep loop. Neither of these is bounded-count (they run for the process
lifetime), so **neither is a drop-in template for the bounded ceiling this fix needs** — the
fast-recheck here must stop itself after `fastRecheckAttempts` fires, not just on `ctx.Done()`.
No existing "N bounded fast attempts, then give up" pattern exists elsewhere in `session/` or
`server/services/` — this will be a new (small) pattern, not a reuse of one.

`review_queue_poller.go:121` and `session/git/worktree.go:69` are `ponytail:` comments
documenting lock-free-read tradeoffs (xsync.Map / atomic.Value), not concurrency-timing
ceilings — useful as *comment-style* precedent (name the mechanism + why), but not as
*design* precedent for a bounded retry.

## 2. Edge cases the fast-recheck design must handle

Grounded in the actual `TmuxServerRegistry` struct (`session/tmux/server_registry.go:42-58`)
and its concurrency discipline (`subsMu` comment at lines 48-50: never `close(ch)` while
holding `subsMu`):

- **`Stop()` during a fast-recheck cycle.** `Stop()` (`server_registry.go:101-120`) calls
  `r.cancel()` first, which cancels `r.ctx` — the same context `reconnectLoop` selects on.
  A fast-recheck goroutine/ticker must also select on `r.ctx.Done()` (not a locally-derived
  context that isn't wired to cancel) or it leaks past `Stop()`. Additionally, `Stop()`
  already closes all subscribers directly; a fast-recheck's own `syncSessions()` call racing
  concurrently with `Stop()`'s subscriber-close is safe *only if* `firePaneExit` tolerates
  operating on an already-emptied `r.subscribers` map (it does — `subs := r.subscribers[sessionName]`
  on a missing key just yields `nil`, ranging over `nil` is a no-op). No new lock ordering
  hazard as long as fast-recheck reuses the existing `syncSessions()`/`firePaneExit` methods
  rather than reimplementing session-diffing.
- **Multiple concurrent `SubscribePaneExit` calls** for the same or different session names
  while a fast-recheck cycle is in flight. `syncSessions()` already handles this correctly
  today (locks `r.mu`, computes `disappeared`, unlocks, then fires) — a fast-recheck is just
  another caller of the same `syncSessions()` method, so no new subscriber-list race is
  introduced *if* the fast-recheck literally calls `r.syncSessions()` rather than a parallel
  reimplementation. Risk only appears if the fast-recheck is implemented as a **second**
  code path that duplicates the diff logic — don't do that.
- **A session that both appears and disappears within one fast-recheck window.** Since
  `syncSessions()` (`server_registry.go:210-246`) only diffs against the map's snapshot at
  call time, a session created and destroyed entirely between two fast-recheck ticks is
  invisible to both diff and to any `SubscribePaneExit` caller that subscribed after the
  session was already gone — this is pre-existing behavior (same blind spot exists for the
  slow path today) and out of scope per the requirements' non-goals; the fast-recheck must
  not be asked to fix it, only to shrink the *worst-case latency window* in which a
  still-registered session's exit goes undetected.
- **Registry never having connected at all (server never reachable).** `Start()`
  (`server_registry.go:77-98`) calls `syncSessions()` once synchronously before
  `reconnectLoop` even starts, then `reconnectLoop`'s first iteration is a normal
  `startControlMode()` attempt — if that repeatedly fails, `reconnectLoop` sits in its
  `err != nil` branch's own `backoff` wait (lines 324-338), which is a **different** select
  from the post-connect "readLines returned" backoff wait (lines 382-398). A fast-recheck
  keyed only off the "control-mode dropped, waiting to reconnect" path could miss the
  case where control-mode has *never yet* connected once. The design should trigger
  fast-recheck attempts from both backoff waits (or from a single shared point both paths
  route through), not just the post-connection-drop one, or a session created and killed
  entirely before the first successful `startControlMode()` won't get the fast-recheck ceiling.
- **Goroutine leaks if fast-recheck isn't cancelled properly.** Because `reconnectLoop` runs
  for the life of the registry and each backoff wait is transient, the natural
  implementation is to start the fast-recheck timer *inline* inside the existing
  `select { <-r.ctx.Done(); <-time.After(backoff) }` blocks (e.g. replace the bare
  `time.After(backoff)` wait with a small helper that races bounded fast-recheck ticks
  against the backoff deadline) rather than spawning a long-lived separate goroutine per
  backoff cycle. If a separate goroutine approach is chosen instead, it must be scoped with
  its own child context derived from `r.ctx` and torn down (via defer/cancel) the instant
  the outer backoff wait resolves (either by reconnect succeeding or `ctx.Done()`) — an
  attempt counter alone doesn't guarantee cleanup if the outer wait exits early.

## 3. Failure modes of a naive `time.Ticker` calling `syncSessions()`

- **Process fork amplification.** `syncSessions()` (`server_registry.go:213-215`) does a real
  `safeexec.CommandContext(ctx, Binary(), args...)` fork/exec of the tmux binary
  (`list-sessions -F ...`) with a 10s timeout per call. An *unbounded* ticker (e.g. "recheck
  every 100ms for as long as backoff is active") during a 30s-capped backoff would spawn up to
  ~300 tmux subprocesses per stalled reconnect cycle — multiplied across every registry
  instance (`globalRegistries` in `server_registry.go:527` is one entry per socket, and CI/dev
  can have many isolated test sockets alive concurrently per `-p` parallelism). This is exactly
  the "fork-rate explosion" the existing `minStableConnection` reset-guard comment at
  `server_registry.go:362-365` already calls out as a concern for the *reconnect* backoff —
  the fast-recheck must not reintroduce that risk on the *detection* side. The requirements'
  explicit ceiling (`fastRecheckAttempts × (fastRecheckSyncTimeout + fastRecheckInterval) =
  700ms`, AC 5) is precisely the bound that prevents this: a small fixed attempt count, not an
  open-ended ticker.
- **`-race` detector overhead compounding subprocess cost.** Each `safeexec.CommandContext`
  fork/exec is already slow under `-race` (instrumented runtime, larger memory footprint per
  goroutine); a fast-recheck that fires too aggressively (short interval, high attempt count)
  risks the *fast-recheck itself* not completing within its own budget under `-race`-detector
  and CI-shared-runner load — i.e., the fix could reintroduce a version of the same "detection
  bound by external timing" flake it's meant to eliminate, just with a smaller (700ms) but
  still real risk surface if `fastRecheckSyncTimeout` is set too tight relative to real
  `list-sessions` latency under load. Keep `fastRecheckSyncTimeout` generous enough to survive
  `-race`+CI-load fork/exec latency, not just aiming for the minimum that passes locally.
- **CI resource limits (ulimit -u / cgroup pids.max).** Every additional subprocess spawn
  during a flaky-test rerun (`-count=20`, per AC 1) multiplies register-wide fork pressure;
  with the 700ms ceiling and a small fixed attempt count this stays bounded and proportional
  to one detection event, not to backoff duration — the naive infinite-ticker design has no
  such bound and would scale badly with `backoffCap` (30s).

## 4. Regression risk to existing tests from more frequent `syncSessions()` calls

- **`TestEnsureServerRunning_NoOp`** (`session/tmux/tmux_test.go:252`) and
  **`TestKillOrphanedControlModeClients`** (`session/tmux/kill_orphaned_control_mode_clients_test.go:22`)
  do not exercise `TmuxServerRegistry`/`reconnectLoop` at all — they test `EnsureServerRunning`
  and `KillOrphanedControlModeClients` respectively, separate functions in the `tmux` package
  unrelated to `server_registry.go`'s reconnect/backoff machinery. A fast-recheck confined to
  `TmuxServerRegistry.reconnectLoop`'s backoff waits cannot regress either — confirmed by
  grepping both test files for `TmuxServerRegistry`/`syncSessions`/`reconnectLoop`: no matches.
  Both are named in AC 3 purely as "confirm still green," not because the fix touches their
  code paths.
- **Unit tests in `session/tmux/server_registry_test.go`** (`TestRegistry_*`, using
  `newTestRegistry`, `server_registry_test.go:18-30`) construct a `TmuxServerRegistry` via
  `NewTmuxServerRegistry("")` and manually set `r.healthy = true`, **bypassing
  `Start()`/`reconnectLoop()`/`startControlMode()` entirely** — events are injected via a fake
  pipe into `readLines`. Since `reconnectLoop` never runs in these tests, a fast-recheck
  wired into `reconnectLoop`'s backoff-wait branches cannot fire during them — zero regression
  risk to this file from added ticker/subprocess calls.
- **Integration tests in `server_registry_integration_test.go`** all use
  `startIsolatedRegistry` (line 75) with a real isolated tmux socket and go through the real
  `Start()`/`reconnectLoop()` path, so they *will* exercise the fast-recheck machinery. The
  existing `registryPollTimeout = 3 * time.Second` constant (line 107) used by every
  `pollUntil` call in the file gives ample headroom over the new 700ms fast-recheck ceiling —
  no existing assertion window needs to shrink or grow. `TestTmuxServerRegistry_ConcurrentSubscriptions`
  (line ~219-260, 10 concurrent `SubscribePaneExit` calls before `kill-session`) is the most
  relevant existing regression check for edge case 2 above (concurrent subscribers) and should
  continue to pass unmodified since fast-recheck only changes *when* `syncSessions()`/
  `firePaneExit` run, not their locking behavior.

## Sources

- `session/tmux/server_registry.go:42-58` (struct + concurrency discipline comment)
- `session/tmux/server_registry.go:77-98` (`Start`)
- `session/tmux/server_registry.go:101-120` (`Stop`)
- `session/tmux/server_registry.go:159-194` (`SubscribePaneExit`)
- `session/tmux/server_registry.go:196-207` (`firePaneExit`)
- `session/tmux/server_registry.go:209-246` (`syncSessions`)
- `session/tmux/server_registry.go:307-400` (`reconnectLoop`, both backoff branches)
- `session/tmux/zombie_reaper.go:20-32` (ticker+ctx.Done idiom precedent)
- `session/hibernation_sweeper.go:178` (second ticker+ctx.Done precedent)
- `session/tmux/server_registry_test.go:18-30` (`newTestRegistry` bypasses reconnectLoop)
- `session/tmux/server_registry_integration_test.go:72-260` (`startIsolatedRegistry`,
  `registryPollTimeout`, `pollUntil`, `TestTmuxServerRegistry_PaneExitChannel`,
  `TestTmuxServerRegistry_ConcurrentSubscriptions`)
- `session/tmux/tmux_test.go:252` (`TestEnsureServerRunning_NoOp` — confirmed unrelated
  function, no `TmuxServerRegistry` reference)
- `session/tmux/kill_orphaned_control_mode_clients_test.go:22`
  (`TestKillOrphanedControlModeClients` — confirmed unrelated function, no
  `TmuxServerRegistry` reference)
