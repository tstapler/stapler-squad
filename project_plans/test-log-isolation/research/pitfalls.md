# Research: Pitfalls and Risks — test-log-isolation

Agent 4 (Pitfalls), SDD Phase 2. All file/line references verified against
`origin/main` via `git show`/`git grep origin/main` (this worktree is ~559
commits behind — see requirements.md's note).

## 1. Common mistakes when teams "fix" shared-global-logger test races

**(a) Opt-in synchronization instead of structural safety — confirmed anti-pattern, and confirmed insufficient here.**
`slogDefaultMu` (`server/services/autonomous_orchestration_service_test.go:414`)
is a textbook opt-in mutex: it only serializes tests that know the convention
exists and explicitly take it around their own swap-to-restore window (the
doc comment at `:414-417` names the exact call sites that cooperate:
`captureLogs`, `captureInfoLog`/`captureErrorLog` in
`session_service_client_log_test.go`, and inline swaps in
`search_service_test.go`/`slack_notifier_test.go`). `TestAnthropicAIClient_Complete_CancelsOnCtxDone`
never touches `slog` and has no reason to take `slogDefaultMu` — it's not in
the set of "tests that know." This is the general failure mode of opt-in
locking: the lock only protects the participants who remember to ask for it,
and the set of non-participants is unbounded (any future test that starts a
background goroutine, timer, or slow-closing resource). It is a known
anti-pattern (cf. "shared mutable global + convention-based access" vs.
"encapsulate the state so misuse doesn't compile/run") — the fix has to make
the *default* safe, not add one more door that only careful people walk
through.

**(b) Disabling `-race` or `t.Parallel()` as a workaround.** Both are
explicitly rejected in requirements.md's Constraints ("must not weaken
`-race` coverage… not `-race`-disabling the affected test file") and
Alternatives Considered ("serialize the whole package's test run… papers
over the actual bug rather than fixing it — rejected"). Worth restating why
in the pitfalls doc: `-race`-disabling a file removes coverage for every
*other* race that file's tests might also carry, not just this one — a
common way "known flake" tickets quietly widen a blind spot. `-p 1`/no
`t.Parallel()` package-wide would fix the symptom but permanently taxes
`make ci` for the whole package, and does nothing for any other package that
adopts the same `captureLogs` pattern later.

**(c) Partial migration to a scoped logger, leaving a residual race.** This
is the single highest-probability failure mode for Direction 1. Two ways it
happens in practice, both plausible here given 386 call sites route through
`server/services`'s own `log` wrapper around `slog.Default()`:
  - Some call sites get the injected/scoped logger threaded through; others
    (especially ones reached via a shared helper, a background goroutine
    spawned before the scoped logger was available, or code in another
    package like `session/` that still calls `slog.Default()`/`log.Print`
    directly) keep reading the process-global default. The race moves, it
    doesn't disappear — now it's between the *un-migrated* call sites and
    `captureLogs`.
  - Even a **complete** migration of this codebase's own call sites does not
    close the race, because of point 3 below: stdlib itself (via
    `net/http/httptest`) calls `log.Print`, which is unreachable by any
    scoped-logger DI this codebase does. This is not a partial-migration bug,
    it's a hole in the whole direction — see §3.

## 2. Other candidate triggers in `server/services/*_test.go` beyond the two confirmed instances

Enumerated via `git grep origin/main -n "httptest.NewServer\|go func("  -- server/services`
(command run; full match list retained in this research, ~40 `httptest.NewServer`
call sites and ~90 `go func(` sites across the package as of `origin/main`).
Spot-checked for the specific pattern that makes `anthropic_client_test.go`
dangerous — a **handler that can block past 5s with a plain `defer
srv.Close()`/`t.Cleanup(srv.Close)`** (the precondition for tripping
`httptest.Server.Close()`'s internal hang detector):

- **`callback_dispatcher_test.go:79`** (`TestCallbackDispatcher_Dispatch_NonBlocking`)
  and **`callback_dispatcher_test.go:113`** (`TestCallbackDispatcher_Dispatch_DropsBeyondCapacity`)
  both spin a handler that blocks on `<-block` indefinitely until the test
  explicitly `close(block)`s it, with a bare `defer srv.Close()`. **Lower risk
  than the confirmed instance, but not zero**: both tests do call
  `close(block)` before returning today (verified: `NonBlocking` closes it
  after its elapsed-time assertion and waits `require.Eventually` for
  in-flight drain; `DropsBeyondCapacity` closes it — explicitly commented as
  *not* `t.Parallel()` specifically to avoid racing its own
  `captureInfoLog()` — before reading `restoreLog()`). Unlike the anthropic
  test, there is no hardcoded multi-second fallback if the release is
  skipped; the risk here is only "if a future edit adds an early return /
  assertion failure between server startup and `close(block)`," which would
  leave the handler goroutine blocked on `<-block` forever and turn
  `srv.Close()` into an unbounded (not just 5s) hang. This is a *latent*
  version of the same class, not an active one today.
- **`watch_sessions_native_streaming_integration_test.go:210`** (`ghStub`) —
  handler responds immediately (`200` + `{}`) with no blocking; explicitly
  documented in-file as replacing a real network dial specifically to avoid
  an unbounded-hang goroutine leak (a related but distinct class: idle
  pooled HTTP/2 connections outliving `goleak`'s window, not the
  `Close()`-hang-detector class). Not a candidate for this race.
- **`headless_service_test.go:31`**, **`notification_service_test.go:32`**,
  **`import_service_test.go:402`**, **`session_service_bench_test.go:59`** —
  all wrap a real ConnectRPC handler (`mux.Handle(path, handler)`) serving
  actual service logic that returns promptly per-request; no observed
  indefinite-block-on-channel/timer pattern in the handler bodies. Lower
  risk, but *unverified exhaustively* — these were not traced end-to-end
  through the full RPC handler call graph for a hidden slow path.

**Conclusion for scoping the fix**: the anthropic test is the only
**currently-active, unbounded (10s), no-early-release** instance of this
specific trigger shape. The callback-dispatcher tests are a **latent**
version of the same shape that a careless future edit could reactivate.
This matches requirements.md's Out of Scope call ("do not chase hypothetical
future instances speculatively") but is worth naming explicitly in the plan:
a *structural* fix (closing the class) covers the callback-dispatcher latent
risk for free; an *instance-specific* fix (Direction 2, bounding only the
anthropic test's trigger) does not, and leaves that latent risk exactly
where it is today.

## 3. Scoped-logger injection cannot stop stdlib-internal `log.Print` calls — confirmed against stdlib source, and this changes the fix

This is the most important finding in this pass. Verified directly against
the Go stdlib installed locally (`go env GOROOT` → `/home/linuxbrew/.linuxbrew/Cellar/go/1.26.4/libexec`):

**`net/http/httptest/server.go:260,282,290`**: the hang detector is
unconditional stdlib code —
```go
t := time.AfterFunc(5*time.Second, s.logCloseHangDebugInfo)
...
func (s *Server) logCloseHangDebugInfo() {
    ...
    log.Print(buf.String())   // line 290 — calls the STANDARD LIBRARY log package
}
```
This is `log.Print`, i.e. `log.Default()` — a stdlib package-global, not
anything this codebase's `server/services` package owns, injects into, or
can intercept via dependency injection on its own call sites. **No amount of
scoped-`*slog.Logger` threading through this codebase's 386 call sites
touches this line** — it fires regardless of whether the *application's* own
logging is perfectly isolated.

**`log/slog/logger.go:62-75` (`SetDefault`)** confirms exactly how this
reaches the test's buffer:
```go
func SetDefault(l *Logger) {
    defaultLogger.Store(l)
    if _, ok := l.Handler().(*defaultHandler); !ok {
        ...
        log.SetOutput(&handlerWriter{l.Handler(), &logLoggerLevel, capturePC})
        ...
    }
}
```
`slog.SetDefault` calls `log.SetOutput`, which redirects the **stdlib `log`
package's own default `*log.Logger`'s output writer** process-wide. This is
exactly the mechanism `captureLogs` (`autonomous_orchestration_service_test.go:418-427`)
relies on to capture log lines into its buffer — and it is exactly the
mechanism that also makes `httptest`'s internal `log.Print(...)` land in
that same buffer whenever `captureLogs`'s swap is active, with zero
awareness of or participation from this codebase's own logging call sites.

**Is the per-instance handler mutex a mitigation? No — confirmed insufficient, and here's why precisely.**
`log/slog/handler.go:191-218` (`commonHandler`) and `text_handler.go:28-40`
(`NewTextHandler`) show `mu *sync.Mutex` is allocated fresh per handler
instance (`mu: &sync.Mutex{}` in `NewTextHandler`) and shared only among
*clones* of that same handler (`clone()` at `:206-218` copies the pointer,
never allocates a new one). So: **creating a new `slog.NewTextHandler(&buf,
…)` per test does get its own independent mutex — there is no global/shared
mutex across handler instances that could cause cross-test contamination
via the handler layer itself.** That part of the reasoning in the research
question is correct.

But that mutex (locked/unlocked at `handler.go:320-321` inside
`commonHandler.handle`) only guards the *write* path — calls that go through
`Handler.Handle()` → eventually `h.w.Write(...)`. It provides **zero
protection** for `captureLogs`'s own `buf.String()` read, because
`captureLogs` returns `*bytes.Buffer` directly (`autonomous_orchestration_service_test.go:418`,
`var buf bytes.Buffer`) and the caller reads it with a plain `buf.String()`
— a call that never acquires `h.mu` at all. So even a "perfect" single
`TextHandler` instance, entirely un-cloned, still races: one goroutine
inside `Handle()` holds `h.mu` while calling `buf.Write(...)`, while the
test goroutine calls `buf.String()` with no lock — `bytes.Buffer` itself has
no internal synchronization, and the handler's mutex was never designed to
protect anything outside `Handle()`'s own call path.

**Conclusion — confirms the reasoning posed in the research question:**
1. Scoped-logger injection (Direction 1), even done exhaustively and
   correctly across all 386 call sites, **cannot** prevent stdlib's own
   `log.Print` calls (from `net/http/httptest`, or any other stdlib package
   that logs via `log.Default()`) from racing a `captureLogs`-style buffer,
   because `slog.SetDefault`/`log.SetOutput` redirection is inherently
   process-global and stdlib internals have no way to be told about an
   application-level DI seam.
2. Therefore **the buffer `captureLogs` hands out must be made
   internally synchronized regardless of which direction (1 or 2) is
   chosen** — e.g., wrap it in a mutex-guarded type (`type syncBuffer struct{
   mu sync.Mutex; buf bytes.Buffer }` with locked `Write`/`String` methods),
   or otherwise ensure reads take the same lock the handler's writes go
   through. This is not an alternative to Direction 1 or 2 — it is a
   necessary component of either, and arguably the actual root-cause fix,
   since it's the one piece that closes the race regardless of which
   *trigger* (this codebase's own logging, or stdlib's) fires it.
3. This should be surfaced explicitly in Phase 3 planning: a plan that
   implements Direction 1 alone (scoped logger for application call sites)
   and considers the race "fixed" would still have a live race against any
   stdlib-originated `log.Print`/`log.Println` call during a `captureLogs`
   window — the exact failure mode in the confirmed instance. The
   requirements' Direction 1 description already hints at this risk
   ("Feasibility Risks: … may not hold — flag this early") but does not
   name the specific stdlib mechanism; this research makes that concrete
   enough to act on.

## 4. CI-runner-load sensitivity of Direction 2 (bounding the trigger)

If Direction 2 is chosen — shortening `TestAnthropicAIClient_Complete_CancelsOnCtxDone`'s
handler fallback (`time.After(10 * time.Second)` at line ~31) or otherwise
forcing a fast, deterministic teardown, mirroring instance #1's
`ForceTeardown()`-per-iteration fix — the risk is shrinking one timing
margin at the cost of another:

- The **existing** 10s fallback is a *safety net* for "the request context
  never gets cancelled" (a correctness bug in the client), not the expected
  path — the client's `context.WithTimeout(ctx, 100*time.Millisecond)` (line
  ~35) should trigger `r.Context().Done()` on the server side well before
  10s in the passing case. Shortening the fallback to, say, a few hundred
  milliseconds is *usually* safe because there's a ~100x margin today.
- But this codebase's own file (`callback_dispatcher_test.go:174,212,246`
  and `watch_sessions_native_streaming_integration_test.go:335,345,402,414`)
  already uses the identical `select { case <-...Done(): case
  <-time.After(N*time.Second): t.Fatal(...) }` pattern elsewhere, with N
  ranging from 5–10s specifically because these are **timeout-as-failure**
  guards, not steady-state waits — they're sized to tolerate a loaded CI
  runner's scheduling jitter, not just the fast/local case. If Phase 3's fix
  generalizes "shorten the fallback" into a project-wide convention (per the
  Open Question about a structural/lint-enforced guard) rather than scoping
  it narrowly to the one confirmed test, the risk is real: a runner under
  load could delay delivery/observation of a legitimate `ctx.Done()` signal
  past a too-tight new bound, and a test whose fallback branch means "test
  failed" (unlike the anthropic test, where either branch is currently
  inert) would flip from a race-detector false failure to a
  timeout-assertion false failure — trading one flake class for another.
- Recommendation for Phase 3: treat the anthropic test's fallback shortening
  as **local and instance-specific** (matching Out of Scope's "fix the
  confirmed instance," not "add new tooling"), and keep enough margin (e.g.
  1-2s, not 100ms) that it remains a true safety net rather than a tight
  race against scheduler jitter. Do not fold this into a repo-wide
  "shorten all timeout selects" convention as part of this Small-appetite
  fix — that is a distinct, larger change with its own CI-flakiness
  evaluation.

## Summary of implications for Phase 3 planning

- Whichever direction is chosen, `captureLogs`'s buffer needs its own
  internal synchronization (§3) — this is not optional and not covered by
  either Direction 1 or Direction 2 as currently described in
  requirements.md's Scope. It should be added as an explicit task
  regardless of the chosen direction, since it's the one change that
  actually closes the race against stdlib-originated log lines.
- The confirmed-instance fix (Direction 2 applied narrowly to
  `anthropic_client_test.go`) is safe but keep the fallback timeout in the
  1-2s range, not sub-second, to avoid trading a race-detector flake for a
  scheduler-jitter flake (§4).
- `callback_dispatcher_test.go`'s two `<-block`-based tests are a latent,
  not active, version of the same trigger class (§2) — worth a one-line
  comment noting the dependency on `close(block)` happening before
  `srv.Close()`, but not in scope for this Small-appetite fix per
  requirements.md's explicit Out of Scope.
