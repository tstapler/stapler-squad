# BUG-099: `server/services` package's ~500s test runtime intermittently blows CI job budgets [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-09-04, shipping PR #693 (`fix/streamhub-self-heal-started-flag`)
**Impact**: CI-only. Two different CI jobs failed with `CI-BUDGET-EXCEEDED` on the same PR within the
same hour, both after `go test ./server/services/...` (or a superset including it) ran ~500s — every
individual test the logs show actually **passed**; the job was killed for exceeding its own wall-clock
budget, not a test failure.

## Problem Description

Two separate CI jobs on PR #693 failed back to back:

- `MCP Integration Tests`: `FAIL github.com/tstapler/stapler-squad/server/services 495.728s`, then
  `CI-BUDGET-EXCEEDED: 'mcp-integration' exceeded its 25m budget`.
- `Test (server/session/config packages)`: `FAIL github.com/tstapler/stapler-squad/server/services 500.996s`
  (and again at `496.367s` in a second invocation within the same job), then
  `CI-BUDGET-EXCEEDED: 'test-rest' exceeded its 22m budget`.

Both reruns (via `gh run rerun --failed`, no code change) eventually passed. Recent `main`-branch history
for the `MCP Integration Tests` workflow shows 2 of the last 8 runs also failed — this is a pre-existing,
intermittent budget-tightness issue, not something introduced by PR #693's diff (which added ~9-14s of
new tests to this package, confirmed locally, and touched no other package in this job's matrix).

## Root Cause Hypothesis

`server/services` is documented in `CLAUDE.md` as needing `-timeout=20m` for `go test`, i.e. its own
maintainers already know it's slow. A local timing run (`go test ./server/services/... -v`, 2026-09-04)
found ~30 tests each taking 7-22s, the top offenders being:

- Real, intentional timeouts (`TestListWorktrees_TimesOutOnHungGitCommand`, ~20-22s, BUG-077 raised
  `listWorktreesTimeout` from 5s specifically to tolerate CI host contention).
- Real filesystem I/O at scale (`TestListFiles_NodeCap`, ~22s, writes `maxDirEntries+1` real files).
- `t.Setenv`-based fixtures (`withFakeHome` and friends, used by most `TestCreateSession_*` tests,
  10-20s each) — these are structurally incompatible with `t.Parallel()` (Go's testing package forbids
  combining them), so they can't be parallelized without first replacing the fixture with dependency
  injection.

Summed serially, the top ~30 slow tests alone account for 300+ seconds; the package's total is ~500s.
Two CI jobs apparently run this same package as part of a broader package list, and whichever job also
has other slow work scheduled in the same 20-25 minute window can tip over budget on ordinary
scheduling variance (shared runner contention), without any code regression.

## Suggested Fix

Not a quick fix — see the `deterministic-fast-tests` skill (`.claude/skills/deterministic-fast-tests/SKILL.md`,
added alongside this bug report) for the posture going forward on *new* tests. Retrofitting the existing
`t.Setenv`-based fixtures (`withFakeHome` etc.) to dependency injection so their tests become
`t.Parallel()`-eligible would meaningfully cut this package's wall-clock time, but it's a multi-file
refactor touching most of `session_service_create_test.go` and friends — real regression risk, not a
drive-by fix. Scope as its own task.

## Workaround

When either `MCP Integration Tests` or `Test (server/session/config packages)` fails with
`CI-BUDGET-EXCEEDED` and every individual test in the log shows `PASS`, treat it as this known
budget-tightness issue: `gh run rerun --failed` rather than searching for a regression, but still check
the log's `PASS`/`FAIL` lines to confirm no test genuinely failed before assuming this.
