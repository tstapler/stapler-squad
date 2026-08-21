# Requirements: fix-flaky-headless-tests

## Complexity

1 (quick task — bug-fix/test-hardening investigation across 3 pre-existing
test clusters, no new feature surface, no UX, nothing to build-vs-buy in the
product sense). Per `sdd:2-research` calibration this runs Agents 1
(stack/subprocess-exec patterns), 4 (pitfalls), and 6 is skipped (no
build-vs-buy question — this is fixing existing tests, not sourcing a new
capability). Agent 3 (architecture) is additionally run despite the
complexity-1 default because requirements above already surfaced an
architectural question (how `ProcessRunner`/`executor.StartProcess` invokes
fake-claude test scripts) that benefits from a focused look. Agents 2
(features) and 5 (UX) are skipped — no comparable-feature landscape or
user-facing surface applies to fixing 3 test clusters.

## Source

Backlog item `04e6841f-ff53-44bb-b737-04e8b52fe1c3` — "Flaky/broken tests in
session/headless and session/unfinished/gogitstore (pre-existing, unrelated to
keychain migration)". Filed per `.claude/rules/fix-flaky-tests-dont-defer.md`
after `go test ./session/...` surfaced three failure clusters while validating
an unrelated change, confirmed pre-existing via `git stash` + re-run on clean
`main` (`ba7604308`).

This doc was written non-interactively (no user present in this session) per
the item's own instructions — skips the `sdd:1-ideate` interview and derives
requirements directly from the item body plus fresh investigation below.

## Pre-planning investigation (this session, Linux/go1.26.4, HEAD `98e53c380`)

Before writing acceptance criteria, the three clusters were re-run on current
`main` to confirm they still reproduce and to sharpen the root-cause
hypothesis (per `.claude/CLAUDE.md`'s "no fix without root cause" and "run it,
don't read it" rules).

| Cluster | Command | Result on this machine |
|---|---|---|
| 1 — `capability_check_test.go` | `go test ./session/headless/... -run TestCodebaseReadCapabilitySelfCheck -v -race -count=1` (x5) | **PASS**, all 5 runs, with `-race` |
| 2 — `pool_test.go` WorkDir test | `go test ./session/headless/... -run TestPool_CallBlocking_WithWorkDir_ReturnsCostAndUsesWorkDir -v -count=3` | **PASS**, all 3 runs |
| 3 — `mmap_truncation_test.go` | `go test ./session/unfinished/gogitstore/... -run TestMmapIndexHandle_TruncateWhileMapped_CrashesWithoutProtection -v -count=1` (x5) | **PASS**, all 5 runs — subprocess genuinely SIGBUS/SIGSEGV faulted each time |

None of the three clusters reproduce on this Linux/go1.26.4 environment,
including under `-race` and repeated runs. `git log --follow` shows
`capability_check.go`/`capability_check_test.go` have exactly one commit in
their entire history (`db3b7225a`, the PR that introduced them) — the caching
logic (`sync.Once` + `atomic.Bool`) has never been touched since, so a "100%
reproducible" caching regression reported against it is inconsistent with
both the code's simplicity and its clean re-run here.

**Root-cause hypothesis, clusters 1+2 (same failure, one cause):**
`capability_check_test.go`, `pool_test.go`, and `caller_test.go` all share an
identical pattern — write a `#!/bin/sh` fake-claude script to a fresh
`t.TempDir()` path with `os.WriteFile(..., 0o755)`, then have
`ProcessRunner` (`session/headless/runner.go:129`,
`executor.StartProcess(ctx, r.claudeBin, args, ...)`) fork/exec it **directly
by path**, not via an explicit `sh <path>` wrapper. Cluster 2's own error —
`fork/exec .../fake-claude.sh: operation not permitted` — is the textbook
signature of macOS Gatekeeper/TCC (or an EDR/MDM agent) refusing to execute a
just-written, unsigned temp file. If that same OS-level exec refusal happens
inside `capability_check_test.go`'s identical script-write-then-exec pattern,
`pool.CallBlocking` would return an error *before* ever reaching the marker
comparison — producing exactly cluster 1's symptom (subprocess invocation
count stuck at 0, cached result `false`) without requiring any bug in
`CodebaseReadCapabilitySelfCheck`'s caching logic itself. This reframes
clusters 1 and 2 as **one shared root cause** (sandboxed/restricted exec of
freshly-written temp scripts, environment-specific — almost certainly macOS,
where the user's `~/.claude/CLAUDE.md` notes they also work), not two
independent code bugs.

**Root-cause hypothesis, cluster 3:** already self-documented in the test
file's own comments (`mmap_truncation_test.go:98-113`) as deliberately
platform/kernel-dependent undefined behavior (SIGBUS/SIGSEGV vs. a clean
non-faulting read), and the harness already has a tolerant branch for the
non-fault outcome (`NO_FAULT_OCCURRED`/`ORDINARY_RECOVER_CAUGHT_IT`, lines
132-135) that returns without failing the test. On this machine the fault
reliably occurs and the test passes. The item's own text acknowledges this is
the exact test named in `.claude/rules/fix-flaky-tests-dont-defer.md` as
having been "re-excused as known pre-existing flake... without ever being
fixed" — this item is the first attempt to actually close it rather than
re-defer it again.

**Implication for scope:** because none of the three clusters reproduce here,
this plan cannot verify a code fix against a live failure in this session.
The plan (written in `sdd:3-plan`) must produce changes that are: (a)
correct by inspection/root-cause reasoning, (b) verifiable by a maintainer on
the platform where the failure actually occurs (macOS), and (c) fail closed
— i.e. make the tests robust/self-diagnosing across platforms rather than
merely papering over a symptom only reproducible elsewhere.

## Acceptance Criteria

0. Root cause is stated and evidenced (not assumed) for each of the three
   clusters before any test/production code changes are made.
1. Cluster 1 (`capability_check_test.go`) and cluster 2 (`pool_test.go`,
   and the same pattern in `caller_test.go`) no longer depend on the OS
   permitting direct fork/exec of a freshly-written, freshly-chmod'd temp
   script — the fake-claude invocation path is made robust to
   sandbox/Gatekeeper-style exec restrictions (e.g. by invoking through an
   already-trusted interpreter, or by giving a clear skip/diagnostic instead
   of a confusing count-mismatch failure when exec is refused).
2. If cluster 1's failure is confirmed to be a genuine caching bug distinct
   from the exec-permission issue (not merely a downstream symptom), the
   `CodebaseReadCapabilitySelfCheck.Ensure`/`run` caching logic is fixed and
   `TestCodebaseReadCapabilitySelfCheck_RunsOnceAcrossConcurrentCallers`
   passes deterministically under `-race -count=10`.
3. Cluster 3 (`TestMmapIndexHandle_TruncateWhileMapped_CrashesWithoutProtection`)
   is updated so a "no fault occurred, subprocess exited cleanly" outcome
   with no recognized marker is either (a) accepted as a documented,
   non-failing platform variance the same way the two known markers already
   are, or (b) if a genuine gap is found in the test's exit-classification
   logic, fixed so all real subprocess outcomes are correctly classified.
4. No change to this fix silently loosens the test's actual guarantee — if a
   test's assertion is relaxed for portability, the doc comment says exactly
   what platform variance it now tolerates and why, per this repo's
   `fix-flaky-tests-dont-defer.md` (root-cause and fix, not defer) and
   general "no fix without root cause" discipline.
5. `make build && make test` passes on this (Linux) environment after the
   change, with the specific tests from all 3 clusters run at least 5x
   (`-count=5`, cluster 1 additionally with `-race`) showing no regression
   from their current passing state here.
6. The plan explicitly flags which parts of the fix cannot be verified in
   this (Linux) session and must be confirmed on macOS before merge —
   surfaced as a task, not silently assumed fixed.
7. Findings that show a cluster was never a real code bug (e.g. cluster 3's
   existing tolerant handling already covers the reported symptom) are
   documented as such rather than triggering an unnecessary code change —
   consistent with `ponytail`/YAGNI: don't build a fix for a bug that isn't
   there.

## Out of scope

- Fixing unrelated flakes named in `fix-flaky-tests-dont-defer.md`
  (`session/tmux`'s `TestEnsureServerRunning_NoOp`,
  `TestKillOrphanedControlModeClients`, `server/services`'
  `TestRemoveHooksConfig_...`) — not part of this item.
- The keychain migration change that originally surfaced these clusters.
- Any change to `executor.StartProcess`/`safeexec` beyond what's needed for
  the fake-claude test-script invocation path specifically.
