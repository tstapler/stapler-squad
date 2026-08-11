# Plan: Switching Session Programs Does Not Save Correctly

> **Superseded 2026-07-01.** This file replaces the earlier version of `plan.md` (2026-06-30), which
> planned RC1–RC4 below as *upcoming* work. Commit `914138ec` ("fix(session): program switching now
> saves correctly for all cases", 2026-06-29) already shipped that work to `main`. This revision
> reconciles the plan against the current tree — verified line-by-line in
> `research/architecture.md` and `research/pitfalls.md` — and re-scopes the backlog item to the
> **residual gaps** that the original fix did not close.

## Executive Summary

The reported bug — program changes silently failing to save — is fixed on `main`: the backend guard
that dropped "System default" saves is gone, empty string resolves to the config default before the
`NotEmpty()` ent constraint, the save is ordered before the destructive restart, React state re-syncs
from the server, and a "Change Program" menu item now exists in the session overflow menu. What
remains is real but lower-severity: the shipped feature has **zero automated test coverage**, a
**second undocumented code path** (`UpdateSessionProgram`, used by capacity-monitor auto-fallback) can
drift from the RPC handler and race with it, and a few UX/data-integrity gaps (no restart confirmation,
no "pending on resume" indicator, stale Claude conversation UUID on a claude→other→claude round trip)
were scoped in the original plan but never implemented.

## Implementation Approach

**Phase 1 — Close the test gap (highest value, lowest risk).** Write the Go and Jest tests the
original `validation.md` specified but that were never added. This is pure test-writing against
already-correct behavior; low risk, immediately shrinks `docs/registry/coverage-gaps.json`.

**Phase 2 — Consolidate the duplicate code path.** `UpdateSessionProgram`
(`server/services/session_service.go:3957-3991`, called only from `capacity_monitor.go:308`) reimplements
the `UpdateSession` RPC handler's program-switch logic with a weaker guard (no empty→default
resolution) and a duplicated claude/antigravity substring heuristic. Either delete it if truly unused
beyond the auto-fallback caller, or refactor both call sites onto one shared internal function so a
future change to program-switch semantics can't land in only one of them.

**Phase 3 — Close the concurrency gap between the two paths.** No serialization exists between a
user-triggered `UpdateSession` and an automatic capacity-monitor `UpdateSessionProgram` firing on the
same session — both can read-decide-write independently and double-restart or double-port history.
Phase 2's consolidation is a prerequisite; add a per-instance "program change in flight" guard
(actor-routed, per `.claude/rules/go-double-checked-locking.md` and the `actor-field-guard` CI check).

**Phase 4 — Data-integrity fix: clear stale Claude conversation linkage.** On a program switch that
*leaves* the claude/antigravity family entirely (not just claude↔antigravity), clear
`claudeSession.ConversationUUID` / `HistoryFilePath` via a new actor-routed setter, mirroring the
existing `SetProgram` pattern. Without this, claude → aider → claude passes a stale `--resume <uuid>`.

**Phase 5 — UX polish.** Add a proper two-step confirmation dialog before restarting an Active session
(matching the existing `Restart`/`Delete`/`Autonomous` pattern in `SessionActionsOverflow.tsx`, instead
of today's passive text hint), a "pending on next resume" badge for Paused/Stopped sessions whose
program was just changed, and a re-sync guard on the overflow-menu program picker (it lacks the
`useEffect` re-sync that `SessionDetailView.tsx` already has, so a stale dialog can silently clobber an
auto-fallback change).

**Phase 6 — Registry/CI hygiene.** Flip `docs/registry/features/backend/session/update.json` `tested`
to `true` with real test IDs once Phase 1 lands; add a missing frontend registry entry + `// +feature:`
marker for the "Change Program" overflow-menu feature; run `make registry-generate`.

## Task Breakdown

| # | Task | Estimate | Category |
|---|------|----------|----------|
| 1 | Go tests: `UpdateSession` program branch — active/restart, stopped/no-restart, empty→default resolution, no-op on unchanged value | 2h | test |
| 2 | Jest tests: `SessionActionsOverflow` program picker — open pre-fills current value, save calls `onChangeProgram`, "System default" sends `""`, restart hint only on Active | 1.5h | test |
| 3 | Consolidate `UpdateSessionProgram` and `UpdateSession`'s program block into one shared function; fix the missing empty→default guard in the capacity-monitor path | 2h | backend |
| 4 | Add per-instance "program change in flight" guard to serialize manual vs. auto-fallback program switches | 2h | backend |
| 5 | Add actor-routed setter to clear `claudeSession.ConversationUUID`/`HistoryFilePath` when switching out of the claude/antigravity family; wire into both program-switch paths | 1.5h | backend |
| 6 | Replace passive "session will restart" text hint with a two-step confirmation dialog for Active-session program changes, matching `Restart`/`Delete` pattern | 1.5h | frontend |
| 7 | Add re-sync `useEffect` to `SessionActionsOverflow`'s program picker (mirror `SessionDetailView`'s fix) so an open dialog reflects concurrent server-side changes | 0.5h | frontend |
| 8 | "Pending on next resume" indicator on session card/row when a Paused/Stopped session's program was changed but not yet applied | 1h | frontend |
| 9 | Playwright e2e spec: overflow menu → change program → save → session list reflects new program | 1.5h | test |
| 10 | Frontend feature registry entry + `// +feature:` marker for "Change Program"; flip backend `session:update` registry entry to `tested: true` with new test IDs; `make registry-generate` | 0.5h | docs |
| 11 | Replace magic `session.status === 3` literal with `SessionStatus.ACTIVE` in `SessionActionsOverflow.tsx` | 0.25h | frontend |

## Dependencies and Blockers

- **Task 4 depends on Task 3** — can't add a shared in-flight guard until there's one code path to guard.
- **Task 5 requires a new actor setter** in `session/instance_actor_setters.go` — `make ci`'s
  `actor-field-guard` target will fail the build if the UUID/history-path fields are mutated directly
  from `server/services/session_service.go` instead of through an actor-routed method.
- **No proto or ent schema changes required** for any task in this plan — everything is implementable
  within the existing `UpdateSessionRequest.program` field and `Instance` actor methods.
- **Tasks 1–2 have no code dependencies** and can start immediately/in parallel with everything else;
  recommended as the first PR since it's the lowest-risk, highest-registry-value work.
- **Task 9 (e2e) requires the test server** running per `.claude/rules/e2e-test-conventions.md`
  (`STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local`).
