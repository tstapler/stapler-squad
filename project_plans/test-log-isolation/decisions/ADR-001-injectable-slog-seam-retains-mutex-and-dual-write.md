# ADR-001: Injectable slog seam retains `slogDefaultMu` and writes production's logger to two slots

**Status**: Proposed (Phase 3 planning output for `test-log-isolation`)
**Date**: 2026-08-29

## Context

The fix adds `log/log.go`'s `slogDefault atomicSlogLogger` + `SetSlogDefaultForTest`
seam (see `implementation/plan.md` Story 1) so `server/services` test helpers
stop calling the real, process-global `slog.SetDefault()`. Two non-obvious
questions came up during planning that a future maintainer could plausibly
get wrong:

1. **Does `slogDefaultMu` (`server/services/autonomous_orchestration_service_test.go:414`)
   become unnecessary once nothing calls `slog.SetDefault()` anymore?**
   `atomic.Pointer`-based swaps are torn-read-free by construction, so it's
   tempting to conclude the mutex is now redundant.
2. **Does `log/log.go`'s production code (`initializeWithConfig`) need to
   write the configured logger into *two* places** — the real
   `slog.SetDefault()` and the new `slogDefault.Store()` — **or would picking
   one be simpler?**

## Decision

1. **Keep `slogDefaultMu`, repointed to guard `SetSlogDefaultForTest` swaps
   instead of raw `slog.SetDefault()` swaps.** Do not delete it.
2. **`initializeWithConfig` writes the same `*slog.Logger` instance into both
   `slog.SetDefault()` and `slogDefault.Store()`.** Neither call is removed.

## Rationale

### Why the mutex is still required

An atomic pointer swap prevents a *torn read* of "which pointer is current" —
it does not prevent two logical owners from disagreeing about who owns the
slot during an overlapping window. If two `t.Parallel()` tests both call
`log.SetSlogDefaultForTest(bufA)` and `log.SetSlogDefaultForTest(bufB)`
without mutual exclusion, whichever swap lands last "wins," and any log line
emitted by the *other* test's code in the interim silently goes missing from
its own buffer — a logical/assertion bug, not a `-race` finding, but a real
correctness problem for the exact reason `slogDefaultMu` was introduced in
the first place.

This is not a hypothetical: this repo already has a proven case of the same
class of seam needing the same treatment. `server/services/backlog_service_pipeline_mode_test.go`'s
`warningLogMu` (a package-level `sync.Mutex`) exists specifically to guard
`SetWarningLogForTest` — which is exactly the `atomicLogger`/`atomic.Pointer`
swap-with-restore pattern this fix's `slogDefault`/`SetSlogDefaultForTest`
mirrors. Its own doc comment states the reasoning verbatim: *"SetWarningLogForTest
reassigns the shared package-level logger wholesale, so two parallel tests
calling it concurrently would each redirect the same global and race over
whose buffer is 'current.'"* The fact that `atomicLogger` already uses
`atomic.Pointer` internally did not make `warningLogMu` unnecessary; the same
reasoning applies unchanged to `slogDefault`/`SetSlogDefaultForTest`.

**Conclusion**: keeping `slogDefaultMu` (repointed) is correct, not
leftover caution. Its doc comment is updated (Task 2.1) to describe the new
target, but the mutex itself, its call sites, and the tests that rely on
`t.Parallel()` being absent from these paths, are otherwise unchanged.

### Why production still writes to both slots

`initializeWithConfig`'s existing `slog.SetDefault(...)` call
(`log/log.go:979`) does double duty today: it makes `slog.Default()` return
the configured production handler, *and* (per stdlib's documented behavior)
it redirects stdlib `log.Print`/`log.Printf` calls process-wide into that
same handler — this is intentional production behavior (the file's own
comment: *"Install async slog bridge so log.Printf calls route through
slog"*), used to capture any bare `log.Print` call anywhere in the binary
(third-party libraries, stdlib internals) into the structured log pipeline.
Removing this call, or replacing it with only `slogDefault.Store(...)`, would
silently stop that capture in production — a regression the requirements
explicitly forbid ("Must not change production logging behavior/observability").

Conversely, if only `slog.SetDefault()` were called (today's behavior) and
`slogDefault` were left unseeded by production config, then after Story 1's
`logAt`/`ForSession` rewire, every `log.Warn`/`log.Info`/`log.ForSession(...)`
call in production would revert to the plain, unconfigured stdlib default
(no file output, no JSON formatting, no trace IDs) — a severe, silent
production logging regression, while stdlib `log.Print` calls would
*still* correctly reach the real configured handler. This asymmetry (one
logging path silently degrades, the other doesn't) would be a confusing,
hard-to-diagnose partial regression if introduced.

**Conclusion**: both calls are required, permanently, not just during a
migration window. `implementation/plan.md`'s Task 1.3 constructs the
`*slog.Logger` once and stores it into both slots specifically to make this
pairing visible at the call site and prevent the two from drifting apart
(e.g., a future edit that "simplifies" this by reconstructing the logger
twice, or removing one call thinking it's redundant).

## Consequences

- `log/log.go` permanently carries two logically-linked writes
  (`slog.SetDefault` + `slogDefault.Store`) at every point production
  configures logging. A code comment at the call site (Task 1.3) and this
  ADR are the durable record of why; a future refactor should grep for both
  `slog.SetDefault(` and `slogDefault.Store(` in `log/log.go` before removing
  either.
- `slogDefaultMu` remains in `server/services/autonomous_orchestration_service_test.go`
  indefinitely (or until every `captureLogs`-style test in the package is
  refactored to not need cross-test exclusion at all — out of scope here).
- No new external dependency, no schema/config change — this ADR documents a
  design nuance within an otherwise Small-appetite fix, not a reversal of any
  Alternative already rejected in requirements.md.
