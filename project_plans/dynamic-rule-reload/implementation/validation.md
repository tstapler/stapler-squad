# Validation Plan: dynamic-rule-reload

**Date**: 2026-08-06

## Happy Path Scenario

Given a running stapler-squad server whose classifier already has claude-settings
rules loaded from `~/.claude/settings.json` at startup, when the operator hand-edits
that file to add a new `permissions.allow` pattern and either waits for the
`ClaudeSettingsWatcher` debounce window to fire or clicks "Reload rules" in the
`ApprovalRulesPanel`'s Claude Settings tab, then the new rule appears in
`classifier.Rules()` / `ListApprovalRules` and is honored by the next `Classify()`
call, with no process restart, a log line, and a success toast confirming the reload.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-1: `LoadClaudeSettingsRules()` output merged into live classifier at startup | `server/services/claude_settings_parser_test.go` | `TestLoadClaudeSettingsRulesDetailed_AllPathsValid_ReturnsPerPathRules` | Unit | Happy path — all settings paths parse; per-path `ClaudeSettingsPathResult` slice returned with populated `Rules`, nil `Err` |
| REQ-1 | `server/services/claude_settings_parser_test.go` | `TestLoadClaudeSettingsRulesDetailed_AllPathsMissingOrUnreadable_ReturnsEmptyResultsNoError` | Unit (error/edge path) | No `~/.claude/settings.json` exists (fresh machine) — function returns empty `Rules` per path, not an error, and startup wiring doesn't panic on a nil/empty slice |
| REQ-1 | `server/services/session_service_test.go` | `TestNewSessionService_LoadsClaudeSettingsRulesAtStartup` | Integration | Real temp `HOME`-style settings dir + real `NewSessionService` construction; asserts `svc.GetClassifier().Rules()` contains a `Source == classifier.SourceClaudeSettings` entry — exercises the actual wiring bug this project fixes (previously zero call sites) |
| REQ-2: edits to settings.json reflected within bounded time, no restart | `server/services/claude_settings_watcher_test.go` | `TestClaudeSettingsWatcher_Reload_ValidPath_UpdatesLastGoodAndReturnsRuleCount` | Unit | Happy path — `Reload(ctx)` on a valid file returns the correct `ruleCount` and populates `lastGood[path]` |
| REQ-2 | `server/services/claude_settings_watcher_test.go` | `TestClaudeSettingsWatcher_Start_DebouncesRapidWrites` | Unit (edge path) | 5 writes to the watched file within 50ms → `onReload` invoked once, not 5 times (spy channel) |
| REQ-2 | `server/services/claude_settings_watcher_test.go` | `TestClaudeSettingsWatcher_Start_DetectsExternalFileEdit_ReloadsWithoutRestart` | Integration | Real `fsnotify.Watcher` against a real `t.TempDir()` file: write, then atomic-rename-save edit (mimics an editor save), assert `onReload` fires within the 250ms debounce window + one reload cycle and the process never restarts (single long-lived goroutine) |
| REQ-3: reload is atomic — in-flight classification never sees a mixed rule set | `server/services/rules_service_test.go` | `TestRebuildClaudeSettingsRules_ReplaceRules_SwapsSliceUnderClassifierLock` | Unit | Happy path — `rebuildClaudeSettingsRules` calls `classifier.ReplaceRules`, verified via a spy/stub classifier that the full new slice is swapped in one call, not incrementally |
| REQ-3 | `server/services/rules_service_test.go` | `TestClassify_DuringConcurrentReload_NeverObservesPartialRuleSet` | Unit (error/edge path — race path) | A `Classify()` goroutine holding the classifier's `RLock` overlaps with a `rebuildClaudeSettingsRules` call waiting on `Lock()`; asserts the classification result reflects exactly one full snapshot (old or new), run under `-race` |
| REQ-3 | `server/services/rules_service_test.go` | `TestRulesService_ConcurrentClassifyAndClaudeSettingsReload_NoRaceUnderLoad` | Integration | N goroutines calling `Classify()` concurrently with M goroutines triggering `rebuildClaudeSettingsRules`, run under `go test -race`, asserting zero race reports and no panic |
| REQ-4: manual reload trigger (UI button + RPC) works with server running | `server/services/rules_service_test.go` | `TestReloadClaudeSettingsRules_ValidSettings_ReturnsSuccessAndRuleCount` | Unit | Happy path — RPC handler returns `success=true`, correct `rule_count`, and the documented message format |
| REQ-4 | `server/services/rules_service_test.go` | `TestReloadClaudeSettingsRules_WatcherNotConfigured_ReturnsUnimplemented` | Unit (error path) | `rs.claudeSettingsWatcher == nil` (e.g. fsnotify unavailable at startup) → `connect.CodeUnimplemented`, not a panic or silent no-op |
| REQ-4 | `server/services/rules_service_test.go` | `TestReloadClaudeSettingsRulesRPC_EndToEnd_ListApprovalRulesReflectsNewRule` | Integration | Full path: edit a temp settings file → call `ReloadClaudeSettingsRules` RPC → call `ListApprovalRules({source_filter:"claude-settings"})` → new rule is present, proving the RPC-to-store-to-list round trip, not just the handler in isolation |
| REQ-5: reload events visible (log line; toast if a surface exists) | `server/services/claude_settings_watcher_test.go` | `TestClaudeSettingsWatcher_Reload_OnReloadCallbackInvokedWithRuleCountAndOrigin` | Unit | Happy path — `onReload(rules, origin)` callback fires with correct count/origin args on a successful reload |
| REQ-5 | `server/services/rules_service_test.go` | `TestReloadClaudeSettingsRules_FailedPath_MessageNamesFailedPathAndSafetyGuarantee` | Unit (error path) | Malformed file → response `message` contains the failed path reference and "previous rules still active" (not a bare "failed"), matching the observability contract |
| REQ-5 | `web-app/src/components/sessions/ApprovalRulesPanel.test.tsx` | `handleReloadClaudeSettings_should_ShowSuccessToast_When_ReloadSucceeds` | Integration (Jest/RTL, component + mocked RPC) | Click triggers `reloadClaudeSettingsRules()` → `showActionToast` called with success copy — the visible signal end of the log+toast requirement |
| REQ-6: no regression to DB-backed rule hot-swap path | `server/services/rules_service_test.go` | `TestUpsertApprovalRule_StillHotSwapsClassifierRules_AfterRebuildMuAdded` | Unit | Happy path — regression guard: `UpsertApprovalRule` → `rebuildClassifier()` (now `rebuildMu`-guarded) still results in the new rule being classify-visible immediately, unchanged from pre-project behavior |
| REQ-6 | `server/services/rules_service_test.go` | `TestDeleteApprovalRule_InvalidRuleID_LeavesExistingRulesIntact` | Unit (error path) | Deleting a non-existent rule ID returns an error and does not corrupt/clear the live classifier's existing DB-backed rules (guards against a `rebuildMu` deadlock or partial-clear regression) |
| REQ-6 | `server/services/rules_service_test.go` | `TestRebuildClassifier_ConcurrentWithClaudeSettingsRebuild_NeitherUpdateIsLost` | Integration | The plan's Task 3.1.1d test: goroutine A upserts a DB rule while goroutine B reloads claude-settings rules concurrently; `rebuildMu` ensures final classifier state has both changes, run under `-race` |
| REQ-7: malformed settings.json doesn't crash server or wipe working rules | `server/services/claude_settings_parser_test.go` | `TestLoadClaudeSettingsRulesDetailed_OneMalformedPath_ReturnsErrForThatPathOnly` | Unit | Happy-adjacent path — one valid + one truncated/invalid JSON path; valid path's rules still returned, malformed path's `Err` is set, no panic, no aborted call |
| REQ-7 | `server/services/claude_settings_watcher_test.go` | `TestClaudeSettingsWatcher_Reload_MalformedPath_KeepsLastKnownGood` | Unit (error path) | Valid reload followed by a corrupted rewrite of the same file — second `Reload()` returns unchanged `ruleCount` and a non-empty `failedPaths`, `lastGood` is reused rather than dropped |
| REQ-7 | `server/services/rules_service_test.go` | `TestReloadClaudeSettingsRulesRPC_MalformedPath_ReturnsFailureWithLastKnownGoodCountAndServerStaysUp` | Integration | Full RPC path against a real temp file corrupted mid-test: response `success=false`, `rule_count` equals prior known-good count, and the server process (test harness) does not crash — proves the "no crash" half of REQ-7 end to end, not just the parser unit |

## Regression Coverage for Adversarial-Review BLOCKERs

These are not derived from requirements.md — they were found as review BLOCKERs against
the plan (`implementation/adversarial-review.md`) and require explicit coverage before
implementation starts, since the plan's own Task 4.1.1e/4.2.1b test lists do not exercise
either failure mode.

| Finding | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| BLOCKER 1 — `ClaudeSettingsWatcher.lastGood` map race: `Reload()` is reachable concurrently from the fsnotify debounce-timer goroutine and from multiple simultaneous `ReloadClaudeSettingsRules` RPC calls, with no lock held around the map reads/writes | `server/services/claude_settings_watcher_test.go` | `TestClaudeSettingsWatcher_ConcurrentReloadCalls_NoRace` | Unit (concurrency regression) | Fire N goroutines calling `w.Reload(ctx)` directly (simulating concurrent RPC calls) simultaneously with the debounce-timer path from `Start()`'s event loop, run under `go test -race`; asserts zero `fatal error: concurrent map read and map write` and zero `-race` reports. Must fail against the plan's current Task 4.1.1a/b (no `w.mu.Lock()` in `Reload()`) and pass once `Reload()`'s body is wrapped in `w.mu.Lock()/Unlock()` |
| BLOCKER 2 — project-level and global-level settings paths resolve to the identical file when `cwd == $HOME` (the actual deployed configuration, confirmed via `systemctl --user show stapler-squad -p WorkingDirectory` = `/home/tstapler`), producing duplicated `classifier.Rule` entries and a misleading `origin=mixed` tag for a benign single-file edit | `server/services/claude_settings_parser_test.go` | `TestLoadClaudeSettingsRulesDetailed_ProjectDirEqualsHomeDir_DedupesToSinglePathEntry` | Unit (regression) | Call `LoadClaudeSettingsRulesDetailed(home)` where `projectDir == os.UserHomeDir()` (mirrors the live deployed unit's `WorkingDirectory=$HOME`) against a single real settings file; asserts the returned `[]ClaudeSettingsPathResult` contains exactly one entry for that resolved path, not one `"global"`-labeled and one `"project"`-labeled entry for the same file |
| BLOCKER 2 (origin-tagging half) | `server/services/claude_settings_watcher_test.go` | `TestClaudeSettingsWatcher_Reload_CwdEqualsHome_OriginIsGlobalNotMixed` | Unit (regression) | With `projectDir == home`, editing the single shared settings file and reloading must tag `origin == "global"`, never `"mixed"` — proves the dedup fix (above) also fixes Task 4.2.1a's origin computation, not just the duplicate-rule symptom |
| BLOCKER 2 (end-to-end) | `server/services/session_service_test.go` | `TestNewSessionService_CwdEqualsHome_ClassifierRulesContainNoDuplicateClaudeSettingsRules` | Integration | Constructs `NewSessionService` with a working directory forced equal to the temp `HOME` used for the test's fake settings file (mirrors the live deployed config exactly, not a hypothetical `<projectDir>`); asserts `classifier.Rules()` has no two `Source == "claude-settings"` entries with identical `CommandPattern` + priority pairs originating from the same source file |

## UX Acceptance Tests

Source: `project_plans/dynamic-rule-reload/design/ux.md` §9 (9 UX acceptance criteria across
7 surfaces S1–S7). All are Jest/RTL component tests against `ApprovalRulesPanel.tsx` with a
mocked `useApprovalRules()`/ConnectRPC client and mocked `useNotifications()` — no Playwright
warranted (single-panel, single-button interaction, no cross-page navigation).

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| 1. Task completion in 1 click | `ApprovalRulesPanel.test.tsx` | `ReloadRulesButton_should_TriggerReloadAndUpdateTable_When_ClickedOnce` | Jest/RTL | Render panel with `sourceFilter="claude-settings"` → click "Reload rules" once → assert `reloadClaudeSettingsRules` called and `refresh()`-driven table re-render happens with no further user input |
| 2. Discoverability (no scroll, both viewport widths) | `ApprovalRulesPanel.test.tsx` | `ClaudeSettingsHintRow_should_BeVisibleWithoutScroll_When_ClaudeSettingsTabSelectedOnMobileViewport` | Jest/RTL (jsdom viewport resize) | Render at a mobile width (≤480px) with `claude-settings` tab active → assert the hint row/button is present in the DOM and not inside a `headerButtonsHiddenOnMobile`-classed ancestor |
| 3. In-flight feedback within 100ms | `ApprovalRulesPanel.test.tsx` | `ReloadRulesButton_should_ShowReloadingAndBeDisabled_When_ClickedBeforeRpcResolves` | Jest/RTL (deferred promise) | Click with an unresolved mock promise → assert immediately (before `await`) button text is "Reloading…" and `disabled` is true |
| 4. Success state — toast + table already updated | `ApprovalRulesPanel.test.tsx` | `handleReloadClaudeSettings_should_ShowSuccessToastAndUpdatedTable_When_ReloadSucceeds` | Jest/RTL | Mock RPC resolves `{success:true, ruleCount:4, message:"Reloaded 4 claude-settings rule(s)."}` → assert `showActionToast` called with that exact message/"success" and table reflects `refresh()`'s new data before/at toast render |
| 5. Error state (malformed file) — no raw JSON trace, retry available | `ApprovalRulesPanel.test.tsx` | `handleReloadClaudeSettings_should_ShowSanitizedErrorToastAndReenableButton_When_ReloadReportsFailure` | Jest/RTL | Mock RPC resolves `{success:false, message:"Failed to reload Claude settings rules — previous rules still active (1 path failed to parse)."}` → assert toast shows that message verbatim (no JSON parse detail), button re-enabled |
| 6. Error state (network/timeout) — closes ux.md §7.1 gap | `ApprovalRulesPanel.test.tsx` | `handleReloadClaudeSettings_should_ShowNetworkErrorToastAndReenableButton_When_RpcCallThrows` | Jest/RTL | Mock RPC call rejects (simulated network error) → assert `try/catch` in the handler produces the "Could not reach the server to reload rules. Try again." toast (error variant, 10s), not an unhandled promise rejection, and button re-enables in `finally` |
| 7. No dead ends across 3 sequential outcomes | `ApprovalRulesPanel.test.tsx` | `ReloadRulesButton_should_RemainClickableAfterEachOutcome_When_ClickedThreeTimesWithDifferentResults` | Jest/RTL | Click → success; click → `{success:false}`; click → thrown error; assert button is enabled and clickable after each of the three, no stuck/disabled-forever state |
| 8. Double-click safety — closes ux.md §7.2 gap | `ApprovalRulesPanel.test.tsx` | `ReloadRulesButton_should_FireExactlyOneRpcCall_When_DoubleClickedRapidly` | Jest/RTL | Fire two `click` events synchronously before the mock promise resolves → assert `reloadClaudeSettingsRules` called exactly once (second click is a no-op on the disabled button) |
| 9. Accessibility — keyboard operable, accessible name, ARIA | `ApprovalRulesPanel.test.tsx` | `ReloadRulesButton_should_BeKeyboardOperableWithAccessibleName_When_TabbedToAndActivatedViaEnter` | Jest/RTL (`@testing-library/user-event`) | Tab to the button, assert accessible name is "Reload rules" (or "Reloading…" while disabled and removed from tab order), press Enter → same handler as click fires |

Also covers §7.3 (button styling) as a non-blocking implementation detail — no dedicated test
required; verified visually + by the existing Axe Core CI gate on PRs touching `web-app/src/`
(per `CLAUDE.md`'s "UX analysis CI" section), which enforces color-contrast ≥4.5:1 regardless
of which existing CSS class (`retryButton` vs `refreshButton`) is chosen.

## Test Stack

- **Unit**: Go stdlib `testing` + `testify` (existing repo convention, matches `server/services/rules_service_test.go`)
- **Concurrency regression**: Go stdlib `testing` run under `go test -race` — mandatory for the two BLOCKER tests and REQ-3/REQ-6's concurrent-rebuild tests; a green run without `-race` does not satisfy these rows
- **Integration**: Go, in-process — real `t.TempDir()` settings files, real `fsnotify.Watcher`, real `NewSessionService`/`RulesService` construction (no SQLite involved for claude-settings rules specifically, since that source is file-backed, not DB-backed; the DB-backed regression rows under REQ-6 do exercise the existing SQLite test-DB pattern via `RulesStore`)
- **E2E / UX**: Jest/RTL for `ApprovalRulesPanel.tsx`; Playwright not warranted — this is a single-component, single-button interaction with no navigation or multi-page flow, and the existing Jest/RTL suite already covers the panel's other buttons (Export/Import/Refresh) the same way

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./server/services/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line, with `-race` required (not optional) for `claude_settings_watcher_test.go` and the `rebuildMu` concurrency tests specifically |
| TypeScript/Jest | `cd web-app && npx jest --testPathPatterns="ApprovalRulesPanel|useApprovalRules" --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line |

## Migration Test

N/A — plan.md's Risk Control section states no schema/data migration is introduced ("No
schema/data changes to unwind (see Migration Plan, omitted)"). No migration test required.
