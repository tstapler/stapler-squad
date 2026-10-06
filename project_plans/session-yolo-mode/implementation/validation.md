# Validation Plan: session-yolo-mode

**Date**: 2026-08-06

## Happy Path Scenario

Given a paused-and-restarted-clean session creation flow with `program: "claude"`, when the
user checks the "⚡ Auto-approve" checkbox in the Omnibar creation panel and submits, then the
created session persists `auto_approve = true`, its launch command contains
`--dangerously-skip-permissions`, the session card immediately shows the `data-testid="badge-auto-approve"`
badge, and reloading the page (session read back via the session API) still shows
`auto_approve: true` and the badge.

## Requirement -> Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| R1: `auto_approve` field persisted, survives reload, readable via API (requirements.md Success Criteria + Must Have "proto request + persisted session state") | `session/instance_test.go` | `TestNewInstance_should_SetAutoApprove_When_CreateOptionsHasAutoApproveTrue` | Unit (happy) | Given `CreateOptions{AutoApprove: true, Program: "claude"}`, when `NewInstance` builds the `Instance`, then `instance.AutoApprove == true`. |
| R1 (cont.) | `session/instance_serialization_test.go` | `TestInstanceSerialization_should_DefaultAutoApproveFalse_When_OptionsOmitField` | Unit (error/edge) | Given `CreateOptions{}` (field not set, Go zero value), when `NewInstance`/`buildSnapshot` runs, then `Instance.AutoApprove == false` and no panic/nil-deref occurs downstream in `buildLaunchCommand`. |
| R1 (cont.) | `session/ent_repository_test.go` | `TestEntRepository_should_RoundTripAutoApprove_When_SessionSavedAndReloaded` | Integration (SQLite via ent) | Given a session created with `AutoApprove: true` and `SaveInstances` called, when `LoadInstances` reads it back from a fresh ent client against the same SQLite file (simulating a process restart), then the reloaded `Instance.AutoApprove == true`. |
| R2: Omnibar creation-panel checkbox sets `auto_approve` at creation without hand-typing flags (Must Have) | `web-app/src/components/sessions/Omnibar.test.tsx` | `dispatchOmnibarAction_should_IncludeAutoApprove_When_CheckboxCheckedAndFormSubmitted` | Unit (happy) | Given `OmnibarFormState.autoApprove = true` after checking the box, when the form submits, then `createSession` is called with `autoApprove: true` in the payload. |
| R2 (cont.) | `web-app/src/components/sessions/OmnibarCreationPanel.test.tsx` | `isAutoApproveSupported_should_ReturnFalse_When_ProgramIsUnsupportedAgent` | Unit (error) | Given `program = "codex"`, when `isAutoApproveSupported(program)` is called, then it returns `false` and the rendered checkbox has the `disabled` attribute with hint text `Not supported for "codex" yet.`. |
| R2 (cont.) | `web-app/src/components/sessions/OmnibarCreationPanel.test.tsx` | `OmnibarCreationPanel_should_ForceAutoApproveFalse_When_ProgramTransitionsToUnsupportedAfterChecked` | Integration (RTL, multi-component: form state + panel render) | Given the checkbox was checked while `program = "claude"`, when the user changes `program` to `"codex"`, then the checkbox becomes unchecked+disabled and `formState.autoApprove` is forced back to `false` (closes the gap flagged in `design/ux.md:64` — must be resolved during implementation, not left as a silent payload mismatch). |
| R3: Correct per-agent flag injected into launch command based on detected binary, not hardcoded to one agent; no-op (not a crash) for unsupported agents (Must Have + Success Criteria) | `session/instance_tmux_test.go` | `TestYoloFlagFor_should_ReturnDangerouslySkipPermissions_When_ProgramIsClaude` | Unit (happy) | Given `program = "claude"`, when `yoloFlagFor(program)` is called, then it returns `"--dangerously-skip-permissions"`. |
| R3 (cont.) | `session/instance_tmux_test.go` | `TestYoloFlagFor_should_ReturnEmpty_When_ProgramUnsupported` | Unit (error) | Given `program = "codex"`, when `yoloFlagFor(program)` is called, then it returns `""`, and `TestBuildLaunchCommand_should_NotAppendFlag_When_AutoApproveTrueButAgentUnsupported` confirms `buildLaunchCommand` leaves the command unchanged with no panic. |
| R3 (cont.) | `session/instance_tmux_test.go` | `TestBuildLaunchCommand_should_AppendYoloFlag_When_AutoApproveTrueAndAgentSupported` | Integration (collaboration across `classifyProgram` + `yoloFlagFor` + command builder; also covers Aider via a table-driven sub-case) | Given `Instance{Program: "claude", AutoApprove: true}` and, in a second table case, `Instance{Program: "aider --model ollama_chat/gemma3:1b", AutoApprove: true}`, when `buildLaunchCommand` runs, then the resulting command string ends with `--dangerously-skip-permissions` / `--yes-always` respectively — proving the flag is resolved dynamically per detected agent, not hardcoded. |
| R4: Session card badge shown when auto-approve is active; pending variant shown when persisted value disagrees with launch command (Must Have) | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard_should_RenderAutoApproveBadge_When_AutoApproveTrueAndCommandMatches` | Unit (happy) | Given `session = {status: ACTIVE, autoApprove: true, launchCommand: "claude --dangerously-skip-permissions"}`, when `SessionCard` renders, then `getByTestId("badge-auto-approve")` is visible with text `⚡ Auto` and `badge-pending-auto-approve` is absent. |
| R4 (cont.) | `web-app/src/components/sessions/SessionCard.test.tsx` | `hasPendingAutoApproveChange_should_ReturnTrue_When_AutoApproveTrueButFlagAbsentFromLaunchCommand` | Unit (error/edge) | Given `session = {status: PAUSED, autoApprove: true, launchCommand: "claude"}` (flag not yet baked in), when `hasPendingAutoApproveChange(session)` is called, then it returns `true`; also assert it returns `false` when `launchCommand` is empty (pre-first-launch guard, `design/ux.md:155`). |
| R4 (cont.) | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard_should_InvokeOnToggleAutoApprove_When_BadgeClicked` | Integration (RTL component interaction: click handler wired through props) | Given `onToggleAutoApprove` is provided as a prop and the badge is rendered as a `<button>`, when the user clicks it, then `onToggleAutoApprove(session.id, false)` fires exactly once and the click does not propagate to the parent card's row handler (`e.stopPropagation()`). |
| R5: Post-creation toggle is an explicit session action, takes effect on next launch/restart — never a live in-place flag swap on a running process (Should Have) | `session/instance_actor_setters_test.go` | `TestSetAutoApprove_should_RestartSession_When_StatusActive` | Unit (happy) | Given an `Active` `Instance` with `AutoApprove: false` and a fake `ProcessManager`, when `SetAutoApprove(true, persistFn)` is called, then `Instance.AutoApprove == true`, `persistFn` was invoked, and the fake `ProcessManager` recorded a `KillSession`/`new-session` pair (i.e. `Restart(true)` fired) — proving the change is not applied as a live in-place swap. |
| R5 (cont.) | `session/instance_actor_setters_test.go` | `TestSetAutoApprove_should_ReturnErrorAndSkipRestart_When_PersistFails` | Unit (error) | Given `persistFn` returns an error, when `SetAutoApprove(true, persistFn)` is called, then the error propagates, `Instance.AutoApprove` was already flipped in memory (matches the documented ordering: state set → persist → restart), but no `Restart`/tmux call occurs — a crash between set and restart never leaves a half-restarted process. |
| R5 (cont.) | `server/services/session_service_test.go` | `TestUpdateSession_should_RestartAndPersistAutoApprove_When_ToggledOnActiveSession` | Integration (ConnectRPC handler + real storage + fake `ProcessManager`) | Given a session `{id: "sess-1", program: "claude", status: ACTIVE, auto_approve: false}`, when `UpdateSession({id: "sess-1", auto_approve: true})` is called, then the response's `session.auto_approve == true`, `updated_fields` contains `"auto_approve"`, storage has the persisted row, and the fake `ProcessManager` shows a restart occurred with the new launch command containing `--dangerously-skip-permissions`. |
| R6: `auto_approve` is independent of `auto_yes` — additive only, no regression to existing `AutoYes`/`PermissionMode` behavior, no crash on overlap (plan.md Risk Control accepted edge case) | `session/instance_tmux_test.go` | `TestBuildLaunchCommand_should_NotDoubleAppendFlag_When_AutoApproveAndAutoYesBothTrue` | Unit (happy — documents the accepted edge case) | Given `Instance{Program: "claude", AutoApprove: true, AutoYes: true}`, when `buildLaunchCommand` runs, then the command contains both `--permission-mode bypassPermissions` (once) and `--dangerously-skip-permissions` (once) — neither literal duplicated. |
| R6 (cont.) | `session/instance_tmux_test.go` | `TestBuildLaunchCommand_should_LeaveAutoYesOnlyBehaviorUnchanged_When_AutoApproveFalse` | Unit (error/regression guard) | Given `Instance{Program: "claude", AutoApprove: false, AutoYes: true}` (pre-existing behavior, unrelated to this feature), when `buildLaunchCommand` runs, then the command is byte-identical to what it was before this feature shipped (only `--permission-mode bypassPermissions`, no `--dangerously-skip-permissions`) — regression guard for the untouched `auto_yes` code path. |
| R6 (cont.) | `server/services/session_service_test.go` | `TestCreateSession_should_PersistAutoApproveAndAutoYesIndependently_When_BothSetInRequest` | Integration (service + ent) | Given `CreateSessionRequest{Program: "claude", AutoApprove: true, AutoYes: true}`, when `CreateSession` runs, then the persisted ent row has `auto_approve = true` AND `auto_yes = true` as independent columns, and reading the session back via `GetSession` returns both fields correctly (no cross-contamination between the two booleans). |
| R7: Ent schema migration is additive; existing rows get `false` (plan.md Migration Plan) | `session/ent_repository_test.go` | `migration_should_be_reversible` | Migration | See dedicated section below. |

**Frontend integration tests above use React Testing Library** ("integration" here means multiple
collaborating units — form state + panel render, or card + click handler — not a real network
call; there is no frontend datastore to integrate against for R2/R4).

## Migration Test Detail

**Test**: `migration_should_be_reversible` (`session/ent_repository_test.go`)

Per plan.md's stated rollback strategy ("standard PR revert... the ent column is additive-only
and defaults to `false`... leaves an unused, harmless column behind"), a schema *rollback* isn't
mechanically testable (there's no down-migration tooling in this repo). What **is** testable and
is the substance of this test:

1. **Pre-migration state simulated**: open a fresh SQLite file and manually `CREATE TABLE
   sessions (...)` via raw SQL *without* the `auto_approve` column (mirrors a DB created before
   this feature shipped), then `INSERT` one row.
2. **Migration applied**: open the same file with the current ent client and call
   `client.Schema.Create(ctx)` (the same auto-migration path invoked at startup per
   `session/ent_repository.go:76`).
3. **Assert additive correctness**:
   - The `auto_approve` column now exists (query `PRAGMA table_info(sessions)`).
   - The pre-existing row (inserted in step 1, before the column existed) reads back
     `auto_approve == false` — proving existing rows get the default, not a NULL/zero-value
     crash.
   - A newly inserted row with no explicit `auto_approve` value also defaults to `false`.
4. **Note in the test's doc comment** (not re-litigated in this doc per the Proportionality
   principle): "Reversibility here means additive-and-safe-to-leave-behind, per plan.md's
   Migration Plan — this test does not exercise a down-migration because none exists or is
   expected for this repo's ent workflow."

## UX Acceptance Tests

All Playwright specs live in `tests/e2e/session-auto-approve.spec.ts` unless noted, start with
`// @feature session:create, session:update`, use only `data-testid`/ARIA locators, and use
`expect(...).toBeVisible()`/`toHaveAttribute()`/`toBeDisabled()` polling instead of
`waitForTimeout` per `.claude/rules/e2e-test-conventions.md`. Shared setup (create a session with
a given program, open the overflow menu) is factored into `tests/e2e/pages/SessionCardPage.ts` /
`OmnibarPage.ts` helpers per the same rules.

| UX Criterion (design/ux.md) | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| 2 & 6: Discover unsupported state in 0 extra clicks; disabled-but-visible checkbox + hint | `session-auto-approve.spec.ts` | `describe('auto-approve session creation') > 'disables checkbox for unsupported agent'` | Playwright | Open Omnibar, set program to an unsupported value (e.g. `codex`); assert `getByRole('checkbox', {name: /auto-approve/i})` has `disabled` attribute and hint text `Not supported for "codex" yet.` is visible — no extra click/menu open needed. |
| 1: Enable at creation in 1 click | `session-auto-approve.spec.ts` | `describe('auto-approve session creation') > 'creates session with auto_approve flag'` | Playwright | Open Omnibar, set program to `claude`, click the auto-approve checkbox once, fill title, submit; assert the created session (via API response or card) has `autoApprove: true`. |
| 5: Identify at a glance in the collapsed list | `session-auto-approve.spec.ts` | `describe('auto-approve session creation') > 'shows badge on created session'` | Playwright | After creating the session from the previous scenario, assert `getByTestId('badge-auto-approve')` is visible directly in the session list card without opening/hovering it. |
| 3: Enable post-creation in 3 clicks | `session-auto-approve.spec.ts` | `describe('auto-approve post-creation toggle') > 'enables via overflow menu with confirm dialog'` | Playwright | Create a session with `autoApprove: false`; click overflow menu (1), click "Enable auto-approve" (2), click "Enable Auto-Approve" in the confirm dialog (3); assert exactly 3 interactions occurred and the badge appears afterward. |
| 4: Disable post-creation, 1 click via badge | `session-auto-approve.spec.ts` | `describe('auto-approve post-creation toggle') > 'disables via badge click with no confirm'` | Playwright | With an auto-approve-enabled session, click the `badge-auto-approve` button; assert no dialog appears and `updateSession` is called immediately (badge disappears or `badge-pending-auto-approve` appears if the session is Active mid-restart). |
| 4: Disable post-creation, 2 clicks via menu | `session-auto-approve.spec.ts` | `describe('auto-approve post-creation toggle') > 'disables via overflow menu with no confirm'` | Playwright | With an auto-approve-enabled session, open overflow menu (1), click "Disable auto-approve" (2); assert no confirm dialog appears. |
| Surface (c), supports criterion 5 for the transitional state | `session-auto-approve.spec.ts` | `describe('auto-approve post-creation toggle') > 'shows pending badge when paused session has mismatched flag'` | Playwright | Pause a session, toggle auto-approve on while paused (no restart occurs per R5), assert `badge-pending-auto-approve` renders with text `⏳ Auto-approve pending` and the steady-state badge is absent. |
| 9 (dialog exit path, Cancel) | `session-auto-approve.spec.ts` | `describe('auto-approve post-creation toggle') > 'confirm dialog exits via Cancel without calling updateSession'` | Playwright | Open the enable-confirm dialog, click "Cancel"; assert dialog closes, `session.autoApprove` unchanged, and no network request to `UpdateSession` fired (via `mcp__playwright__browser_network_requests`-style assertion or intercepted route). |
| 9 (dialog exit path, Escape) | `session-auto-approve.spec.ts` | `describe('auto-approve post-creation toggle') > 'confirm dialog exits via Escape without calling updateSession'` | Playwright | Open the enable-confirm dialog, press `Escape`; assert same outcome as the Cancel test. |
| 10: Keyboard navigation | `session-auto-approve.spec.ts` | `describe('auto-approve accessibility') > 'all interactive elements reachable via keyboard'` | Playwright | Tab through the Omnibar checkbox, session-card badge button, overflow-menu item, and confirm-dialog buttons; assert each receives focus in sequence and activates via Enter/Space (no mouse interaction used in this test). |
| 11: Screen-reader labels match spec strings | `session-auto-approve.spec.ts` | `describe('auto-approve accessibility') > 'aria labels match spec for badge and menu item'` | Playwright | Assert `getByLabel("Auto-approve enabled — this session skips permission prompts; click to disable")` resolves the badge button, and the overflow menu item has `role="menuitemcheckbox"` with `aria-checked` matching `session.autoApprove`. |
| 7: Unsupported agent, post-creation (**blocked** — design/ux.md:200 gap, not yet satisfiable per plan.md) | `session-auto-approve.spec.ts` | `describe('auto-approve error handling') > 'overflow toggle disabled for unsupported agent post-creation'` | Playwright (`test.fixme()` until the gap is closed) | Intended: create/switch a session to an unsupported agent post-creation, open overflow menu, assert the auto-approve toggle item is disabled with a hint rather than clickable-and-no-op. Marked `fixme` because plan.md's Task 6.2.1 has no disabled/support-check branch yet — do not implement this test as "passing" until that gap is closed (see design/ux.md's explicit "not yet satisfiable" note). |
| 8: RPC failure shows visible error, no optimistic state (**blocked** — design/ux.md:117/242/286 gap, not yet satisfiable per plan.md) | `session-auto-approve.spec.ts` | `describe('auto-approve error handling') > 'shows visible error when toggle RPC fails'` | Playwright (`test.fixme()` until the gap is closed), intercept `UpdateSession` route to return an error | Intended: toggle auto-approve, force the `UpdateSession` RPC to reject (route intercept), assert a visible error (toast/inline message) appears and the badge/checkbox reverts to the last-confirmed server state rather than staying on the optimistic click target. Marked `fixme` — plan.md's Task 6.1.1 handler only does `console.error`, no visible-error UI exists to test yet. |
| 12: Color contrast ≥ 4.5:1 (text) / 3:1 (non-text) | *(not a new Playwright test)* | — | Axe Core (existing CI gate on `web-app/src/` PRs, per root `CLAUDE.md`'s "UX analysis CI") | The new badge markup rides the existing Axe Core scan that already blocks on WCAG AA violations — no bespoke test needed. **Follow-up flagged in design/ux.md:373-383**: the badge's 1px border (`vars.color.warning` on `vars.color.warningBg`) measured 1.93:1 for the light theme, below the 3:1 non-text WCAG 1.4.11 threshold — Axe Core's default ruleset does not reliably catch border-only contrast, so this should be spot-checked manually across all 6 themes during implementation review, not assumed caught by CI. |

Two of the above (`fixme`, criteria 7 & 8) are deliberately not written as passing tests: writing
them "green" would either falsely certify an unimplemented behavior or require scope-creeping
this validation pass into closing UX gaps that `design/ux.md` explicitly flagged as unresolved
implementation decisions. They are included so the gap has a named, trackable test rather than
silently missing coverage — flip `fixme` → real assertions once Epic 6 closes the gap (see
Unresolved Questions below).

## Test Stack

- Unit: Go stdlib testing (table-driven), Jest/RTL for frontend
- Integration: Go tests against a real (temp-file) SQLite ent client and a fake/in-memory
  `ProcessManager` (existing test pattern in `session/` — no live tmux process); frontend
  "integration" tests are React Testing Library multi-component render/interaction tests (no
  real network layer on the frontend side)
- E2E/UX: Playwright per `tests/e2e/` conventions, run against the isolated test server
  (`global-setup.ts`), not the live `:8543` deployment

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | >=80% line |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | >=80% line |
