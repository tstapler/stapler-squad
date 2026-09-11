# Validation Plan: pi-support

**Date**: 2026-09-02

## Happy Path Scenario
Given a user with the `pi-support` feature flag enabled and pi installed on `PATH` (Baseline: pi already launchable as free-text but with no resume, no preset, no status, no approval enforcement), when the user picks "pi" from the program picker, creates a session, has a pi tool call classified and allowed/blocked by the same `RulesService` rules that gate Claude Code, stops the session, and resumes it, then the resumed session continues the same pi conversation (`--session <id>` injected), the session card shows a `Loaded` approval-extension health badge, the session list reflects pi's live working/idle state, and the tool-call decision appears in the approval audit log exactly as a Claude-hook decision would.

---

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-1: Resume support — `isPi` detection | `session/instance_tmux_test.go` | `TestIsPi_ShouldMatchPiInvocations_WhenBasenameIsPi` | Unit | Happy path: `"pi"`, `"/usr/local/bin/pi"`, `"pi --model x"` → `true` |
| REQ-1: Resume support — `isPi` rejects lookalikes | `session/instance_tmux_test.go` | `TestIsPi_ShouldReturnFalse_WhenBasenameIsNotPi` | Unit | Error path: `"pipenv run pi-helper"` → `false` (basename `pipenv`) |
| REQ-1: Resume support — `buildPiCommand` injects resume flag | `session/instance_tmux_test.go` | `TestBuildPiCommand_ShouldAppendShellQuotedResumeFlag_WhenPiSessionIDPresent` | Unit | Happy path: `piSession.SessionID = "abc123"` → `pi --session 'abc123'` (flag syntax per Story 1.1.3's spike finding) |
| REQ-1: Resume support — no session data is a no-op | `session/instance_tmux_test.go` | `TestBuildPiCommand_ShouldOmitResumeFlag_WhenPiSessionIsNil` | Unit | Error/edge path: `piSession == nil` → exactly `pi`, no flag, no error |
| REQ-1: Resume support — end-to-end continuity across restart | `session/instance_test.go` | `TestInstance_ShouldPreserveConversation_WhenPiBackedSessionIsStoppedAndResumed` | Integration | Full `Restart` cycle: captures `PiSessionData` from the live tmux/pi process on stop (external call), persists it via `session/storage.go`, and re-injects it via `buildLaunchCommand` on resume — mirrors the existing `claudeSessionID` capture-on-`Restart` integration coverage at `instance.go:2164-2168` |
| REQ-2: UI preset — `PROGRAMS` ordering | `web-app/src/lib/constants/programs.test.ts` | `TestPrograms_ShouldOrderPiBetweenClaudeAndAider_WhenListed` (Jest: `programs.test.ts › orders pi between Claude and Aider`) | Unit | Happy path: `PROGRAMS.findIndex(p => p.value === "pi")` is `>` Claude's index and `<` Aider's |
| REQ-2: UI preset — unknown program falls back | `web-app/src/lib/constants/programs.test.ts` | `TestGetProgramDisplay_ShouldFallBackToRawString_WhenProgramIsUnrecognized` | Unit | Error path: `getProgramDisplay("some-random-cli")` returns the raw string, not a preset label; `isKnownProgram("some-random-cli")` returns `false` |
| REQ-2: UI preset — N/A | — | — | Integration | N/A — `PROGRAMS` is a static client-side array with no store/external call; covered by the e2e spec under REQ-6 instead |
| REQ-3: Status parsing — JSONL reader decodes captured transcript | `session/pi_adapter_test.go` | `TestPiAdapter_ShouldDecodeEveryKnownEventType_WhenFedCapturedTranscript` | Unit | Happy path: `testdata/pi/basic_session.jsonl` fed line-by-line → every line decodes, no "unrecognized type" error, header `version` field read |
| REQ-3: Status parsing — malformed/unknown event line | `session/pi_adapter_test.go` | `TestPiAdapter_ShouldReturnUnrecognizedTypeError_WhenEventTypeIsUnknown` | Unit | Error path: a line with `"type":"totally_unknown_type"` is reported (counted, per Story 6.1.1), not silently dropped or panicking |
| REQ-3: Status parsing — status flows into session-list model | `session/instance_status_test.go` | `TestInstanceStatusManager_ShouldFallBackToPiStatus_WhenNoClaudeControllerRegistered` | Integration | Real `PiStatusSource` (owns the `pi --mode json` subprocess, external process) registered in `piSources`; `GetStatus()` returns `ClaudeStatus == detection.StatusExecuting`, `IsControllerActive == true` |
| REQ-4: Approval parity — extension template renders valid, safely-embedded TS | `cmd/ssq-hooks/main_test.go` | `TestSsqApprovalExtensionContent_ShouldSafelyEmbedBothURLs_WhenRendered` | Unit | Happy path: `json.Marshal`-embedded permission/health URLs, mirrors `openCodePluginContent`'s test |
| REQ-4: Approval parity — uncaught exception still denies | `cmd/ssq-hooks/main_test.go` | `TestSsqApprovalExtensionTemplate_ShouldWrapHandlerInTryCatchDefaultingToDeny_WhenRendered` | Unit | Error path: rendered source's catch block invokes the confirmed blocking mechanism (Task 1.1.2c's finding), not pi's own uncaught-exception default — string/AST assertion on generated source, since no TS runtime executes in `go test` |
| REQ-4: Approval parity — pi tool call blocked like Claude's | `server/services/approval_handler_test.go` | `TestHandlePermissionRequest_ShouldDenyPiSourcedToolCall_WhenClassifierRuleAutoDenies` | Integration | A `source:"pi"` payload matching an existing high-risk rule is classified by the live `RulesService` and denied — same code path Claude's hook uses, per requirements' "real `RulesService` integration" decision |
| REQ-5: Feature flag — defaults false | `config/config_test.go` | `TestFeatureFlag_PiSupport_ShouldReturnFalse_WhenUnset` | Unit | Happy/default path: `Config{FeatureFlags: nil}` → `GetFeatureFlag("pi-support") == false` |
| REQ-5: Feature flag — round-trips on disk | `config/config_test.go` | `TestFeatureFlag_PiSupport_ShouldPersistAndReload_WhenSetTrue` | Unit | Error/edge path (regression guard against silent drop): `SetFeatureFlag("pi-support", true)` then reload from the on-disk `feature_flags` JSON object returns `true` |
| REQ-5: Feature flag — gates every pi surface | `server/services/pi_extension_health_test.go` | `TestHandlePiExtensionLoaded_ShouldReject_WhenPiSupportFlagIsOff` | Integration | A health-ping POST arriving while the flag is off is not recorded (prevents a stale/leftover ping from ever surfacing UI state for opted-out users) |
| REQ-6: Multi-agent-in-one-session UX — picker + capability gate | `web-app/src/lib/sessions/autoApprove.test.ts` | `TestIsApprovalExtensionSupported_ShouldReturnTrue_WhenProgramIsPi` | Unit | Happy path: `isApprovalExtensionSupported("pi") === true` |
| REQ-6: Multi-agent-in-one-session UX — other agents unaffected | `web-app/src/lib/sessions/autoApprove.test.ts` | `TestIsApprovalExtensionSupported_ShouldReturnFalse_WhenProgramIsOpenCode` | Unit | Error/negative path: `isApprovalExtensionSupported("opencode") === false` — scoped narrowly to pi, not a general capability map |
| REQ-6: Multi-agent-in-one-session UX — e2e preset selection | `tests/e2e/pi-session.spec.ts` | `pi-session.spec.ts › creates a session with the pi preset and resumes it` | Integration (E2E) | Full picker → create → (simulated) resume flow against the isolated test-mode server; see UX table below for the granular ACs this rolls up |
| REQ-7: Observability — extension install failure is logged | `cmd/ssq-hooks/main_test.go` | `TestInstallPi_ShouldLogErrorWithTargetPath_WhenExtensionWriteFails` | Unit | Happy-of-the-error-path: a simulated permissions failure on `~/.pi/agent/extensions/` produces an error-level log line containing the target path and the OS error, not a generic "install failed" |
| REQ-7: Observability — event-throughput counter never silently drops | `session/pi_status_source_test.go` | `TestPiStatusSource_ShouldIncrementEventCounter_ForEveryEventIncludingUnrecognizedType` | Unit | Error path: `{type="totally_unknown_type"}` still increments a labeled counter bucket, not dropped |
| REQ-7: Observability — approval audit distinguishes pi from Claude | `server/services/approval_handler_test.go` | `TestHandlePermissionRequest_ShouldRecordSourcePi_WhenPayloadIncludesSourceField` | Integration | `PermissionRequestPayload{Source:"pi"}` POSTed to the live handler → the resulting audit/analytics record has `Source == "pi"`; Claude's unmodified curl body (`Source` omitted) still parses with `Source == ""` (backward compatibility) |

### Dedicated coverage: named risk areas

These call out the plan's explicitly flagged risk areas by name, some of which already appear in the table above under their owning requirement — listed again here together so none is missed during implementation.

| Risk area (plan reference) | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Phase 1 spike go/no-go gate (Story 1.1.2) | *(no automated test — see Manual Verification Steps below)* | — | Manual | Pre-implementation spike confirming whether `{block:true}`/`ctx.ui.confirm()`/throw actually blocks a pi tool call; blocks all of Phase 4 if none do |
| Fail-closed on network error/timeout (ADR-003) | `cmd/ssq-hooks/main_test.go` | `TestSsqApprovalExtensionContent_ShouldSetFetchTimeout_ToApprovalTimeoutPlusMargin` | Unit | Rendered source's `fetch()` timeout value matches `ApprovalHandler.approvalTimeout()` (4 min) + margin, not OpenCode's 8s — asserted on generated source text |
| Fail-closed on network error/timeout (ADR-003), live behavior | *(no automated test — see Manual Verification Steps below)* | — | Manual | Live-verify: kill the stapler-squad server mid-tool-call and confirm pi's extension denies rather than allows on the resulting `fetch()` failure |
| Health tracker: `Unknown` is the initial state | `server/services/pi_extension_health_test.go` | `TestPiExtensionHealthTracker_ShouldReturnUnknown_WhenNoPingHasArrived` | Unit | Fresh tracker, no ping yet → `Unknown`, never defaults to `Loaded` |
| Health tracker: ping flips to `Loaded` | `server/services/pi_extension_health_test.go` | `TestPiExtensionHealthTracker_ShouldTransitionToLoaded_WhenPingArrivesWithinGraceWindow` | Unit | Ping within grace window → `Loaded` |
| Health tracker: no ping flips to `Failed` (Story 4.2.1e) | `server/services/pi_extension_health_test.go` | `TestPiExtensionHealthTracker_ShouldTransitionToFailed_WhenGraceWindowElapsesWithNoPing` | Unit | No ping past grace window → `Failed`, not stuck at `Unknown` forever |
| Health tracker: late ping after `Failed` recovers (Story 4.2.1e) | `server/services/pi_extension_health_test.go` | `TestPiExtensionHealthTracker_ShouldTransitionBackToLoaded_WhenLatePingArrivesAfterFailed` | Unit | A ping arriving after the state already flipped to `Failed` still flips it to `Loaded` — "current truth over worst historical state" per Story 4.2.1e / ux.md's Surface 3 edge-case table |
| Health tracker: server-restart survival via periodic re-ping (Story 4.2.3) | `server/services/pi_extension_health_test.go` | `TestPiExtensionHealthTracker_ShouldRecoverToLoaded_WhenRepingArrivesAfterTrackerReset` | Unit | Tracker recorded `Loaded`, is discarded/recreated (simulating a server restart), one re-ping arrives → `Loaded` again without the pi session itself restarting |
| Health tracker: grace window tolerates one missed re-ping (Story 4.2.3) | `server/services/pi_extension_health_test.go` | `TestPiExtensionHealthTracker_ShouldNotFlapToFailed_WhenOneRepingIntervalIsMissed` | Unit | Grace window ≥ 2× the re-ping interval — one missed re-ping alone does not flip `Loaded` → `Failed` |
| U+2028 JSONL regression (Story 5.1.1 AC) | `session/pi_adapter_test.go` | `TestPiAdapter_ShouldNotSplitLine_WhenLineContainsEmbeddedU2028Character` | Unit | A synthetic JSONL buffer with a literal `U+2028` inside a string value, followed by a real newline-terminated line, produces exactly two events, not three (PITFALL-2 regression) |
| Status-subprocess death detection (Story 5.2.3) | `session/pi_status_source_test.go` | `TestPiStatusSource_ShouldTransitionToStatusUnavailable_WhenSubprocessDiesAndRetriesExhaust` | Unit | Subprocess killed out-of-band, `cmd.Wait()` goroutine observes exit; after N failed relaunch attempts, `CurrentStatus()` returns `detection.StatusUnavailable`, not a frozen stale status |
| Status-subprocess bounded relaunch success (Story 5.2.3) | `session/pi_status_source_test.go` | `TestPiStatusSource_ShouldResumeNormalInferenceAndResetRetryCounter_WhenRelaunchSucceeds` | Unit | Subprocess dies, a relaunch succeeds and emits an event before retries exhaust → normal status inference resumes, retry counter resets |
| Status-subprocess death surfaced distinctly from idle (Story 5.2.3) | `session/instance_status_test.go` | `TestInstanceStatusManager_ShouldSurfaceStatusUnavailable_WhenPiStatusSourceRetriesExhausted` | Integration | `GetStatus()` for an instance whose `PiStatusSource` is confirmed-dead returns a status distinguishable from `StatusIdle`, so the session list never freezes silently |

**Coverage fraction**: 7/7 Success-Metrics/In-Scope requirements have at least one mapped test (100%); 5/5 explicitly-named risk areas (Phase 1 go/no-go, ADR-003 fail-closed, three-state health tracker, U+2028 regression, subprocess death/restart) have dedicated coverage, 2 of those 5 partially manual by design (see below).

---

## UX Acceptance Tests

Tool key: **PW** = Playwright (`tests/e2e/`, per `ui-playwright` skill and `e2e-test-conventions`); **Go** = Go test asserting CLI stdout/stderr or log-line content; **Jest** = component-level test in `web-app/`.

### Surface 1: Program picker entry (4 ACs)

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC1: pi selectable in ≤2 actions | `tests/e2e/pi-session.spec.ts` | `should select pi in two interactions from an open creation panel` | PW | Open Omnibar → expand "Advanced Options" (if collapsed) → open `#omnibar-program` `<select>` → choose "pi" → assert selected value is `"pi"` |
| AC2: label association / AT announcement | `tests/e2e/pi-session.spec.ts` | `should keep the label-htmlFor association when pi is selected` | PW | `page.getByLabel("Program")` resolves to the same `<select id="omnibar-program">`; assert accessible name includes "Program" and current value "pi" |
| AC3: focus order unchanged | `tests/e2e/pi-session.spec.ts` | `should not alter tab order when pi is inserted into the program select` | PW | Tab from the field preceding Program through to the field following it; assert the same sequence of `data-testid`s as before pi was added (snapshot list) |
| AC4: no dead end — reversible selection | `tests/e2e/pi-session.spec.ts` | `should allow reselecting a different program after choosing pi` | PW | Select "pi", then reselect "Claude Code"; assert form state `program === "claude"` with no residual pi-only UI left mounted |

### Surface 2: Session-creation panel capability warning (4 ACs)

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC1: exact alert text on Failed | `tests/e2e/pi-session.spec.ts` | `should show the exact unenforced-tool-calls alert text when health is Failed` | PW | Seed a resumed/duplicated session config with cached health = Failed (via test-mode API seed) → open creation panel with program pi → assert `getByRole("alert")` text equals `"Approval extension not loaded for pi — tool calls will run WITHOUT rule enforcement for this session."` verbatim |
| AC2: `role="alert"`, not passive span | `tests/e2e/pi-session.spec.ts` | `should expose the pi health warning via role=alert not a passive hint` | PW | With health = Failed seeded, assert the warning element's accessible role is `alert` (not `programWarning`'s existing passive span role) |
| AC3: no dead end — Create never blocked | `tests/e2e/pi-session.spec.ts` | `should leave the Create button enabled while the pi health alert is showing` | PW | With the alert visible, assert the Create button is not `disabled` and clicking it proceeds to session creation |
| AC4: contrast ≥4.5:1 in both themes | `tests/e2e/pi-session.spec.ts` (or an Axe-driven check) | `should meet WCAG AA contrast for the pi health alert in light and dark themes` | PW + axe-core | Render the alert under `data-theme="light"` and `data-theme="dark"`; run `AxeBuilder` scoped to the alert element; assert zero color-contrast violations in each theme |

### Surface 3: Session-card health badge (6 ACs)

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC1: never renders Loaded before a signal | `tests/e2e/pi-session.spec.ts` | `should render the health badge as Unknown on first paint of a fresh pi session card` | PW | Create a pi session, immediately snapshot the card before the grace window elapses → assert badge `aria-label` contains "status unknown", never "loaded" |
| AC2: exact `role="img"` + `aria-label` per state | `tests/e2e/pi-session.spec.ts` | `should expose role=img and the exact aria-label text for each health state` | PW | Seed each of Failed/Loaded/Unknown via test-mode API → assert `getByRole("img", { name: /pi approval extension/ })`'s `aria-label` matches the exact string per state (Failed's is plan-pinned verbatim; Loaded/Unknown per this design's proposed wording) |
| AC3: color is not the only signal | `tests/e2e/pi-session.spec.ts` | `should render a distinct text label and icon glyph per health state, independent of color` | PW | For each state, assert the badge's accessible text includes the literal label `"pi"` plus a state-distinguishing icon character/name, and that removing CSS color (`page.emulateMedia({ colorScheme: 'no-preference' })` + forced-colors check) still leaves the label legible |
| AC4: contrast ≥4.5:1, all 3 states × 2 themes | `tests/e2e/pi-session.spec.ts` | `should meet WCAG AA contrast for all three badge states in light and dark themes` | PW + axe-core | 3 states × 2 themes = 6 renders, each scoped through `AxeBuilder`; assert zero color-contrast violations |
| AC5: Failed tooltip names the consequence | `tests/e2e/pi-session.spec.ts` | `should name the unenforced-tool-calls consequence in the Failed badge tooltip/label` | PW | Seed Failed state → assert `aria-label` text contains "tool calls are unenforced" (not just "failed") |
| AC6: keyboard-discoverable without hover | `tests/e2e/pi-session.spec.ts` | `should expose the badge label to screen readers without requiring mouse hover` | PW | Tab focus near the badge (or query via accessibility tree directly, no `hover()` call) → assert the `aria-label` text is available via the accessibility snapshot with no pointer interaction performed |

### Surface 4: Resume behavior (1 AC)

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC1: resume is silent — no dialog, no visible flag, conversation just continues | `tests/e2e/pi-session.spec.ts` | `pi-session.spec.ts › resumes a stopped pi session with no confirmation dialog and prior conversation visible` | PW | Create a pi session, send a message, stop it, reconnect/resume it → assert (a) no dialog/modal appeared during resume, (b) the terminal/conversation view shows the pre-stop message content, matching Claude's existing silent-resume UX |

### Surface 5: `ssq-hooks install pi` CLI output (4 ACs)

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC1: exit 0, prints absolute path, idempotent re-run | `cmd/ssq-hooks/main_test.go` | `TestHandleInstallPi_ShouldExitZeroAndPrintAbsolutePath_WhenRunTwice` | Go | Run `install pi` in a clean temp `$HOME`; assert exit code 0 and stdout contains the absolute extension path; re-run; assert byte-identical stdout |
| AC2: pi not found → stderr message, non-zero exit | `cmd/ssq-hooks/main_test.go` | `TestHandleInstallPi_ShouldExitNonZeroWithPathHint_WhenPiNotOnPath` | Go | Run with `PATH` stripped of `pi`; assert stderr equals `"pi not found on PATH — install pi first (see https://pi.dev/docs)"` and no extension file is written |
| AC3: permission error surfaces the real OS error | `cmd/ssq-hooks/main_test.go` | `TestHandleInstallPi_ShouldPrintUnderlyingOSError_WhenExtensionsDirIsUnwritable` | Go | Point `HOME` at a directory made read-only; assert stderr contains the actual OS permission error text, not a generic "install failed" |
| AC4: never claims "enforcement active" | `cmd/ssq-hooks/main_test.go` | `TestHandleInstallPi_ShouldNeverPrintEnforcementActiveClaim_OnSuccess` | Go | Run a successful install; assert stdout does not contain the substring "enforcement active" (or equivalent overclaim) — wording stays scoped to "installed" |

### Surface 6: Health-ping endpoint / server log (3 ACs)

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC1: one log line per Loaded/Failed transition | `server/services/pi_extension_health_test.go` | `TestPiExtensionHealthTracker_ShouldLogExactlyOneLine_PerLoadedOrFailedTransition` | Go | Drive a tracker through Unknown→Loaded and (separately) Unknown→Failed; capture the log sink; assert exactly one greppable line per transition, tagged with `session_id` |
| AC2: grace window stated in the log line itself | `server/services/pi_extension_health_test.go` | `TestPiExtensionHealthTracker_ShouldIncludeGraceWindowSeconds_InFailedLogLine` | Go | Force a Failed transition; assert the log line's `grace_window_s` field is present and matches the configured constant |
| AC3: no log spam — one line, not one per missed ping | `server/services/pi_extension_health_test.go` | `TestPiExtensionHealthTracker_ShouldNotLogOnEveryMissedPingCheck_OnlyOnStateChange` | Go | Run the grace-window checker across multiple poll ticks with no ping arriving; assert only one Failed-transition log line total, not one per poll tick |

### Cross-surface acceptance criteria (5 ACs)

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| Consistency: Failed wording matches between Surface 2 and Surface 3 | `tests/e2e/pi-session.spec.ts` | `should use consistent unenforced-tool-calls wording between the creation-panel alert and the card badge` | PW | Seed Failed state; capture both the Surface 2 alert text and the Surface 3 badge `aria-label`; assert both contain the same "tool calls … unenforced" phrasing |
| No dead ends anywhere | `tests/e2e/pi-session.spec.ts` | `should provide a stated exit path for every pi-specific error/warning state` | PW | Table-driven over {PATH-missing warning, extension-Failed alert, badge-Failed state}; for each, assert an actionable next step is visibly present (install link, switch-program control, or Create button remaining enabled) |
| Opt-in invisibility: flag off = zero diff | `tests/e2e/pi-session.spec.ts` | `should render none of the pi-specific UI when the pi-support flag is off` | PW | With the flag off, assert: no "pi" option in the program picker's rendered DOM is reachable pre-flag *(N/A if pi always lists — see note)*, no capability-warning region mounted, no health badge mounted on any session card |
| Accessibility floor: no new bare icon-only controls | `tests/e2e/pi-session.spec.ts` + axe-core | `should have zero Axe Core violations for icon-only or unlabeled controls across all pi surfaces` | PW + axe-core | Run `AxeBuilder` over the full creation panel and a session card with a pi session present; assert zero violations in the "aria-*"/"button-name" rule categories |
| CI backstop: Axe + Lighthouse already gate this | *(no new test — process check)* | — | CI config review | Confirm `web-app/src/` CI already runs Axe Core (blocking) and Lighthouse CI (warn <70) per top-level `CLAUDE.md`; no new pipeline step needed, just confirm the new files fall under the existing glob |

**UX acceptance tests count**: 27 (4 + 4 + 6 + 1 + 4 + 3 + 5, per the 6-surface inventory plus the cross-surface acceptance-criteria section).

---

## Manual Verification Steps (pre-implementation / non-automatable)

These are explicitly **not** automated tests — they are the Phase 1 spike's documented go/no-go findings that later automated tests are written *against*, per the plan's "do not hand-code from docs alone" instruction (PITFALL-3).

| Step | Plan Reference | What it confirms | Pass/fail record location |
|---|---|---|---|
| Capture a real `pi --mode json` transcript | Story 1.1.1 | Actual event type names/fields exist as documented, or don't | `session/detection/testdata/pi/basic_session.jsonl` + a comment/NOTES.md noting the observed idle gap |
| Probe `tool_call`/`ctx.ui.confirm()`/throw for actual blocking | Story 1.1.2 | **Go/no-go gate**: whether any mechanism blocks a pi tool call at all — if none do, Phase 4 does not start until escalated to the user | A recorded note (per Task 1.1.2b) feeding `ssqApprovalExtensionTemplate`'s implementation comment |
| Probe an unexpected uncaught exception's default behavior | Task 1.1.2c | Whether pi defaults to allow or block on a bug (not a deliberate deny) — required input to ADR-003's residual-risk note and the try/catch requirement | Same note as above |
| Confirm pi's resume flag and session-ID format | Story 1.1.3 | `--session <id>` vs `-c`; UUID vs opaque token — blocks Story 2.2.1's implementation | Recorded finding feeding `buildPiCommand` |
| Confirm global-extension trust-gate exemption | Story 1.1.4 | Whether `~/.pi/agent/extensions/*.ts` loads without a trust prompt on a brand-new directory (ADR-002's premise) | Recorded finding; if refuted, ADR-002 must be revisited before Phase 4 |
| Live-verify the rendered (non-probe) extension loads and a real deny blocks | Task 4.1.1d | The actual generated template — not the throwaway probe — behaves per the confirmed contract | Comment near `ssqApprovalExtensionTemplate` in `cmd/ssq-hooks/main.go` |
| Live-verify fail-closed on a real network/server-down condition (ADR-003) | ADR-003's Consequences section | Killing the stapler-squad server mid-tool-call causes the extension to deny, not allow, matching the documented trade-off | Recorded alongside Task 4.1.1d's verification note |

---

## Test Stack
- **Unit**: Go `testing` package, table-driven (repo convention); Jest for `web-app/src/` TypeScript units.
- **Integration**: Go `testing` package exercising real cross-component wiring (live `RulesService`, real subprocess-backed `PiStatusSource`, real `InstanceStatusManager`/`PiExtensionHealthTracker`) — no separate integration framework; distinguished from unit tests by "crosses a component boundary or spawns a real process/file," per this repo's existing `session/services` test conventions.
- **E2E / UX**: Playwright + Allure (`tests/e2e/`), per the `ui-playwright` skill and `e2e-test-conventions` skill — feature-annotated spec headers, `data-testid`/ARIA-only locators, no `waitForTimeout`, page helpers under `tests/e2e/pages/`.

## Coverage Targets and How to Measure
| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |
