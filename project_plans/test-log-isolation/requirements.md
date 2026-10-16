# Requirements: test-log-isolation

**Date**: 2026-08-29
**Type**: bug fix (test reliability / `-race` flake)
**Complexity**: 1 — bug fix with Small appetite (root cause and mechanism are already conclusively identified from a primary-source CI log; scope is a targeted synchronization/isolation fix in `server/services` test code, not a redesign)

## Problem Statement
`server/services`'s test binary shares one process-global logger
(`slog.Default()`, which Go's stdlib also uses to back `log.Print` — see
Baseline) across every test in the package. Any test whose code path (directly
or via a background goroutine, timer, or slow-closing resource such as
`httptest.Server`) logs while a `captureLogs(t)`-style test elsewhere in the
binary has swapped that global logger for a `bytes.Buffer` can race that
buffer: one goroutine writes into it via the slog handler while the owning
test concurrently reads it with `buf.String()`. `-race` correctly flags this
as a genuine, unsynchronized data race — and because `go test -race` fails
every test running in the same process at the moment a race fires, one such
collision fails a double-digit number of otherwise-unrelated, otherwise-green
tests in the same CI run, forcing an investigation (or a reflexive rerun) each
time it lands on someone's PR.

`slogDefaultMu` (`server/services/autonomous_orchestration_service_test.go:414`,
added by PR #576) already serializes the tests that know about the
convention and explicitly hold it around their own swap-to-restore window.
It does nothing for a test that has no reason to know it exists — which is
exactly the failure mode below.

## Baseline
Two confirmed instances of this race class exist today:

1. **Fixed** — `TestHubRegistryAndStreamOwnershipLock_should_NeverProduceTwoOwners_When_RacedConcurrently`
   (`server/services/connectrpc_websocket_test.go:758`) used to leak a
   `pumpControlModeOutputIntoHub` goroutine per iteration (up to 1000) that
   only exited on `HubTornDown`; each leaked goroutine kept `log.Warn`-ing
   into whatever the global logger currently pointed at. Confirmed fixed on
   `origin/main`: the loop now calls `hub.ForceTeardown()` every iteration
   (`connectrpc_websocket_test.go:808`, `:835`).

2. **Not fixed** — `TestAnthropicAIClient_Complete_CancelsOnCtxDone`
   (`server/services/anthropic_client_test.go:22`) spins an `httptest.Server`
   whose handler blocks on `r.Context().Done()` with a `time.After(10*time.Second)`
   fallback (lines 29-32). This test does not use `captureLogs` and has no
   reason to hold `slogDefaultMu` — it never touches `slog` directly. But
   under `-race` + CI-runner load, `httptest.Server.Close()`'s internal
   5-second hang detector fired and called `net/http/httptest.(*Server).logCloseHangDebugInfo`
   → stdlib `log.Print` → (per Go's documented behavior: `slog.SetDefault`
   redirects the stdlib `log` package's default logger through the same
   handler) → `log/slog.(*TextHandler).Handle` → `bytes.(*Buffer).Write`,
   racing `TestSlackNotifier_NeverLogsWebhookURL`'s `buf.String()` read
   (`slack_notifier_test.go:704`) in the same process.

   **Verified against the primary source**: the exact stack trace above is
   from the real CI failure
   ([job 97997339203, run 32907965981](https://github.com/tstapler/stapler-squad/actions/runs/32907965981/job/97997339203)),
   fetched via `gh api repos/tstapler/stapler-squad/actions/jobs/97997339203/logs`.
   That single race pair cascaded into **15** `--- FAIL: ... race detected
   during execution of test` lines in that run — entirely unrelated tests
   (`TestDeleteSession_ByUUID`, `TestForkSession_SessionNotFound`,
   `TestFileChangeToProto`, `TestValidateCallbackURL_Rejects*`,
   `TestScanForSecrets_*`, etc.) that happened to be running in parallel in
   the same process when the detector fired — confirming the item's
   "~14 unrelated test failures" claim (actual count: 15).

   This instance is unfixed on `origin/main` as of this triage
   (`anthropic_client_test.go` still uses a plain `defer func(){
   srv.CloseClientConnections(); srv.Close() }()` with no bound on Close()'s
   internal hang detector, and does not hold `slogDefaultMu`).

Without a fix, any future test anywhere in `server/services` that starts a
background goroutine, timer, or slow-closing resource is a latent trigger —
the failure is not localized to the file that "caused" it, which is exactly
the pattern `.claude/rules/fix-flaky-tests-dont-defer.md`-style guidance
warns gets re-excused as "known pre-existing flake."

**Note on this triage's own worktree**: this triage was performed in a
worktree branched ~559 commits behind `origin/main`; none of the symbols
above (`slogDefaultMu`, `ForceTeardown`, `slack_notifier_test.go`) exist in
that stale worktree. All facts in this document were independently
re-verified directly against `origin/main` (via `git show`/`git grep
origin/main`) and the real GitHub Actions log, not the stale worktree
checkout. Implementation (Phase 5) must branch from current `origin/main`,
not from this triage worktree's base commit.

## Users / Consumers
- stapler-squad contributors whose PRs run `make ci` / the `Test` GitHub
  Actions job (`go test -race ./...`, `.github/workflows/build.yml`).
- Future contributors adding any test file to `server/services` — the fix's
  durability determines whether they inherit a footgun or a safe default.

## Success Metrics
- `TestAnthropicAIClient_Complete_CancelsOnCtxDone` no longer races any
  `captureLogs`-style test, verified locally via a stress repro (`go test
  -race -run 'TestAnthropicAIClient_Complete_CancelsOnCtxDone|TestSlackNotifier_NeverLogsWebhookURL'
  -count=20 ./server/services/...`) before and after the fix.
- No regression in the existing, working synchronization for instance #1
  (`connectrpc_websocket_test.go`'s `ForceTeardown`-per-iteration pattern) or
  for the tests that already cooperate via `slogDefaultMu`.
- The fix generalizes: a new test added later that starts an
  `httptest.Server`/background goroutine without knowing about
  `slogDefaultMu` must not be able to reintroduce this race class (either
  because nothing in the package still depends on swapping the process
  global, or because the convention is enforced structurally rather than by
  every author remembering to opt in).

## Appetite
Small (1–2 days). This is a targeted synchronization/test-isolation fix in
already-identified files, not a redesign of the package's logging
architecture.

## Constraints
- Must not weaken `-race` coverage anywhere in `./...` — the fix must
  eliminate the race, not paper over it (e.g. not `-race`-disabling the
  affected test file).
- Must not change production logging behavior/observability (this is a test
  infrastructure fix).
- No new external dependencies.

## Non-functional Requirements
- **Performance SLO**: not applicable (test-only change).
- **Scalability**: not applicable.
- **Security classification**: internal (test code only).
- **Data residency**: not applicable.

## Scope
### In Scope
- Eliminating the confirmed race between `TestAnthropicAIClient_Complete_CancelsOnCtxDone`
  (or any test whose `httptest.Server` can trigger the stdlib hang-detector
  log line) and any `captureLogs`-style test.
- Choosing and implementing one of the two directions the backlog item
  already named:
  1. Give `captureLogs`-style tests (and any code they exercise) a
     scoped `*slog.Logger` instance instead of swapping the process-global
     default, eliminating the shared-state race at the root — evaluate how
     much call-site threading this actually requires (see Research: 386
     `log.Warn/Info/Error/Debug` call sites in `server/services/*.go` route
     through the package's own `log` wrapper around `slog.Default()`,
     not `slog` directly).
  2. Or: bound/eliminate the specific trigger (e.g., give
     `TestAnthropicAIClient_Complete_CancelsOnCtxDone`'s `httptest.Server`
     a fast, deterministic teardown that can't hit the 5s hang detector
     under `-race` load — the same class of fix already applied to instance
     #1).
- Whichever direction is chosen, documenting the resulting convention
  clearly enough that it doesn't silently rot the way the current
  `slogDefaultMu` doc comment's limitation did.
- A regression check (stress-run command and/or a small test) proving the
  specific confirmed race is gone.

### Out of Scope
- A full audit/rewrite of every `httptest.Server`/background-goroutine test
  in `server/services` "just in case" — fix the confirmed instance and make
  the general pattern structurally safer; do not chase hypothetical future
  instances speculatively (that is explicitly deferred as "Suggested fix
  direction 2 (incremental audit)" in the backlog item, and is a
  poor fit for a Small appetite).
- Any change to production logging code paths' behavior or output format.
- Re-litigating or reverting PR #576's `atomicLogger`
  (`WarningLog`/`InfoLog`/`ErrorLog`) fix — that addressed a different race
  (package-level `*log.Logger` var swaps) and is confirmed still correct.

## Rabbit Holes
- **Threading a logger through 386 call sites**: if Direction 1 (scoped
  logger injection) is chosen, resist expanding it into a full DI pass over
  every `log.Warn`/`log.Info` call in `server/services/*.go` — the interface
  used by `captureLogs`-based tests may be a much smaller subset. Time-box
  discovery of the actual affected call sites before committing to full
  threading.
- **Go stdlib internals**: confirming exactly how `slog.SetDefault` redirects
  `log.Print` is already done (it's documented stdlib behavior, and directly
  confirmed via the CI stack trace) — do not re-derive this from scratch in
  Phase 2 research; treat it as settled.

## Alternatives Considered
- **Widen the hang-detector's tolerance / avoid triggering it** (e.g. close
  the `httptest.Server`'s client connections more aggressively, or use a
  context that resolves before 5s under load): fixes this specific trigger
  cheaply, consistent with how instance #1 was fixed, but doesn't address
  the root shared-global-state problem — a different background
  goroutine/slow resource in a future test could reintroduce the same class.
- **Scoped logger injection**: addresses the root cause for good, at the
  cost of touching more call sites and needing to verify no production
  behavior changes.
- **Serialize the whole package's test run** (`-p 1`, or disable
  `t.Parallel()` package-wide): would eliminate the race by removing
  concurrency entirely, but drastically slows `make ci` for a package this
  size and papers over the actual bug rather than fixing it — rejected.

## Feasibility Risks
- If Direction 1 (scoped logger) is chosen and it turns out `captureLogs`
  needs to observe log lines from code paths that are hard to inject a
  logger into (e.g. via context rather than a struct field), the appetite
  may not hold — flag this early in Phase 3 planning and fall back to
  Direction 2 if so.
- The fix must be validated against `origin/main`, not this stale triage
  worktree (559 commits behind) — implementation must re-verify file
  line numbers cited here against the actual branch point used.

## Observability Requirements
Not applicable (complexity 1) — standard CI failure visibility (GitHub
Actions run status + logs) is sufficient.

## Risk Control
Not needed — low risk (complexity 1). Test-only change; standard rollback
via PR revert if it destabilizes CI further.

## Open Questions
- Should the fix target only the two confirmed instances, or add a
  structural guard (e.g. a lint rule or `TestMain` check) that prevents a
  third instance from recurring? Resolve during Phase 3 planning — appetite
  currently assumes "fix confirmed instances + make the convention safer by
  construction," not "add new tooling."
