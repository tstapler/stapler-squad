# Validation Plan: session-history-metadata

**Date**: 2026-06-22
**Feature**: Async JSONL artifact extraction — PR links, commit SHAs, external URLs in a new Artifacts tab
**Plan Reference**: `project_plans/session-history-metadata/implementation/plan.md`

---

## Test Stack

- **Unit (Go)**: `go test ./session/artifacts/...` with `testify/assert`; mock storeFn/readFn/lookupTitle via injected closures
- **Integration (Go)**: `go test ./session/...` with SQLite in-process (same ent setup as existing storage tests)
- **Frontend Unit**: Jest + React Testing Library (`cd web-app && npx jest --no-coverage`)
- **E2E**: Playwright against `http://localhost:8544`

---

## Requirement → Test Mapping

| # | Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|---|
| R1 | PR links displayed in session detail | `session/artifacts/extractor_test.go` | `ExtractFromToolResult_should_extractPRURL_When_toolResultContainsPRLink` | Unit happy | tool_result text with a valid GitHub PR URL |
| R2 | PR links displayed in session detail | `session/artifacts/extractor_test.go` | `ExtractFromToolResult_should_returnNoPRURLs_When_textContainsNoGitHubPRPattern` | Unit error | plain text with no GitHub PR URL |
| R3 | PR links displayed in session detail | `session/artifacts/store_test.go` | `ScanFile_should_persistPRURLs_When_jsonlContainsToolResultWithPRLink` | Integration | SQLite in-process; write JSONL, scan, assert DB row updated |
| R4 | Commit SHAs displayed in session detail | `session/artifacts/extractor_test.go` | `ExtractFromToolResult_should_extractCommitSHA_When_toolResultContains40HexString` | Unit happy | tool_result text with a 40-char hex SHA |
| R5 | Commit SHAs displayed in session detail | `session/artifacts/extractor_test.go` | `ExtractFromToolResult_should_notExtractShortHex_When_SHAIsFewerThan40Chars` | Unit error | 39-char hex string must not match |
| R6 | External URLs displayed in session detail | `session/artifacts/extractor_test.go` | `ExtractFromToolResult_should_extractExternalURLs_When_toolResultContainsHTTPSLinks` | Unit happy | tool_result text with multiple https:// URLs |
| R7 | External URLs capped at 50 | `session/artifacts/extractor_test.go` | `ExtractFromToolResult_should_cap50ExternalURLs_When_toolResultContainsMoreThan50URLs` | Unit error | 60 distinct URLs in input; expect len == 50 after cap |
| R8 | Extraction runs automatically in background | `session/artifacts/store_test.go` | `OnHistoryFileChanged_should_enqueueFile_When_fileIsValidJSONL` | Unit happy | call `OnHistoryFileChanged` with valid path; assert queue receives it |
| R9 | Extraction skips non-JSONL files | `session/artifacts/store_test.go` | `OnHistoryFileChanged_should_dropFile_When_fileExtensionIsNotJSONL` | Unit error | `.log` file path; assert queue not written |
| R10 | Extraction skips agent-*.jsonl files | `session/artifacts/store_test.go` | `OnHistoryFileChanged_should_dropFile_When_filenameHasAgentPrefix` | Unit error | `agent-abc123.jsonl`; assert queue not written |
| R11 | Extraction adds < 100 ms to session load (incremental) | `session/artifacts/store_test.go` | `ScanFile_should_readOnlyNewBytes_When_fileIsAppendedAfterFirstScan` | Unit happy | write 2-line JSONL, scan, record offset; append 2 more lines, scan again; assert second `storeFn` call arg offset == file size |
| R12 | Must not re-read unchanged JSONL files | `session/artifacts/store_test.go` | `ScanFile_should_notCallStoreFn_When_noNewBytesAppended` | Unit error | scan once; scan again without appending; assert `storeFn` called only once |
| R13 | Metadata persists across restarts | `session/artifacts/store_test.go` | `SeedOffsets_should_restoreByteOffset_When_blobExistsInDB` | Integration | write blob with non-zero `ScanOffsetBytes` to DB; call `SeedOffsets`; assert `offsets[path] == blob.ScanOffsetBytes` |
| R14 | Metadata persists across restarts | `session/artifacts/store_test.go` | `MergeAndPersist_should_preserveExistingPRURLs_When_newScanAddsMoreURLs` | Unit happy | readFn returns existing blob with 1 PR; new scan finds 1 more; merged blob has both |
| R15 | Gracefully handle missing JSONL files | `session/artifacts/store_test.go` | `ScanFile_should_returnGracefully_When_fileDoesNotExist` | Unit error | path `/tmp/nonexistent.jsonl`; assert no panic, `storeFn` not called |
| R16 | Gracefully handle unreadable JSONL files | `session/artifacts/store_test.go` | `ScanFile_should_returnGracefully_When_scannerErrorOccurs` | Unit error | mock scanner with injected error; assert offset not advanced |
| R17 | Partial last line does not corrupt scan | `session/artifacts/store_test.go` | `ScanFile_should_notAdvanceOffsetPastPartialLine_When_lastLineIsIncompleteJSON` | Unit error | JSONL file whose last byte is truncated mid-JSON; scan; assert offset == end of last complete line |
| R18 | PR URL feeds PR status poller | `session/artifacts/store_test.go` | `OnScanComplete_should_updateInstancePRNumber_When_newPRURLDiscoveredAndNoPRNumberSet` | Unit happy | `OnScanComplete` callback invoked with PR URL; mock `UpdateInstancePRNumber`; assert called with parsed PR number |
| R19 | PR URL feeds PR status poller — no duplicate update | `session/artifacts/store_test.go` | `OnScanComplete_should_notUpdatePRNumber_When_instanceAlreadyHasPRNumber` | Unit error | `inst.GitHubPRNumber != 0`; `OnScanComplete` fires; assert `UpdateInstancePRNumber` NOT called |
| R20 | Command artifacts extracted from bash tool_use | `session/artifacts/extractor_test.go` | `ExtractFromBashCommand_should_extractGHPRCreate_When_commandContainsTitleFlag` | Unit happy | `gh pr create --title "feat: foo"` → `{Type: "gh_pr_create", Detail: "feat: foo"}` |
| R21 | Command artifacts — git commit message | `session/artifacts/extractor_test.go` | `ExtractFromBashCommand_should_extractGitCommitMessage_When_commandUsesMinusM` | Unit happy | `git commit -m "fix: bar"` → `{Type: "git_commit", Detail: "fix: bar"}` |
| R22 | Command artifacts — gh pr merge | `session/artifacts/extractor_test.go` | `ExtractFromBashCommand_should_extractPRNumber_When_commandIsMergePR` | Unit happy | `gh pr merge 42 --squash` → `{Type: "gh_pr_merge", Detail: "42"}` |
| R23 | Command artifacts — npm install | `session/artifacts/extractor_test.go` | `ExtractFromBashCommand_should_extractPackageName_When_commandIsNPMInstall` | Unit happy | `npm install react-query` → `{Type: "package_install", Detail: "react-query"}` |
| R24 | Command artifacts — no match | `session/artifacts/extractor_test.go` | `ExtractFromBashCommand_should_returnNil_When_commandMatchesNoPattern` | Unit error | `ls -la` → nil |
| R25 | Deduplication of PR URLs | `session/artifacts/extractor_test.go` | `ExtractFromToolResult_should_deduplicatePRURLs_When_sameURLAppearsMultipleTimes` | Unit happy | same URL 5× in text → single entry in prURLs |
| R26 | PR URLs not also listed as external URLs | `session/artifacts/extractor_test.go` | `ExtractFromToolResult_should_excludePRURLsFromExternalURLs_When_overlap` | Unit happy | single PR URL in text; assert `externalURLs` does not contain the PR URL |
| R27 | Storage: UpdateInstanceArtifacts persists blob | `session/storage_test.go` | `UpdateInstanceArtifacts_should_persistBlob_When_sessionExists` | Integration | insert session; call `UpdateInstanceArtifacts`; read back; assert blob matches |
| R28 | Storage: UpdateInstanceArtifacts not found | `session/storage_test.go` | `UpdateInstanceArtifacts_should_returnError_When_sessionTitleNotFound` | Integration | nonexistent title; assert non-nil error |
| R29 | Storage: GetInstanceArtifacts returns empty for new session | `session/storage_test.go` | `GetInstanceArtifacts_should_returnEmptyString_When_noArtifactsStored` | Integration | insert session with no artifacts; assert `("", nil)` |
| R30 | FindInstanceByHistoryPath helper | `session/artifact_lookup_test.go` | `FindInstanceByHistoryPath_should_returnTitle_When_instanceMatchesPath` | Unit happy | slice with one instance whose `HistoryFilePath` matches; assert title returned |
| R31 | FindInstanceByHistoryPath — not found | `session/artifact_lookup_test.go` | `FindInstanceByHistoryPath_should_returnFalse_When_NoInstanceMatchesPath` | Unit error | no instance matches; assert `("", false)` |
| R32 | Frontend: extraction pending state | `web-app/src/components/sessions/__tests__/ArtifactsTab.test.tsx` | `ArtifactsTab_should_showExtractionPending_When_artifactsIsUndefined` | Frontend unit | `session.artifacts === undefined` renders "Extraction pending" |
| R33 | Frontend: empty artifacts state | `web-app/src/components/sessions/__tests__/ArtifactsTab.test.tsx` | `ArtifactsTab_should_showNoArtifactsFound_When_allArraysAreEmpty` | Frontend unit | `{prUrls:[], commitShas:[], externalUrls:[]}` renders "No artifacts found" |
| R34 | Frontend: PR links rendered as owner/repo#N | `web-app/src/components/sessions/__tests__/ArtifactsTab.test.tsx` | `ArtifactsTab_should_renderPRLinks_When_artifactsHasPRURLs` | Frontend unit | `prUrls: ["https://github.com/owner/repo/pull/42"]` → `owner/repo#42` in DOM |
| R35 | Frontend: PR links open in new tab | `web-app/src/components/sessions/__tests__/ArtifactsTab.test.tsx` | `ArtifactsTab_should_openPRInNewTab_When_PRLinkClicked` | Frontend unit | `<a>` has `target="_blank"` and `rel="noopener noreferrer"` |
| R36 | Frontend: commit SHAs shortened to 7 chars | `web-app/src/components/sessions/__tests__/ArtifactsTab.test.tsx` | `ArtifactsTab_should_renderSevenCharSHA_When_commitSHAsPresent` | Frontend unit | 40-char SHA → rendered as 7-char monospace code element |
| R37 | Frontend: external URLs collapsed by default | `web-app/src/components/sessions/__tests__/ArtifactsTab.test.tsx` | `ArtifactsTab_should_collapseExternalURLs_When_initialRender` | Frontend unit | URLs present but not visible until toggle clicked |
| R38 | Frontend: external URLs expand on toggle | `web-app/src/components/sessions/__tests__/ArtifactsTab.test.tsx` | `ArtifactsTab_should_expandExternalURLs_When_toggleButtonClicked` | Frontend unit | click "Show N external URLs" → list becomes visible |
| R39 | Frontend: URL truncated to 60 chars | `web-app/src/components/sessions/__tests__/ArtifactsTab.test.tsx` | `ArtifactsTab_should_truncateURLDisplay_When_URLExceeds60Chars` | Frontend unit | URL 80 chars long → display ends with "…" at position 60 |
| R40 | E2E: Artifacts tab visible and clickable | `tests/e2e/session-artifacts.spec.ts` | `T-E2E-ARTIFACTS-001 Artifacts tab visible in session detail` | E2E | navigate to session; click Artifacts tab; assert panel visible |
| R41 | E2E: Artifacts tab shows empty state or content | `tests/e2e/session-artifacts.spec.ts` | `T-E2E-ARTIFACTS-002 Artifacts tab panel renders without error` | E2E | assert no console error after tab click; panel text matches known states |

---

## Test Suite by File

### `session/artifacts/extractor_test.go` — Regex Extractor Unit Tests

```go
// Package: session/artifacts
// Run: go test ./session/artifacts/... -run TestExtract

func TestExtractFromToolResult_should_extractPRURL_When_toolResultContainsPRLink(t *testing.T)
func TestExtractFromToolResult_should_returnNoPRURLs_When_textContainsNoGitHubPRPattern(t *testing.T)
func TestExtractFromToolResult_should_extractCommitSHA_When_toolResultContains40HexString(t *testing.T)
func TestExtractFromToolResult_should_notExtractShortHex_When_SHAIsFewerThan40Chars(t *testing.T)
func TestExtractFromToolResult_should_extractExternalURLs_When_toolResultContainsHTTPSLinks(t *testing.T)
func TestExtractFromToolResult_should_cap50ExternalURLs_When_toolResultContainsMoreThan50URLs(t *testing.T)
func TestExtractFromToolResult_should_deduplicatePRURLs_When_sameURLAppearsMultipleTimes(t *testing.T)
func TestExtractFromToolResult_should_excludePRURLsFromExternalURLs_When_overlap(t *testing.T)
func TestExtractFromBashCommand_should_extractGHPRCreate_When_commandContainsTitleFlag(t *testing.T)
func TestExtractFromBashCommand_should_extractGitCommitMessage_When_commandUsesMinusM(t *testing.T)
func TestExtractFromBashCommand_should_extractPRNumber_When_commandIsMergePR(t *testing.T)
func TestExtractFromBashCommand_should_extractPackageName_When_commandIsNPMInstall(t *testing.T)
func TestExtractFromBashCommand_should_returnNil_When_commandMatchesNoPattern(t *testing.T)
```

**Count**: 13 unit tests

---

### `session/artifacts/store_test.go` — Scanner and Worker Unit Tests

```go
// Package: session/artifacts
// Run: go test ./session/artifacts/... -run TestScanFile|TestOnHistory|TestSeedOffsets|TestMergeAndPersist|TestOnScanComplete

func TestOnHistoryFileChanged_should_enqueueFile_When_fileIsValidJSONL(t *testing.T)
func TestOnHistoryFileChanged_should_dropFile_When_fileExtensionIsNotJSONL(t *testing.T)
func TestOnHistoryFileChanged_should_dropFile_When_filenameHasAgentPrefix(t *testing.T)
func TestScanFile_should_readOnlyNewBytes_When_fileIsAppendedAfterFirstScan(t *testing.T)
func TestScanFile_should_notCallStoreFn_When_noNewBytesAppended(t *testing.T)
func TestScanFile_should_returnGracefully_When_fileDoesNotExist(t *testing.T)
func TestScanFile_should_returnGracefully_When_scannerErrorOccurs(t *testing.T)
func TestScanFile_should_notAdvanceOffsetPastPartialLine_When_lastLineIsIncompleteJSON(t *testing.T)
func TestScanFile_should_persistPRURLs_When_jsonlContainsToolResultWithPRLink(t *testing.T)
func TestMergeAndPersist_should_preserveExistingPRURLs_When_newScanAddsMoreURLs(t *testing.T)
func TestSeedOffsets_should_restoreByteOffset_When_blobExistsInDB(t *testing.T)
func TestOnScanComplete_should_updateInstancePRNumber_When_newPRURLDiscoveredAndNoPRNumberSet(t *testing.T)
func TestOnScanComplete_should_notUpdatePRNumber_When_instanceAlreadyHasPRNumber(t *testing.T)
```

**Count**: 13 unit tests (10 unit, 3 integration-style using real temp files)

---

### `session/storage_test.go` — Storage Integration Tests

```go
// Package: session
// Run: go test ./session/... -run TestUpdateInstanceArtifacts|TestGetInstanceArtifacts

func TestUpdateInstanceArtifacts_should_persistBlob_When_sessionExists(t *testing.T)
func TestUpdateInstanceArtifacts_should_returnError_When_sessionTitleNotFound(t *testing.T)
func TestGetInstanceArtifacts_should_returnEmptyString_When_noArtifactsStored(t *testing.T)
```

**Count**: 3 integration tests (SQLite in-process via ent test setup)

---

### `session/artifact_lookup_test.go` — FindInstanceByHistoryPath Unit Tests

```go
// Package: session
// Run: go test ./session/... -run TestFindInstanceByHistoryPath

func TestFindInstanceByHistoryPath_should_returnTitle_When_instanceMatchesPath(t *testing.T)
func TestFindInstanceByHistoryPath_should_returnFalse_When_NoInstanceMatchesPath(t *testing.T)
```

**Count**: 2 unit tests

---

### `web-app/src/components/sessions/__tests__/ArtifactsTab.test.tsx` — Frontend Unit Tests

```tsx
// Run: cd web-app && npx jest --no-coverage --testPathPatterns="ArtifactsTab"

describe("ArtifactsTab", () => {
  it("ArtifactsTab_should_showExtractionPending_When_artifactsIsUndefined")
  it("ArtifactsTab_should_showNoArtifactsFound_When_allArraysAreEmpty")
  it("ArtifactsTab_should_renderPRLinks_When_artifactsHasPRURLs")
  it("ArtifactsTab_should_openPRInNewTab_When_PRLinkClicked")
  it("ArtifactsTab_should_renderSevenCharSHA_When_commitSHAsPresent")
  it("ArtifactsTab_should_collapseExternalURLs_When_initialRender")
  it("ArtifactsTab_should_expandExternalURLs_When_toggleButtonClicked")
  it("ArtifactsTab_should_truncateURLDisplay_When_URLExceeds60Chars")
})
```

**Count**: 8 frontend unit tests

---

### `tests/e2e/session-artifacts.spec.ts` — E2E Smoke Tests

```ts
// Run: cd tests/e2e && npx playwright test session-artifacts.spec.ts

test.describe("session-artifacts", () => {
  test("T-E2E-ARTIFACTS-001 Artifacts tab visible in session detail")
  test("T-E2E-ARTIFACTS-002 Artifacts tab panel renders without error")
})
```

**Count**: 2 E2E tests

---

## Coverage Targets

| Layer | Target | Notes |
|---|---|---|
| `session/artifacts/` Go unit | ≥ 80% line coverage | All regex paths, enqueue/skip logic, scan loop, merge, seed offsets |
| `session/storage.go` (new methods) | 100% line coverage | 3 integration tests cover both success and error paths |
| `session/artifact_lookup.go` | 100% line coverage | 2 unit tests; trivial helper |
| `ArtifactsTab.tsx` | ≥ 90% statement coverage | 8 frontend tests; all conditional render paths covered |
| E2E smoke | 1 user flow | Tab navigation + panel visible |

---

## Requirements Coverage Fraction

**41 test cases covering 9 functional requirements**:

| Requirement | Tests | Covered |
|---|---|---|
| Session detail displays PR links, commit SHAs, external URLs | R1–R7, R25–R26, R34–R36 | Yes |
| Extraction runs automatically in background | R8–R10 | Yes |
| Metadata persists across app restarts | R13–R14, R27–R29 | Yes |
| Extraction adds < 100 ms (incremental scan) | R11–R12 | Yes (functional correctness; perf verified by benchmarks separately) |
| PR URL feeds existing PR status poller | R18–R19 | Yes |
| Command artifacts extracted from bash tool_use | R20–R24 | Yes |
| Must not block main request path (async) | R8 (enqueue only) | Structural (tested via non-blocking enqueue) |
| Gracefully handle missing/unreadable JSONL files | R15–R16 | Yes |
| Must not re-read unchanged JSONL files | R11–R12, R17 | Yes |

**Coverage fraction**: 9 / 9 functional requirements covered = **100%**

**Test case totals**:

| Type | Count |
|---|---|
| Go unit tests | 26 |
| Go integration tests (SQLite in-process) | 4 |
| Frontend unit tests (Jest/RTL) | 8 |
| E2E tests (Playwright) | 2 |
| **Total** | **40** |

> Note: R3 (`ScanFile_should_persistPRURLs_When_jsonlContainsToolResultWithPRLink`) uses a real temp file but a mock storeFn/readFn, so it is classified as unit rather than integration. The 4 integration tests are those that touch the ent SQLite in-process DB directly.
