# BUG-079: `classifyHeadlessCallError` has two structurally unreachable branches, leaving most triage failures undiagnosable as "other" [SEVERITY: Medium]

**Status**: ✅ Fixed (2026-08-16)
**Discovered**: 2026-08-16 — 17 of 19 currently-parked production backlog items (auto-triage
retries exhausted at the 5-attempt cap) have failure reason "other" with no captured raw
output, making their actual root cause permanently unrecoverable.
**Fixed**: 2026-08-16 — `session/headless/runner.go`, `session/headless/caller.go`,
`server/services/backlog_service_triage.go`

## Problem Description

`classifyHeadlessCallError` (`server/services/backlog_service_triage.go:2317-2347`) buckets a
failed headless (`claude -p`) call into `timeout`/`shutdown`/`process_error`/`claude_not_found`/
`other` for log-grepping and for the `EndReason` persisted on the item's session row. Two of its
branches were structurally unreachable:

1. **`process_error` was dead code.** It matched `errors.Is(err, headless.ErrLLMError)`,
   `ErrUsageError`, `ErrInterrupted` — sentinels doc-commented as firing "when claude exits with
   code 1/2/130" — but no production call site ever constructed them. `executor/managed_process.go`'s
   `reap()` computes the real subprocess exit code via `exitErr.ExitCode()`, but that value only
   ever feeds the audit log (`executor/audit.go`); it is never translated into these sentinels and
   returned up through `ProcessRunner.Run`/`session/headless/caller.go`'s `call()`. The only
   "failure" `call()`'s first-call branch actually inspects is the CLI's own JSON `is_error` field
   — a distinct, app-level signal from the OS-level exit code. Confirmed via repo-wide grep
   (excluding tests): `grep -rn "ErrLLMError\|ErrUsageError\|ErrInterrupted"` matched only the
   sentinel declarations, the dead `classifyHeadlessCallError` case, and three test rows that
   existed purely to exercise that dead case.

2. **Subprocess-start failures always produced `raw==""`, and nothing classified them.** In
   `caller.go`'s `call()`, if `runner.Run(ctx, args, stdinReader)` itself failed (`os.Pipe()`
   failing under fd exhaustion, or `cmd.Start()` failing under `ENOMEM`/`ENOENT`/`EACCES`), the
   function returned immediately before any `StreamChunk` was ever produced. `CallBlocking`
   therefore returned `("", 0, err)` — `raw` was structurally empty. `classifyHeadlessCallError`
   had no case for this, so it fell to `default: return "other"`. Because `raw==""`,
   `captureHeadlessFailure` also returned `""` immediately — nothing gets written to
   `~/.stapler-squad/headless-failures/`, so there's no diagnostic file to inspect after the fact
   either. Verified: none of the 19 real parked items' session UUIDs have a corresponding capture
   file on the production host.

Leading hypothesis for what's affecting production: the 2026-07-24 stuck-triage incident
correlated with swap exhaustion and zombie `claude -p` processes — exactly the conditions under
which `os.Pipe()`/`cmd.Start()` fail at the OS level, invisibly swallowed into "other".

**A third, smaller, related bug** in the same file: `caller.go`'s read-error branch (triggered
when the subprocess is killed mid-write, e.g. OOM-killed) discarded any partial `data` already
read instead of sending it as a `Text` chunk before the terminal `Err` chunk. The adjacent
parse-failure branch three lines later DID send accumulated `data` as a `Text` chunk before its
error — this asymmetry meant a subprocess killed partway through with useful partial output got
zero diagnostic capture, while one that completed with malformed JSON did.

## Fix

1. **`process_error` (dead code):** removed rather than wired up. Wiring real exit-code
   translation would require calling `stop()`'s exit-code-bearing return *before* each of
   `call()`'s several error-return branches (JSON-parse-failure, read-error, resumed-call
   scanner-error) instead of leaving it in the trailing `defer`, and would mostly be moot in
   practice anyway: exit 130 (SIGINT) is already pre-empted by this codebase's own ctx-cancellation
   kill path, which `classifyHeadlessCallError` already buckets as `timeout`/`shutdown` *before*
   reaching this case (switch order). That was judged not "minimal" for this fix — removed the
   dead branch, the three unused sentinels (`ErrLLMError`/`ErrUsageError`/`ErrInterrupted`), and
   corrected the doc comment to state plainly that OS exit-code classification isn't implemented
   today (only captured in the audit log). A future change can reintroduce it as its own,
   deliberately scoped piece of work if the exit-code signal turns out to matter in practice.
2. **Subprocess-start failures:** added `headless.ErrSubprocessStart`
   (`session/headless/runner.go`), wrapped around the runner-start error in `caller.go`'s `call()`
   (`return ch, fmt.Errorf("headless runner start: %w: %w", ErrSubprocessStart, err)`).
   `classifyHeadlessCallError` gained a matching case bucketing it as `"subprocess_start_error"`
   instead of `"other"`. `raw` is still `""` for this bucket (nothing was ever read from the
   subprocess) — that part is unfixable, but the failure is now labeled distinctly in logs and the
   persisted `EndReason`, closing the class going forward (existing parked items won't retroactively
   get better data).
3. **Partial-data-on-read-error:** mirrored the adjacent parse-failure branch's existing behavior —
   send accumulated `data` as a `Text` chunk before the terminal `Err` chunk when the read itself
   fails mid-stream.

Scope guard respected: did not touch `result.Title` sanitization, `filepath.Join` calls, or
commit-message construction in the same file (owned by a parallel fix-bug run), and did not touch
remediation-gating control flow (`RemediationDue`, `evaluateRemediation`, the backoff schedule, or
which reasons trigger which retries) — confirmed nothing branches control flow on the `errType`/
`EndReason` string value except the existing `"shutdown"` special-case in
`session/backlog_lifecycle_triage.go:237`, which is untouched (`"shutdown"`'s bucket name and
trigger condition, `errors.Is(err, context.Canceled)`, are both unchanged).

## Regression Tests

- `server/services/backlog_service_triage_test.go`:
  `TestClassifyHeadlessCallError_should_BucketErrorsForLogGrepping` — replaced the three
  `process_error` rows (now dead) with `"subprocess start error"` and `"wrapped subprocess start
  error"` rows asserting `headless.ErrSubprocessStart` (bare and `fmt.Errorf`-wrapped, matching the
  file's existing "wrapped ctx deadline exceeded" style) classify as `"subprocess_start_error"`.
- `session/headless/pool_test.go`:
  - `TestPool_CallBlocking_PropagatesSubprocessError` — extended to assert
    `errors.Is(err, ErrSubprocessStart)` and that the underlying OS error remains inspectable via
    `errors.Is`.
  - `TestPool_CallBlocking_ReadError_ReturnsPartialDataAsRaw_When_SubprocessKilledMidWrite` (new) —
    a `partialErrRunner`/`partialErrReadCloser` test double returns real data on the first `Read`
    then a non-EOF error on the next, proving `CallBlocking`'s `raw` return now contains the
    partial output instead of `""`.

## Verification

`go test ./session/headless/... ./session/... ./server/services/...` and `make lint` all pass
(see PR for full output). `make build` (protobuf/ent codegen + web UI + Go binary) succeeds.

## Phase D — Reflect

**Classification**: API Contract Gap (bucket 1) + Integration Gap (bucket 2). The
`classifyHeadlessCallError` doc comment described a contract ("these sentinels fire on exit
1/2/130") that no code ever implemented — the doc and the switch statement drifted from what
`ProcessRunner`/`caller.go` actually do. Separately, `runner.Run`'s start-failure path was never
integrated with the classification layer at all — a new failure mode (subprocess-start failure)
was added to the system's behavior (implicitly, by relying on the OS) without a corresponding
classification case ever being added.

**Earliest catchable point**: A unit test (Phase B's regression tests) is the earliest achievable
level here — there's no type-system or lint-rule way to enforce "every sentinel error type
declared in package X has a corresponding `errors.Is` case in classifier Y in package Z" in Go
without a custom static-analysis rule, which would be disproportionate machinery for a five-case
switch statement. The dead-code detector class of bug (declared-but-never-constructed sentinel)
*could* in principle be caught by `unused`-style linting if Go's linters tracked "constructed vs.
only referenced in a type-guard" — but standard `go vet`/`staticcheck` correctly consider a
sentinel "used" once it appears in any `errors.Is` call, so this specific shape (dead in
*production* code, alive only in a classifier and its tests) isn't mechanically detectable short
of a custom check. The regression tests are the right level.

**Recurring shape**: None identified — this is the first instance of "a classifier's doc comment
promises a translation the code never performs" found in this codebase's bug history.
