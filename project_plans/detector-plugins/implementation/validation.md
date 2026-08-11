# Validation Plan: detector-plugins

**Date**: 2026-08-01

## Happy Path Scenario

Given a running stapler-squad with no `~/.stapler-squad/detectors/` directory
yet, when the user starts stapler-squad (creating the directory and its
example seed file automatically) and then drops a well-formed `my-agent.toml`
into it — declaring `id = "my-agent"`, `binary_names = ["my-agent"]`, and a
`processing` pattern `Thinking\.\.\.` — then within 2 seconds
`DetectForProgram([]byte("Thinking..."), "my-agent")` returns
`StatusProcessing` and `DetectorProvenance()["my-agent"]` points at that file,
with zero restart, zero rebuild, and the five built-in detectors (`claude`,
`gemini`, `aider`, `opencode`, `agy`) continuing to behave exactly as before.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| REQ-1: TOML file detects agent status without rebuild/restart | `session/detection/plugins_test.go` | `parsePluginFile_should_returnDTO_When_fileIsWellFormed` | Unit | Happy path |
| REQ-1: status keys map to `StatusPatterns` fields | `session/detection/plugins_test.go` | `statusField_should_returnFieldPointer_When_validStatusKey` (10 subtests, one per status key) | Unit | Happy path |
| REQ-1: pattern order within a status category is preserved | `session/detection/plugins_test.go` | `toStatusPatterns_should_preserveDeclarationOrder_When_multiplePatternsSameStatus` | Unit | Happy path |
| REQ-1: absent `version` defaults to schema v1 | `session/detection/plugins_test.go` | `validatePluginFile_should_acceptMissingVersion_When_versionAbsent` | Unit | Happy path |
| REQ-1: one file, multiple binary names → one detector each, sharing patterns | `session/detection/plugins_test.go` | `LoadPluginDir_should_returnDetectorPerBinaryName_When_fileClaimsMultipleBinaries` | Integration | Happy path (fs + registry) |
| REQ-1: a valid plugin becomes detectable after a directory scan | `session/detection/detector_snapshot_test.go` | `rebuildSnapshot_should_makePluginDetectable_When_validPluginFileScanned` | Integration | Happy path (fs + registry + snapshot) |
| REQ-1: dropping a file into a watched directory is picked up live | `session/detection/plugin_watcher_test.go` | `PluginWatcher_should_detectAddedFile_When_fileWrittenToWatchedDir` | Integration | Happy path (fs watcher, end-to-end) |
| REQ-2: unknown TOML key is a loud error, not silently dropped | `session/detection/plugins_test.go` | `parsePluginFile_should_returnError_When_unknownKeyPresent` | Unit | Error path |
| REQ-2: `priority` key is rejected (ADR-003) | `session/detection/plugins_test.go` | `parsePluginFile_should_returnError_When_priorityKeyPresent` | Unit | Error path |
| REQ-2: syntactically invalid TOML is rejected | `session/detection/plugins_test.go` | `parsePluginFile_should_returnError_When_TomlSyntaxInvalid` | Unit | Error path |
| REQ-2: empty file is rejected | `session/detection/plugins_test.go` | `parsePluginFile_should_returnError_When_fileEmpty` | Unit | Error path |
| REQ-2: unknown `status` value is rejected with full valid-key list | `session/detection/plugins_test.go` | `statusField_should_returnFalse_When_unknownStatusKey` | Unit | Error path |
| REQ-2: missing `id` is rejected by field name | `session/detection/plugins_test.go` | `validatePluginFile_should_rejectMissingID_When_idFieldAbsent` | Unit | Error path |
| REQ-2: empty `binary_names` is rejected | `session/detection/plugins_test.go` | `validatePluginFile_should_rejectEmptyBinaryNames_When_binaryNamesEmpty` | Unit | Error path |
| REQ-2: non-compiling regex is rejected with the compile error attached | `session/detection/plugins_test.go` | `validatePluginFile_should_rejectInvalidRegex_When_regexFailsToCompile` | Unit | Error path |
| REQ-2: unsupported `version` is rejected (ADR-003) | `session/detection/plugins_test.go` | `validatePluginFile_should_rejectUnsupportedVersion_When_versionIsNot1` | Unit | Error path |
| REQ-2: a file with three separate problems yields three errors | `session/detection/plugins_test.go` | `validatePluginFile_should_returnMultipleErrors_When_fileHasThreeProblems` | Unit | Error path |
| REQ-2 (NFR resource cap): >50 patterns rejected | `session/detection/plugins_test.go` | `validatePluginFile_should_rejectFile_When_patternCountExceeds50` | Unit | Error path |
| REQ-2 (NFR resource cap): regex >4096 bytes rejected | `session/detection/plugins_test.go` | `validatePluginFile_should_rejectPattern_When_regexExceeds4096Bytes` | Unit | Error path |
| REQ-2 (NFR resource cap): file >256 KiB rejected without being read | `session/detection/plugins_test.go` | `LoadPluginDir_should_rejectFile_When_fileSizeExceeds256KiB` | Integration | Error path (fs stat-then-read ordering) |
| REQ-2: valid files load, invalid ones are reported and skipped | `session/detection/plugins_test.go` | `LoadPluginDir_should_loadValidAndSkipInvalid_When_directoryHasBothWithGoodOneNamed` | Integration | Error path (directory scan) |
| REQ-2: two user files sharing an `id` — later filename rejected | `session/detection/plugins_test.go` | `LoadPluginDir_should_rejectLaterFile_When_duplicateIDAcrossFiles` | Integration | Error path (collision) |
| REQ-2: two user files sharing a binary name — later filename rejected | `session/detection/plugins_test.go` | `LoadPluginDir_should_rejectLaterFile_When_duplicateBinaryNameAcrossFiles` | Integration | Error path (collision) |
| REQ-2: collision winners are deterministic across repeated scans | `session/detection/plugins_test.go` | `LoadPluginDir_should_beDeterministic_When_runMultipleTimes` | Integration | Edge case (10-iteration loop) |
| REQ-2: non-`.toml` entries, subdirectories, symlinks are skipped | `session/detection/plugins_test.go` | `LoadPluginDir_should_skipNonTomlEntriesSubdirsAndSymlinks_When_present` | Integration | Edge case |
| REQ-2: a missing plugin directory is not an error | `session/detection/plugins_test.go` | `LoadPluginDir_should_returnEmpty_When_directoryMissing` | Integration | Edge case |
| REQ-2: per-file rejections don't block the rest of a rebuild | `session/detection/detector_snapshot_test.go` | `rebuildSnapshot_should_loadValidAndLogInvalid_When_directoryHasBoth` | Integration | Error path |
| REQ-2: a directory-level scan failure leaves the previous snapshot intact | `session/detection/detector_snapshot_test.go` | `rebuildSnapshot_should_retainPreviousSnapshot_When_directoryScanFails` | Integration | Error path |
| REQ-3: `Upsert` replaces an entry without panicking | `session/detection/registry_test.go` | `TestDetectorRegistry_should_replaceEntry_When_upsertCalledWithExistingName` | Unit | Happy path |
| REQ-3: a plugin overriding a built-in wins; registry does not grow | `session/detection/registry_test.go` | `MergedRegistry_should_overrideBuiltin_When_pluginClaimsSameBinaryName` | Unit | Happy path |
| REQ-3: a plugin with a fresh binary name is added alongside built-ins | `session/detection/registry_test.go` | `MergedRegistry_should_addEntry_When_pluginHasNewBinaryName` | Unit | Happy path |
| REQ-3: `MergedRegistry` does not mutate its input registry | `session/detection/registry_test.go` | `MergedRegistry_should_notMutateInputRegistry_When_called` | Unit | Edge case |
| REQ-3: removing a file that overrode a built-in restores the built-in | `session/detection/plugin_watcher_test.go` | `PluginWatcher_should_restoreBuiltin_When_overridingFileRemoved` | Integration | Happy path (watcher + registry) |
| REQ-4: adding a file is picked up live (duplicate anchor of REQ-1 watcher row, distinct edit/remove focus below) | `session/detection/plugin_watcher_test.go` | `PluginWatcher_should_detectAddedFile_When_fileWrittenToWatchedDir` | Integration | Happy path |
| REQ-4: editing a file's regex is picked up live | `session/detection/plugin_watcher_test.go` | `PluginWatcher_should_detectEditedFile_When_regexChanged` | Integration | Happy path |
| REQ-4: removing a file is picked up live | `session/detection/plugin_watcher_test.go` | `PluginWatcher_should_detectRemovedFile_When_fileDeleted` | Integration | Happy path |
| REQ-4: a burst of fs events causes exactly one reload | `session/detection/plugin_watcher_test.go` | `PluginWatcher_should_triggerSingleReload_When_burstOfEventsOccurs` | Integration | Edge case (debounce) |
| REQ-4: fsnotify unavailable degrades to periodic rescan, not failure | `session/detection/plugin_watcher_test.go` | `StartPluginWatcher_should_fallBackToPeriodicRescan_When_fsnotifyUnavailable` | Integration | Error path (soft-fail) |
| REQ-5: the plugin directory is created on first run | `session/detection/plugins_test.go` | `EnsurePluginDir_should_createDirectory_When_absent` | Integration | Happy path (fs) |
| REQ-5: the example seed file documents every status key | `session/detection/plugins_test.go` | `EnsurePluginDir_should_seedExampleWithAllStatusKeys_When_freshDirectory` | Integration | Happy path (fs) |
| REQ-5: the example seed file is not itself loaded as a plugin | `session/detection/plugins_test.go` | `LoadPluginDir_should_ignoreExampleSampleFile_When_onlySampleFilePresent` | Integration | Edge case |
| REQ-5: bootstrap is idempotent, never clobbers a user-edited example | `session/detection/plugins_test.go` | `EnsurePluginDir_should_notClobberExistingExample_When_alreadyEdited` | Integration | Edge case |
| REQ-5: startup bootstraps, loads, and starts watching in order | `session/detection/plugins_test.go` | `InitPlugins_should_bootstrapLoadAndWatch_When_freshStart` | Integration | Happy path |
| REQ-5: the kill switch fully disables the feature | `session/detection/plugins_test.go` | `InitPlugins_should_skipEverything_When_killSwitchSet` | Integration | Edge case (risk control) |
| REQ-5: a plugin-directory create failure never blocks daemon startup | `session/detection/plugins_test.go` | `InitPlugins_should_logAndReturnNil_When_directoryCannotBeCreated` | Integration | Error path |
| REQ-6: with no plugins loaded, `DetectForProgram` behaves identically to today | `session/detection/detector_snapshot_test.go` | `DetectForProgram_should_matchBuiltinBehavior_When_noPluginsLoaded` | Unit | Happy path (regression) |
| REQ-6: provenance is reported for every claimed binary name | `session/detection/detector_snapshot_test.go` | `DetectorProvenance_should_reportBuiltins_When_noPluginsLoaded` | Unit | Happy path |
| REQ-6: `Register`'s panic-on-duplicate invariant is untouched | `session/detection/registry_test.go` | `TestDetectorRegistry_should_panic_When_duplicateNameRegistered` (byte-for-byte unmodified) | Unit | Regression |
| REQ-6: the full existing detection suite and fixtures pass unmodified | `session/detection/*_test.go`, `session/detection/testdata/` | `go test ./session/detection/...` (incl. `snapshot_test.go`, `bug_regression_test.go`) | Integration | Regression gate |
| REQ-6 (concurrency NFR): concurrent snapshot writes/reads are race-free | `session/detection/*_test.go` | `go test -race ./session/detection/...` | Integration | Regression gate |
| Epic 2.4 (closes pre-mortem P1 #1): a per-program plugin detector is actually used by the live session status path | `session/claude_controller_test.go` | `ClaudeController_should_useResolvedDetector_When_pluginRegisteredForProgram` | Integration | Happy path |
| Epic 2.4: falls back to `getDefaultPatterns()` when no detector is registered for the program | `session/claude_controller_test.go` | `ClaudeController_should_useDefaultDetector_When_noMatchingPluginOrBuiltin` | Unit | Regression (no change for unmatched programs) |
| Epic 2.4: `DetectorForProgram` wrapper hits and misses correctly | `session/detection/detector_snapshot_test.go` | `DetectorForProgram_should_returnDetectorAndTrue_When_programRegistered` / `_should_returnFalse_When_programNotRegistered` | Unit | Happy path + error path |
| Hardening 1: a cap-compliant but expensive file (50 patterns × `(4000-byte-literal){500}`) is rejected on `patterns`, not left to compile for 6s+ | `session/detection/plugins_test.go` | `validatePluginFile_should_rejectOnField_patterns_When_cumulativeCompileTimeExceedsBudget` | Unit | Error path (adversarial boundary) |
| Hardening 2: a seed-file-write failure does not abort scanning real plugins already in the directory | `session/detection/plugins_test.go` | `EnsurePluginDir_should_stillReturnDirectory_When_seedFileWriteFails` | Integration | Error path (non-fatal) |
| Hardening 3: more than `maxPluginFiles` (200) `.toml` files yields exactly 200 detectors plus one count-cap error | `session/detection/plugins_test.go` | `LoadPluginDir_should_capAt200Detectors_When_moreThan200TomlFilesPresent` | Integration | Edge case |
| Hardening 3: the count-cap error does not trip `rebuildSnapshot`'s fatal "keep previous snapshot" path (distinct `Field: "file_count"` vs `"directory"`) | `session/detection/detector_snapshot_test.go` | `rebuildSnapshot_should_stillPublishSnapshot_When_fileCountCapExceeded` | Integration | Edge case (regression guard for the `Field` disambiguation) |
| Hardening 4: `rebuildSnapshot` returns immediately without rebuilding when `ctx` is already cancelled | `session/detection/detector_snapshot_test.go` | `rebuildSnapshot_should_returnCtxErr_When_contextAlreadyCancelled` | Unit | Error path |
| Hardening 5: a second `InitPlugins` call in the same process is a no-op, not a duplicate watcher | `session/detection/plugins_test.go` | `InitPlugins_should_beNoOp_When_calledTwice` | Integration | Edge case (re-entrancy) |
| Phase 3 (optional, no AC dependency): `detectors list` shows plugin + built-in source | `main_test.go` | `TestDetectorsListCmd_should_listPluginsAndBuiltins_When_pluginLoaded` | Integration | Happy path |
| Phase 3 (optional, no AC dependency): `detectors list` reports a rejected file with its reason | `main_test.go` | `TestDetectorsListCmd_should_reportRejectedFile_When_fileInvalid` | Integration | Error path |
| Phase 3 (optional, no AC dependency): `detectors test` reports a matching pattern | `main_test.go` | `TestDetectorsTestCmd_should_reportMatch_When_patternMatches` | Integration | Happy path |
| Phase 3 (optional, no AC dependency): `detectors test` lists patterns tried on no match | `main_test.go` | `TestDetectorsTestCmd_should_reportNoMatch_When_patternDoesNotMatch` | Integration | Error path |

## UX Acceptance Tests

N/A — this is backend-only Go work (TOML loader + hot-reload watcher); `research/ux.md` confirms no UI ships with this plan, and the plan's own Scope note excludes all `web-app/` changes, so there is no rendered surface to test.

## Test Stack

- **Unit**: Go `testing` + `testify` (`require`/`assert`), consistent with existing `session/detection/*_test.go` files (e.g. `registry_test.go`, `pattern_set_test.go`).
- **Integration**: Go `testing` with `t.TempDir()` for real filesystem fixtures, `t.Setenv("STAPLER_SQUAD_TEST_DIR", ...)` for config-dir isolation, and `require.Eventually`-style polling (never `time.Sleep`, per `docs/adr/003-no-static-sleeps-in-tests.md`) for the watcher's async pickup. No external test containers or mocked filesystem — real `os` calls against a temp directory are cheap and match this package's existing convention (`snapshot_test.go`, `bug_regression_test.go`).
- **E2E / UX**: N/A — no UI surface; see UX Acceptance Tests above.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |

- All public service methods (`LoadPluginDir`, `EnsurePluginDir`, `InitPlugins`, `MergedRegistry`, `DetectorProvenance`, `StartPluginWatcher`, `PluginDir`) have happy-path and error-path coverage per the mapping table above.
- All external integrations (filesystem reads/writes, fsnotify) are covered both by unit tests that exercise pure functions on in-memory byte slices (`parsePluginFile`, `validatePluginFile`, `statusField`, `toStatusPatterns`) and by at least one integration test that exercises the real filesystem/watcher path (`LoadPluginDir`, `EnsurePluginDir`, `PluginWatcher`).
- `go test -race ./session/detection/...` is mandatory (Task 2.3.3e) because this feature introduces the first concurrent write to a structure (`activeSnapshot`) that was previously write-once at package `init()`.
- `make nil-safety` is mandatory because `lookupBinaryDetector`'s `activeSnapshot.Load()` nil-guard is exactly the shape NilAway flags.
