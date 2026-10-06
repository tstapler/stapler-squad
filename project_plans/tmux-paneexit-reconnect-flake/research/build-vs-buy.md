# Research: Build vs Buy — fast-recheck mechanism for `reconnectLoop`

## Question

Should the bounded "fast recheck" decoupled from `reconnectLoop`'s backoff (goal 1
in `requirements.md`) be hand-rolled, or should it lean on an existing
retry/backoff library?

## What's already a dependency

Checked `go.mod` (repo root):

- **`golang.org/x/sync v0.20.0`** — direct dependency, but only its `singleflight`
  subpackage is used anywhere in this repo (`session/git/worktree.go:15`,
  `session/tmux/tmux.go:27`). No `errgroup`/`semaphore` usage found. `singleflight`
  doesn't fit here — the need is "N bounded retries on a timer," not "collapse
  concurrent identical calls."
- **`github.com/cenkalti/backoff/v5 v5.0.3`** — present in `go.mod`, but it's
  **indirect**: `go mod why` traces it to
  `telemetry → otlpmetricgrpc → otlpmetricgrpc/internal/retry → cenkalti/backoff/v5`.
  It's not imported by any of this repo's own packages. Reaching for it here would
  turn a transitive, otel-internal dependency into a direct one just for a ~20-line
  bounded-retry loop — and the requirements explicitly forbid introducing a new
  dependency (`requirements.md` Constraints: "Must not introduce a new
  dependency").

No other retry/backoff library (`avast/retry-go`, `hashicorp/go-retryablehttp`,
etc.) appears anywhere in `go.mod`.

## Existing internal precedent (same package, same shape)

`session/tmux/tmux.go:478` already has a hand-rolled bounded-retry-with-backoff
helper in the *same package* as the file being changed:

```go
// ensureServerRunningWithRetry retries serverStartAttempt up to attempts
// times with exponential backoff (capped at backoffMax) between tries...
func ensureServerRunningWithRetry(startServer func() ([]byte, error), isNotRunning func() bool, attempts int, backoffStart, backoffMax time.Duration) (out []byte, err error) {
	backoff := backoffStart
	for i := 0; i < attempts; i++ {
		ok, o, e := serverStartAttempt(startServer, isNotRunning)
		out, err = o, e
		if ok {
			return out, nil
		}
		if i < attempts-1 {
			time.Sleep(backoff)
			backoff *= 2
			if backoff > backoffMax {
				backoff = backoffMax
			}
		}
	}
	return out, err
}
```

This is direct in-package precedent for exactly this shape of problem (bounded
attempts, private helper, no library) — the new fast-recheck logic should follow
the same convention, not introduce a different pattern.

`reconnectLoop` itself (`server_registry.go:307-400`) already hand-rolls its own
`select { case <-r.ctx.Done(): ...; case <-time.After(backoff): }` pattern for
context-aware sleeping. A fast-recheck loop needs to integrate with `r.ctx` the
same way, which a generic external retry library wouldn't give for free — it
would still need a bespoke context/cancellation wrapper around it either way.

## `testutil/wait.go`'s `RetryOperation` / `WaitForCondition`

`testutil/wait.go:178` has `RetryOperation(operation func() error, config
WaitConfig)` and `WaitForCondition`. These are **test-only helpers** (package
`testutil`, imported only by `_test.go` files across the repo). Two problems with
reusing them here:

1. Layering: `session/tmux/server_registry.go` is production code; having it
   import `testutil` would invert the intended dependency direction (tests depend
   on `testutil`, not production code).
2. Shape mismatch: they poll a `func() bool` / `func() error` on a fixed
   `PollInterval` for up to `Timeout`, with no support for the specific ceiling
   math the requirements ask for (`fastRecheckAttempts ×
   (fastRecheckSyncTimeout + fastRecheckInterval) = 700ms`, goal 2) or for
   integrating with `r.ctx.Done()` / `reconnectLoop`'s existing goroutine
   lifecycle.

No other internal retry/polling helper was found outside `session/tmux/tmux.go`
and `testutil/wait.go` (checked via `grep -rn "func.*Retry\|func.*Poll\|func.*Backoff"`
across `session/`, `server/`, `github/`) — the rest are unrelated `*Poller` types
(`PRStatusPoller`, `ReviewQueuePoller`, etc.) with their own independent polling
loops for GitHub/PR status, not something a control-mode reconnect fast-path
should couple to.

## Recommendation

**Hand-roll it.** A private ~10-30 line helper alongside `reconnectLoop`, using
the same `select`/`time.After`/bounded-attempts idiom as
`ensureServerRunningWithRetry` in the same package. Reasons:

1. The only two "available" retry libraries are either the wrong shape
   (`singleflight`) or not actually a direct dependency yet (`cenkalti/backoff/v5`),
   and pulling the latter in directly would violate the explicit
   no-new-dependency constraint (making it direct is effectively adding a new
   dependency edge, even though the module is already in `go.sum`).
2. There's already an in-package convention for exactly this problem shape
   (`ensureServerRunningWithRetry`) — consistency argues for following it, not
   diverging to a library with its own config/API surface for a single call site.
3. The logic is tightly coupled to private registry state (`r.ctx`, `r.healthy`,
   `syncSessions()`, `subsMu` discipline) that a generic library wouldn't reduce —
   the loop-and-sleep part a library would provide is already the easy 20% of
   this; the correctness-sensitive part is the ctx-aware interruption and the
   `subsMu`/close-outside-lock discipline, which has to be hand-written either way.
4. Scope: single call site, no near-term second use — reaching for a dependency
   here would itself be the kind of speculative abstraction this repo's own
   `interface-pollution-checklist.md` warns against (smell #5, unjustified
   generic/dependency for a single call site).
