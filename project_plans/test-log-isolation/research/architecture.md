# Architecture Research: test-log-isolation

All facts below verified directly against `origin/main` @ `90925039d1e87ebede530cc60374fef3fef6c786`
via `git show`/`git grep`, per the requirements doc's instruction to bypass this stale
(~559-commits-behind) worktree checkout.

## 1. Integration points — every `slogDefaultMu`/`captureLogs` call site today

`git grep -n "slogDefaultMu\|captureLogs" origin/main -- server/services` surfaces exactly
one shared helper plus its call sites:

- **Definition** — `server/services/autonomous_orchestration_service_test.go:407-430`:
  - `var slogDefaultMu sync.Mutex` (:414) — package-global mutex, doc comment names it as
    required for *every* `slog.Default()` swap site in the package.
  - `func captureLogs(t *testing.T) *bytes.Buffer` (:418-430) — locks `slogDefaultMu`,
    swaps `slog.SetDefault()` to a `bytes.Buffer`-backed `slog.NewTextHandler`, restores
    via `t.Cleanup`.
- **`captureLogs(t)` call sites** (all get whatever `captureLogs` does internally, with
  zero per-call-site code — this is the key structural fact for Direction 1, see §4):
  `autonomous_orchestration_service_test.go` (:454, :505, :548),
  `connectrpc_websocket_test.go:503`, `deep_link_resolver_test.go:341`,
  `session_service_test.go:665`, `slack_interactive_handler_test.go:78`,
  `slack_notifier_test.go` (:116, :165, :192, :213, :634, :690, :709 — via `captureLogs`
  or the local `signalingLogHandler` wrapper at :542).
- **Inline `slogDefaultMu`-holding swaps that don't go through `captureLogs`** (each
  hand-rolls the same lock → `slog.SetDefault` → defer/cleanup unlock+restore pattern):
  - `session_service_client_log_test.go:24-38` — `captureInfoLog()`/`captureErrorLog()`.
  - `search_service_test.go:137-146` — inline in
    `TestSearchClaudeHistory_LogsExcludedCountOnlyWhenSessionsActuallyExcluded`.
  - `slack_notifier_test.go:536-548` — inline, uses a custom `signalingLogHandler`.

All of these are **mutually consistent today**: they all hold `slogDefaultMu` for their
full swap-to-restore window, and none of them run `t.Parallel()` (each has a comment
saying so). The gap is exactly what the requirements doc states: `TestAnthropicAIClient_Complete_CancelsOnCtxDone`
neither calls `captureLogs` nor holds `slogDefaultMu` (it doesn't touch `slog` at all) —
it races these tests purely as a *side effect* of `httptest.Server.Close()`'s internal
5s hang detector shelling out to stdlib `log.Print`, which lands in whatever handler is
currently installed as `slog.Default()` (Go's documented `log`/`slog` interop — settled
per the requirements doc's Rabbit Holes section, not re-derived here).

## 2. Direction 1 — scoped `*slog.Logger` injection: what actually has to change

Checked `log/log.go` on `origin/main` directly (not the stale worktree's copy).

**No existing seam today** for the slog-backed path — `logAt` (the single choke point
all four package-level wrappers route through) calls `slog.Default()` fresh on every
invocation:

```go
// log/log.go:638-651
func logAt(level slog.Level, msg string, args ...any) {
	logger := slog.Default()
	ctx := context.Background()
	if !logger.Enabled(ctx, level) {
		return
	}
	...
	_ = logger.Handler().Handle(ctx, r)
}

func Info(msg string, args ...any)  { logAt(slog.LevelInfo, msg, args...) }
func Warn(msg string, args ...any)  { logAt(slog.LevelWarn, msg, args...) }
func Error(msg string, args ...any) { logAt(slog.LevelError, msg, args...) }
func Debug(msg string, args ...any) { logAt(slog.LevelDebug, msg, args...) }
```

`ForSession` (log/log.go:605-607) also calls `slog.Default()` directly:
`return slog.Default().With("session", sessionID)`.

There are exactly **two** call sites of `slog.Default()` in the entire slog-backed path
(`logAt` and `ForSession`) — not 386. The 386-call-site number from the requirements doc
is `log.Warn/Info/Error/Debug` *call sites* in `server/services/*.go`, all of which are
consumers of `logAt`/`ForSession`, not places that touch `slog.Default()` themselves.
Confirmed via `git grep -n "slog\.\(Info\|Warn\|Error\|Debug\|Default\)(" origin/main --
'server/services/*.go'` returning **zero** matches in non-test files — production code in
`server/services` never calls `slog` directly; it exclusively goes through the `log`
package wrapper (`log.Warn`/`log.Info`/etc., 65 files with at least one call, and
`log.ForSession(sessionID).Info/Warn/Debug(...)`, e.g. `approval_handler.go`,
`connectrpc_websocket.go`).

**This is a small, well-precedented change.** PR #576 already established the exact
pattern needed, just for the non-slog path: `warningLog`/`infoLog`/`errorLog`/`debugLog`
are each an `atomicLogger` (`atomic.Pointer[log.Logger]` wrapper, log/log.go:135-148) with
paired `WarningLog()`/`SetWarningLogForTest()` accessor/setter functions (:182-211) that
do a lock-free atomic swap-and-restore, no mutex required, because "a `*log.Logger` is
always replaced wholesale... atomic.Pointer gives lock-free reads."

Direction 1 = replicate that same shape one level up, for the slog path:

1. Add `var defaultLogger atomic.Pointer[slog.Logger]` (or reuse `atomicLogger`'s pattern
   with a `slog.Logger`-typed variant), seeded to `slog.Default()` at package init.
2. Add `log.SetTestLogger(l *slog.Logger) *slog.Logger` — atomic swap, returns the
   previous value (mirrors `SetWarningLogForTest`'s signature exactly).
3. Change `logAt` and `ForSession` to read `defaultLogger.Load()` instead of calling
   `slog.Default()`.
4. Change `captureLogs` (and the three inline-swap sites in §1) to call
   `log.SetTestLogger(...)`/restore via the returned previous value, instead of
   `slog.SetDefault()`/`slog.Default()`. Because `captureLogs` is a single shared helper,
   this is a **one-function edit**, not a per-call-site migration — every one of its ~10
   call sites across 6 files is fixed by construction. Only the 3 inline sites
   (`session_service_client_log_test.go`, `search_service_test.go`,
   `slack_notifier_test.go:536-548`) need their own small edits, since they don't route
   through `captureLogs`.

**Why this actually closes the confirmed race (not just relocates it):** the trigger is
stdlib `log.Print` (inside `net/http/httptest`) writing through whatever is currently
`slog.Default()`. If `captureLogs`-style tests stop calling `slog.SetDefault()` entirely
and instead swap only `log`'s own `defaultLogger` atomic pointer, then `slog.Default()`
is **never touched** by any `server/services` test — httptest's stdlib `log.Print` still
resolves to the untouched, real default handler (harmless stderr/discard), and no test's
capture buffer is anywhere in its path. This is the crux: Direction 1 only removes the
race if it eliminates `slog.SetDefault()` calls from the test convention, not merely if
it adds an alternate read path alongside the existing one. A half-migration (new seam
added, but `slog.SetDefault()` still called somewhere) would not close the confirmed
race, because the specific trigger (httptest → stdlib `log.Print`) never goes through
`log.Warn`/`logAt` at all — it only cares about `slog.Default()`.

## 3. Direction 2 — bound/eliminate the specific trigger

`server/services/anthropic_client_test.go:20-40`
(`TestAnthropicAIClient_Complete_CancelsOnCtxDone`):

```go
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	select {
	case blocker <- struct{}{}:
	default:
	}
	select {
	case <-r.Context().Done():
	case <-time.After(10 * time.Second):
	}
}))
defer func() {
	srv.CloseClientConnections()
	srv.Close()
}()
```

The test's own context has a 100ms timeout (`ctx, cancel :=
context.WithTimeout(context.Background(), 100*time.Millisecond)`), and asserts
`client.Complete` returns within 500ms. In the success path the handler goroutine's
`r.Context().Done()` fires almost immediately once the client cancels/disconnects — the
`time.After(10*time.Second)` branch is a dead-code fallback for a request that never gets
cancelled (which shouldn't happen given the test's own 100ms ctx timeout drives the
client to disconnect quickly). `srv.Close()` blocks up to its internal ~5s threshold
waiting for all outstanding handler goroutines to return; under `-race` + CI-runner load,
enough scheduling delay accumulates that `Close()` nears that threshold and fires the
hang-detector's `log.Print`, even though the handler *does* eventually exit via
`r.Context().Done()` — it's a **timing/load** issue, not a functional deadlock.

Structural fix options for Direction 2 alone:
- Shorten or remove the `10*time.Second` fallback (e.g., no fallback — block solely on
  `<-r.Context().Done()`) so there's no code path where the handler goroutine could ever
  need the full 10s; this doesn't change today's actual behavior (the fallback is already
  provably not what's being hit — the request is cancelled well under 100ms in the test),
  it only removes a red herring that makes the intent ambiguous. Goroutine-leak risk from
  removing it: **negligible** — `r.Context()` is guaranteed to be `Done()` once the
  client's request is cancelled or the connection drops (Go's `net/http` server always
  cancels the request context when the connection closes), so the handler goroutine exits
  deterministically either way; the fallback exists only as defense-in-depth against a
  hypothetical httptest bug, not a real code path this test exercises.
- The real fix for the flake is tightening `Close()`'s wait, e.g. calling
  `srv.CloseClientConnections()` *before* the client-side timeout fires (already done,
  but only in the `defer`, i.e. after `client.Complete` returns) or restructuring so the
  handler is guaranteed to observe cancellation fast enough under load that `Close()`
  finishes well inside 5s even with CI scheduling jitter. This is the same class of fix
  already applied to instance #1 (`connectrpc_websocket_test.go`'s `ForceTeardown`-per-
  iteration) — bound the resource's teardown so it can't approach the detector's window.

This is a small, surgical, single-file change with low risk, consistent with how instance
#1 was fixed.

## 4. Does fixing only the one instance leave a structurally-recurring risk?

**Yes — Direction 2 alone does not close the race class.** The prior scan found ~31
`go func(` launches across 14 `server/services` test files; any one of them — or any
future `httptest.Server`, `time.After`, or slow-closing resource added by a contributor
with no reason to know `slogDefaultMu` exists — is a latent trigger for the exact same
mechanism (background/stdlib code emitting a log line that lands in `slog.Default()`
while some other test in the process has swapped it for a capture buffer). Fixing
`anthropic_client_test.go` removes today's *known* trigger but leaves the underlying
shared-global-state hazard fully intact for the next one. This mirrors the requirements
doc's own framing: "the failure is not localized to the file that 'caused' it."

**Direction 1 closes the whole class regardless of how many httptest servers/goroutines
exist**, because it removes the precondition the race depends on: a `server/services`
test ever calling `slog.SetDefault()`. Once no test in the package touches the *process*-
global slog default, it doesn't matter how many background goroutines, timers, or
httptest servers exist or get added later — none of them can race a capture buffer that
no longer lives behind `slog.Default()`. This is the structural, by-construction
generalization the requirements doc's Success Metrics section asks for ("the convention
is enforced structurally rather than by every author remembering to opt in").

**Recommendation: do both, but Direction 1 is load-bearing; Direction 2 is a
low-cost, high-confidence local hardening on top, not a substitute.**
- Direction 1 (scoped/injectable `*slog.Logger` for the `log` package's slog-backed path,
  via a `SetTestLogger` atomic-swap seam analogous to PR #576's `SetWarningLogForTest`
  family) eliminates the *root* shared-global-state problem for every current and future
  `server/services` test, at low cost given only two `slog.Default()` call sites
  (`logAt`, `ForSession`) and one shared `captureLogs` helper to migrate, plus three
  small inline sites.
- Direction 2 (tightening `anthropic_client_test.go`'s handler/teardown, matching the
  already-proven `ForceTeardown`-per-iteration pattern from instance #1) is worth doing
  regardless, since it's cheap and removes today's concretely-reproducing trigger
  immediately, independent of whether/when Direction 1 lands. It does not by itself
  satisfy the requirements doc's "fix generalizes" success metric.
- If Direction 1's discovery in Phase 3 turns up a `captureLogs` consumer that needs to
  observe log lines from a code path with no reachable injection seam (the documented
  Feasibility Risk), fall back to Direction 2 alone for the confirmed instance — but per
  §2 above, no such gap was found in this research: every production caller in
  `server/services` already routes through `log.Warn/Info/Error/Debug`/`log.ForSession`,
  i.e., through the two seams (`logAt`, `ForSession`) Direction 1 needs to change.

## 5. Consistency requirements — verified non-regression surface

- **Instance #1 (`ForceTeardown`-per-iteration, `connectrpc_websocket_test.go:758-835`)**:
  untouched by either direction — it doesn't call `captureLogs`/`slog.SetDefault` at all
  in the racy loop; it already tears down each iteration's `pumpControlModeOutputIntoHub`
  goroutine via `hub.ForceTeardown()` (:808, :835) precisely so it stops `Warn`-logging
  into whatever the (then-still-global) default logger pointed at. Direction 1 doesn't
  change this file. `TestRecordControlModeStreamStart_should_LogOverlapWarning_...`
  (:503) does call `captureLogs`, so it inherits Direction 1's fix automatically, with no
  edit needed to this file beyond what `captureLogs` itself requires.
- **PR #576's `atomicLogger`/`SetWarningLogForTest` family (log/log.go:135-211)**: this is
  a *different*, already-solved race class (legacy `*log.Logger`-based
  `warningLog`/`infoLog`/`errorLog`/`debugLog` swaps), explicitly out of scope per the
  requirements doc ("Out of Scope" — do not re-litigate). Direction 1 is architecturally
  the *same pattern* applied to the slog path, so it should be implemented as a sibling
  addition (a new `atomic.Pointer[slog.Logger]` + `SetTestLogger`), not a modification of
  the existing `atomicLogger` type/variables — keeping the two families independent avoids
  any risk of touching the already-correct legacy-logger swap mechanism.

## Recommendation for Phase 3 planning

**Pursue Direction 1 as the primary fix, with Direction 2 as a cheap complementary
hardening — not either/or.** Concretely:
1. Add a `SetTestLogger`-style atomic-swap seam to `log/log.go` for the slog-backed path
   (new `atomic.Pointer[slog.Logger]`, `logAt`/`ForSession` read from it instead of
   `slog.Default()`).
2. Migrate `captureLogs` (one function, ~10 call sites fixed for free) and the three
   inline `slogDefaultMu`-holding swap sites (`session_service_client_log_test.go`,
   `search_service_test.go`, `slack_notifier_test.go:536-548`) to use it instead of
   `slog.SetDefault()`.
3. Separately, tighten `anthropic_client_test.go`'s handler/teardown (remove or shorten
   the dead-code 10s fallback; ensure `Close()` can't near the 5s hang-detector window
   under load) as a fast, independent, low-risk fix for today's concretely reproducing
   trigger.
4. `slogDefaultMu` can likely be deleted once every swap site is migrated off
   `slog.SetDefault()` — flag this as a Phase 3/5 cleanup decision, not required for the
   fix itself (keeping it briefly as a no-op safety net during migration is fine).
