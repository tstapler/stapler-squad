# ADR-001: Opt-in interpreter field on ProcessRunner for test-only fixture invocation

**Status**: Accepted
**Date**: 2026-08-06

## Context

Three test clusters in `session/headless` (`capability_check_test.go`,
`pool_test.go`) write a `#!/bin/sh` fake-claude fixture script to a fresh
`t.TempDir()` path via `os.WriteFile(path, script, 0o755)`, then hand that
path to `ProcessRunner` as `claudeBin`. `ProcessRunner.Run`
(`session/headless/runner.go:129`) forks/execs that path directly via
`executor.StartProcess` → `safeexec.CommandContext` → `exec.CommandContext`
— Go does not consult `PATH` or a shell when `name` contains a path
separator; it execs the literal path via `syscall.Exec`. This direct
exec-by-path of a freshly-written, freshly-chmod'd file is exactly the
pattern OS-level exec restrictions (Gatekeeper, TCC, or third-party endpoint
security/MDM software on macOS) can refuse with `operation not permitted` —
see `project_plans/fix-flaky-headless-tests/research/stack.md` §1 and §3 for
the full evidence trail. This cannot be reproduced or verified on this
(Linux) machine; the fix must be correct by inspection and root-cause
reasoning, then confirmed on macOS post-merge (acceptance criterion 6).

Three shapes were considered for the fix (full comparison in
`implementation/plan.md`'s Step 0.5 write-up, repeated in the Pattern
Decisions table below). The chosen shape: give `ProcessRunner` an opt-in,
unexported `interpreter string` field. When set, `Run` execs `interpreter`
with `claudeBin` prepended to argv instead of exec'ing `claudeBin` directly
— i.e. `sh <scriptPath> <claude CLI args...>` instead of `<scriptPath>
<claude CLI args...>`. The field defaults to `""` for every production
caller and is populated only by a new test constructor
(`NewShellWrappedProcessRunnerForTesting` in `fake_runner.go`).

## Decision

Add the `interpreter` field to `ProcessRunner` (`session/headless/runner.go`)
rather than (a) leaving the exec path as direct-by-path and only adding a
skip/diagnostic on refusal, or (b) branching on `runtime.GOOS` at the call
site.

## Consequences

- **Zero production behavior change**: every non-test `ProcessRunner`
  construction leaves `interpreter` as the zero value `""`, so `Run` takes
  the exact same `executor.StartProcess(ctx, r.claudeBin, args, ...)` path
  it does today. Verified by `git grep 'ProcessRunner{' session/headless/*.go`
  outside `_test.go` — the only non-test constructor never sets this field.
- **`executor.StartProcess`/`safeexec` are untouched** — matches
  requirements.md's explicit scope boundary ("no change to
  `executor.StartProcess`/`safeexec` beyond what's needed for the fake-claude
  test-script invocation path").
- **New pattern for this repo**: no existing test in this codebase invokes a
  written-to-disk fixture script through an explicit interpreter (confirmed
  by `grep -rn '"sh"\|"/bin/sh"' --include='*_test.go'` — every existing hit
  launches `sh` itself as the subprocess, e.g. an interactive tmux shell,
  never as a wrapper around a separately-written script file). This ADR
  exists specifically because it is a new idiom, not a reuse of an existing
  one — future readers should not assume precedent elsewhere in the repo.
- **Unverifiable in this session**: this fix cannot be confirmed against a
  live recurrence on Linux, since the failure mode never reproduces here.
  Verification is deferred to a human running the affected tests on macOS
  post-merge — tracked as its own task (see `implementation/plan.md` Story
  for AC6).
- **The `interpreter` field boundary is convention-based**:
  `fake_runner.go` is an exported test-helper file; constructor boundaries are enforced by naming conventions and code review rather than compiler-enforced build-tag isolation.

## Alternatives rejected

| Alternative | Why rejected |
|---|---|
| Skip-with-diagnostic on exec-refusal error string match (mirrors `backlog_triage_harness_test.go`'s `checkPoolStartAllowed`) | Strictly weaker: leaves the tests unable to actually exercise the code path on an affected machine — a skip, not a fix. Documented as the fallback if interpreter-invocation proves insufficient at some call site, not adopted as the primary fix. |
| `runtime.GOOS`-conditional invocation (branch to `sh <path>` only on `darwin`) | A GOOS-conditional fix path is itself only verifiable on the platform it targets, and it's a strictly worse version of the interpreter fix — invoking via `sh` is safe/inert on Linux too, so there is no reason to gate it by OS at all. Adds branching complexity for no benefit. |
