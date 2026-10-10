# Architecture Review: program-env-not-applied
**Date**: 2026-09-24
**Verdict**: CONCERNS

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repo — no constitution
constraints apply. `kibitzer` is on `PATH` and `.claude/inspect.json` exists, but this plan adds
no production code and no new components/layers, so kibitzer's dependency/naming/content-rule
checkers (which gate Clean/Hexagonal layering) have nothing to check here; review is grounded
directly in the existing Go source and test files instead (all specific file:line references
below were opened and verified against HEAD, `dd1848f9b`).

## Blockers

None.

## Concerns

- **Story 1.2.2 / Task 1.2.2a** — proposes adding
  `TestInitTmuxSession_ReuseBranchSkipsWireTmuxSession` to prove `initTmuxSession()`'s reuse
  guard (`HasSession()==true && IsAlive()==true` → skip rebuild) is intentional and covered.
  That exact case is **already tested**: `TestInitTmuxSession_ReuseRequiresAliveNotJustConstructed`
  (`session/instance_tmux_test.go:135-170`, landed in PR #797, "fix(session): require IsAlive()
  alongside HasSession() before reusing tmux session", committed 2026-09-13, clean on HEAD) is a
  table-driven test whose second case is literally `{"live session: reuses, no rebuild", true,
  true, false}` — `hasSession=true, isAlive=true, wantRebuild=false` — asserted via
  `inst.LaunchCommand` staying at a sentinel value (proof `wireTmuxSession`/`buildLaunchCommand`
  never ran). Implementing Story 1.2.2 as scoped produces a near-duplicate test, which risks
  tripping this repo's own `make ready-complexity-gate` `dupl` CI gate — the same gate the plan's
  own Pattern Decisions table cites as justification for using table-driven tests elsewhere — and
  wastes an implementation task on coverage that already exists. The plan's Task 1.2.2a text also
  misdirects the implementer toward the wrong precedent, saying to reuse "whatever fake
  processManager/TmuxBackend test double this file's existing tests
  (`TestInstance_BuildExtraEnv_...`) already use" — but `TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars`
  (line 1206) uses a bare `&Instance{}` literal with **no** process manager at all; the actually
  relevant, ready-made double is `mockTmuxManager{hasSessionReturn, isAliveReturn}` +
  `NewTmuxBackend(mock)`, used by the very sibling test this finding cites. Both symptoms point to
  the same root cause: this story was scoped without grepping `session/instance_tmux_test.go` for
  existing reuse-branch coverage before writing it.
  **Remediation**: retarget Story 1.2.2 at the real remaining gap instead of re-adding a covered
  case. `initTmuxSession`'s actual guard is `HasSession() && (IsAlive() || IsBackendProcessAlive())`
  (`session/instance_tmux.go:682`), and the existing table only ever varies `HasSession`/`IsAlive`
  via `mockTmuxManager` — the `IsBackendProcessAlive()` disjunct
  (`HasLiveSessionNoCache()`/`CachedPanePIDStillAlive()`, `session/instance_tmux.go:888-901`) is
  never exercised as the *sole* reason a session is reused (i.e. `IsAlive()==false` but
  `IsBackendProcessAlive()==true`). Either add that one case to the existing table, or drop Story
  1.2.2 outright if that narrower gap isn't judged worth a task.

## Nitpicks

- **Epic 1.2.4 / Task 1.2.4a** (extend `TestWrapRemoteCommand` with an `-e KEY=VALUE` case) adds
  negligible regression value. `wrapRemoteCommand` (`session/tmux/remote_env.go:41-45`) is a blind
  prepend — `append([]string{"-u","TMUX","TERM=...",name}, args...)` — with zero parsing or
  special-casing of `args`; it structurally cannot distinguish an `-e KEY=VALUE` pair from any
  other two-token flag/value pair. The existing `"tmux new-session with socket flag"` case
  (`session/tmux/remote_env_test.go:29-34`) already proves multi-token argv passes through
  unchanged and in order. The plan's stated rationale ("explicitly proven ... not just inferred
  from the existing generic cases") overstates the case's value — no code change to
  `wrapRemoteCommand` could pass the new case while failing the existing ones. Harmless at ~2 min
  of effort; not worth blocking, but don't cite it as closing a real risk in the PR description.

- Tech Debt Disposition's `ExtraEnv` "not dead code" correction and its "8 call sites" count were
  spot-checked (`grep -n '\.ExtraEnv = append' session/instance.go` → exactly 8 matches at the
  cited lines) and are accurate — a good example of research grounded in the actual repo rather
  than carried forward from a stale claim.

## What checked out clean

- Epic 1.1's fixtures (`envtest.NewIsolatedStateDir`, `createTestStorage`,
  `initGitRepoWithCommit`, `newCreateTestService`/`destroyCreatedSession`) all exist with the
  signatures the plan assumes; the non-parallel + `t.Skip("tmux not available")` pattern
  correctly mirrors `TestCreateSession_StatusManagerWiredBeforeDriver`. Goroutine hygiene is
  already covered without a separate `goleak` task: `destroyCreatedSession` joins the session's
  `SessionDriver` goroutine on cleanup (`server/services/session_service_create_test.go:669-695`),
  and `goleak` usage in this package is opt-in per-test (7 of 188 `server/services/*_test.go`
  files), not a blanket convention this new test would be skipping.
- Epic 1.2.1 (resume-picks-up-env-change) is genuinely new coverage — `session/pause_resume_test.go`'s
  existing tests are all permission/state-transition focused, none touch env vars or program
  config, and `Instance.Resume()`'s worktree-recreation-then-`IsAlive()`-check flow
  (`session/instance.go:2184-2223`) supports the plan's described repro path.
- Epic 1.2.3 (`tmux_session_start_test.go`) targets a file that doesn't exist yet and a real,
  verified duplication: `start()`'s inline loop (`tmux_session_start.go:214-219`) and
  `newSessionArgs()` (`tmux_session_start.go:616-624`) are two independent, textually-duplicated
  `-e` argv-building loops — confirmed by direct read. This is legitimate, valuable new coverage.
- Epic 1.3's premise was independently verified: `useAvailablePrograms.ts:24` maps `value: p.id`,
  and `dispatch.ts:55,100` pass `action.program` straight through — the client already sends the
  program ID, and the existing Jest fixture's `id === command === "aider"` genuinely can't
  distinguish a `value: p.id` bug from a `value: p.command` bug. The proposed fixture change
  (`useAvailablePrograms.test.ts`) closes that gap correctly.
- `research/build-vs-buy.md`'s "no dependency addition warranted" recommendation matches the
  plan's ADR-free, extend-in-place approach.
- The Tech Debt Disposition table's other two "Extend as-is" verdicts (duplicate argv loops
  deferred with a specific follow-up pointer; `ExtraEnv`/`extraEnv` two-field design) are honestly
  justified, not paper-over — the duplicate-loop deferral names the exact `//nolint:gocognit,gocyclo`
  hotspot it's avoiding scope creep into and points at Epic 1.2.3's new test as the regression net
  for that future refactor.
