# Implementation Plan: program-env-not-applied

**Feature**: Close the regression-test gap that let a custom program's registered env vars
silently fail to reach a spawned session's process (already fixed on HEAD by commit
`cdfd4e5cf2`), and verify the untested adjacent code paths (resume/restore, tmux `-e` argv
construction, remote/SSH, client-side program-ID plumbing) that the original bug report's
narrow repro never exercised.
**Date**: 2026-09-24
**Status**: Ready for implementation
**ADRs**: None — no new technology, dependency, or architectural pattern is introduced; every
task extends existing test infrastructure in place.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `ProgramConfig` | Persisted custom-program record (`config/types.go:487-494`): `ID`, `Label`, `Command`, `CLIFlags`, `Env map[string]string`. | Written by `UpsertProgramConfig`, read by `ResolveProgramConfig`. |
| `UpsertProgramConfig` | RPC handler (`server/services/defaults_service.go:709`) that validates and persists a `ProgramConfig` into `cfg.SessionDefaults.Programs` via `config.SaveConfig`. | The "write" side of the pipeline. |
| `ResolveProgramConfig` | Pure function (`config/defaults.go:211`) that case-insensitively looks up a program ID in `cfg.SessionDefaults.Programs` and returns its resolved `EnvVars`/`IsCustom`. | Verified correct in isolation by all six research docs. |
| `Instance` | In-memory session object (`session/instance.go`); carries `Program`, `EnvVars`, and the actor-owned mutable state that `Snapshot()` republishes. | |
| `InstanceSnapshot` / `Snapshot()` | Lock-free, atomically-published read view of an `Instance`'s fields (`session/instance_snapshot.go`), per `.claude/rules/instance-lock-free-reads.md`. | `resolveExtraEnvVars()` reads `snap.Program`/`snap.EnvVars` from this, not raw fields. |
| `resolveExtraEnvVars` | `Instance` method (`session/instance_tmux.go:585-599`) merging program-level `EnvVars` (from `ResolveProgramConfig`) with instance-level `EnvVars`, instance wins on collision. | Shared by `buildExtraEnv` and `claudeSettingsEnvOverrideArgs`. |
| `buildExtraEnv` | `Instance` method (`session/instance_tmux.go:604-614`) rendering `resolveExtraEnvVars()`'s map plus `STAPLER_SESSION_UUID` into `"KEY=VALUE"` strings for tmux `-e` flags. | |
| `claudeSettingsEnvOverrideArgs` | `Instance` method (`session/instance_tmux.go:631-642`) rendering the same resolved env as a `--settings` CLI flag, the GH #852 fix so Claude Code's own `settings.json` doesn't shadow it. | AC4 target — must keep passing unmodified. |
| `wireTmuxSession` | `Instance` method (`session/instance_tmux.go:646-676`) that constructs a `*tmux.TmuxSession`, calls `SetExtraEnv(buildExtraEnv())` on it, and attaches it to the process manager. | 7 call sites total (see Pattern Decisions). |
| `initTmuxSession` | `Instance` method (`session/instance_tmux.go:681-698`) that either **reuses** an existing live `TmuxSession` (skipping `wireTmuxSession` entirely) or calls `wireTmuxSession` to build a fresh one. | The reuse branch is the "leading suspect" pitfalls.md flagged; Story 1.2.2 verifies it's intentional, not a bug. |
| `TmuxSession` | `session/tmux/tmux.go` struct wrapping one tmux session's lifecycle. Has two independent env-carrying fields: exported `ExtraEnv []string` (DISPLAY/CDP injection, `session/instance.go` restart/VNC/CDP branches) and unexported `extraEnv` (program/instance env, set only via `SetExtraEnv`). | **Correction to stack.md**: `ExtraEnv` is *not* dead code — see Unresolved Questions. |
| `SetExtraEnv` | `TmuxSession` method (`session/tmux/tmux.go:1174-1176`) that sets the private `extraEnv` field. | |
| `newSessionArgs` | `TmuxSession` method (`session/tmux/tmux_session_start.go:611-624`) building the `tmux new-session` argv for `recreateMissingSession`, appending `-e` flags for both `ExtraEnv` and `extraEnv`. | Structurally duplicates the inline argv-building loop in `start()` (`tmux_session_start.go:214-220`). |
| `wrapRemoteCommand` | Pure function (`session/tmux/remote_env.go:41-46`) prefixing a remote-bound tmux invocation with `env -u TMUX TERM=xterm-256color`; passes through the rest of argv unchanged. | Verified via the existing table-driven `TestWrapRemoteCommand`. |
| `testTmuxServerSocket` | Per-test isolated tmux socket name (`server/services/session_service.go:314-321`), applied to every `Instance` a test `SessionService` creates. | The mechanism that lets an integration test shell out to the real tmux binary safely. |
| `NewIsolatedStateDir` | Test helper (`envtest/envtest.go:71-76`) that sets `STAPLER_SQUAD_TEST_DIR` via `t.Setenv` for the calling test, giving every `config.LoadConfig()`/`SaveConfig()` call in that test process the same isolated config directory. | Calling it once, before constructing both the `DefaultsService` and the `SessionService`, is what makes the write-then-read config path share one directory (closes the divergence stack.md flagged as unverified). |
| Program ID vs. resolved command | The `program` field on `CreateSessionRequest` must carry the program's `ID` (e.g. `"netflix-model-gateway"`), never its resolved `Command` (e.g. `"claude"`) — `ResolveProgramConfig` keys on ID. | Verified client-side in Epic 1.3: `web-app/src/lib/hooks/useAvailablePrograms.ts:24` maps dropdown `value: p.id`. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Regression test for "config value reaches the live process" | Integration/black-box characterization test: real `UpsertProgramConfig` RPC → real `CreateSession` RPC → real tmux socket → `tmux show-environment` | xUnit Patterns' "Back Door Verification"/State Verification via the system's own outputs | Unit test on `buildExtraEnv()`'s return value in isolation (the pattern the existing, insufficient tests already use) | The existing unit tests construct `&Instance{...}` as a struct literal and call the resolver directly — this is exactly what let the original bug ship unnoticed (features.md, pitfalls.md §5); only exercising the real construction + tmux path proves the wiring, not just the pure function. |
| Coverage across the 7 `wireTmuxSession` call sites | Table-driven / Parameterized Test (one test function, one case per launch path: fresh-create, resume-after-pause-with-dead-session, reuse-skip) | Meszaros, *xUnit Test Patterns* | One bespoke test function per call site | A single parameterized structure keeps the "what varies" (launch path) explicit and prevents 5+ near-duplicate test bodies from drifting out of sync — mirrors this repo's own `dupl` gate philosophy (`make ready-complexity-gate`). |
| `-e` argv construction (`start()` vs. `newSessionArgs()`) | Fake/spy collaborator (`MockCmdExec.RunFunc` capturing `cmd.Args`) | GoF Command pattern's test-double corollary; already this repo's convention (`spyCommandRunner`, `session/tmux/command_runner_test.go:205`) | Real tmux binary + `show-environment` for this layer too | The already-planned Epic 1.1 integration test covers the real-tmux case end-to-end once; testing the tmux-layer argv construction itself doesn't need a real binary — a captured `exec.Cmd.Args` is the closest unit-testable proxy (pitfalls.md §5) and runs without a tmux dependency in CI. |
| Client-side program selection (Omnibar) | Lock the existing invariant with a disambiguating fixture in the existing Jest test, not new code | N/A (verification-only, per task brief: "a fix task ONLY if that check finds a real problem") | Add a new `dispatch.test.ts` assertion tracing the RPC payload end-to-end | The check (Epic 1.3) confirmed the client already sends `p.id`, not `p.command` — the existing fixture (`id: "aider", command: "aider"`) just can't distinguish the two because they're equal by coincidence. Strengthening that one fixture is cheaper and closer to the actual risk than adding a new test surface. |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `initTmuxSession`'s reuse-vs-rebuild branch (`session/instance_tmux.go:681-685`) | pitfalls.md §2e flagged this as the "leading suspect" for stale env on reuse — untested | **Extend as-is**, backed by existing coverage (`TestInitTmuxSession_ReuseRequiresAliveNotJustConstructed`, PR #797) plus a new characterization test (Story 1.2.2, retargeted per architecture-review.md to the untested `IsBackendProcessAlive()`-only disjunct) | The branch's own doc comment traces to a deliberate fix for #791 (don't rebuild a still-alive session, which would race the live pane process). Between the existing `IsAlive()==true` test case and Story 1.2.2's new `IsBackendProcessAlive()`-only case, the full `HasSession() && (IsAlive() || IsBackendProcessAlive())` guard is now reached only when a session is genuinely still alive (where rebuilding env is moot — the running process already has whatever env it started with; only a real restart re-reads config), confirming it's intentional, not a gap. No code change; the tests are the enforcement. |
| Duplicate `-e` argv construction: `start()`'s inline loop (`tmux_session_start.go:214-220`) vs. `newSessionArgs()` (`tmux_session_start.go:611-624`) | features.md item 6 flags the duplication as a maintainability risk (a future edit to one loop could silently diverge from the other) | **Isolate via seam — deferred, not a task in this plan** | Consolidating the two loops into one shared helper is a real, low-risk cleanup, but it's orthogonal to closing this item's test gap and touches a `//nolint:gocognit,gocyclo` hotspot (`start()`'s own comment: "pre-existing complexity relocated verbatim... reducing it is a separate follow-up"). Story 1.2.3's new table-driven test gives that future refactor a regression net; do the consolidation then, as its own small PR. Recorded here so it isn't lost, per the task brief's "sized as small follow-up... not blown up into its own epic" instruction. |
| `TmuxSession.ExtraEnv` (exported) vs. `extraEnv` (unexported) two-field design | stack.md characterized `ExtraEnv` as dead code; **this plan's verification found that claim incorrect** — `session/instance.go` has 8 live call sites (`sess.ExtraEnv = append(...)` at lines 1538, 1545, 1643, 1650, 1810, 1817, 1906, 1913) for DISPLAY/CDP env injection | **Extend as-is** | Both fields are genuinely live, independently owned (DISPLAY/CDP vs. program/instance env), and already correctly non-clobbering per features.md's own trace (§4 in that doc) — confirmed again here. No cleanup needed; noted only to correct the research record (see Unresolved Questions). |

---

## Migration Plan

Omitted — no schema or data changes.

## Observability Plan

- **Logs**: No new logging. `initTmuxSession`'s existing `log.Info("reusing existing tmux session", ...)` (`instance_tmux.go:683`) already distinguishes the reuse branch Story 1.2.2 tests; no change needed.
- **Metrics**: None added — this is a test-only change plus one Jest fixture edit.
- **Alerts**: None applicable.

## Risk Control

- **Feature flag**: None — no behavior change ships; all Phase 1 tasks add test coverage for already-correct-on-HEAD behavior.
- **Rollback procedure**: Revert the test commits; no production code path changes.
- **Staged rollout**: Not applicable — CI-gated test additions only.

## Unresolved Questions

- **stack.md's "`ExtraEnv` is dead code" claim is incorrect** (see Tech Debt Disposition row above) — its own `grep` apparently missed `session/instance.go`'s 8 DISPLAY/CDP call sites, which `features.md`'s independent trace *did* find. No action needed for this plan; flagged so a future reader doesn't act on the stale claim in isolation.
- **Multi-process config-dir divergence** (stack.md/pitfalls.md §2c): a divergence between the process that ran `UpsertProgramConfig` and the process that later resolves `resolveExtraEnvVars()` is only a real bug shape if two different `stapler-squad` processes (e.g. two workspace-scoped instances) are involved — out of scope for this item's single-process repro, and no evidence in any research doc that this occurs in practice. Not resolved by this plan; would need its own item if a future report reproduces it.

## Dependency Visualization

```
Epic 1.1 (fresh-create integration test, commits AC1/AC2/AC3/AC5)
   |
   +--> Epic 1.2.1 (resume-path test)        -- reuses Epic 1.1's fixtures/helpers
   +--> Epic 1.2.2 (reuse-skip characterization test) -- independent, session/instance_tmux_test.go
   +--> Epic 1.2.3 (tmux -e argv table test) -- independent, new file session/tmux/tmux_session_start_test.go
   +--> Epic 1.2.4 (remote -e survival case) -- independent, session/tmux/remote_env_test.go

Epic 1.3 (client program-ID fixture)         -- fully independent, web-app only
Epic 1.4 (AC4 verification, no code change)  -- fully independent, run-only task
```

No task in Epic 1.2/1.3/1.4 depends on another; only Epic 1.2.1 shares test scaffolding
(`newCreateTestService`/`createTestStorage`) with Epic 1.1 and should land after it to reuse
that fixture rather than duplicate it.

---

## Phase 1: Close the Regression-Test Gap

### Epic 1.1: Commit the confirmed repro as a permanent regression test (fresh-create path)

**Goal**: Turn architecture.md's throwaway, deleted integration test into a permanent, committed
test in `server/services/session_service_create_test.go`, closing AC1/AC2/AC3/AC5 at once.

**Contingency**: If Task 1.1.1d's red/green check shows the new test passes even against the
pre-fix stub (i.e. the "already fixed" premise doesn't hold for this environment), halt before
starting Epic 1.2-1.4 and escalate — this item reverts to a bug-fix plan requiring fresh
root-cause analysis, not a test-only hardening pass.

#### Story 1.1.1: Fresh `SESSION_TYPE_NEW_WORKTREE` session picks up a registered custom program's env vars
**As a** maintainer, **I want** a committed integration test that registers a custom program via
the real `UpsertProgramConfig` RPC, creates a real session via `CreateSession`, and inspects the
live tmux session's environment table — proven by red/green to actually catch the original bug,
not just to pass on HEAD — **so that** a future regression to `resolveExtraEnvVars`/
`wireTmuxSession`/`initTmuxSession` is caught by CI instead of requiring a manual `printenv` repro.

**Acceptance Criteria**:
- Registering `netflix-model-gateway` (`Env: {"ANTHROPIC_BASE_URL": "http://127.0.0.1:47000"}`)
  and creating a `SESSION_TYPE_NEW_WORKTREE` session with `Program: "netflix-model-gateway"`
  results in the var being present in the spawned session's process environment, verified two
  ways: `tmux show-environment` (session-scoped) and `printenv` run inside the pane itself
  (process-scoped — the two diverged in the original bug report, which is why both are checked).
  - *Given* a `DefaultsService` and `SessionService` sharing one `STAPLER_SQUAD_TEST_DIR` (via
    one `envtest.NewIsolatedStateDir(t)` call), and a `ProgramConfig{ID: "netflix-model-gateway",
    Env: {"ANTHROPIC_BASE_URL": "http://127.0.0.1:47000"}}` registered through
    `svc.UpsertProgramConfig`, *When* `CreateSession` is called with
    `SessionType: SESSION_TYPE_NEW_WORKTREE, Program: "netflix-model-gateway"` against a real
    `git init`-ed repo and the resulting `*session.Instance` is polled to `Status: Active`,
    *Then* `tmux -L <svc.testTmuxServerSocket> show-environment -t <session-tmux-name>` includes
    the line `ANTHROPIC_BASE_URL=http://127.0.0.1:47000`, **and** `tmux send-keys "printenv
    ANTHROPIC_BASE_URL" Enter` followed by `capture-pane -p` on that session shows the same value
    in the captured pane output. (AC1, AC2)
- The test is committed (not deleted after one run) and runs under `go test ./server/services
  -run TestCreateSession_CustomProgramEnv`. (AC3)
- **The test is proven, by an explicit red/green check (Task 1.1.1d), to actually catch the
  original bug** — it fails against a pre-fix stub reproducing the `cdfd4e5cf2^`
  `SetExtraEnv([]string{"STAPLER_SESSION_UUID=" + i.UUID})`-only behavior, and passes again on
  HEAD — not merely observed to pass on HEAD today. (AC3)
- The test's doc comment states the root cause finding — the actual defect (pre-fix
  `initTmuxSession` called `SetExtraEnv` directly with only `STAPLER_SESSION_UUID`; no code path
  merged `ResolveProgramConfig(...).EnvVars` into the tmux `-e` set at all, since
  `resolveExtraEnvVars`/`buildExtraEnv` didn't exist pre-fix), not just the commit that fixed it —
  and names the fix commit `cdfd4e5cf2` (merged 2026-09-21) for traceability, not reproducible on
  HEAD as of `dd1848f9b`. (AC5)

**Files**: `server/services/session_service_create_test.go`

##### Task 1.1.1a: Add `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` skeleton + config seeding (~5 min)
- In `server/services/session_service_create_test.go`, add a new test function. Not
  `t.Parallel()` (shells out to a real tmux binary via a dedicated socket; mirror
  `TestCreateSession_StatusManagerWiredBeforeDriver`'s non-parallel pattern).
- Call `envtest.NewIsolatedStateDir(t)` once, before constructing either service, so
  `UpsertProgramConfig`'s write and `CreateSession`'s later `config.LoadConfig()` read resolve
  to the same `STAPLER_SQUAD_TEST_DIR`.
- Construct `defaultsSvc := NewDefaultsService()` and call
  `defaultsSvc.UpsertProgramConfig(ctx, connect.NewRequest(&sessionv1.UpsertProgramConfigRequest{
  Program: &sessionv1.ProgramConfig{Id: "netflix-model-gateway", Label: "Netflix Gateway",
  Command: "claude", Env: map[string]string{"ANTHROPIC_BASE_URL": "http://127.0.0.1:47000"}}}))`;
  `require.NoError`.
- Files: `server/services/session_service_create_test.go`

##### Task 1.1.1b: Create the session via the real `CreateSession` RPC and poll to `Active` (~5 min)
- `storage := createTestStorage(t)`, `svc := newCreateTestService(t, storage)`.
- Build a real git repo: `repoDir := t.TempDir()`, then `initGitRepoWithCommit(t, repoDir)`
  (`server/services/git_fixture_test.go:78` — go-git init + one commit with a fixed
  author/committer, the repo's existing consolidated helper for this exact fixture shape; avoids
  reintroducing a duplicate subprocess-based git-init helper).
- Call `svc.CreateSession(ctx, connect.NewRequest(&sessionv1.CreateSessionRequest{Title:
  "program-env-repro", Path: repoDir, SessionType: sessionv1.SessionType_SESSION_TYPE_NEW_WORKTREE,
  Program: "netflix-model-gateway"}))`; `require.NoError`.
- `t.Cleanup(func() { destroyCreatedSession(t, svc, resp.Msg.Session.Id) })`.
- Poll `svc.FindLiveInstance(resp.Msg.Session.Id)` until `Status() == session.Active` or a 30s
  deadline (mirror the polling loop in `TestCreateSession_StatusManagerWiredBeforeDriver`); if
  tmux is unavailable the instance goes `Stopped` instead — `t.Skip("tmux not available")` in
  that case, matching that test's established skip convention.
- Files: `server/services/session_service_create_test.go`

##### Task 1.1.1c: Assert `tmux show-environment` + `printenv` in-pane, and document the root cause (~7 min)
- Shell out: `exec.CommandContext(ctx, "tmux", "-L", svc.testTmuxServerSocket,
  "show-environment", "-t", <tmux session name from inst.Snapshot()>).Output()`.
- `assert.Contains(t, string(out), "ANTHROPIC_BASE_URL=http://127.0.0.1:47000")`.
- Also verify AC1 directly (not just via the `show-environment` proxy that satisfies AC2): run
  `tmux -L <socket> send-keys -t <session> "printenv ANTHROPIC_BASE_URL" Enter`, then **poll**
  `tmux -L <socket> capture-pane -p -t <session>` against a bounded deadline (mirror
  `TestCreateSession_StatusManagerWiredBeforeDriver`'s existing `time.Now().Before(deadline)`
  polling-loop pattern, already used elsewhere in this same file — do NOT use a fixed sleep; a
  real shell/tmux pane's command-execution timing is not deterministic enough for one, and this
  repo has a dedicated `fix-flaky-tests-dont-defer` skill specifically because a fixed-sleep wait
  here would produce exactly the kind of intermittent CI failure that gets "fixed" by disabling
  the test instead of by making the wait deterministic — pre-mortem.md Failure #2) until
  `assert.Contains(t, string(captured), "http://127.0.0.1:47000")` passes or the deadline is hit
  — this actually runs `printenv` inside the spawned pane's process, matching requirements.md's
  AC1 literally rather than relying solely on tmux's session-scoped environment table.
- Add a doc comment above the test naming the actual defect, not just the fix commit: pre-fix
  `initTmuxSession` called `SetExtraEnv` directly with only `STAPLER_SESSION_UUID`, and no code
  path merged `ResolveProgramConfig(...).EnvVars` into the tmux `-e` set at all — the
  `resolveExtraEnvVars`/`buildExtraEnv` merge function simply didn't exist pre-fix. State that
  this reproduces requirements.md's exact repro method, that it passes on HEAD, and that the fix
  landed in commit `cdfd4e5cf2` (merged 2026-09-21, two days before `dd1848f9b`) — so a future
  reader who sees this test doesn't re-litigate "is this actually fixed" or "what, specifically,
  was broken."
- Files: `server/services/session_service_create_test.go`

##### Task 1.1.1d: Red/green-verify the test against the pre-fix behavior (~30-40 min)
*(Re-estimated from an initial ~8 min during triad review — the scratch-worktree/stash setup,
hand-edit, historical-diff comparison, dual test run, `git status --short` check, and pasting
captured output into the PR is a real multi-step ceremony, not a quick edit.)*
- Prove the test in Task 1.1.1c would actually have caught the original bug, per AC3 — this is
  the step that turns "passes today" into "proven to detect a regression."
- **Git hygiene (required, not optional):** do this in a scratch git worktree
  (`git worktree add /tmp/program-env-redgreen HEAD`) or under a tagged
  `git stash push -u -m "redgreen-1.1.1d-scratch"` in the main tree — never a bare, unstashed
  local edit to `session/instance_tmux.go`. This task reverts production code to a known-broken
  state as a deliberate step; if the test run panics or the task is interrupted mid-way, an
  unstashed/un-worktreed edit can leave the repo silently sitting on pre-fix (broken) behavior
  with no signal. A scratch worktree isolates the revert entirely from the main tree; a tagged
  stash makes the revert trivially recoverable (`git stash apply <sha for the tag>`) even if the
  session drops.
- Reproduce the pre-fix `initTmuxSession` behavior in that scratch location via a **scoped
  hand-edit only** — do NOT `git show cdfd4e5cf2^:session/instance_tmux.go >
  session/instance_tmux.go` (whole-file revert). **This whole-file option is invalid and must not
  be used**: commit `cdfd4e5cf2` is the commit that *extracted* `wireTmuxSession` as its own
  method — the pre-`cdfd4e5cf2` version of `instance_tmux.go` does not define it at all, while
  HEAD's `session/instance.go` (lines 2252, 2453) and `session/instance_serialization.go` (lines
  463, 468, 521, 535, 548) — 7 call sites total — call `i.wireTmuxSession(program)` directly.
  Reverting only `instance_tmux.go` deletes a method 7 other call sites depend on and breaks
  compilation of the whole `session` package, producing a build error instead of the intended
  targeted assertion failure (pre-mortem.md Failure #1). Instead, hand-edit `wireTmuxSession`'s
  body in place (keep its signature and all 7 call sites intact) to temporarily call
  `session.SetExtraEnv([]string{"STAPLER_SESSION_UUID=" + i.UUID})` only, matching the exact
  pre-fix call shape — diff this hand-edit against the `wireTmuxSession`/`buildExtraEnv` hunks of
  `git show cdfd4e5cf2^:session/instance_tmux.go` (not the whole file) to confirm semantic
  equivalence, including the instance-level `EnvVars` merge path, not just the program-level one
  (pre-mortem.md Failure #5).
- Run `go test ./server/services -run TestCreateSession_CustomProgramEnv -v` against that
  hand-edited state; confirm it reports `FAIL` **with the specific `ANTHROPIC_BASE_URL` assertion
  failing** (not a build/compile error — a build error means the hand-edit broke something other
  than the intended env-merge behavior and must be fixed before this step counts). **Paste the
  actual failing `go test -v` output — naming the specific failed assertion — into the PR
  description; a task is not done, and this step is not satisfied, by reasoning "the diff clearly
  removes the merge logic, so it would obviously fail" without actually running it and capturing
  that output** (pre-mortem.md Failure #3, this repo's Evidence-and-Claims "green first, then
  done" norm).
- Restore `session/instance_tmux.go` to HEAD (discard/remove the scratch worktree via
  `git worktree remove /tmp/program-env-redgreen`, or `git stash pop`/`git stash drop` the
  tagged stash) and re-run the same test command in the main tree; confirm it reports `PASS`
  again.
- **Closing check (required):** run `git status --short` in the main tree as this task's last
  step and confirm it shows no unexpected modification to `session/instance_tmux.go` (or any
  other production file) before considering the task done — this is the explicit signal that the
  revert-and-restore cycle left no residue.
- If the test does **not** fail against the pre-fix stub, stop — do not proceed to Epic 1.2-1.4.
  See Epic 1.1's Contingency note: this means the test's assertion doesn't actually exercise the
  bug (e.g. it's asserting on something always-true), and the test needs to be fixed before this
  plan can rely on it, or the item needs to re-enter root-cause analysis.
- Files: `server/services/session_service_create_test.go` (no net change), `session/instance_tmux.go`
  (touched only inside the scratch worktree/stash — never committed, restored to HEAD by this
  task's own last steps)

---

### Epic 1.2: Extend coverage to untested `wireTmuxSession` call sites and adjacent mechanisms

**Goal**: pitfalls.md and features.md independently flagged four adjacent, untested paths in
the same injection pipeline. Close each with a small, targeted test — no production code change
unless a test actually fails.

**Scope note**: this epic broadens the item's scope beyond requirements.md's literal ACs,
hardening adjacent untested paths the research phase flagged (pitfalls.md, features.md) — not
required to close AC1-5, but recommended so the regression-test gap this item exists to close
doesn't just move one function over (e.g. into the resume path or the argv-construction layer).

#### Story 1.2.1: Program env changes are picked up on resume after a paused session's tmux process died
**As a** user, **I want** an env var I add to a custom program after a session was first created
to take effect the next time that session's tmux process is actually rebuilt (not just on brand
new sessions), **so that** editing Program Configurations isn't only effective for sessions
created after the edit.

**Acceptance Criteria**:
- A session resumed after its tmux process died (the `!i.pm().IsAlive()` branch,
  `session/instance.go:2241-2258`, which calls `wireTmuxSession`) reflects a program env var
  added *after* the session was originally created.
  - *Given* an `Instance` created with `Program: "netflix-model-gateway"` before
    `ANTHROPIC_BASE_URL` was registered on that program, then paused (tmux session killed) and
    the program's `ProgramConfig.Env` updated via `UpsertProgramConfig` while paused, *When* the
    session is resumed (`Instance.Resume`/equivalent, hitting the dead-tmux rebuild branch),
    *Then* `tmux show-environment -t <session>` after resume includes the newly-added
    `ANTHROPIC_BASE_URL`.
**Files**: `server/services/session_service_create_test.go` — reuses Task 1.1.1's
RPC-level fixture (`svc.FindLiveInstance(id)` returns the live `*session.Instance`, which
exposes `Pause()`/`Resume()` directly), so the pause/resume cycle stays in the same
RPC-layer test file rather than a new `session`-package one.

##### Task 1.2.1a: Add the resume-picks-up-env-change regression test (~5 min)
- Reuse Task 1.1.1's fixture (session created, program registered) as a starting point.
- After the instance reaches `Active`, call `inst.Pause()` (`session/instance.go:2099`) — it
  kills the tmux session as part of pausing (`instance.go:2242`'s comment: "Tmux session is dead
  (killed on pause to free memory)"), which is what forces `Resume()`'s dead-tmux rebuild branch
  (`instance.go:2241-2258`, which calls `wireTmuxSession`) rather than the live-restore branch
  (`instance.go:2224-2240`, which does not).
- Update the registered program's `Env` via a second `defaultsSvc.UpsertProgramConfig` call,
  adding a new key not present at session-creation time.
- Call `inst.Resume()` (`session/instance.go:2184`); poll to `Active` again; assert via
  `tmux show-environment` that the new key is now present.
- Files: `server/services/session_service_create_test.go`

#### Story 1.2.2: `initTmuxSession`'s reuse guard is also correct when only the backend process is alive — characterize the untested disjunct
**Retargeted per architecture-review.md's CONCERN finding**: the `HasSession()==true &&
IsAlive()==true` reuse case this story originally proposed to test is **already covered** by
`TestInitTmuxSession_ReuseRequiresAliveNotJustConstructed` (`session/instance_tmux_test.go:135-170`,
landed in PR #797) — its second table case is literally `{"live session: reuses, no rebuild",
true, true, false}`. Re-adding that case would produce a near-duplicate test and risks tripping
this repo's own `dupl` CI gate. The genuinely uncovered case is `initTmuxSession`'s full guard,
`HasSession() && (IsAlive() || IsBackendProcessAlive())` (`session/instance_tmux.go:682`) — the
`IsBackendProcessAlive()` disjunct (`HasLiveSessionNoCache()`/`CachedPanePIDStillAlive()`,
`session/instance_tmux.go:888-901`) is never exercised as the *sole* reason a session is reused,
i.e. the case where `IsAlive()==false` but `IsBackendProcessAlive()==true`.

**As a** maintainer, **I want** a test proving `initTmuxSession`'s reuse branch also correctly
skips rebuild when `IsAlive()` alone is false but the backend process is still detected alive,
**so that** the full reuse guard — not just the already-tested `IsAlive()==true` half — is
proven intentional and covered.

**Acceptance Criteria**:
- Calling `initTmuxSession()` on an `Instance` whose `HasSession()==true`, `IsAlive()==false`,
  and `IsBackendProcessAlive()==true` reuses the session (does not call `wireTmuxSession`, and
  thus does not re-run `buildExtraEnv`).
  - *Given* an `Instance` whose `i.pm().HasSession()` reports `true`, `IsAlive()` reports
    `false`, and `IsBackendProcessAlive()` reports `true` (session reused solely via the backend-
    process-alive disjunct), *When* `initTmuxSession()` is called, *Then* the
    `log.Info("reusing existing tmux session", ...)` path is taken and no new
    `*tmux.TmuxSession` is constructed — asserted primarily via `tb.TmuxManager().SetSession` not
    being called a second time (the stronger of the two assertion options the original story
    left ambiguous; a pointer-equality check alone wouldn't catch a `wireTmuxSession` call that
    happens to reconstruct an equal-looking session).
**Files**: `session/instance_tmux_test.go`

##### Task 1.2.2a: Add `TestInitTmuxSession_ReuseViaBackendProcessAliveOnly` (~5 min)
- Use the same test double the existing sibling test
  (`TestInitTmuxSession_ReuseRequiresAliveNotJustConstructed`) uses to drive this exact guard:
  `mockTmuxManager{hasSessionReturn, isAliveReturn}` + `NewTmuxBackend(mock)` — **not**
  `TestInstance_BuildExtraEnv_...`'s bare `&Instance{}`-literal, no-process-manager pattern (that
  test exercises `resolveExtraEnvVars`/`buildExtraEnv` directly and has no process manager to
  drive `HasSession`/`IsAlive`/`IsBackendProcessAlive` at all).
- Extend or add a case alongside the existing table (or a new table if `IsBackendProcessAlive()`
  isn't already a field on `mockTmuxManager` — add it if missing) with `hasSession=true,
  isAlive=false`, and the backend-process-alive signal set to `true`.
- Call `i.initTmuxSession()` and assert `tb.TmuxManager().SetSession` is not called a second time
  (primary assertion — see AC above), mirroring the existing sibling test's sentinel-value
  pattern (e.g. `inst.LaunchCommand` staying at a sentinel) as a secondary check if useful.
- Files: `session/instance_tmux_test.go`

#### Story 1.2.3: `-e` flags survive both tmux argv-construction paths (`start()` and `recreateMissingSession`'s `newSessionArgs()`)
**As a** maintainer, **I want** a unit test capturing the actual `tmux new-session` argv built by
both code paths that construct it, **so that** a future edit to one loop that forgets to also
edit the other (features.md item 6's flagged duplication risk) is caught immediately, without
needing a real tmux binary.

**Acceptance Criteria**:
- Both `TmuxSession.start()`'s inline argv-building loop and `TmuxSession.newSessionArgs()`
  include `-e KEY=VALUE` for every entry set via `SetExtraEnv`.
  - *Given* a `*tmux.TmuxSession` constructed via `NewTmuxSessionWithDeps` with a `MockCmdExec`
    whose `RunFunc` captures `cmd.Args`, and `SetExtraEnv([]string{"ANTHROPIC_BASE_URL=http://127.0.0.1:47000"})`
    called on it, *When* `start(workDir, false, nil)` is called, *Then* the captured `cmd.Args`
    contains `"-e"` immediately followed by `"ANTHROPIC_BASE_URL=http://127.0.0.1:47000"`.
  - *Given* the same `*tmux.TmuxSession`, *When* `newSessionArgs(workDir, program)` is called
    directly, *Then* its returned `[]string` contains the same `"-e",
    "ANTHROPIC_BASE_URL=http://127.0.0.1:47000"` pair.
**Files**: `session/tmux/tmux_session_start_test.go` (new file)

##### Task 1.2.3a: Add table-driven `TestTmuxNewSessionArgs_IncludesExtraEnv` covering both paths (~5 min)
- New file `session/tmux/tmux_session_start_test.go`. Table cases: `{name: "start()", path:
  callStart}`, `{name: "newSessionArgs()", path: callNewSessionArgs}` (two small closures
  wrapping each entry point), both asserting the same `-e KEY=VALUE` pair appears in the
  resulting argv.
- Reuse `MockCmdExec` (`session/tmux/test_helpers.go:8`) for the `start()` case, capturing
  `cmd.Args` inside `RunFunc`.
- Files: `session/tmux/tmux_session_start_test.go`

#### Story 1.2.4: `-e` flags survive `wrapRemoteCommand`'s rewrite for SSH-backed sessions
**As a** maintainer, **I want** the existing `TestWrapRemoteCommand` table extended with an
explicit `-e KEY=VALUE`-shaped case, **so that** the one other place tmux invocations get
rewritten for remote sessions is explicitly proven not to touch env flags, not just inferred
from the existing generic cases.

**Acceptance Criteria**:
- `wrapRemoteCommand("tmux", [..., "-e", "ANTHROPIC_BASE_URL=http://127.0.0.1:47000", ...])`
  preserves that `-e` pair unchanged, in order, in the returned argv.
  - *Given* `cmdArgs := []string{"-L", "isolated", "new-session", "-d", "-s", "staplersquad_foo",
    "-e", "ANTHROPIC_BASE_URL=http://127.0.0.1:47000", "-c", "/work/dir", "claude"}`, *When*
    `wrapRemoteCommand("tmux", cmdArgs)` is called, *Then* the returned `wantArgs` is `["-u",
    "TMUX", "TERM=xterm-256color", "tmux", "-L", "isolated", "new-session", "-d", "-s",
    "staplersquad_foo", "-e", "ANTHROPIC_BASE_URL=http://127.0.0.1:47000", "-c", "/work/dir",
    "claude"]`.
**Files**: `session/tmux/remote_env_test.go`

##### Task 1.2.4a: Add one table case to `TestWrapRemoteCommand` (~2 min)
- Append a new `tests` entry to the existing table in `TestWrapRemoteCommand`
  (`session/tmux/remote_env_test.go:12-48`) named `"tmux new-session with custom program env"`,
  using the `cmdArgs`/`wantArgs` from the Given/Then above.
- Files: `session/tmux/remote_env_test.go`

---

### Epic 1.3: Lock in the client-side program-ID invariant (Omnibar → `CreateSession`)

**Goal**: architecture.md flagged, as a residual risk, that the web client might resolve a
custom program to its `command` before sending `CreateSessionRequest.program`. Verification
during planning (`web-app/src/lib/hooks/useAvailablePrograms.ts:23-29`: dropdown `value: p.id`;
`web-app/src/lib/omnibar/actions/dispatch.ts`'s `create_session` case: `program: action.program
?? ""` passed straight to `deps.createSession`) confirms this is **not** a real problem — the
client already sends the program ID. Per the task brief, this closes with a test that locks in
the invariant, not a code fix. The MCP tool session-creation entry point
(`server/mcp/tools_lifecycle.go`) is separately confirmed moot for this same risk: its `program`
parameter is `mcpgo.Enum("claude", "aider")`, a closed enum with no way to select a
custom-registered program ID at all, so there's no command-vs-ID confusion possible on that path
— noted here rather than left as an open question (triad review / adversarial-review.md).

#### Story 1.3.1: `useAvailablePrograms` maps dropdown value to program ID, not resolved command
**As a** maintainer, **I want** the existing Jest fixture strengthened so `id` and `command`
differ, **so that** a future change that accidentally maps `value: p.command` is caught (the
current fixture can't catch it, since `id === command === "aider"` in both cases).

**Acceptance Criteria**:
- `useAvailablePrograms()`'s returned `ProgramOption.value` equals the server's `ProgramConfig.id`,
  even when `id` and `command` differ.
  - *Given* `mockList` resolves `{programs: [{id: "netflix-model-gateway", label: "Netflix
    Gateway", command: "claude", cliFlags: ""}]}`, *When* `useAvailablePrograms()` is rendered
    and awaited, *Then* `result.current[0].value === "netflix-model-gateway"` (not `"claude"`).
**Files**: `web-app/src/lib/hooks/useAvailablePrograms.test.ts`

##### Task 1.3.1a: Change the existing fixture's `id`/`command` to differ and assert on `value` (~3 min)
- In `useAvailablePrograms.test.ts`'s first `it(...)` block, change the mocked program from
  `{id: "aider", ..., command: "aider"}` to `{id: "netflix-model-gateway", label: "Netflix
  Gateway", description: "", command: "claude", cliFlags: ""}`.
- Update the `waitFor`/`toMatchObject` assertions to expect `value: "netflix-model-gateway"`
  and `command: "claude"` as two distinct values (proving the mapping keys on `id`, not
  `command`).
- Files: `web-app/src/lib/hooks/useAvailablePrograms.test.ts`

---

### Epic 1.4: Confirm no regression to the GH #852 precedence fix (AC4)

**Goal**: The task brief requires proving `claudeSettingsEnvOverrideArgs()` still wins over a
global `~/.claude/settings.json` `env` block. No code in this plan touches that function or
`resolveExtraEnvVars()` — this is a verification-only task.

#### Story 1.4.1: Existing #852 regression tests still pass unmodified after this plan's changes
**As a** maintainer, **I want** confirmation that `TestClaudeSettingsEnvOverrideArgs_CarriesResolvedEnvVars`
and `TestClaudeSettingsEnvOverrideArgs_EmptyWhenNoEnvVars` still pass, **so that** AC4 is closed
with evidence, not assumption.

**Acceptance Criteria**:
- `go test ./session -run TestClaudeSettingsEnvOverrideArgs -v` passes with both subtests green,
  run after all Phase 1 tasks land.
  - *Given* the full set of Epic 1.1-1.3 test additions applied, *When* `go test ./session -run
    TestClaudeSettingsEnvOverrideArgs -v` is run, *Then* both
    `TestClaudeSettingsEnvOverrideArgs_CarriesResolvedEnvVars` and
    `TestClaudeSettingsEnvOverrideArgs_EmptyWhenNoEnvVars` report `PASS`.
**Files**: `session/instance_tmux_test.go` (read-only — no edits expected)

##### Task 1.4.1a: Run and record the #852 regression suite (~2 min)
- Run `go test ./session -run TestClaudeSettingsEnvOverrideArgs -v` from the repo root.
- Paste the `PASS`/`ok` output into the PR description (per this repo's Evidence-and-Claims
  norm: "green first, then done") — no file changes if it already passes, which is expected
  since this plan makes no change to `resolveExtraEnvVars`/`claudeSettingsEnvOverrideArgs`.
- Files: none (verification only)

---

## Suggestions (not tasks — deferred, per task-brief sizing guidance)

- **UX feedback loop** (ux.md): add a session-detail "Environment" section showing which
  registered env var *keys* were actually applied, closing the "write acknowledgment ≠ effect
  acknowledgment" gap that let this bug ship silently. Sizeable enough (new UI surface +
  backend telemetry of what was actually passed to tmux) to warrant its own backlog item rather
  than a task here.
- **Duplicate `-e` argv-building loop consolidation** (`start()` vs. `newSessionArgs()`): see
  Tech Debt Disposition — do this once Story 1.2.3's test lands as its regression net.
