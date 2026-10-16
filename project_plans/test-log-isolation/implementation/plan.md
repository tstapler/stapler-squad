# Implementation Plan: test-log-isolation

**Complexity**: 1 (bug fix, Small appetite). Calibration applied per Phase 3
instructions: Domain Glossary, Migration Plan, Observability Plan, and Risk
Control are all marked N/A below rather than padded; Dependency Visualization
and Pattern Decisions are kept brief.

All file/line references verified directly against `origin/main` @
`90925039d1e87ebede530cc60374fef3fef6c786` via `git show`/`git grep` (this
worktree is ~559 commits behind; do not trust its own checkout — re-verify
line numbers against the actual branch point at implementation time, per
requirements.md's Feasibility Risks).

## Recap of the confirmed bug (not re-derived, see research/*.md)

`slog.SetDefault()` is process-global and also redirects stdlib `log.Print`
(`log/slog/logger.go`'s documented behavior). `server/services`'s
`captureLogs`-style test helpers swap it for a `bytes.Buffer`-backed handler.
`TestAnthropicAIClient_Complete_CancelsOnCtxDone`
(`server/services/anthropic_client_test.go:22`) never touches `slog`, but its
`httptest.Server.Close()` can trip stdlib's internal 5s hang detector
(`net/http/httptest/server.go`), which calls `log.Print` — landing in
whatever `captureLogs` buffer happens to be installed at that moment,
racing that buffer's `buf.String()` read. Confirmed via real CI stack trace
(requirements.md's Baseline, instance #2). Only two production call sites in
this repo read `slog.Default()` at all: `log/log.go`'s `logAt` (:638-639) and
`ForSession` (:605-606) — both single-line reads, not 386 call sites (the 386
figure is `log.Warn/Info/Error/Debug` *callers*, all downstream of these two
seams).

## Step 0.5 — Creative pass (alternatives considered)

| # | Alternative | Strength | Weakness |
|---|---|---|---|
| A | **Direction 2 only**: shorten `anthropic_client_test.go`'s `time.After(10s)` fallback to bound `Close()`'s latency, matching instance #1's `ForceTeardown` fix. | Smallest possible diff (1 line); zero risk to production code; matches an already-reviewed precedent in this exact file class. | Does not close the race *class* — any future `httptest.Server`/background goroutine in `server/services` remains a latent trigger (pitfalls.md §2 names `callback_dispatcher_test.go`'s two `<-block`-based tests as already-latent). Fails Success Metric #3 ("the fix generalizes"). |
| B | **Full DI of `*slog.Logger` through all `server/services` call sites** (thread a logger through every struct that calls `log.Warn`/`log.Info`). | Textbook "no shared global" fix if carried to completion. | Provably cannot close the confirmed instance on its own — the actual failing call (`httptest`'s internal `log.Print`) is stdlib code this repo doesn't own and has no injection point for (architecture.md §2, pitfalls.md §3). Also the largest diff of the three, touching hundreds of call sites for no marginal benefit over C. Rejected as disproportionate to a Small appetite and to the actual failure mode. |
| C | **(Chosen) Injectable seam at the two `log/log.go` choke points + mutex-guarded capture buffer + narrow Direction-2 hardening of the confirmed instance.** Add a `SetSlogDefaultForTest`-style atomic seam (mirroring PR #576's `atomicLogger`/`SetWarningLogForTest`) so `server/services` test helpers stop calling `slog.SetDefault()` at all; hand out a mutex-guarded buffer from `captureLogs` regardless; shorten the anthropic test's dead-code fallback as cheap extra insurance. | Closes the race at its structural root (no `server/services` test touches the real process-global anymore, so stdlib's `log.Print` can never land in a test's capture buffer again) *and* hardens the buffer itself *and* removes today's concretely-reproducing trigger — three independent, cheap, low-risk layers, each ~1 file, matching the two sibling `syncBuffer`/`syncLogBuffer` precedents already in this repo. | Slightly more surface than A alone (4 test files + 1 production file instead of 1); requires keeping `log/log.go`'s production `slog.SetDefault()` call and the new seam's `Store()` call in sync (see ADR-001) — a subtlety a future maintainer could get wrong if not documented. |

**Decision: C.** This is what research/architecture.md and research/pitfalls.md
independently converge on (§4 and §3 respectively), and it's the only option
that satisfies all three of requirements.md's Success Metrics simultaneously.

### Pattern Decisions (rejected)

| Pattern | Rejected because |
|---|---|
| Extend the existing `atomicLogger` type (`log/log.go:142-148`) to also hold `*slog.Logger` (e.g. via a type parameter) | `atomicLogger` is a small, already-correct, already-reviewed type solving a *different* race (PR #576, legacy `*log.Logger` swaps). Generic-izing it risks that working code for no benefit — architecture.md §5 explicitly recommends a sibling type, not a modification. |
| Serialize the whole `server/services` package's tests (`-p 1` / no `t.Parallel()`) | Explicitly rejected in requirements.md's Alternatives Considered — taxes `make ci` package-wide and papers over the bug. |
| Adopt an OSS slog-test-capture library (`slogt`, `slog-mock`) | build-vs-buy.md Q1: neither addresses the actual failure (global swap under concurrency); both would still require the same Direction-1 seam to be built by hand; violates "no new external dependencies" for no offsetting benefit. |
| Delete `slogDefaultMu` once no test calls raw `slog.SetDefault()` | Verified against this exact codebase's own precedent (`server/services/backlog_service_pipeline_mode_test.go`'s `warningLogMu`, guarding `SetWarningLogForTest` — a pointer-swap seam, same shape as the one being added here) that an atomic-swap seam still needs a mutex to serialize *which test currently owns the slot*, even though the swap itself is torn-read-free. See ADR-001. |

## Chosen architecture

```
Production:                                   Tests (server/services):
initializeWithConfig()                        captureLogs(t) / captureInfoLog() /
  builds prodLogger *slog.Logger                inline swaps in search_service_test.go,
  ├─ slog.SetDefault(prodLogger)  (unchanged,     slack_notifier_test.go
  │   keeps routing 3rd-party/stdlib log.Print      │
  │   into the real structured log pipeline —       ▼
  │   untouched by this fix)                  log.SetSlogDefaultForTest(buf-backed logger)
  └─ slogDefault.Store(prodLogger)  (new)        (atomic Swap on `slogDefault`, guarded by
                                                   slogDefaultMu for the swap-to-restore
       ▼                                          window — same shape as today, repointed)
log.logAt() / log.ForSession()
  read slogDefault.Load()  (new — was slog.Default())
       │
       ▼
server/services's log.Warn/Info/Error/Debug calls
  route through the test's buffer when a capture is active,
  and NEVER touch the real slog.Default() — so httptest's
  stdlib log.Print (which still reads the real slog.Default())
  can no longer land in any test's buffer.
```

Independent, cheap hardening layers (not diagrammed above, orthogonal):
- `captureLogs`'s buffer becomes a mutex-guarded `syncLogBuffer` (belt-and-braces
  against any future un-migrated or stdlib-originated writer).
- `anthropic_client_test.go`'s dead-code `time.After(10s)` fallback shortens to
  `2s`, capping `Close()`'s worst case well under the 5s hang-detector window.

## Domain Glossary

N/A — complexity 1. No new domain types are introduced. `atomicSlogLogger` and
`syncLogBuffer` (below) are test-infrastructure/logging-plumbing types mirroring
existing repo patterns (`atomicLogger`, `executor/safeexec`'s `syncBuffer`,
`session/streamhub`'s `syncLogBuffer`), not business/domain concepts.

## Dependency Visualization

Brief, given the small scope:
- `log/log.go` is the only production file touched; it has no new external
  dependency (still stdlib `log`, `log/slog`, `sync/atomic`).
- `server/services/autonomous_orchestration_service_test.go`,
  `session_service_client_log_test.go`, `search_service_test.go`,
  `slack_notifier_test.go` each depend on `log/log.go`'s new
  `SetSlogDefaultForTest` (a new, additive export — no existing caller of
  `log/log.go` is broken).
- `anthropic_client_test.go` has zero dependency on any of the above — its
  fix (Task 4.1) is independent and can be implemented/reviewed/landed
  separately from Stories 1–3 if needed for scheduling, though doing all in
  one PR is expected given the Small appetite.
- All ~10 other `captureLogs(t)` call sites (`connectrpc_websocket_test.go`,
  `deep_link_resolver_test.go`, `session_service_test.go`,
  `slack_interactive_handler_test.go`, plus `autonomous_orchestration_service_test.go`'s
  own 3 direct calls, plus `slack_notifier_test.go`'s 7) require **zero**
  edits — they inherit the fix transitively through the shared `captureLogs`
  helper. Verified: every call site only ever calls `.String()` on the
  returned value (`git grep -n "captureLogs(t)" origin/main -- server/services`
  cross-checked against each file's usage), so `captureLogs`'s return-type
  change from `*bytes.Buffer` to `*syncLogBuffer` (both expose `String()`) is
  source-compatible everywhere.

## Epic: Close the server/services slog-capture race class

### Story 1 — Injectable slog seam in `log/log.go` (mirrors PR #576's `atomicLogger`)

**Files**: `log/log.go` only, across 3 tasks (kept separate for reviewability;
each is independently a 2-3 minute diff).

#### Task 1.1 — Add `atomicSlogLogger` type, `slogDefault` var, and `SetSlogDefaultForTest` setter

- Add, immediately after `atomicLogger`'s methods (after `log/log.go:148`):
  ```go
  // atomicSlogLogger holds a *slog.Logger behind an atomic.Pointer, mirroring
  // atomicLogger above but for the slog-backed logging path (logAt, ForSession).
  // A sibling type rather than a generic atomicLogger[T] to avoid touching the
  // already-correct, already-reviewed legacy-*log.Logger swap mechanism (PR #576).
  type atomicSlogLogger struct {
      ptr atomic.Pointer[slog.Logger]
  }

  func (a *atomicSlogLogger) Load() *slog.Logger              { return a.ptr.Load() }
  func (a *atomicSlogLogger) Store(l *slog.Logger)            { a.ptr.Store(l) }
  func (a *atomicSlogLogger) Swap(l *slog.Logger) *slog.Logger { return a.ptr.Swap(l) }
  ```
- Add `slogDefault atomicSlogLogger` to the `var (...)` block starting at
  `log/log.go:151` (alongside `warningLog`/`infoLog`/`errorLog`/`debugLog` at
  :152-155; the block continues further with unrelated globals — insert the
  new var next to its three siblings, not at the block's end).
- Seed it in the existing `init()` at `log/log.go:31-34`, add:
  `slogDefault.Store(slog.Default())`.
- Add the test setter near `SetWarningLogForTest` etc. (`log/log.go:199-211`):
  ```go
  // SetSlogDefaultForTest atomically replaces the slog-backed default logger
  // (read by logAt/ForSession) and returns the previous value, so tests can
  // restore it via t.Cleanup instead of calling slog.SetDefault() — which
  // would also rewire stdlib log.Print process-wide (see log/slog's
  // documented log.SetOutput side effect) and is the root cause this fix
  // removes tests from touching at all.
  func SetSlogDefaultForTest(l *slog.Logger) *slog.Logger { return slogDefault.Swap(l) }
  ```

**Given/When/Then**:
- Given `log/log.go` before this change, when a caller wants to redirect
  `log.Warn`/`log.Info`/etc. for a test, then the only available seam is the
  real `slog.SetDefault()`. Given this change, when the same caller calls
  `log.SetSlogDefaultForTest(l)`, then `logAt`/`ForSession` (Task 1.2) observe
  `l` without any call to `slog.SetDefault()`/`slog.Default()`.
- Given the package has just been initialized (no `Initialize*` call yet),
  when `log.Warn(...)` is called, then it behaves identically to before this
  change (routes through the plain stdlib `slog.Default()`, captured at
  `init()` time) — no observable behavior change for callers that never
  configure logging.

#### Task 1.2 — Rewire `logAt` and `ForSession` to read the new seam

- `log/log.go:638-639` (`logAt`): change `logger := slog.Default()` to
  `logger := slogDefault.Load()`.
- `log/log.go:605-606` (`ForSession`): change
  `return slog.Default().With("session", sessionID)` to
  `return slogDefault.Load().With("session", sessionID)`.

**Given/When/Then**:
- Given production has called `Initialize`/`InitializeWithConfig` (which Task
  1.3 updates to also populate `slogDefault`), when any `server/services` code
  calls `log.Warn("msg")`, then the log line is handled by the exact same
  handler chain (`TraceIDHandler → PackageLevelHandler → AsyncHandler →
  JSONHandler`) as before this change — verified by `make test` passing
  `log/log_test.go` and any `server/services` test asserting on structured
  log output.
- Given a `server/services` test has called `log.SetSlogDefaultForTest(buf-backed logger)`,
  when code under test calls `log.Warn`/`log.Info`/`log.ForSession(...).Info`,
  then the emitted line appears in that buffer — verified by the existing
  assertions in `autonomous_orchestration_service_test.go` (`buf.String()`
  contains expected substrings) continuing to pass unmodified after Story 2.

#### Task 1.3 — Keep the seam in sync with production's real `slog.SetDefault()` call

- `log/log.go:970-979` (inside `initializeWithConfig`): currently:
  ```go
  slog.SetDefault(slog.New(NewTraceIDHandler(packageLevelHandler)))
  ```
  Change to construct once and store into both slots, so the seam and the
  real global stay identical in production (this call must remain — it also
  redirects stdlib `log.Print` into structured logs process-wide, a real,
  intentional production behavior this fix must not touch per requirements.md's
  Constraints):
  ```go
  prodLogger := slog.New(NewTraceIDHandler(packageLevelHandler))
  slog.SetDefault(prodLogger)
  slogDefault.Store(prodLogger)
  ```

**Given/When/Then**:
- Given the process calls `log.Initialize(false)` (or `InitializeWithConfig`/
  `InitializeForTests`), when `initializeWithConfig` runs, then both
  `slog.Default()` and `log.logAt`'s `slogDefault.Load()` return the *same*
  `*slog.Logger` instance — verified by a unit assertion (or manual check)
  that `slog.Default() == slogDefault.Load()` (pointer equality) immediately
  after `Initialize` returns.
- Given this task is skipped (hypothetically), when `initializeWithConfig`
  runs, then `log.Warn` would silently revert to the plain unconfigured
  default (no file output, no JSON, no trace IDs) while `log.Print`/stdlib
  calls would still go to the configured production handler — a production
  regression. This G/W/T exists to make the dependency on Task 1.3 explicit
  for review; do not ship Task 1.2 without Task 1.3 in the same PR.

---

### Story 2 — Migrate `server/services`'s 4 slog-capture swap sites off `slog.SetDefault()`

**Files**: one per task, 4 tasks, ≤1 file each.

**Import note (verified)**: none of the 4 files below (`autonomous_orchestration_service_test.go`,
`session_service_client_log_test.go`, `search_service_test.go`,
`slack_notifier_test.go`) currently imports
`github.com/tstapler/stapler-squad/log` — each needs a new import added.
None of the 4 imports stdlib `"log"` either (only `"log/slog"`), so the
import can be added unaliased as `"github.com/tstapler/stapler-squad/log"`
(package name `log`) in each file without colliding with `slog` — Go
resolves `log.X` vs `slog.X` by the distinct local identifiers already in
use. This repo already has two different aliases for this same import path
elsewhere in the package (`tslog` in `backlog_service_pipeline_mode_test.go`,
`ssqlog` in `backlog_triage_harness_test.go`) — there is no single
established convention to match, so use the unaliased `log` in these 4 files
unless `goimports`/`make lint` flags a conflict.

#### Task 2.1 — `autonomous_orchestration_service_test.go`: migrate `captureLogs`

- `autonomous_orchestration_service_test.go:418-430`, change:
  ```go
  func captureLogs(t *testing.T) *bytes.Buffer {
      t.Helper()
      slogDefaultMu.Lock()
      var buf bytes.Buffer
      prev := slog.Default()
      slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
      t.Cleanup(func() {
          slog.SetDefault(prev)
          slogDefaultMu.Unlock()
      })
      return &buf
  }
  ```
  to:
  ```go
  func captureLogs(t *testing.T) *bytes.Buffer {
      t.Helper()
      slogDefaultMu.Lock()
      buf := &syncLogBuffer{} // see Task 3.1
      prev := log.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
      t.Cleanup(func() {
          log.SetSlogDefaultForTest(prev)
          slogDefaultMu.Unlock()
      })
      return buf
  }
  ```
  (Return type becomes `*syncLogBuffer`, added in Task 3.1 — sequence Task 3.1
  before or together with this task since this edit references it. `log`
  above is this repo's `github.com/tstapler/stapler-squad/log` package — see
  the Import note above this story for how to add it.)
- Update the `slogDefaultMu` doc comment at `autonomous_orchestration_service_test.go:407-413`
  to describe its new role: it now serializes `log.SetSlogDefaultForTest`
  swaps (the `log` package's own injectable seam), not raw `slog.SetDefault()`
  swaps — see ADR-001 for why the mutex is retained rather than deleted.

**Given/When/Then**:
- Given `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_LogsNotLinkedAtDebug`
  (or any other `captureLogs(t)` consumer) runs, when it asserts
  `buf.String()` contains an expected log substring, then the assertion
  passes identically to before this change (same log content, same
  handler config) — verified by `go test ./server/services -run TestAutonomousOrchestrationService -race`.
- Given `TestAnthropicAIClient_Complete_CancelsOnCtxDone` (`t.Parallel()`,
  never touches `slog`/`slogDefaultMu`) runs concurrently with
  `TestSlackNotifier_NeverLogsWebhookURL` (`t.Parallel()`,
  `slack_notifier_test.go:688-703` — the actual confirmed racing partner per
  requirements.md's Baseline stack trace, which holds `slogDefaultMu` for its
  `captureLogs` window), when `httptest.Server.Close()`'s hang detector fires
  and calls stdlib `log.Print` during that window, then that call reads the
  real `slog.Default()` — which `captureLogs` no longer touches after this
  change — so it cannot land in `TestSlackNotifier_NeverLogsWebhookURL`'s
  buffer, eliminating the confirmed race. `slogDefaultMu` was never able to
  prevent this (the anthropic test doesn't participate in it), which is
  exactly why the fix has to be structural rather than adding the anthropic
  test to the mutex's participant list. Verified by the Story 5 stress
  command.

#### Task 2.2 — `session_service_client_log_test.go`: migrate `captureInfoLog`

- `session_service_client_log_test.go:26-38`, change the body's
  `slog.Default()`/`slog.SetDefault()` calls (lines 30-31, 33) to
  `log.SetSlogDefaultForTest(...)` following the same before/after shape as
  Task 2.1. `captureErrorLog` (line 40) delegates to `captureInfoLog` and
  needs no separate edit.

**Given/When/Then**:
- Given a test calls `captureInfoLog()`, when it later calls the returned
  restore-and-read closure, then the returned string still contains the
  expected `INFO:`-level log lines emitted during the window — verified by
  `go test ./server/services -run TestSessionService.*ClientLog -race`
  (adjust to the actual test names in this file) continuing to pass.

#### Task 2.3 — `search_service_test.go`: migrate the inline swap

- `search_service_test.go:137-146` (inside
  `TestSearchClaudeHistory_LogsExcludedCountOnlyWhenSessionsActuallyExcluded`):
  replace the inline `prev := slog.Default()` / `slog.SetDefault(...)` /
  `defer` block with the `log.SetSlogDefaultForTest` equivalent, keeping the
  existing `slogDefaultMu.Lock()`/`Unlock()` wrapping unchanged.

**Given/When/Then**:
- Given the test's `ExcludeAutomationSessions` case runs, when it asserts on
  the captured buffer's excluded-count log line, then the assertion is
  unaffected by this change — verified by
  `go test ./server/services -run TestSearchClaudeHistory_LogsExcludedCountOnlyWhenSessionsActuallyExcluded -race`.

#### Task 2.4 — `slack_notifier_test.go`: migrate the inline `signalingLogHandler` swap

- `slack_notifier_test.go:536-548` (inside
  `TestSlackNotifier_RecoversFromPanic_And_LogsError`): replace
  `prev := slog.Default()` / `slog.SetDefault(slog.New(&signalingLogHandler{...}))`
  / `slog.SetDefault(prev)` with `log.SetSlogDefaultForTest(...)`, keeping the
  `signalingLogHandler` wrapper and `sigCh` signaling mechanism unchanged —
  only the swap target changes.

**Given/When/Then**:
- Given a panic occurs inside `dispatchAsync`'s callback, when the recovery
  path logs the error, then `signalingLogHandler.Handle` still fires
  (unchanged — it wraps whatever handler is installed) and `sigCh` still
  receives the signal within the test's 2s timeout — verified by
  `go test ./server/services -run TestSlackNotifier_RecoversFromPanic_And_LogsError -race -count=10`.

#### Task 2.5 — Update stale "process-global slog.Default()" comments

- `callback_dispatcher_test.go:106,225` and
  `session_service_client_log_test.go:98,116,134,155,172,190,205`: these
  `// Not t.Parallel(): captureInfoLog() mutates the process-global slog.Default()`
  comments become inaccurate after Task 2.2 (they now mutate `log`'s
  `slogDefault`, not `slog`'s own default). Update the wording to say
  `log.SetSlogDefaultForTest`'s shared seam instead of "process-global
  slog.Default()". Comment-only change, no behavior impact.

**Given/When/Then**:
- Given a future reader opens `callback_dispatcher_test.go:106`, when they
  read the comment explaining why the test isn't `t.Parallel()`, then the
  comment accurately names the mechanism (`log.SetSlogDefaultForTest`) they'd
  need to look up, not a stale reference to `slog.Default()` that no longer
  applies after this fix.

---

### Story 3 — Harden the capture buffer itself (defense in depth)

#### Task 3.1 — `autonomous_orchestration_service_test.go`: add `syncLogBuffer`, use it in `captureLogs`

- Add, near `captureLogs` (before it, e.g. above `slogDefaultMu` at line 407),
  modeled directly on `executor/safeexec/safeexec_pg_test.go`'s `syncBuffer`
  and `session/streamhub/observability_test.go`'s `syncLogBuffer`:
  ```go
  // syncLogBuffer is a mutex-guarded bytes.Buffer. Even after Story 2 stops
  // this package's tests from ever calling slog.SetDefault(), captureLogs's
  // buffer can still legitimately be written to by a background goroutine
  // the test under exercise itself spawns (via log.Warn/Info) concurrently
  // with the owning test's buf.String() read. A plain *bytes.Buffer would
  // make that a genuine data race under -race; this makes the access itself
  // safe regardless of which code path writes to it.
  type syncLogBuffer struct {
      mu  sync.Mutex
      buf bytes.Buffer
  }

  func (b *syncLogBuffer) Write(p []byte) (int, error) {
      b.mu.Lock()
      defer b.mu.Unlock()
      return b.buf.Write(p)
  }

  func (b *syncLogBuffer) String() string {
      b.mu.Lock()
      defer b.mu.Unlock()
      return b.buf.String()
  }
  ```
- This is folded into Task 2.1's edit to `captureLogs` (same file, same
  logical change) — listed separately here only so the new type's rationale
  and precedent are reviewable on their own.

**Given/When/Then**:
- Given `captureLogs`'s buffer is now a `*syncLogBuffer`, when two goroutines
  concurrently call `Write` and `String` on it, then `-race` reports no data
  race — verified by `go test -race -run TestAutonomousOrchestrationService -count=20 ./server/services/...`.
- Given every existing `captureLogs(t)` call site only ever calls `.String()`
  on the return value (verified in Dependency Visualization above), when
  `captureLogs`'s return type changes to `*syncLogBuffer`, then no other file
  needs edits — confirmed by `make build` succeeding with zero changes
  outside `autonomous_orchestration_service_test.go`.

---

### Story 4 — Bound the confirmed instance's teardown latency (Direction 2, cheap insurance)

#### Task 4.1 — `anthropic_client_test.go`: shorten the dead-code fallback

- `anthropic_client_test.go:32`, change:
  ```go
  case <-time.After(10 * time.Second):
  ```
  to:
  ```go
  // Safety net only: the test's own 100ms ctx timeout drives r.Context().Done()
  // well before this fires. Capped at 2s (not sub-second) so it stays a true
  // safety margin under CI scheduler jitter, not a tight race against it
  // (see pitfalls.md §4).
  case <-time.After(2 * time.Second):
  ```

**Given/When/Then**:
- Given the test's context has a 100ms timeout (line ~35, unchanged), when
  `client.Complete` is called and the context is cancelled, then the
  handler's `r.Context().Done()` branch fires almost immediately (as it does
  today) and the `time.After` branch remains dead code in the passing case —
  verified by the existing assertion `assert.Less(t, elapsed, 500*time.Millisecond, ...)`
  continuing to pass.
- Given a hypothetical future regression where `r.Context()` is never
  cancelled (a real bug this fallback guards against), when that bug is
  present, then the handler now returns within 2s instead of 10s, so
  `srv.Close()`'s wait for outstanding handlers finishes well under the
  stdlib hang detector's 5s threshold even under CI load — verified by the
  Story 5 stress command showing no `logCloseHangDebugInfo`-triggered
  `log.Print` in `-count=20` runs.

---

### Story 5 — Verify the fix

#### Task 5.1 — Stress-repro the confirmed race, before and after

Run exactly the command requirements.md's Success Metrics specifies, against
a build *before* Stories 1-4 (stash/checkout the pre-fix state or use the
current `origin/main`) and again *after*:

```bash
go test -race -run 'TestAnthropicAIClient_Complete_CancelsOnCtxDone|TestSlackNotifier_NeverLogsWebhookURL' -count=20 ./server/services/...
```

**Given/When/Then**:
- Given the pre-fix `origin/main` code, when this command runs, then it is
  expected to intermittently fail with a `WARNING: DATA RACE` report
  matching the `bytes.Buffer` Write/Read collision named in requirements.md's
  Baseline (may require a few attempts to reproduce, since it's a genuine
  race, not a deterministic failure).
- Given the post-fix code (Stories 1-4 applied), when this command runs
  (recommend `-count=50` for higher confidence given the race's
  intermittency), then it passes cleanly with no race report, 50/50 times.

#### Task 5.2 — Full regression pass

```bash
make build && make test
go test -race ./server/services/... -count=2
make lint
```

**Given/When/Then**:
- Given all Story 1-4 changes are applied, when `make ci` (or the three
  commands above) runs, then every previously-passing test in
  `server/services`, `log`, and any package touching `log.ForSession`/
  `log.Warn`/etc. still passes — specifically:
  `TestHubRegistryAndStreamOwnershipLock_should_NeverProduceTwoOwners_When_RacedConcurrently`
  (instance #1, untouched by this fix) and every `slogDefaultMu`-cooperating
  test named in Story 2 continue to pass under `-race`.
- Given `make lint` runs, when it checks the new `atomicSlogLogger`/
  `syncLogBuffer` types and `SetSlogDefaultForTest` function, then it reports
  no new violations (in particular: `slogDefault`/`atomicSlogLogger` should
  carry the same `//nolint:gochecknoglobals` treatment as the existing
  `warningLog`/`infoLog`/etc. package vars, if that linter is configured to
  flag package-level vars).

## Migration Plan

N/A — complexity 1. No data migration; this is a test-infrastructure and
logging-plumbing change with no schema, config, or persisted-state impact.

## Observability Plan

N/A — complexity 1. Standard CI failure visibility (GitHub Actions run status
+ `-race` output) is sufficient, per requirements.md.

## Risk Control

N/A — complexity 1. Low risk, test-only + a 3-line production change (Task
1.3) that preserves existing production behavior by construction (same
`*slog.Logger` instance stored in both the real global and the new seam).
Standard rollback via PR revert if it destabilizes CI further.

## Open Question resolution (from requirements.md)

> Should the fix target only the two confirmed instances, or add a
> structural guard...?

Resolved: neither "only the two confirmed instances" nor "add new lint
tooling." The chosen design (Story 1+2+3) is structural by construction — it
removes the *precondition* for the race (a `server/services` test ever
calling the real `slog.SetDefault()`) rather than only patching the two known
triggers or bolting on a new enforcement mechanism. This matches
requirements.md's own framing ("fix confirmed instances + make the
convention safer by construction, not add new tooling") without requiring a
lint rule, `TestMain` check, or any new tooling — the structural safety comes
from the seam itself, not from policing usage of it.
