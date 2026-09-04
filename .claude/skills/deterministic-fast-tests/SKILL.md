---
name: deterministic-fast-tests
description: Use when writing or reviewing a new Go test in stapler-squad — prefer fast, deterministic tests over real sleeps/timeouts/subprocess waits, and prefer parallel-safe fixtures over t.Setenv/global-state mutation, so the suite doesn't keep growing slower or flakier.
---

# Prefer Deterministic, Fast Tests Over Real Waits

When writing a new test (or touching one), default to a fast, deterministic design. Reach for a real sleep, timeout, or subprocess wait only when the thing under test genuinely is timing/OS behavior — not as the easy way to assert "eventually X happens."

## Why this posture

An audit of `server/services`'s test suite (2026-09-04, investigating a CI budget failure on PR #693) found the dominant cost wasn't flaky logic — it was structural:

- **Real timeouts baked into test bodies.** `TestListWorktrees_TimesOutOnHungGitCommand` deliberately waits ~20s for a `pathCompletionTimeout` to fire (BUG-077 raised it from 5s to tolerate CI host contention — a real, considered tradeoff, not an oversight).
- **Real I/O at scale.** `TestListFiles_NodeCap` writes `maxDirEntries+1` real files to disk — already `t.Parallel()`, but the wall-clock cost is inherent to the fixture, not serialization.
- **`t.Setenv`-based fixtures** (`withFakeHome`, PATH-prepending fake-git tests) that are structurally incompatible with `t.Parallel()` — Go's testing package forbids combining them, since `t.Setenv` mutates the whole process's environment, which every concurrently-running parallel test's goroutine shares.

None of these are fixable after the fact without either weakening the thing being tested (shortening a timeout the project already tuned for CI stability) or a real refactor (injecting fakeable dependencies instead of mutating global env). The layer where this is cheap to fix is the moment a new test is written — not months later when the suite has quietly become the thing blowing the CI job's budget.

## Do

- **Inject time, don't sleep on it.** If a component takes a `time.Duration` or reads `time.Now()`, thread it through so a test can substitute a near-zero duration or a fake clock — rather than the test waiting out the real value.
- **Inject the dependency, don't `t.Setenv` it.** If a fixture needs a fake HOME, PATH, or similar, prefer passing it as a constructor/function parameter over mutating the process environment — this also makes the test `t.Parallel()`-eligible, which the project already leans on heavily (112 of ~158 test files in `server/services` use it as of this audit).
- **Bound subprocess/IO fixtures deliberately.** If a test must exercise a real timeout or a real filesystem write count, keep the bound as small as the behavior under test allows, and say why in a comment (see `TestListWorktrees_TimesOutOnHungGitCommand`'s BUG-077 comment for the model to follow).
- **Add `t.Parallel()` to new tests by default** unless they mutate shared state (env vars, global caches, a fixed file path) — check for that before assuming it's safe to add to an *existing* test, since retrofitting it onto a `t.Setenv`-based fixture is a real (not cosmetic) change.

## Don't

- Don't reach for `time.Sleep(N)` / a real timeout to make a flaky-looking assertion pass — fix the underlying race or inject a fake clock instead.
- Don't add a new `t.Setenv`-based fixture when a plain function parameter would do the same job and stay parallel-safe.
- Don't treat "it passed on retry" as resolution for a new test — that's exactly the pattern `fix-flaky-tests-dont-defer` exists to stop; this skill is its "shift left" counterpart, for before the flake exists.

## Retrofitting existing slow/env-mutating tests

Converting an existing `t.Setenv`-based fixture (e.g. `withFakeHome`) to dependency injection so it can run in parallel is a legitimate, valuable change — but it's a multi-file refactor with real regression risk, not a drive-by fix. Scope it as its own PR/task, not a rider on unrelated work; see `docs/bugs/open/` for tracking it if you find a concrete case worth doing.
