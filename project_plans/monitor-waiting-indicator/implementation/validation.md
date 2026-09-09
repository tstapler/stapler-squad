# Validation Plan: monitor-waiting-indicator

**Date**: 2026-09-09

## Happy Path Scenario
Given a Claude Code session whose tmux pane emits the turn-completion line `"✻ Cogitated for 1m 6s · done 3:06 PM · 1 shell, 1 monitor still running"` (the Baseline problem statement in requirements.md), when the session's status is next detected, then `PatternSet.MatchLines`/`DetectWithContextAndCountFromLines` returns `StatusWaitingForAgent` with a combined count of `2` (1 shell + 1 monitor, not just the 1 monitor it undercounts today), and the existing `SubStatusChip` renders `"⏳ Waiting for 2 Tasks"` in the UI.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC1: comma-joined "N shell, M monitor still running" detected and summed | `session/detection/pattern_set_test.go` | `TestPatternSet_MatchLines_should_returnCombinedCount_When_shellsAndMonitorsCommaJoined` (new, alongside `TestPatternSet_MatchLines_should_returnCount_When_shellsStillRunningMatches` at line 63) | Unit | Happy path — `MatchLines("... 1 shell, 1 monitor still running")` → `StatusWaitingForAgent`, count `2` |
| AC1: malformed/non-matching input falls through, no false match | `session/detection/pattern_set_test.go` | `TestPatternSet_MatchLines_should_returnZero_When_noWaitingForAgentPatternMatches` (existing, line 78 — already asserts `"Thinking..."` → count `0`; no new test needed, cited as existing coverage for the error/non-match path) | Unit | Error path — no pattern matches, count stays `0` |
| AC1/AC5: `matchWaitingForAgent` sums all non-empty capture groups, zero-count guard falls through instead of returning `ok=true, count=0` | `session/detection/bug_regression_test.go` | `TestBug_ShellsAndMonitorsStillRunning` (new sibling to `TestBug_ShellsStillRunning`, line 613) — table cases: `"1 shell, 1 monitor still running"` → count `2`; `"2 shells, 3 monitors still running"` → count `5`; `"1 monitor still running"` (monitor-only, no shell) → count `1` (regression guard) | Unit | Happy path (combined) + regression (monitor-only unaffected) |
| AC1: singular vs. plural phrasing (`shell`/`shells`, `monitor`/`monitors`) both match | `session/detection/bug_regression_test.go` | `TestBug_ShellsAndMonitorsStillRunning` (same test, singular `"1 shell, 1 monitor"` and plural `"2 shells, 3 monitors"` sub-cases) | Unit | Happy path variants |
| AC3: indicator clears once a more-recent line no longer reports outstanding work | `session/detection/bug_regression_test.go` | `TestBug_ShellsAndMonitorsStillRunning_ClearsOnNewerPromptLine` (new, mirrors `TestBug_AutoModeFooter_NoFooterLine_NotOverridden` shape at line 1211) | Integration (multi-line `DetectWithContextAndCountFromLines` backward-scan, not a single-pattern unit match) | Given `[]string{"✻ ... 1 shell, 1 monitor still running", "❯ "}`, backward-scan resolves to the newer `"❯ "` line → non-`StatusWaitingForAgent`, count `0` |
| AC6: no regression to existing detection/tag-organization behavior | `session/detection/bug_regression_test.go`, `session/detection/pattern_set_test.go`, full `session/detection/...` suite | `TestBug_ShellsStillRunning` (line 613), `TestBug_AutoModeFooter_WaitingWhenIdleWithBackgroundShells` (line 1142), `TestBug_AutoModeFooter_SingularShell` (line 1188), `TestBug_AutoModeFooter_NoFooterLine_NotOverridden` (line 1211), and all pre-existing `TestPatternSet_MatchLines_*` cases | Regression suite run (Task 1.1.1d) | `go test ./session/... -count=1` — every previously-passing case unchanged |
| AC2: distinct UI indicator (verification only, no code change per plan.md Epic 1.2) | `web-app/src/components/sessions/__tests__/SubStatusChip.test.tsx` | `"renders count in Waiting for Agents chip when subagentCount > 0"` (existing, line 37 — already asserts `subagentCount=2` renders `"2 Tasks"` text and the corresponding `title`) | Unit (existing coverage, cited not duplicated) | `SubStatusChip` given `subagentCount={2}` renders `"⏳ Waiting for 2 Tasks"` |
| AC4: control-mode/legacy-polling parity (verification only, no code change per plan.md Epic 1.2) | none — architectural guarantee | N/A | Architectural guarantee, not a new test | `ClaudeController.GetStatusAndIdleInfo` (`session/claude_controller.go:1105-1138`) has no `STAPLER_SQUAD_USE_CONTROL_MODE`-conditional branch; both modes read through the same `ptyAccess.GetRecentHash`/`statusCache` path into the same detector fixed by AC1's tests above, so no mode-specific test can exercise a code path that doesn't exist. Covered by grep-confirming the absence of such a branch (Task 1.2.1b), not by a test. |

## UX Acceptance Tests
(This is a backend counting fix with no new UI surface — per `design/ux.md`'s scope note, `SubStatusChip.tsx` needs no code change. The table below documents how each UX acceptance criterion in `design/ux.md` Step 3 is already covered, by existing test or manual check, not new e2e work.)

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| 1. Correct combined count shown with 0 clicks | `web-app/src/components/sessions/__tests__/SubStatusChip.test.tsx` | `"renders count in Waiting for Agents chip when subagentCount > 0"` (existing, line 37) | Jest/RTL | Render `<SubStatusChip subStatus={WAITING_FOR_AGENT} subagentCount={2}/>`, assert `"2 Tasks"` text present, no interaction required |
| 2. Singular/plural grammar correct | `web-app/src/components/sessions/__tests__/SubStatusChip.test.tsx` | `"renders singular Task for subagentCount === 1"` (existing, line 47) | Jest/RTL | Render with `subagentCount={1}`, assert `"1 Task"` not `"1 Tasks"` |
| 3. Chip distinguishable from Idle/Ready/NeedsApproval by label+icon+CSS class, not color alone | manual | Visual spot-check after fix lands | Manual | Open the session list with a real "1 shell, 1 monitor still running" session; confirm the chip's label text, glyph, and background differ visibly from a plain Idle/Ready/NeedsApproval session's chip |
| 4. Screen-reader accessible (`role="status"`, `aria-label`, `title`) | `web-app/src/components/sessions/__tests__/SubStatusChip.test.tsx` | `"renders count in Waiting for Agents chip when subagentCount > 0"` (existing, line 37 — asserts `aria-label="Waiting for agents"` and the `title` text) | Jest/RTL | Assert `getByRole("status")` has the expected `aria-label`/`title` attributes |
| 5. No dead ends — chip never shows a wrong/blank count | `web-app/src/components/sessions/__tests__/SubStatusChip.test.tsx` | `it.each` `"omits count when subagentCount is %s"` (existing, line 57) | Jest/RTL | Render with `0`/`undefined`/negative/`NaN`, assert fallback to unnumbered `"Waiting for Agents"`, never a wrong number |
| 6. Count clears on next refresh once scrollback no longer reports outstanding work | manual (backed by `session/detection/bug_regression_test.go`'s `TestBug_ShellsAndMonitorsStillRunning_ClearsOnNewerPromptLine`) | N/A | Manual | Start a session, let a background shell+monitor line appear, confirm chip shows `"Waiting for N Tasks"`; let the work finish so the next line is a plain prompt; wait for the next poll tick; confirm the chip changes |
| 7. Consistent across card/row views | manual | Visual spot-check | Manual | Toggle between card and list view for the same session; confirm identical chip text in both (`SessionCard.tsx:787-792` vs. `SessionRow.tsx:349-361`, same component/props) |
| 8. No misclassification into "idle" bucket | manual (backed by `deriveWorkingState.ts:34,49` — cited in design/ux.md as already-verified) | N/A | Manual | In a Status-grouped tag-organization view, confirm a session showing the chip appears in the Processing/Active bucket, never Idle |
| 9. Color contrast ≥ 4.5:1 | none — token-reuse argument, not independently re-measured (see design/ux.md Gaps) | N/A | N/A (documented gap, pre-existing, out of scope for this fix) | Not a new check; `chipWaitingForAgent` reuses `chipProcessing`'s already-shipped token pair |

## Test Stack
- **Unit (Go)**: `go test` + stdlib `testing`, table-driven cases (existing convention in `session/detection/*_test.go`) — no new dependency.
- **Integration (Go)**: same `go test` binary — the AC3 clearing case exercises the multi-line `DetectWithContextAndCountFromLines` path rather than a single-pattern `MatchLines` call, but needs no external service/test double (pure in-memory scan).
- **Unit (TS)**: Jest + `@testing-library/react`, existing `SubStatusChip.test.tsx` conventions — no new test file needed for AC2/UX criteria; cited as existing coverage.
- **E2E / UX**: No new Playwright spec — this is a backend counting fix behind an unchanged UI surface; UX acceptance criteria are covered by the existing Jest suite plus the manual checklist above (clearing behavior, cross-surface consistency, idle-bucket exclusion), consistent with `design/ux.md`'s scope note that no new component or interaction exists to automate.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./session/detection/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line (existing `session/detection` package baseline; this change only adds lines to already-covered functions) |
| Go (full regression, AC6) | `go test ./session/... -count=1` | 100% pass, zero regressions |
| TypeScript/Jest | No new coverage run required — `SubStatusChip.test.tsx` is unchanged by this fix; existing `cd web-app && npx jest --coverage` run (unrelated to this change) still applies project-wide | ≥80% line (pre-existing target, unaffected) |

- All public service methods touched (`PatternSet.MatchLines`, `matchWaitingForAgent`): happy path + error/no-match path covered above.
- No external integrations are involved (pure regex/string parsing over already-in-memory PTY tail bytes) — no mocked-integration test needed beyond the "integration" row above (multi-line scan).
- UX acceptance criteria: every criterion in `design/ux.md` Step 3 has a corresponding existing test or manual step in the table above; none are uncovered.
- Migration test: N/A — no schema change.
