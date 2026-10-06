# Research: Stack

## Go version / module deps

- `go.mod:3` — `go 1.26.3`. `context.AfterFunc` (added 1.21) and all stdlib
  timer/context primitives are available.
- No new third-party deps needed or justified — `session/tmux/server_registry.go`
  already imports only stdlib (`bufio`, `context`, `errors`, `fmt`, `io`,
  `os/exec`, `runtime`, `strings`, `sync`, `time`) plus two internal packages
  (`executor/safeexec`, `log`). The fix must stay stdlib-only per the
  requirements' "must not introduce a new dependency" constraint.

## Existing idiom in this file/package (what to imitate)

`reconnectLoop` (`session/tmux/server_registry.go:307-400`) already establishes
this repo's idiom for bounded/backoff waits: a `for` loop with a `select` on
`r.ctx.Done()` vs. `time.After(backoff)`, no `time.Timer`/`time.Ticker` value
kept around, no `sync/atomic`. The sibling production poller
`session/review_queue_poller.go` (`pollLoop`, line ~335) uses the exact same
`select { case <-ctx.Done(): ...; case <-time.After(backoff): ... }` shape for
its own error-backoff. This is the dominant pattern across `session/` for
bounded polling — **not** `time.Ticker` (tickers fire indefinitely and are for
steady-state periodic work, not a capped N-attempt retry) and **not**
`sync/atomic` (no repo precedent for a hand-rolled atomic flag where a channel
close or `ctx.Done()` already expresses the same "stop" signal).

**Recommended primitive for the fast-recheck mechanism**: a small bounded loop
using `time.After(fastRecheckInterval)` inside a `select` against `r.ctx.Done()`,
capped at `fastRecheckAttempts` iterations, each iteration calling
`syncSessions()` under a `context.WithTimeout(..., fastRecheckSyncTimeout)`
(mirroring the existing hardcoded `10*time.Second` timeout pattern already in
`syncSessions()` at `server_registry.go:211-212` — that will need to become
parameterized, or the fast-recheck path needs its own short-timeout variant,
since 10s is far larger than the 700ms total ceiling required by AC 5).

This should run as its own goroutine, started when `reconnectLoop` enters the
post-failure backoff `time.After(backoff)` wait (both call sites at
`server_registry.go:~326` and `~394`), so it executes *concurrently* with —
not sequentially after — the backoff sleep. That's the actual mechanism that
decouples detection latency from backoff, per requirements Goal 1.

No `sync/atomic` needed to coordinate stop/success: since the goroutine is
self-bounding (fixed attempt count × fixed interval = 700ms hard ceiling) and
idempotent (`syncSessions()` is already safe to call concurrently — it takes
`r.mu` internally per the existing "replaces the in-memory map atomically"
comment at `server_registry.go:209`), the simplest and most idiomatic choice
is to just let it run to completion or `r.ctx.Done()`, rather than adding a
second coordination channel to short-circuit it early when the main reconnect
succeeds first. That keeps the diff minimal (AC 6: confined to
`server_registry.go` + its integration test) and avoids introducing new
synchronization primitives beyond what the file already uses.

`context.AfterFunc` was considered as a lighter-weight alternative to a
manually spawned goroutine + `select` loop, but rejected: it doesn't naturally
express "N bounded attempts with a per-attempt sub-timeout," and would be a
stylistic outlier next to the existing `for { select { ... time.After ... } }`
idiom used twice already in this same file.

## Internal polling/retry helpers — none reusable for the production fix

- `testutil/wait.go` and `testutil/wait/wait.go` both provide
  `WaitForCondition(condition func() bool, config WaitConfig)` /
  `WaitForConditionWithError` — polling helpers built on `time.Ticker` +
  `context.WithTimeout`. These are **test-only** utilities (package
  `testutil`, imported by test files across `session/`, `session/tmux/`,
  `session/mux/`, `session/detection/...`). They are appropriate to reuse in
  the new `TestTmuxServerRegistry_PaneExitDetectedDespiteElevatedBackoff`
  regression test (e.g. to poll for the pane-exit channel closing within the
  700ms ceiling instead of a raw `time.After`/`select`), but must **not** be
  imported into `server_registry.go` itself — that's production code, and
  `testutil` importing patterns exist specifically to avoid pulling test-only
  packages into non-test code (`testutil/wait/wait.go`'s own doc comment notes
  it exists as a *second* copy specifically to dodge an import cycle with the
  `session` package — i.e., even test helpers here are deliberately kept out
  of production import graphs).
- No hand-rolled retry/backoff helper exists elsewhere in `session/` or
  `session/tmux/` that fits "bounded N attempts with fixed interval" — the
  other `retry`/`backoff`-named functions found repo-wide
  (`server/services/connectrpc_websocket.go`, `session/backlog_lifecycle.go`,
  `session/external_discovery.go`, `session/session_driver.go`,
  `session/worktree_pr_poller.go`, `session/tmux/tmux.go`,
  `session/review_queue_poller.go:backoffDuration`) are each local,
  purpose-built backoff calculators/loops embedded in their own files, not a
  shared utility — consistent with this codebase's general aversion to
  premature shared abstractions (see
  `.claude/rules/interface-pollution-checklist.md`). This further supports
  hand-writing the fast-recheck loop directly in `server_registry.go` rather
  than searching for/introducing a shared helper.

## Concurrency-safety note for implementation

`syncSessions()` (`server_registry.go:210+`) takes `r.mu` (a `sync.RWMutex`)
before mutating `r.sessions`, and pane-exit firing already follows the
documented "copy subscribers out under `subsMu`, close outside the lock"
discipline (`server_registry.go:48-50`, enforced by the `paneExitSub.once`
`sync.Once` at line ~35). A concurrently running fast-recheck goroutine calling
`syncSessions()` while `reconnectLoop`'s own post-reconnect `syncSessions()`
call (line ~343) is in flight is therefore safe as long as the new code path
reuses `syncSessions()` unmodified (or a thin variant) and does not bypass
`r.mu` or the `subsMu`-then-close-outside ordering.
