# Validation Plan: alias-settings-manager

**Date**: 2026-06-21

## Happy Path Scenario

Given no aliases exist in `config.json`, when the user opens Settings > General, clicks "New Alias", fills in name `"myproj"` and path `"~/code/myproject"`, and clicks "Save", then the alias row `"myproj"` appears immediately in the list, a success banner reads `Alias "@myproj" saved.`, and `~/.stapler-squad/config.json` contains `{ "name": "myproj", "path": "~/code/myproject" }` in its `aliases` array without requiring a service restart.

---

## Requirement → Test Mapping

| # | Requirement | Test File | Test Name | Type | Scenario |
|---|-------------|-----------|-----------|------|----------|
| R1a | UpsertAlias RPC — empty name returns error | `server/services/defaults_service_test.go` | `TestUpsertAlias_EmptyName` | Unit | Call `UpsertAlias` with `alias.name = ""`; expect `connect.CodeInvalidArgument`, config unchanged |
| R1b | UpsertAlias RPC — invalid name (space) returns error | `server/services/defaults_service_test.go` | `TestUpsertAlias_InvalidName` | Unit | Call `UpsertAlias` with `alias.name = "my project"`; expect `connect.CodeInvalidArgument`, message contains `^[\w-]+$` |
| R1c | UpsertAlias RPC — creates alias that does not exist | `server/services/defaults_service_test.go` | `TestUpsertAlias_CreatesAlias` | Integration | Pre-populate config with zero aliases; call `UpsertAlias` with `{name:"myproj",path:"~/code"}`; assert `config.json` `aliases` array has exactly 1 entry with matching fields |
| R1d | UpsertAlias RPC — updates existing alias in-place (no duplicate) | `server/services/defaults_service_test.go` | `TestUpsertAlias_UpdatesExistingAlias` | Integration | Pre-populate config with `{name:"myproj",description:"old"}`; call `UpsertAlias` with `{name:"myproj",description:"new"}`; assert aliases slice has exactly 1 entry, `description == "new"` |
| R1e | UpsertAlias RPC — case-insensitive collision treated as update | `server/services/defaults_service_test.go` | `TestUpsertAlias_CaseInsensitiveDuplicate` | Unit | Pre-populate with `{name:"MyProj"}`; call `UpsertAlias` with `{name:"myproj"}`; assert exactly 1 alias (overwrite-in-place, no duplicate) |
| R1f | UpsertAlias RPC — nil alias body returns error | `server/services/defaults_service_test.go` | `TestUpsertAlias_NilAlias` | Unit | Call `UpsertAlias` with `req.Msg.Alias == nil`; expect `connect.CodeInvalidArgument` |
| R2a | DeleteAlias RPC — empty name returns error | `server/services/defaults_service_test.go` | `TestDeleteAlias_EmptyName` | Unit | Call `DeleteAlias` with `name = ""`; expect `connect.CodeInvalidArgument`, config unchanged |
| R2b | DeleteAlias RPC — not-found returns CodeNotFound | `server/services/defaults_service_test.go` | `TestDeleteAlias_NotFound` | Unit | Config has 0 aliases; call `DeleteAlias` with `name = "nonexistent"`; expect `connect.CodeNotFound`, config unchanged |
| R2c | DeleteAlias RPC — deletes matching alias, preserves others | `server/services/defaults_service_test.go` | `TestDeleteAlias_DeletesAlias` | Integration | Pre-populate config with 3 aliases; call `DeleteAlias` with name of second; assert exactly 2 entries remain, second is gone |
| R3a | AliasesManager renders alias list on mount | `web-app/src/components/settings/AliasesManager.test.tsx` | `describe("AliasesManager") > it("renders alias list after load")` | Unit | Mock `listAliases` → `{aliases:[sampleAlias]}`; mount component; assert alias name, description, group, path, program all visible |
| R3b | AliasesManager shows empty state when no aliases | `web-app/src/components/settings/AliasesManager.test.tsx` | `describe("AliasesManager") > it("shows empty state when no aliases")` | Unit | Mock `listAliases` → `{aliases:[]}`; mount; assert empty-state message visible, no alias rows |
| R3c | AliasesManager calls upsertAlias on valid save (create) | `web-app/src/components/settings/AliasesManager.test.tsx` | `describe("handleSave") > it("calls upsertAlias with correct payload in create mode")` | Unit | Mount; click "New Alias"; fill name `"newproj"`, path `"~/code"`; click "Save"; assert `mockUpsertAlias` called with `{alias:{name:"newproj",path:"~/code",...}}` |
| R3d | AliasesManager calls upsertAlias on valid save (edit) | `web-app/src/components/settings/AliasesManager.test.tsx` | `describe("handleSave") > it("calls upsertAlias with updated description in edit mode")` | Unit | Mount with `sampleAlias`; click "Edit"; clear description; type "Updated"; click "Save"; assert `mockUpsertAlias` called; name field was disabled |
| R3e | AliasesManager calls deleteAlias on confirmed delete | `web-app/src/components/settings/AliasesManager.test.tsx` | `describe("handleDelete") > it("calls deleteAlias after inline confirmation")` | Unit | Mount with `sampleAlias`; click "Delete" → "Confirm delete?"; click confirm; assert `mockDeleteAlias` called with `{name:"myproj"}` |
| R4a | Form covers all AliasConfig fields — tags add/remove | `web-app/src/components/settings/AliasesManager.test.tsx` | `describe("tag editor") > it("adds and removes tags")` | Unit | Open form; type `"backend"` in tag input; press Enter; chip appears; click Remove; chip disappears |
| R4b | Form covers all AliasConfig fields — env vars add/remove | `web-app/src/components/settings/AliasesManager.test.tsx` | `describe("EnvVarsEditor") > it("adds and removes env var rows")` | Unit | Open form; check "Advanced options"; click "Add Variable"; row appears; fill KEY `"FOO"`, value `"bar"`; click Remove; row disappears |
| R4c | Form covers all AliasConfig fields — env vars included in upsert payload | `web-app/src/components/settings/AliasesManager.test.tsx` | `describe("handleSave") > it("includes envVars in upsertAlias payload")` | Unit | Open form; fill name; check Advanced; add env var `{FOO:bar}`; save; assert `mockUpsertAlias` called with `alias.envVars == {FOO:"bar"}` |
| R4d | Form covers all AliasConfig fields — blank KEY env var rows skipped on save | `web-app/src/components/settings/AliasesManager.test.tsx` | `describe("handleSave") > it("skips blank-key env var rows on save")` | Unit | Open form; add env var row, leave KEY blank, set value "val"; save; assert `mockUpsertAlias` called with `alias.envVars == {}` |
| R5a | Proto generation — UpsertAlias RPC reachable after make generate-proto | `server/services/defaults_service_test.go` | `TestUpsertAlias_CreatesAlias` | Integration | Go compilation itself validates proto bindings; `go build ./...` must exit 0 |
| R5b | Registry — coverage gap count does not grow | CI gate (`make ci`) | N/A — enforced by `make registry-diff` in CI | Integration | After feature branch merges, `coverage-gaps.json` entry count ≤ pre-feature baseline |

---

## Unit Tests: Go — Full List

**File**: `server/services/defaults_service_test.go`

All tests use `STAPLER_SQUAD_TEST_DIR` env injection for config isolation (same pattern as existing defaults tests).

| Test Name | Method Under Test | Scenario |
|-----------|------------------|----------|
| `TestUpsertAlias_NilAlias` | `DefaultsService.UpsertAlias` | `req.Msg.Alias == nil` → `CodeInvalidArgument` |
| `TestUpsertAlias_EmptyName` | `DefaultsService.UpsertAlias` | `alias.name = ""` → `CodeInvalidArgument` |
| `TestUpsertAlias_InvalidName` | `DefaultsService.UpsertAlias` | `alias.name = "my project"` → `CodeInvalidArgument`, message matches `^[\w-]+$` |
| `TestUpsertAlias_CreatesAlias` | `DefaultsService.UpsertAlias` | New alias appended; config slice has exactly 1 entry |
| `TestUpsertAlias_UpdatesExistingAlias` | `DefaultsService.UpsertAlias` | Existing alias replaced in-place; config slice length unchanged; description updated |
| `TestUpsertAlias_CaseInsensitiveDuplicate` | `DefaultsService.UpsertAlias` | `"MyProj"` pre-exists; upsert `"myproj"` → exactly 1 alias (overwrite, no append) |
| `TestDeleteAlias_EmptyName` | `DefaultsService.DeleteAlias` | `name = ""` → `CodeInvalidArgument` |
| `TestDeleteAlias_NotFound` | `DefaultsService.DeleteAlias` | `name = "nonexistent"`, 0 aliases in config → `CodeNotFound`, config unchanged |
| `TestDeleteAlias_DeletesAlias` | `DefaultsService.DeleteAlias` | 3 aliases; delete middle; config has exactly 2 remaining |

Naming convention: `TestMethodName_Condition` where condition describes the input state.

---

## Unit Tests: Jest/RTL — Full List

**File**: `web-app/src/components/settings/AliasesManager.test.tsx`

All RPC calls mocked via `jest.mock("@connectrpc/connect")`. Timer-based tests use `jest.useFakeTimers()`.

| Test Name (`describe > it`) | Scenario |
|-----------------------------|----------|
| `AliasesManager > renders alias list after load` | Mount with mocked aliases; verify name, description, group, path, program visible |
| `AliasesManager > shows empty state when no aliases` | `listAliases` returns `{aliases:[]}`; verify empty-state text, no rows |
| `AliasesManager > shows loading state then list` | Delay `listAliases` promise; verify "Loading…" text; resolve; verify list appears |
| `AliasesManager > shows load error banner` | `listAliases` rejects; verify error banner text starts "Failed to load aliases:" |
| `handleNewAlias > opens form on New Alias click` | Click "New Alias"; form card appears; Name input is focused |
| `handleSave > calls upsertAlias with correct payload in create mode` | Fill name + path; save; verify `mockUpsertAlias` payload |
| `handleSave > calls upsertAlias with updated description in edit mode` | Edit flow; Name input disabled; updated description in payload |
| `handleSave > name validation — empty name shows inline error` | Save with blank name; `nameError` "Name is required." shown; RPC not called |
| `handleSave > name validation — invalid name format shows inline error` | Name = "my project"; error cites regex; RPC not called |
| `handleSave > name validation — conflict in create mode shows inline error` | Name = "myproj" when alias list has "myproj"; conflict message shown; RPC not called |
| `handleSave > name validation — case-insensitive conflict in create mode` | Name = "MYPROJ" when list has "myproj"; collision detected; RPC not called |
| `handleSave > skips uniqueness check in edit mode` | Edit "myproj"; save with same name; no conflict error; RPC called |
| `handleSave > shows success banner after save` | Valid save; banner "Alias \"@newproj\" saved." appears; loadAliases called |
| `handleSave > shows save error banner on RPC failure` | `upsertAlias` rejects; error banner text starts "Failed to save alias:"; form stays open |
| `handleSave > skips blank-key env var rows on save` | Env var row with blank KEY; save; payload `alias.envVars == {}` |
| `handleSave > includes non-blank env vars in payload` | Env var `{FOO:bar}`; save; payload `alias.envVars == {FOO:"bar"}` |
| `handleDelete > shows Confirm delete? button on first Delete click` | Click "Delete" on row; "Confirm delete?" button appears; "Delete" button hidden |
| `handleDelete > calls deleteAlias after inline confirmation` | Click "Delete" → "Confirm delete?"; click confirm; `mockDeleteAlias` called with `{name:"myproj"}` |
| `handleDelete > auto-cancels pending delete after 3 seconds` | Click "Delete"; advance timers 3s; "Delete" button reappears; `mockDeleteAlias` not called |
| `handleDelete > shows success banner after delete` | Confirm delete; RPC resolves; banner "Alias \"@myproj\" deleted." visible |
| `handleDelete > shows delete error banner on RPC failure` | Confirm delete; `deleteAlias` rejects; banner starts "Failed to delete alias:"; row still visible |
| `handleCancel > closes form and resets state on Cancel` | Open form; fill name; cancel; form gone; list unchanged; RPC not called |
| `tag editor > adds tag on Enter key` | Type "backend" in tag input; press Enter; chip "backend" appears; input cleared |
| `tag editor > adds tag on Add button click` | Type "backend"; click "Add"; chip appears |
| `tag editor > ignores duplicate tag` | Tags = ["frontend"]; add "frontend" again; chips = ["frontend"] (no duplicate) |
| `tag editor > removes tag on chip Remove click` | Remove "backend" chip; chip disappears; save payload has empty tags |
| `tag editor > Enter in tag input does not submit form` | Type tag; press Enter; form not submitted; `upsertAlias` not called |
| `EnvVarsEditor > env var row appears after Add Variable click` | Check Advanced; click "Add Variable"; one row visible |
| `EnvVarsEditor > env var key and value update on input change` | Type in KEY input; value in row state matches |
| `EnvVarsEditor > removes env var row on Remove click` | Add 2 rows; Remove first; 1 row remains |
| `Advanced section > hidden by default for new alias` | Open new form; env var editor and CLI flags NOT in DOM |
| `Advanced section > visible after checking Advanced options` | Check checkbox; env var editor and CLI flags appear |
| `Advanced section > collapses but preserves state on uncheck` | Add env var; uncheck Advanced; recheck; env var still present |
| `Advanced section > auto-expanded when editing alias with envVars` | Edit alias that has `envVars = {FOO:"bar"}`; Advanced section expanded; row for FOO visible |
| `Advanced section > auto-expanded when editing alias with cliFlags` | Edit alias that has `cliFlags = "--no-ansi"`; Advanced section expanded; CLI flags input shows value |
| `live preview > shows @name hint updating as name typed` | Type "proj-x" in Name; preview hint shows "@proj-x" |
| `live preview > shows @name placeholder when name empty` | Name field empty; preview hint shows "@name" |
| `edit form > name field is disabled in edit mode` | Click Edit; Name input has `disabled=true` |
| `edit form > form title reads Edit Alias: {name} in edit mode` | Click Edit; form title contains "Edit Alias: myproj" |
| `edit form > form title reads New Alias in create mode` | Click New Alias; form title is "New Alias" |

Total Jest tests: **37**

---

## UX Acceptance Tests

**Tool**: Playwright  
**Base URL**: `http://localhost:8544` (test server port)  
**File**: `tests/e2e/alias-settings.spec.ts`

Naming convention: `test.describe("alias-settings") > test("<AC-label> description")`

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC-01 (create alias in 4 steps) | `alias-settings.spec.ts` | `alias-settings > AC-01 create alias with name and path only` | Playwright | Navigate to Settings > General; click "New Alias"; fill name `"e2e-create"`; fill path `"~/code/e2e"`; click "Save"; assert `data-testid="alias-row-e2e-create"` visible |
| AC-02 (edit alias description in 3 steps) | `alias-settings.spec.ts` | `alias-settings > AC-02 edit alias description` | Playwright | Pre-create alias; click "Edit"; clear + fill Description "Updated E2E"; click "Save"; assert row contains "Updated E2E" |
| AC-03 (delete alias via inline confirmation) | `alias-settings.spec.ts` | `alias-settings > AC-03 delete alias via inline confirmation` | Playwright | Pre-create alias; click `data-testid="alias-delete-e2e-create"`; assert `data-testid="alias-confirm-delete-e2e-create"` visible; click confirm; assert row not visible |
| AC-04 (add env var in ≤5 steps) | `alias-settings.spec.ts` | `alias-settings > AC-04 add environment variable via Advanced section` | Playwright | Open new form; check "Advanced options"; click "Add Variable"; fill KEY `"E2E_VAR"`, value `"hello"`; click "Save"; re-edit alias; assert env var row shows `E2E_VAR` / `hello` |
| AC-05 (add tag in ≤3 steps) | `alias-settings.spec.ts` | `alias-settings > AC-05 add tag via tag input Enter key` | Playwright | Open new form; fill name; click tag input; type "e2e-tag"; press Enter; assert chip "e2e-tag" visible without clicking Save |
| AC-06 (cancel preserves no data) | `alias-settings.spec.ts` | `alias-settings > AC-06 cancel does not persist data` | Playwright | Click "New Alias"; fill name `"should-not-exist"`; click "Cancel"; assert alias row for `"should-not-exist"` not present; verify via `listAliases` or absence of row |
| AC-07 (empty state visible when no aliases) | `alias-settings.spec.ts` | `alias-settings > AC-07 empty state shows @name hint text` | Playwright | Start with clean instance (no aliases); load Settings > General; assert text "No aliases configured" visible; assert "New Alias" button visible and enabled |
| AC-08 (empty state hidden when aliases exist) | `alias-settings.spec.ts` | `alias-settings > AC-08 empty state hidden when aliases present` | Playwright | Pre-create alias; load Settings > General; assert "No aliases configured" text NOT visible |
| AC-09 (inline error — empty name) | `alias-settings.spec.ts` | `alias-settings > AC-09 save with empty name shows inline error` | Playwright | Open new form; click "Save" without filling Name; assert "Name is required." visible below Name input; assert RPC not called (check list unchanged) |
| AC-10 (inline error — name format) | `alias-settings.spec.ts` | `alias-settings > AC-10 save with invalid name shows regex error` | Playwright | Open new form; type "my project" in Name; click "Save"; assert text "letters, digits, hyphens, and underscores" visible |
| AC-11 (inline error — name conflict case-insensitive) | `alias-settings.spec.ts` | `alias-settings > AC-11 name conflict case-insensitive shows already-exists error` | Playwright | Pre-create alias "e2e-dup"; open new form; type "E2E-DUP" in Name; click "Save"; assert "already exists" text visible; list count unchanged |
| AC-12 (edit mode no conflict error for own name) | `alias-settings.spec.ts` | `alias-settings > AC-12 edit mode save with own name does not trigger conflict` | Playwright | Pre-create alias "e2e-edit"; click "Edit"; click "Save" without changing name; assert no "already exists" error; success banner appears |
| AC-13 (correcting field clears error) | `alias-settings.spec.ts` | `alias-settings > AC-13 correcting invalid name allows successful save` | Playwright | Trigger empty-name error; type valid name "e2e-corrected"; click "Save"; assert no inline error; success banner appears |
| AC-14 (save failure — form stays open) | `alias-settings.spec.ts` | `alias-settings > AC-14 save failure shows error banner and form stays open` | Playwright (with server intercept or forced error condition) | Open form; fill name; intercept `UpsertAlias` to return 500; click "Save"; assert error banner text starts "Failed to save alias:"; assert form is still visible with filled name intact |
| AC-15 (delete failure — row stays visible) | `alias-settings.spec.ts` | `alias-settings > AC-15 delete failure shows error banner and row remains` | Playwright | Pre-create alias; intercept `DeleteAlias` to return error; trigger delete confirm; assert error banner starts "Failed to delete alias:"; assert alias row still visible |
| AC-16 (load failure — New Alias button accessible) | `alias-settings.spec.ts` | `alias-settings > AC-16 load failure shows error banner and New Alias button stays accessible` | Playwright | Intercept `ListAliases` to return error; load Settings; assert error banner starts "Failed to load aliases:"; assert "New Alias" button enabled |
| AC-17 (error has exit path) | `alias-settings.spec.ts` | `alias-settings > AC-17 inline error clears after correcting and saving successfully` | Playwright | Trigger name-required error; type valid name; click "Save"; assert inline error gone; success banner present |
| AC-18 (success banner on save) | `alias-settings.spec.ts` | `alias-settings > AC-18 success banner appears and auto-dismisses after save` | Playwright | Create alias; assert banner `Alias "@{name}" saved.` visible; wait 3.5s; assert banner no longer visible |
| AC-19 (success banner on delete) | `alias-settings.spec.ts` | `alias-settings > AC-19 success banner appears and auto-dismisses after delete` | Playwright | Delete alias; assert banner `Alias "@{name}" deleted.` visible; wait 3.5s; assert banner gone |
| AC-20 (focus on Name input for new alias) | `alias-settings.spec.ts` | `alias-settings > AC-20 Name input receives focus when New Alias is clicked` | Playwright | Click "New Alias"; assert `document.activeElement` is the Name input (via `page.evaluate`) |
| AC-21 (focus on Description in edit mode) | `alias-settings.spec.ts` | `alias-settings > AC-21 focus moves to Description when Edit is clicked` | Playwright | Click "Edit" on alias row; assert `document.activeElement` is the Description input |
| AC-22 (Name disabled in edit, enabled in create) | `alias-settings.spec.ts` | `alias-settings > AC-22 Name input disabled in edit mode and enabled in create mode` | Playwright | Edit mode: assert Name input has `disabled` attribute. Create mode: assert Name input does NOT have `disabled` attribute |
| AC-23 (form title create vs edit) | `alias-settings.spec.ts` | `alias-settings > AC-23 form title is New Alias in create and Edit Alias: {name} in edit` | Playwright | Create mode: assert heading text "New Alias". Edit mode: assert heading text "Edit Alias: e2e-alias-name" |
| AC-24 (Advanced auto-expand for existing env vars) | `alias-settings.spec.ts` | `alias-settings > AC-24 Advanced section auto-expands when editing alias with env vars` | Playwright | Pre-create alias with envVar; click "Edit"; assert "Advanced options" checkbox is checked and env var row visible without user interaction |
| AC-25 (Advanced toggle preserves state) | `alias-settings.spec.ts` | `alias-settings > AC-25 unchecking Advanced collapses section but preserves entered values` | Playwright | Open form; check Advanced; add env var KEY=`"FOO"`; uncheck Advanced; assert env var editor hidden; re-check; assert FOO row still present |
| AC-26 (live preview updates on field change) | `alias-settings.spec.ts` | `alias-settings > AC-26 omnibar preview block updates live as Name and Path are typed` | Playwright | Open form; type "proj-abc" in Name; assert preview contains "@proj-abc". Type "~/work" in Path; assert preview contains "~/work" |
| AC-27 (previewHint updates with name) | `alias-settings.spec.ts` | `alias-settings > AC-27 previewHint reads @{name} as user types and @name when empty` | Playwright | Open form; Name empty → assert hint contains "@name". Type "abc" → assert hint contains "@abc" |
| AC-28 (preview omits blank fields) | `alias-settings.spec.ts` | `alias-settings > AC-28 preview omits chips for empty path and program fields` | Playwright | Open form; fill name only; assert preview shows "@name" but no blank path chip and no blank program chip |
| AC-29 (keyboard navigation) | `alias-settings.spec.ts` | `alias-settings > AC-29 all interactive elements reachable via keyboard` | Playwright | Tab through "New Alias" button, Name, Description, Group, Path, Profile, Program, Auto-yes, tag input, "Add" button, "Advanced options" checkbox, Save, Cancel; assert each receives focus |
| AC-30 (section aria-labelledby) | `alias-settings.spec.ts` | `alias-settings > AC-30 form card wrapped in section with aria-labelledby pointing to form title` | Playwright | Open form; assert `section[aria-labelledby]` element exists; assert referenced element contains form title text |
| AC-31 (role alert and role status) | `alias-settings.spec.ts` | `alias-settings > AC-31 error banner uses role=alert, success banner uses role=status` | Playwright | Trigger error; assert `role="alert"` on error div. Trigger success; assert `role="status"` on success div |
| AC-32 (aria-labels on tag/env var remove buttons) | `alias-settings.spec.ts` | `alias-settings > AC-32 remove buttons have descriptive aria-labels` | Playwright | Add tag "backend"; assert tag remove button has `aria-label="Remove tag backend"`. Add env var KEY="FOO"; assert remove button has `aria-label` containing "FOO" |

Total UX acceptance tests: **32**

---

## Test Stack

- **Unit (Go)**: `go test ./server/services/...` — tests `DefaultsService.UpsertAlias` and `DefaultsService.DeleteAlias` using `STAPLER_SQUAD_TEST_DIR` config injection; no network
- **Unit (Jest/RTL)**: `cd web-app && npx jest --no-coverage --testPathPatterns="AliasesManager.test"` — tests the React component in jsdom; all RPCs mocked; `jest.useFakeTimers()` for 3-second delete auto-cancel
- **Integration**: The Go tests that write and re-read `config.json` (R1c, R1d, R2c) constitute integration coverage — they exercise the full `UpsertAlias`/`DeleteAlias` handler + `config.SaveConfig` + `config.LoadConfig` chain
- **E2E / UX**: `cd tests/e2e && npx playwright test alias-settings.spec.ts` — runs against live server at `http://localhost:8544`; uses `data-testid` locators; requires `STAPLER_SQUAD_INSTANCE=e2e-local` test server running

---

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go unit + integration | `go test -coverprofile=coverage.out ./server/services/... && go tool cover -func=coverage.out \| grep defaults_service` | ≥ 80% statement coverage on `defaults_service.go` new functions |
| Jest (component) | `cd web-app && npx jest --coverage --testPathPatterns="AliasesManager.test" --coverageReporters=text` | ≥ 80% line coverage on `AliasesManager.tsx` |
| Playwright E2E | Visual pass rate: all 32 `alias-settings.spec.ts` tests green | 100% pass (no skipped tests) |
| Registry coverage gap | `make registry-diff` | Net zero or negative increase in `coverage-gaps.json` entry count |

---

## Test Case Counts by Type

| Type | Count |
|---|---|
| Go unit tests | 9 |
| Jest/RTL unit tests | 37 |
| Playwright E2E/UX acceptance tests | 32 |
| **Total** | **78** |

---

## Requirements Coverage

| In-Scope Requirement | Tests Designed | Covered? |
|---|---|---|
| R1: `UpsertAlias` RPC | R1a, R1b, R1c, R1d, R1e, R1f (6 Go tests) + R3c, R3d, R3e (Jest) + AC-01, AC-02, AC-12, AC-13, AC-18 (E2E) | Yes |
| R2: `DeleteAlias` RPC | R2a, R2b, R2c (3 Go tests) + R3e (Jest) + AC-03, AC-19 (E2E) | Yes |
| R3: `AliasesManager` component list + form | R3a–R3e (Jest) + AC-01 through AC-32 (E2E) | Yes |
| R4: Form fields covering all `AliasConfig` fields | R4a–R4d (Jest) + AC-04, AC-05, AC-24, AC-25, AC-32 (E2E) | Yes |
| R5: Proto generation + registry update | `go build ./...` compilation gate (implicit in all Go tests) + CI `make registry-diff` gate | Yes |

**Requirements coverage fraction**: **5 / 5 (100%)**

---

## Notes on Implementation

1. **Delete pattern**: The UX spec (Surface 6) describes `window.confirm()`. The implementation plan (Story 2.1.4) overrides this with an inline "Confirm delete?" button. All tests in this plan target the inline confirmation pattern. If the implementation reverts to `window.confirm()`, the Jest tests using `jest.spyOn(window, "confirm")` and the E2E tests using `data-testid="alias-confirm-delete-*"` must be updated.

2. **`ALIAS_NAME_RE` shared constant**: Jest tests for name-format validation (rows `handleSave > name validation — invalid name format`) depend on `ALIAS_NAME_RE` being exported from `AliasDetector.ts` and imported by `AliasesManager.tsx`. If this refactor is not done, the regex behavior is still testable but the tests must instantiate the regex inline.

3. **Test isolation**: Each Playwright `test` block must not depend on state from other tests. Use `test.beforeEach` to create the alias under test and `test.afterEach` (or API teardown) to delete it.

4. **Server intercept for failure tests** (AC-14, AC-15, AC-16): Use Playwright `page.route()` to intercept ConnectRPC calls and return synthetic error responses. Alternatively, test these via Jest (mocking the client) and mark the E2E variants as lower-priority if intercept setup is complex.
