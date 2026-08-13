# Implementation Plan: fix-flaky-headless-tests

**Feature**: Harden clusters 1+2 (`session/headless` fake-claude fixture
invocation) against OS-level exec refusal via an opt-in interpreter seam on
`ProcessRunner`, and tighten cluster 3's (`gogitstore` mmap truncation)
crash-classification to confirm SIGBUS/SIGSEGV specifically — with explicit
documentation of the parts already correct and not touched.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: [ADR-001: Opt-in interpreter field on ProcessRunner for test-only fixture invocation](../decisions/ADR-001-interpreter-invocation-for-test-fixtures.md)

---

## Step 0.5 — Creative pass: alternatives for the clusters 1+2 fix

Three distinct approaches were considered for making the fake-claude fixture
invocation robust to OS-level exec refusal:

**A. Interpreter-invocation via an opt-in `ProcessRunner.interpreter` field.**
Strength: makes the tests actually *work* on an affected machine (not just
fail cleanly), is inert on platforms where the failure never reproduces
(Linux), and requires zero change to `executor.StartProcess`/`safeexec`.
Weakness: it's a genuinely new idiom for this repo (no existing test wraps a
written fixture script through an explicit interpreter), so it needs an ADR
and can't lean on in-repo precedent for review confidence.

**B. Skip-with-diagnostic on exec-refusal error string match** (mirror
`server/services/backlog_triage_harness_test.go`'s `checkPoolStartAllowed`).
Strength: exact in-repo precedent exists, already refined once
(narrowed after `Setpgid`/`Setsid` fix), so the idiom is proven and
low-review-risk. Weakness: strictly weaker outcome — an affected machine
still can't exercise the code path under test, it just fails less
confusingly; doesn't fix anything, only diagnoses.

**C. `runtime.GOOS`-conditional invocation** (branch to `sh <path>` only when
`GOOS == "darwin"`). Strength: scopes the behavior change explicitly to the
platform believed to need it, arguably easier to reason about in review.
Weakness: a GOOS-conditional path is itself only verifiable on the platform
it targets — no safety benefit over unconditional interpreter-invocation,
since `sh <path>` is harmless on Linux too — so the conditional only adds
branching complexity for zero gain.

**Decision: A, primary.** B is retained as the documented fallback idiom for
any call site where A proves infeasible (research found none — all 6 call
sites migrate cleanly onto A). C is rejected outright; not used anywhere in
this plan. See ADR-001 and the Pattern Decisions table below for the
rejected-alternative record.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `ProcessRunner` | Production `ClaudeRunner` implementation (`session/headless/runner.go`) that forks/execs the real `claude` CLI (or, in tests, a fake-claude fixture script) via `executor.StartProcess`. | Struct fields are unexported; tests can only construct one via `fake_runner.go`'s helpers. |
| `interpreter` field | New unexported `string` field on `ProcessRunner`. When non-empty, `Run` execs `interpreter` with `claudeBin` prepended to argv instead of exec'ing `claudeBin` directly. Zero value `""` for every production caller. | The core of this fix's seam; see ADR-001. |
| fake-claude fixture | A small `#!/bin/sh` script a test writes to `t.TempDir()` to stand in for the real `claude` CLI binary, so `ProcessRunner`/`Pool` can be exercised against real subprocess semantics without invoking the actual LLM. | Written via `os.WriteFile(path, script, 0o755)` in 3 near-duplicate helpers. |
| `NewProcessRunnerForTesting` | Existing test constructor (`fake_runner.go:152-154`) that builds a `*ProcessRunner{claudeBin: claudeBin}` for tests needing real WorkDir/tool-access propagation (which `FakeRunner` can't provide — `Pool.CallWithOptions`'s WorkDir path type-asserts on the concrete `*ProcessRunner`). | Remains unchanged; still the right choice for any future test that constructs `ProcessRunner` around a real, already-trusted binary. |
| `NewShellWrappedProcessRunnerForTesting` | New sibling constructor (`fake_runner.go`) that builds a `*ProcessRunner{claudeBin: scriptPath, interpreter: "sh"}` — the fix's primary call-site API. | Added by this plan; used everywhere a test writes its own fake-claude script. |
| `CodebaseReadCapabilitySelfCheck` | Production type (`session/headless/capability_check.go`) whose `Ensure`/`run` methods run a one-shot smoke-test subprocess and cache the result (`sync.Once` + `atomic.Bool`) for the process lifetime. | Its caching logic is confirmed correct by direct reading and by 5x `-race` passes on Linux — see AC2/AC7 below. Not touched by this plan. |
| cluster 1 | `session/headless/capability_check_test.go`'s `TestCodebaseReadCapabilitySelfCheck_*` tests — reported flaky on macOS. | Root cause: shared with cluster 2 (see below), not a caching bug. |
| cluster 2 | `session/headless/pool_test.go`'s WorkDir-path tests, primarily `TestPool_CallBlocking_WithWorkDir_ReturnsCostAndUsesWorkDir`. | Reported error `fork/exec .../fake-claude.sh: operation not permitted` — the textbook signature this plan's fix targets. |
| cluster 3 | `session/unfinished/gogitstore/mmap_truncation_test.go`'s `TestMmapIndexHandle_TruncateWhileMapped_CrashesWithoutProtection`. | Self-documented platform-dependent UB test; already tolerant of both outcomes since commit `dccee742a`. |
| `runTruncHelper` | The re-exec'd subprocess body (`mmap_truncation_test.go:187-223`) that deliberately triggers a truncated-mmap read and prints one of two markers (`NO_FAULT_OCCURRED`, `ORDINARY_RECOVER_CAUGHT_IT`) or crashes outright. | Not modified by this plan — the marker/tolerant-outcome design is architecturally sound (Agent 3 research). |
| `syscall.WaitStatus` signal check | The stdlib facility (`os.ProcessState.Sys().(syscall.WaitStatus)`, `.Signaled()`, `.Signal()`) this plan adds to confirm a subprocess death was actually `SIGBUS`/`SIGSEGV`, not some unrelated failure. | Unix-only type; this repo already gates equivalent usage behind `//go:build !windows` in `session/tmux/zombie_reaper.go` — this plan follows that exact precedent. |
| `safeexec.CommandContext` | This repo's established `exec.CommandContext` wrapper (`executor/safeexec/safeexec.go`) that pre-sets `WaitDelay` to avoid zombie-process accumulation. | Already used by `mmap_truncation_test.go`; no change needed there. The clusters 1/2 fix stays inside `ProcessRunner`/`executor.StartProcess`, which already routes through `safeexec` — no new raw `exec.Command` is introduced anywhere in this plan. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `ProcessRunner` fake-claude invocation (clusters 1+2) | No GoF/PoEAA pattern — a small, additive, opt-in struct field plus a branch in one method. Not a Strategy/Decorator; forcing a pattern name onto a 6-line change would be over-engineering for a bugfix of this size. | ADR-001; `research/architecture.md` §2 | (B) Skip-with-diagnostic on error-string match | Strictly weaker outcome (diagnoses but doesn't fix); retained as documented fallback only, not adopted as primary. |
| `ProcessRunner` fake-claude invocation (clusters 1+2) | (same as above) | ADR-001; `research/stack.md` §1 | (C) `runtime.GOOS`-conditional invocation | A GOOS-conditional fix is itself unverifiable except on the targeted platform, with zero safety benefit over an unconditional fix that's already harmless on Linux. |
| Cluster 3 signal classification | No pattern — a small additive helper function (`isExpectedFaultSignal`) called from one existing branch. Split into two files by `//go:build` tag (matches `session/tmux/zombie_reaper.go`/`zombie_reaper_windows.go` precedent), not a new abstraction layer. | `research/stack.md` §2; `research/architecture.md` §3; `session/tmux/zombie_reaper.go` precedent | Type-asserting `syscall.WaitStatus` directly inline in `mmap_truncation_test.go` with no build tag | Would be this package's first unix-only import with no guard (`grep` confirms `gogitstore` currently has zero `syscall`/`unix` imports anywhere, production or test) — `go build .`/CI never cross-compiles this test file for Windows today, but a native Windows contributor running `go vet ./...` on this package would break. Matches this repo's own established mitigation for the identical hazard in `zombie_reaper.go`. |
| Cluster 1 caching logic (`CodebaseReadCapabilitySelfCheck`) | No change — documented as already correct (AC2/AC7's "document, don't build" branch). | `requirements.md` pre-planning investigation; `research/architecture.md` synthesis | Rewriting/hardening the `sync.Once`/`atomic.Bool` caching logic | No evidence of a bug: single commit in the type's entire history, passes 5x under `-race`, and the reported symptom (`count stuck at 0`) is fully explained by the exec-refusal returning an error *before* the marker comparison — a caching-logic rewrite would be solving a problem that isn't there (YAGNI, AC7). |

---

## Observability Plan
- **Logs**: `t.Logf` messages in `mmap_truncation_test.go`'s `err != nil` branch become more precise (name the actual signal when confirmed, or say explicitly that the signal could not be confirmed as SIGBUS/SIGSEGV) — test-log-only, no production logging changes.
- **Metrics**: no new metrics required (this is test-only).
- **Alerts**: no new alerts required.

## Risk Control
- **Feature flag**: not gated — test-only change (`ProcessRunner.interpreter` defaults to `""` for every production path; `executor.StartProcess`/`safeexec` untouched), inert in production.
- **Rollback procedure**: standard revert via PR close + revert commit.
- **Staged rollout**: full rollout on merge (test-only); the macOS-verification task (Story 5.1.1) is a post-merge human confirmation step, not a rollout gate — clusters 1+2 already pass on Linux today (they don't currently fail there), so merging does not risk breaking Linux CI even if the macOS hypothesis later proves imprecise.

## Unresolved Questions
- [ ] Does the interpreter-invocation fix actually resolve the reported macOS failure? — blocks full confidence in Story 2.1.2 (clusters 1+2) — owner: a maintainer with macOS access (per the user's own `~/.claude/CLAUDE.md`, they work on both Manjaro and macOS) must run Story 5.1.1's verification task on a real Mac before this can be called "fixed" rather than "hardened."

## Dependency Visualization

```
Phase 1: Root-cause docs (AC0)
   └─ 1.1.1 Confirm existing docs satisfy AC0 (no new code)
            │
            ▼
Phase 2: Clusters 1+2 fix (AC1, AC2, AC7)
   2.1.1 ProcessRunner interpreter seam (runner.go, fake_runner.go)
            │
            ▼
   2.1.2 Migrate call sites (capability_check_test.go, pool_test.go)
            │
            ▼
   2.1.3 Document AC2/AC7 "no caching bug" finding (no code change)
            │
            ▼
Phase 3: Cluster 3 tightening (AC3, AC4, AC7)              [independent of Phase 2]
   3.1.1 Add unix/windows signal-check helper files
            │
            ▼
   3.1.2 Wire helper into mmap_truncation_test.go's err!=nil branch
            │
            ▼
Phase 4: Verification (AC5)                    [depends on Phase 2 + Phase 3]
   4.1.1 make build && make test + targeted -count=5/-race reruns
            │
            ▼
Phase 5: macOS follow-up tracking (AC6)         [depends on Phase 4]
   5.1.1 Explicit post-merge macOS verification task (PR body, not code)
```

Phases 2 and 3 have no code dependency on each other and can be implemented
in either order or in parallel; both must complete before Phase 4.

---

## Phase 1: Root-Cause Confirmation

### Epic 1.1: Confirm AC0 is already satisfied
**Goal**: Verify (not re-derive) that root cause is stated and evidenced for
all three clusters before any code changes, per AC0.

#### Story 1.1.1: Root cause documented for all 3 clusters before code changes
**As a** maintainer reviewing this fix, **I want** the root-cause reasoning
for each cluster written down and evidenced before any code changes are
proposed, **so that** the fix is traceable to a stated hypothesis rather than
a guess.
**Acceptance Criteria**:
- AC0: Root cause is stated and evidenced (not assumed) for each of the three
  clusters before any test/production code changes are made.
  - *Given* `project_plans/fix-flaky-headless-tests/requirements.md`'s
    pre-planning investigation table and `research/stack.md` §1 and §3,
    *When* a reviewer checks whether clusters 1+2's root cause (OS-level
    exec refusal of a freshly-written script) is evidenced rather than
    assumed, *Then* they find: (a) cluster 2's own observed error text
    (`fork/exec .../fake-claude.sh: operation not permitted`) matching the
    documented signature, (b) a traced call graph showing both clusters
    share the identical write-then-exec-by-path pattern
    (`research/architecture.md` §1), and (c) confirmation this is a
    known Go/macOS failure class via 3 external sources
    (`golang-nuts` threads, `evilmartians/lefthook#157`). For cluster 3,
    they find prior commit `dccee742a`'s message directly describing the
    same tolerant-outcome fix already in place.
**Files**: `project_plans/fix-flaky-headless-tests/requirements.md`,
`project_plans/fix-flaky-headless-tests/research/stack.md`,
`project_plans/fix-flaky-headless-tests/research/architecture.md`,
`project_plans/fix-flaky-headless-tests/research/pitfalls.md` (all already
written; this story is a verification checkpoint, not new-document work).

##### Task 1.1.1a: Re-read the four existing docs and confirm AC0's evidence bar is met (~2 min)
- Re-read `requirements.md`'s pre-planning investigation table and
  `research/{stack,architecture,pitfalls}.md`'s root-cause sections.
- Confirm each of the 3 clusters has: a stated hypothesis, a traced code
  path, and either a reproduced symptom or an explicit statement of why it
  doesn't reproduce on this (Linux) machine.
- No code or doc changes — this task is a go/no-go gate before Phase 2/3
  begin implementation.
- Files: none (verification only).

---

## Phase 2: Clusters 1+2 — Interpreter Invocation Fix

### Epic 2.1: ProcessRunner interpreter seam
**Goal**: Make the fake-claude fixture invocation robust to OS-level exec
refusal without changing any production behavior, per AC1.

#### Story 2.1.1: Add opt-in interpreter field to ProcessRunner
**As a** test author, **I want** `ProcessRunner` to support exec'ing a fake
binary through an explicit interpreter, **so that** the fixture script is
never itself the thing the OS has to approve for direct execution.
**Acceptance Criteria**:
- AC1: Cluster 1/2's fake-claude invocation no longer depends on the OS
  permitting direct fork/exec of a freshly-written, freshly-chmod'd temp
  script.
  - *Given* a `ProcessRunner` constructed with `interpreter: "sh"` and
    `claudeBin: "/tmp/xyz/fake-claude.sh"`, *When* `Run` is called with args
    `["-p", "prompt", "--output-format", "json"]`, *Then*
    `executor.StartProcess` is invoked with `name="sh"` and
    `args=["/tmp/xyz/fake-claude.sh", "-p", "prompt", "--output-format",
    "json"]` — i.e. the OS is asked to exec the pre-existing, already-trusted
    `sh` binary, never the freshly-written script path directly.
  - *Given* a `ProcessRunner` constructed via the existing, unchanged
    production wiring (`interpreter` left at its zero value `""`), *When*
    `Run` is called, *Then* `executor.StartProcess` is invoked with
    `name=r.claudeBin` exactly as today — zero behavior change.
**Files**: `session/headless/runner.go`

##### Task 2.1.1a: Add `interpreter` field to ProcessRunner struct and preserve it in copy constructors (~4 min)
- In `session/headless/runner.go`, add `interpreter string` to the
  `ProcessRunner` struct (after `claudeBin`, line 50), with a doc comment:
  `// interpreter, when non-empty, is exec'd instead of claudeBin, with
  claudeBin prepended to argv. Opt-in, test-only — see
  NewShellWrappedProcessRunnerForTesting in fake_runner.go. Zero value for
  every production ProcessRunner.`
- Update `WithWorkDir` (lines 60-62) to include `interpreter: r.interpreter`
  in the returned copy.
- Update `WithToolAccess` (lines 66-68) to include `interpreter:
  r.interpreter` in the returned copy.
- Files: `session/headless/runner.go`

##### Task 2.1.1b: Wire the interpreter branch into ProcessRunner.Run (~3 min)
- In `session/headless/runner.go`'s `Run` method (lines 115-143), after
  `args = append(args, r.toolAccessArgs()...)` and before calling
  `executor.StartProcess`, add:
  ```go
  name := r.claudeBin
  if r.interpreter != "" {
      name = r.interpreter
      args = append([]string{r.claudeBin}, args...)
  }
  ```
- Change the `executor.StartProcess(ctx, r.claudeBin, args, opts...)` call
  to `executor.StartProcess(ctx, name, args, opts...)`.
- Files: `session/headless/runner.go`

##### Task 2.1.1c: Add NewShellWrappedProcessRunnerForTesting constructor (~3 min)
- In `session/headless/fake_runner.go`, after `NewProcessRunnerForTesting`
  (line 154), add:
  ```go
  // NewShellWrappedProcessRunnerForTesting constructs a ProcessRunner that execs
  // scriptPath through "sh" instead of forking/exec'ing scriptPath directly.
  // Use this instead of NewProcessRunnerForTesting when the test writes its own
  // fake-claude shell script to a freshly-created temp file: direct exec-by-path
  // of a just-written, just-chmod'd script can be refused by OS-level exec
  // restrictions (Gatekeeper, TCC, or third-party endpoint security software) on
  // some platforms, even though the exec bit and shebang line are both correct.
  // Invoking through the pre-existing, already-trusted "sh" binary sidesteps
  // that restriction because the OS is never asked to approve a freshly-written
  // file for direct execution.
  func NewShellWrappedProcessRunnerForTesting(scriptPath string) *ProcessRunner {
      return &ProcessRunner{claudeBin: scriptPath, interpreter: "sh"}
  }
  ```
- Files: `session/headless/fake_runner.go`

#### Story 2.1.2: Migrate call sites onto the shell-wrapped constructor
**As a** test author, **I want** every test that writes its own fake-claude
fixture to use `NewShellWrappedProcessRunnerForTesting`, **so that** the fix
in Story 2.1.1 actually covers every affected call site, not just some.
**Acceptance Criteria**:
- AC1 (continued): every call site that writes its own fake-claude script and
  execs it migrates onto the new constructor — no outlier bypasses it.
  - *Given* `session/headless/pool_test.go:696`'s
    `TestPool_CallBlocking_WithWorkDir_ReturnsCostAndUsesWorkDir`, which
    today constructs `&ProcessRunner{claudeBin: scriptPath}` directly
    (bypassing `NewProcessRunnerForTesting` entirely), *When* this task's
    change is applied, *Then* the test constructs its runner via
    `NewShellWrappedProcessRunnerForTesting(scriptPath)` instead, so this
    call site is no longer an outlier left uncovered by the fix.
**Files**: `session/headless/capability_check_test.go`,
`session/headless/pool_test.go`

**Scope note (architecture-review concern, resolved by documentation):**
`requirements.md`'s AC1 wording also names `caller_test.go` as sharing "the
same pattern," but this plan deliberately does not migrate it.
`caller_test.go`'s `TestFindClaudeBinary_*` tests write fixture scripts to
disk only to exercise `findClaudeBinary`'s executable-bit discovery
(`os.Stat`) — none of them ever fork/exec the written file (confirmed via
`grep -n "WriteFile\|ProcessRunner{\|exec\.\|StartProcess\|\.Run(" caller_test.go`:
only `os.WriteFile` hits, no exec calls), so they cannot hit the
Gatekeeper/TCC exec-refusal this plan targets and are correctly out of
scope.

##### Task 2.1.2a: Migrate capability_check_test.go's 4 call sites + fix stale doc comment (~4 min)
- In `session/headless/capability_check_test.go`, change all 4 occurrences of
  `NewProcessRunnerForTesting(scriptPath)` (lines 55, 86, 106, 142) to
  `NewShellWrappedProcessRunnerForTesting(scriptPath)`.
- Fix the stale doc-rot at line 19: `writeCapabilityCheckFakeClaudeScript`'s
  comment currently reads "Mirrors the pattern used in
  `session/review_gate_test.go`'s `writeOccupyAwareFakeClaudeScript`" — that
  helper no longer exists in that file (confirmed absent via grep). Replace
  with a comment describing the script's own behavior, e.g. "Writes a fake
  `claude` binary invoked via `NewShellWrappedProcessRunnerForTesting` (not
  exec'd directly by path — see that constructor's doc comment)."
- Files: `session/headless/capability_check_test.go`

##### Task 2.1.2b: Migrate pool_test.go's 2 call sites (~4 min)
- In `session/headless/pool_test.go`, change line 183's
  `NewProcessRunnerForTesting(scriptPath)` to
  `NewShellWrappedProcessRunnerForTesting(scriptPath)`.
- Change line 696's direct construction `&ProcessRunner{claudeBin:
  scriptPath}` to `NewShellWrappedProcessRunnerForTesting(scriptPath)`.
- Files: `session/headless/pool_test.go`

#### Story 2.1.3: Document that no caching-logic fix is needed (AC2, AC7)
**As a** reviewer, **I want** an explicit written finding that
`CodebaseReadCapabilitySelfCheck`'s caching logic is not buggy, **so that**
no unnecessary rewrite is proposed for a problem that doesn't exist.
**Acceptance Criteria**:
- AC2: If cluster 1's failure is confirmed to be a genuine caching bug
  distinct from the exec-permission issue, fix it; otherwise, document that
  it is not.
  - *Given* `git log --follow -- session/headless/capability_check.go
    session/headless/capability_check_test.go` showing exactly one commit
    (`db3b7225a`) in the caching logic's entire history, and 5 consecutive
    `-race` passes of
    `TestCodebaseReadCapabilitySelfCheck_RunsOnceAcrossConcurrentCallers` on
    this machine (`requirements.md`'s pre-planning table), *When* a reviewer
    asks "was this a caching bug," *Then* they find the answer documented
    here: no — the reported "subprocess invocation count stuck at 0" symptom
    is fully explained by the exec-refusal error returning *before*
    `CodebaseReadCapabilitySelfCheck.run` ever reaches its marker comparison,
    not by any defect in the `sync.Once`/`atomic.Bool` caching itself. No
    code change is made to `capability_check.go` by this plan.
- AC7 (cluster 1 branch): findings that a cluster was never a real code bug
  are documented rather than triggering unnecessary work.
  - *Given* the same evidence as above, *When* Phase 2 is scoped, *Then* the
    task list contains zero tasks touching `capability_check.go`'s caching
    logic — this criterion is satisfied by the *absence* of a padded task,
    not by an added one.
**Files**: none (documentation-only finding, recorded in this plan; no
production or test file changes beyond Stories 2.1.1/2.1.2).

##### Task 2.1.3a: No code task — finding recorded in this plan's Domain Glossary and Story 2.1.3 above
- This is intentionally not a code task. Per AC7 and this repo's YAGNI/ponytail
  house style, do not write a task whose only output would be "confirm again
  what Stories 2.1.1/2.1.2 and the pre-planning investigation already show."
- Files: none.

---

## Phase 3: Cluster 3 — Signal Classification Tightening

### Epic 3.1: Confirm SIGBUS/SIGSEGV specifically in the err != nil branch
**Goal**: Close the one real, narrow gap in
`TestMmapIndexHandle_TruncateWhileMapped_CrashesWithoutProtection`'s
crash-classification logic — today *any* non-nil subprocess error is treated
as proof of the expected fault — without touching the already-sound tolerant
branches, per AC3/AC4.

#### Story 3.1.1: Add a portable signal-check helper, gated by build tag
**As a** test maintainer, **I want** a helper that confirms a subprocess died
from `SIGBUS`/`SIGSEGV` specifically, **so that** the test's "expected crash"
branch is no longer just "any non-nil error."

**IMPLEMENTATION DEVIATION (root-cause finding made during Task 3.1.1a, supersedes
the design below):** the `syscall.WaitStatus.Signaled()`/`.Signal()` design this
story specifies does not work — verified empirically, not assumed. Built the test
binary and ran the actual crash helper directly
(`GOGITSTORE_TRUNC_HELPER=1 ./gogitstore.test -test.run=...`): with the default
`GOTRACEBACK`, the process exits via a controlled `os.Exit(2)` — `$?` is `2`, not
a signal-death exit — after printing `fatal error: fault` +
`[signal SIGBUS: bus error ...]`. Testing `GOTRACEBACK=crash` (the mode the Go
docs describe as re-raising a fatal signal) instead exits via `SIGABRT` (`$?`=134),
never the original `SIGBUS`/`SIGSEGV`. **In neither mode does
`syscall.WaitStatus.Signal()` ever equal `SIGBUS`/`SIGSEGV`** — Go's own runtime
signal handler intercepts the hardware fault before the OS can report it as the
process's death signal, so a `WaitStatus`-based check can never match a genuine
crash from this fault and would always fall through to the "not confirmed"
branch. The only place the signal is actually recorded is Go's own crash-dump
text, whose `[signal SIGBUS: ...]`/`[signal SIGSEGV: ...]` line format is stable
across `GOTRACEBACK` modes (confirmed identical in both captured runs above).
**Implemented instead:** `isExpectedFaultSignal(output []byte) (bool, string)`
parses the subprocess's captured stdout+stderr for that line — no `syscall`
import, no `*exec.ExitError`/`WaitStatus` type assertion, no build-tag split.
This also **resolves** the architecture-review's Windows-CI-justification concern
by elimination: since the helper does no syscall-package type assertion at all,
it compiles identically on `windows` (verified: `GOOS=windows go vet
./session/unfinished/gogitstore/...` clean) — `trunc_fault_signal_windows_test.go`
was deleted as dead scaffolding rather than kept.
**Acceptance Criteria**:
- AC3 (infrastructure half): a structural signal check is available and
  portable across `linux`/`darwin`/`windows`. **Correction (architecture-review
  concern):** this is precedent-following scaffolding (mirrors
  `zombie_reaper.go`/`zombie_reaper_windows.go`), not a CI-driven requirement —
  verified that no CI job in this repo ever compiles, vets, or runs this
  package's `_test.go` files on Windows (`build.yml`'s `windows` leg only
  `go build`s the root main package; `go test -race ./...` and
  `golangci-lint` both run exclusively on `ubuntu-latest`, confirmed via
  `grep -rn "runs-on" .github/workflows/*.yml`). The stub exists so a
  hypothetical native-Windows `go vet ./...` run wouldn't break this
  currently-zero-`syscall`-import package, not because CI would catch its
  absence.
  - *Given* an `*exec.ExitError` from a process killed by `SIGBUS` on Linux,
    *When* `isExpectedFaultSignal(err)` is called from
    `trunc_fault_signal_test.go` (built on `!windows`), *Then* it returns
    `(true, "bus error")` (or the platform's equivalent `.Signal().String()`
    text) via `exitErr.Sys().(syscall.WaitStatus).Signaled()` and
    `.Signal() == syscall.SIGBUS`.
  - *Given* the same call compiled on `windows` (via
    `trunc_fault_signal_windows_test.go`), *When* `isExpectedFaultSignal`
    is invoked, *Then* it compiles cleanly and returns `(false, "signal
    classification not implemented on windows")` rather than failing to
    build — this package currently has zero `syscall`/`unix` imports
    (confirmed via `grep -rln "syscall\.\|unix\." session/unfinished/gogitstore/*.go`),
    so this task must not be the one that silently breaks a native Windows
    `go vet` for this package, following the exact `//go:build !windows` /
    `_windows.go` split precedent already established in
    `session/tmux/zombie_reaper.go` / `zombie_reaper_windows.go`.
**Files**: `session/unfinished/gogitstore/trunc_fault_signal_test.go` (new),
`session/unfinished/gogitstore/trunc_fault_signal_windows_test.go` (new)

##### Task 3.1.1a: Create the unix signal-check helper (~4 min)
- Create `session/unfinished/gogitstore/trunc_fault_signal_test.go`:
  ```go
  //go:build !windows

  package gogitstore

  import (
      "errors"
      "os/exec"
      "syscall"
  )

  // isExpectedFaultSignal reports whether err (from exec.Cmd.CombinedOutput /
  // Wait) indicates the subprocess was killed by SIGBUS or SIGSEGV — the two
  // signals a truncated-mmap-read fault can raise. Returns a human-readable
  // detail string for logging in either case (matched or not).
  func isExpectedFaultSignal(err error) (matched bool, detail string) {
      var exitErr *exec.ExitError
      if !errors.As(err, &exitErr) {
          return false, "error is not an *exec.ExitError (process may not have started)"
      }
      ws, ok := exitErr.Sys().(syscall.WaitStatus)
      if !ok || !ws.Signaled() {
          return false, "process did not exit via a signal"
      }
      sig := ws.Signal()
      if sig == syscall.SIGBUS || sig == syscall.SIGSEGV {
          return true, sig.String()
      }
      return false, "process was killed by " + sig.String() + ", not SIGBUS/SIGSEGV"
  }
  ```
- Files: `session/unfinished/gogitstore/trunc_fault_signal_test.go`

##### Task 3.1.1c: Add direct unit tests for isExpectedFaultSignal (~3 min)
- Per `architecture-review.md`'s Concerns (Story 3.1.1/3.1.2): the helper had
  no direct unit test, only indirect coverage through the platform-dependent
  mmap test. `validation.md`'s Requirement → Test Mapping already specifies
  these two tests; this task is the corresponding implementation task so the
  plan's task list doesn't silently omit work `validation.md` assumes exists.
- In `session/unfinished/gogitstore/trunc_fault_signal_test.go`, add:
  - `TestIsExpectedFaultSignal_should_ReturnTrueAndBusError_When_ProcessKilledBySIGBUS`:
    run `exec.Command("sh", "-c", "kill -SIGBUS $$").Run()`, assert
    `isExpectedFaultSignal(err) == (true, "bus error")`.
  - `TestIsExpectedFaultSignal_should_ReturnFalse_When_ProcessExitsNonZeroWithoutSignal`:
    run `exec.Command("sh", "-c", "exit 1").Run()`, assert
    `isExpectedFaultSignal(err) == (false, "process did not exit via a signal")`.
- Files: `session/unfinished/gogitstore/trunc_fault_signal_test.go`

##### Task 3.1.1b: Create the windows stub (~2 min)
- Create `session/unfinished/gogitstore/trunc_fault_signal_windows_test.go`:
  ```go
  //go:build windows

  package gogitstore

  // isExpectedFaultSignal cannot classify POSIX signals on Windows — this
  // test's whole premise (SIGBUS/SIGSEGV on a truncated mmap read) is a
  // POSIX-signal concept that doesn't apply the same way on Windows. Always
  // report "not confirmed" so the caller falls back to its generic diagnostic
  // logging rather than failing to build.
  func isExpectedFaultSignal(err error) (matched bool, detail string) {
      return false, "signal classification not implemented on windows"
  }
  ```
- Files: `session/unfinished/gogitstore/trunc_fault_signal_windows_test.go`

##### As-built (supersedes Tasks 3.1.1a/b/c per the deviation note above)
- `trunc_fault_signal_test.go` has no build tag, no `syscall`/`errors`/`os/exec`
  import for the helper itself (only `bytes` + `testing`, plus `os/exec` in one
  test for a synthetic negative-case error). `isExpectedFaultSignal(output
  []byte) (bool, string)` matches Go's crash-dump `[signal SIGBUS:`/`[signal
  SIGSEGV:` text.
- Three direct unit tests (not two): SIGBUS-line match, SIGSEGV-line match, and
  no-fault-signature-present. The SIGBUS fixture is a byte-for-byte excerpt of
  this package's own genuine crash dump (captured via the same
  `GOGITSTORE_TRUNC_HELPER=1` re-exec `mmap_truncation_test.go` already uses),
  not a hand-guessed format.
- `trunc_fault_signal_windows_test.go` (Task 3.1.1b) was not created —
  deleted after being briefly written, since the portable text-parsing
  implementation compiles identically on `windows` with no stub needed
  (verified: `GOOS=windows go vet ./session/unfinished/gogitstore/...` clean).
- Verified 5/5 passing (`-count=5`) for both the new direct tests and the full
  `TestMmapIndexHandle_TruncateWhileMapped_*` cluster, with the wired-in
  signal-confirmed log line (`"subprocess crashed with a Go runtime-confirmed
  bus error"`) appearing in all 5 crash-branch runs — not just the unconfirmed
  fallback text.

#### Story 3.1.2: Wire the helper into the err != nil branch
**As a** future maintainer debugging a red `gogitstore` CI run, **I want**
the test's "expected crash" log to say whether the signal was actually
confirmed as SIGBUS/SIGSEGV, **so that** an unrelated subprocess failure
(bad `-test.run` regex, missing `git`, an early `t.Fatal`) is never
silently accepted as if the fault had been proven.
**Acceptance Criteria**:
- AC3 (behavior half): a "no fault occurred, no marker present" outcome (the
  one Agent 3's architecture research confirmed is unreachable given
  `runTruncHelper`'s current structure) remains a hard `t.Fatalf` — untouched.
  A non-nil `err` now gets a precision-tightened log message instead of an
  unconditional "expected" assumption.
  - *Given* `mmap_truncation_test.go`'s `err != nil` branch (currently lines
    127-130) after this task's change, *When* the subprocess is killed by
    `SIGBUS` (the common real-world case, confirmed 5/5 on this Linux
    machine per `requirements.md`), *Then* the test logs `"subprocess crashed
    with a Go runtime-confirmed bus error (expected — this IS the point of
    the test)"` and returns without failing — identical externally-observable
    pass/fail outcome to today, strictly better diagnostic text. (As-built:
    "Go runtime-confirmed" — see the As-built note under Story 3.1.1 for why
    this is text-parsed from Go's crash dump rather than a WaitStatus check.)
  - *Given* the same branch, *When* the subprocess instead fails for an
    unrelated reason (e.g. `-test.run` regex matched nothing, produces a
    non-signal exit), *Then* the test logs `"subprocess did not exit cleanly
    (expected but signal not confirmed as SIGBUS/SIGSEGV — process was
    killed by <sig>, not SIGBUS/SIGSEGV" / or "...is not an *exec.ExitError"`
    and **still returns without failing** — per `research/pitfalls.md`'s
    explicit guidance, this tightening must not introduce a new failure mode
    without further live confirmation that such a case is actually
    reachable; it is scoped as a diagnostic-precision improvement only.
- AC4: no change silently loosens the test's guarantee.
  - *Given* the tolerant branches at (current) lines 132-135 (`NO_FAULT_OCCURRED`
    / `ORDINARY_RECOVER_CAUGHT_IT`) and the `t.Fatalf` guard at line 136,
    *When* this task's diff is reviewed, *Then* neither of those lines is
    touched — the change is additive, confined to the `err != nil` branch's
    log message, and this Given/When/Then plus Story 3.1.2's own doc comment
    on `isExpectedFaultSignal` together state exactly which platform variance
    remains tolerated (signal-confirmed crash vs. any non-nil error) and why.
- AC7 (cluster 3 branch): the tolerant two-marker design itself needs no
  change — it was already correctly added by commit `dccee742a` for this
  exact flake.
  - *Given* `git log --all --oneline -- session/unfinished/gogitstore/mmap_truncation_test.go`
    showing `dccee742a`'s commit message ("broadened the truncate-while-mapped
    test's accepted outcomes to also treat a no-fault read as non-failing"),
    *When* this plan scopes Phase 3, *Then* it makes zero changes to lines
    132-136 (the tolerant branches and their guard-rail `t.Fatalf`) — only the
    `err != nil` branch above them is touched.
**Files**: `session/unfinished/gogitstore/mmap_truncation_test.go`

##### Task 3.1.2a: Replace the err != nil branch's log with a signal-confirmed message (~3 min)
- In `session/unfinished/gogitstore/mmap_truncation_test.go`, replace the
  current lines 127-130:
  ```go
  if err != nil {
      t.Logf("subprocess did not exit cleanly (expected — this IS the point of the test): err=%v\noutput:\n%s", err, out)
      return
  }
  ```
  with:
  ```go
  if err != nil {
      if matched, detail := isExpectedFaultSignal(err); matched {
          t.Logf("subprocess killed by %s (expected — this IS the point of the test)", detail)
          return
      } else {
          t.Logf("subprocess did not exit cleanly (expected but signal not confirmed as SIGBUS/SIGSEGV: %s): err=%v\noutput:\n%s", detail, err, out)
          return
      }
  }
  ```
- Do not touch lines 131-136 (the clean-exit marker checks and `t.Fatalf`
  guard) — verify after editing that those lines are byte-identical to
  before.
- Files: `session/unfinished/gogitstore/mmap_truncation_test.go`

---

## Phase 4: Verification

### Epic 4.1: Confirm no regression on this (Linux) environment
**Goal**: Satisfy AC5 — prove the change builds and all 3 clusters' tests
still pass repeatedly on Linux, with no regression from their current
passing state.

#### Story 4.1.1: Run the full build/test suite plus targeted repeated runs
**As a** reviewer, **I want** command output proving `make build && make
test` passes and all 3 clusters' specific tests pass 5x (cluster 1 also
under `-race`), **so that** "done" is backed by evidence, not a claim.
**Acceptance Criteria**:
- AC5: `make build && make test` passes on this (Linux) environment after the
  change, with the specific tests from all 3 clusters run at least 5x
  (`-count=5`, cluster 1 additionally with `-race`) showing no regression.
  - *Given* the changes from Phases 2 and 3 applied, *When*
    `make build && make test` is run, *Then* it exits 0.
  - *Given* the same changes, *When*
    `go test ./session/headless/... -run 'TestCodebaseReadCapabilitySelfCheck|TestPool_CallBlocking_WithWorkDir_ReturnsCostAndUsesWorkDir' -v -race -count=5`
    is run, *Then* all runs pass (matches or improves on the 5/5 and 3/3
    passing baselines recorded in `requirements.md`'s pre-planning table).
  - *Given* the same changes, *When*
    `go test ./session/unfinished/gogitstore/... -run TestMmapIndexHandle_TruncateWhileMapped_CrashesWithoutProtection -v -count=5`
    is run, *Then* all 5 runs pass, and at least one run's log output
    demonstrates the new signal-confirmed message (i.e. `isExpectedFaultSignal`
    matched a real `SIGBUS`), not just the unconfirmed fallback text.
**Files**: none (verification task; no new files).

##### Task 4.1.1a: Run make build && make test (~2 min)
- Run `make build && make test` from the repo root.
- Capture and review output; fix any compile errors from Phases 2/3 before
  proceeding (most likely: a missed call-site migration, or a build-tag typo
  in the two new `trunc_fault_signal*_test.go` files).
- Files: none.

##### Task 4.1.1b: Run targeted repeated tests for clusters 1+2 (~2 min)
- Run:
  ```bash
  go test ./session/headless/... -run 'TestCodebaseReadCapabilitySelfCheck|TestPool_CallBlocking_WithWorkDir_ReturnsCostAndUsesWorkDir|TestPool_CallBlocking_WorkDirPath_ContextTimeout_ReturnsError_NotEmptySuccess' -v -race -count=5
  ```
- Confirm all pass; confirm log output shows the fixture still being invoked
  correctly (e.g. `TestCodebaseReadCapabilitySelfCheck_RunsOnceAcrossConcurrentCallers`
  still reports exactly 1 invocation).
- Files: none.

##### Task 4.1.1c: Run targeted repeated tests for cluster 3 (~2 min)
- Run:
  ```bash
  go test ./session/unfinished/gogitstore/... -run TestMmapIndexHandle_TruncateWhileMapped -v -count=5
  ```
- Confirm all pass (both the without-protection and with-protection tests);
  inspect at least one `-v` log line to confirm the new signal-confirmed
  message appears (`"subprocess killed by bus error (expected...)"` or
  equivalent for `SIGSEGV`) rather than only the unconfirmed fallback text.
- Files: none.

---

## Phase 5: macOS Follow-Up Tracking

**As-built**: filed as backlog item `44b7d757-753c-4be3-b9a4-6fbd3831a039`
("Confirm fix-flaky-headless-tests clusters 1+2 fix on macOS"), created before
PR-open time per Task 5.1.1b.

### Epic 5.1: Explicit post-merge verification task (AC6)
**Goal**: Ensure the parts of this fix that cannot be verified on Linux are
tracked as an explicit task, not silently assumed fixed, per AC6 and
`research/pitfalls.md`'s core warning (zero macOS CI runners in this repo).

#### Story 5.1.1: File the macOS verification step
**As a** repo maintainer, **I want** a concrete, actionable step for
confirming this fix on macOS after merge, **so that** "clusters 1+2 fixed"
is never claimed without someone actually observing the failure go away on
the platform it occurs on.
**Acceptance Criteria**:
- AC6: the plan explicitly flags which parts of the fix cannot be verified in
  this (Linux) session and must be confirmed on macOS before merge is
  treated as "done, not just hardened."
  - *Given* this plan's PR (once opened), *When* the PR description is
    written (per `sdd:7-ship` / `github:pr-ship`), *Then* it includes a
    checklist item reading approximately: "Unverified on macOS — this repo's
    CI has no macOS runners (`.github/workflows/*.yml` confirmed
    `ubuntu-latest`-only). After merge, a maintainer with macOS access must
    run `go test ./session/headless/... -run
    'TestCodebaseReadCapabilitySelfCheck|TestPool_CallBlocking_WithWorkDir_ReturnsCostAndUsesWorkDir'
    -v -race -count=5` on a real Mac (ideally the machine where the original
    flake was observed) and confirm it now passes reliably. If it still
    fails, `xattr -l <fixture-path>` on the failing run's temp script is the
    next diagnostic step (confirms/rules out `com.apple.quarantine`
    specifically, per `research/stack.md` §3)." The PR is not described as
    "fixes the macOS flake," only as "hardens against the identified failure
    mode; macOS confirmation pending."
  - **(P1 fix, pre-mortem Failure #2)** *Given* this repo's own
    reconciliation automation has repeatedly closed items without a human
    following up on a documented "needs external verification" caveat left
    only as PR-body prose, *When* this PR is opened (before merge, not
    after), *Then* a second, explicitly-linked backlog item is created via
    `mcp__stapler-squad__create_backlog_item` titled "Confirm
    fix-flaky-headless-tests clusters 1+2 fix on macOS" containing the same
    verification command and `xattr` fallback step, and the PR description
    links to it by ID — so the macOS confirmation is a durable, queryable
    artifact this parent item's resolution does not silently depend on PR
    text no automation reads.
**Files**: none (PR description text, not a repo file — produced during
`sdd:7-ship`, not this planning phase).

##### Task 5.1.1a: Draft the macOS-verification checklist item for the PR body (~2 min)
- Not a code task. When the implementation is shipped (`sdd:7-ship` /
  `github:pr-ship`), include the checklist text from Story 5.1.1's
  acceptance criterion verbatim (or equivalent) in the PR description.
- Files: none — output lands in the PR description, not a repo file.

##### Task 5.1.1b: Create the linked macOS-verification backlog item before opening the PR (~2 min)
- Not a code task. Before (or at) PR-open time, call
  `mcp__stapler-squad__create_backlog_item` to file "Confirm
  fix-flaky-headless-tests clusters 1+2 fix on macOS" with the verification
  command and `xattr -l <fixture-path>` fallback from Task 5.1.1a, and link
  its ID in the PR description. Resolves pre-mortem.md Failure #2 (P1): the
  macOS caveat must be a durable, separately-trackable artifact, not only
  prose in a PR that automation doesn't read.
- Files: none — output is a backlog item, not a repo file.
