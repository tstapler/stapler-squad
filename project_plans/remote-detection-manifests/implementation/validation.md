# Validation Plan: remote-detection-manifests

**Date**: 2026-08-06

**Status mirrors `plan.md`**: Phase 1 (Unblock) is ready for implementation now. Phase 2
(Remote-Fetch Layer) is **BLOCKED** — do not begin Phase 2 test-writing or implementation until
both plan.md gate conditions resolve: (a) Phase 1 lands on `main`, and (b) the `detector-plugins`
90-day demand checkpoint (target ~2026-10-31) resolves toward "demand confirmed," or an explicit
user/owner override. Phase 2's test suite is designed in full below anyway, per plan.md's own
reasoning, so a future implementer has a real plan rather than a placeholder.

## Happy Path Scenario

Given PR #307's reviewed diff (`session/detection/plugins.go`, `detector_snapshot.go`,
`plugin_watcher.go`, `registry.go`'s `MergedRegistry`) exists only on a closed, unmerged PR and
not on `main`, when a fresh PR rebases that same diff onto current `main` and passes
`go build`/`go vet`/`gofmt`/`make lint`/`go test ./session/detection/... -count=1`/
`go test -race ./session/detection/... -count=1`/`make quick-check`, then merging it lands the
local TOML plugin loader on `main` — `grep -rln "InitPlugins" --include=*.go main.go` matches —
unblocking Phase 2's precondition (a) with zero behavioral regression to any existing detection
test.

## Requirement → Test Mapping

### Phase 1 — Unblock (ready now)

Phase 1 re-lands an already-designed, already-reviewed, already-tested feature. The ~80-row
unit/integration test suite for the loader itself (`parsePluginFile`, `validatePluginFile`,
`LoadPluginDir`, `EnsurePluginDir`, `rebuildSnapshot`, `MergedRegistry`, `Upsert`,
`PluginWatcher`, `InitPlugins`, and the `ClaudeController` wiring) is fully specified in
`project_plans/detector-plugins/implementation/validation.md` and is **reused unchanged, not
re-designed here** — Task 1.2.3's acceptance criterion is that this existing suite passes
byte-for-byte against the rebased code. The rows below cover only what's genuinely new to
*this* item: verifying the closure discrepancy and gating the re-land process itself.

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| Story 1.1 AC1: closure claim (commits not ancestors of `main`) re-verified against current `main`, not just cited from stale research | N/A (Task 1.1a, one-time shell verification, not an automated test — see note below) | `git merge-base --is-ancestor 3c25e94f9 HEAD; echo $?` and same for `32f504c8` | Verification (manual, one-time) | Happy path (confirms Story 1.1's premise holds) |
| Story 1.1 AC1: plugin-loader symbols confirmed absent from `main` before re-landing | N/A (Task 1.1a) | `grep -rln "InitPlugins\|EnsurePluginDir\|ResolveDetectorForProgram" --include=*.go . \| grep -v .claude/worktrees` returns no matches | Verification (manual, one-time) | Happy path |
| Story 1.1 AC1 (inverse — the escape hatch Epic 1.2 names explicitly): if re-verification instead finds the closure WAS correct | N/A (Task 1.2.1) | Same two commands as above, asserting the opposite result | Verification (manual, one-time) | Error path — if this triggers, Epic 1.2 stops and a correction note is written instead of re-landing |
| Story 1.2 AC1: rebased diff reused unchanged from `detector-plugins`' reviewed design (representative happy-path unit test carried over) | `session/detection/plugins_test.go` | `parsePluginFile_should_returnDTO_When_fileIsWellFormed` | Unit (reused, unmodified) | Happy path |
| Story 1.2 AC1: rebased diff's validation pipeline still rejects malformed input (representative error-path unit test carried over) | `session/detection/plugins_test.go` | `validatePluginFile_should_rejectMissingID_When_idFieldAbsent` | Unit (reused, unmodified) | Error path |
| Story 1.2 AC1: a plugin becomes detectable end-to-end post-rebase (representative integration test carried over) | `session/detection/detector_snapshot_test.go` | `rebuildSnapshot_should_makePluginDetectable_When_validPluginFileScanned` | Integration (reused, unmodified) | Happy path |
| Story 1.2 AC2: full test-plan checklist passes post-rebase (`go build`, `go vet`, `gofmt -l`, custom linters, `go test`) | N/A (repo-wide gate) | `go build ./session/... ./.` / `go vet ./session/...` / `gofmt -l session/detection/*.go main.go session/claude_controller.go` / `make lint` / `go test ./session/... -count=1` | Verification (CI gate) | Happy path (regression gate) |
| Story 1.2 AC2: no data race introduced by the concurrent snapshot writer (`activeSnapshot`) | `session/detection/*_test.go` | `go test -race ./session/detection/... -count=1` | Integration (reused, unmodified) | Happy path (regression gate) |
| Story 1.2 AC3: `make lint` and `make quick-check` — this repo's actual merge gate per `CLAUDE.md` — both pass | N/A (repo-wide gate) | `make lint`, `make quick-check` | Verification (CI gate) | Happy path (regression gate) |
| Story 1.2 AC4: the new PR's description states Story 1.1's closure correction, not just the original PR #307 body | N/A (Task 1.2.4, PR body review) | Manual reviewer check: does `gh pr view <new-pr> --json body` contain the correction paragraph from Task 1.1b? | Verification (manual, one-time) | Happy path |
| Task 1.2.5 (post-merge confirmation): `InitPlugins` is actually wired and `main` builds | N/A (post-merge check) | `grep -rln "InitPlugins" --include=*.go main.go` matches; `go build ./...` succeeds on fresh `main` | Verification (CI gate) | Happy path (closes the loop on the Happy Path Scenario above) |

---

### Phase 2 — Remote-Fetch Layer (**BLOCKED — do not implement until plan.md's gate resolves**)

Every row below is planned for a future implementer, per plan.md's own reasoning ("planning it
is not authorization to start it"). Test names use plan.md's Domain Glossary terms
(`RemoteCacheDir`, `RemoteFetcher`, `InitRemoteManifests`, `rebuildSnapshot`, `MergedRegistry`,
`compareManifestVersion`, `shouldAcceptManifest`, `refreshRemoteManifests`) so the eventual
implementation can trace each test straight back to the glossary and to plan.md's own
Given/When/Then acceptance criteria.

#### Epic 2.1 — `RemoteCacheDir` + `RemoteFetcher` — BLOCKED

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| Story 2.1.1: `RemoteCacheDir()` returns a sibling of, not a child of, `PluginDir()` | `session/detection/remote_manifests_test.go` | `TestRemoteCacheDir_should_ReturnSiblingOfPluginDir_When_ConfigDirResolved` | Unit | Happy path |
| Story 2.1.1: `RemoteCacheDir()` inherits `STAPLER_SQUAD_TEST_DIR` isolation exactly like `PluginDir()` | `session/detection/remote_manifests_test.go` | `TestRemoteCacheDir_should_ResolveUnderTestDir_When_STAPLER_SQUAD_TEST_DIR_Set` | Unit | Happy path (edge — test isolation) |
| Story 2.1.1: `RemoteCacheDir()` surfaces a typed error rather than a bad path if `config.GetConfigDir()` fails | `session/detection/remote_manifests_test.go` | `TestRemoteCacheDir_should_ReturnError_When_ConfigDirResolutionFails` | Unit | Error path |
| Story 2.1.2: `RemoteFetcher.Fetch` returns response body bytes on a 200 | `session/detection/remote_manifests_test.go` | `TestRemoteFetcher_Fetch_should_ReturnBodyBytes_When_ServerReturns200` | Unit (`httptest.NewTLSServer`) | Happy path |
| Story 2.1.2: `RemoteFetcher.Fetch` returns a typed error naming the status on a non-2xx | `session/detection/remote_manifests_test.go` | `TestRemoteFetcher_Fetch_should_ReturnErrorNaming404_When_ServerReturns404` | Unit | Error path |
| Story 2.1.2: `RemoteFetcher.Fetch` rejects a plain-`http://` source URL before any connection attempt | `session/detection/remote_manifests_test.go` | `TestRemoteFetcher_Fetch_should_RejectPlainHTTP_When_SourceURLNotHTTPS` | Unit (fail-the-test-if-hit server) | Error path |
| Story 2.1.2: `RemoteFetcher.Fetch` rejects an oversized body via `maxPluginFileSize` before returning bytes to the caller | `session/detection/remote_manifests_test.go` | `TestRemoteFetcher_Fetch_should_RejectResponse_When_BodyExceedsMaxPluginFileSize` | Unit | Error path |
| Story 2.1.2: `RemoteFetcher.Fetch` is bounded by `remoteFetchTimeout` for the *whole* operation, not just the round trip (a server that accepts the TCP connection but never writes) | `session/detection/remote_manifests_test.go` | `TestRemoteFetcher_Fetch_should_ReturnWithinTimeout_When_ServerNeverResponds` | Integration (real TCP connection + `context.WithTimeout`) | Error path (timeout) |

#### Epic 2.2 — Version compare + Never-Downgrade Rule — BLOCKED (also depends on ADR-001 finalized)

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| Story 2.2.1: `compareManifestVersion` returns `1` when fetched is strictly newer | `session/detection/remote_manifests_test.go` | `TestCompareManifestVersion_should_ReturnPositive_When_FetchedVersionHigher` | Unit | Happy path |
| Story 2.2.1: `compareManifestVersion` returns `0` for equal versions with padded/missing segments (`"1.2"` == `"1.2.0"`) | `session/detection/remote_manifests_test.go` | `TestCompareManifestVersion_should_ReturnZero_When_VersionsEqualWithPaddedSegments` | Unit | Happy path (edge) |
| Story 2.2.1: `compareManifestVersion` returns a typed error, not a panic, on a malformed version string | `session/detection/remote_manifests_test.go` | `TestCompareManifestVersion_should_ReturnError_When_VersionStringMalformed` | Unit | Error path |
| Story 2.2.2: `shouldAcceptManifest` accepts a genuinely newer version | `session/detection/remote_manifests_test.go` | `TestShouldAcceptManifest_should_Accept_When_FetchedVersionIsNewer` | Unit | Happy path |
| Story 2.2.2: `shouldAcceptManifest` rejects a lower version (Never-Downgrade Rule) | `session/detection/remote_manifests_test.go` | `TestShouldAcceptManifest_should_Reject_When_FetchedVersionIsLower` | Unit | Error path |
| Story 2.2.2: `shouldAcceptManifest` rejects same-version-different-content (content-must-match-version rule) | `session/detection/remote_manifests_test.go` | `TestShouldAcceptManifest_should_Reject_When_VersionUnchangedButContentDiffers` | Unit | Error path |
| Story 2.2.2: end-to-end — a downgrade fetch through `refreshRemoteManifests` leaves the cache file's contents and mtime untouched and emits the `"downgrade rejected"` log line | `session/detection/remote_manifests_test.go` | `TestRefreshRemoteManifests_should_LeaveCacheUntouched_When_DowngradeRejected` | Integration (`httptest` server + real `t.TempDir()` cache dir) | Error path |

#### Epic 2.3 — `rebuildSnapshot` generalized to two directories + `MergedRegistry` called twice — BLOCKED (needs Epic 2.1, not Epic 2.2)

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| Story 2.3.1: with an empty/missing remote cache dir, behavior is byte-identical to Phase 1's one-arg `rebuildSnapshot` | `session/detection/detector_snapshot_test.go` | `TestRebuildSnapshot_should_MatchPhase1Behavior_When_RemoteCacheDirEmpty` | Integration (two `t.TempDir()`s) | Happy path (regression) |
| Story 2.3.1: a remote-only detector (no local override) resolves and its provenance names the remote cache file | `session/detection/detector_snapshot_test.go` | `TestRebuildSnapshot_should_ResolveRemoteOnlyDetector_When_NoLocalOverride` | Integration | Happy path |
| Story 2.3.1: a local file claiming the same binary name as a remote manifest wins (Three-Layer Precedence) | `session/detection/detector_snapshot_test.go` | `TestRebuildSnapshot_should_PreferLocalOverRemote_When_BothClaimSameBinaryName` | Integration | Happy path |
| Story 2.3.1: a remote-sourced detector with no local override wins over the built-in | `session/detection/detector_snapshot_test.go` | `TestRebuildSnapshot_should_PreferRemoteOverBuiltin_When_NoLocalOverrideExists` | Integration | Happy path |
| Story 2.3.1: a remote-directory-level scan failure (dir replaced by a regular file) does not affect local-directory detectors, and is logged with a distinguishable `dir` field | `session/detection/detector_snapshot_test.go` | `TestRebuildSnapshot_should_KeepLocalDetectorsLoaded_When_RemoteDirScanFails` | Integration | Error path |

#### Epic 2.4 — `InitRemoteManifests` + background `refreshRemoteManifests` goroutine — BLOCKED (needs 2.1, 2.2, 2.3)

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| Story 2.4.1: `InitRemoteManifests` returns fast (ms, not s) even when the network is entirely unreachable, because the network attempt lives only in the goroutine it launches | `session/detection/remote_manifests_test.go` | `TestInitRemoteManifests_should_ReturnFast_When_NetworkUnreachable` | Integration (unroutable test address, timing assertion) | Happy path (structural — proves the sync/async split) |
| Story 2.4.1: a cache populated by a prior run is visible in `DetectorProvenance()` immediately, before any goroutine runs | `session/detection/remote_manifests_test.go` | `TestInitRemoteManifests_should_PopulateProvenance_When_CachePrepopulatedFromPriorRun` | Integration | Happy path |
| Story 2.4.2: a successful background fetch updates the live snapshot without any caller polling | `session/detection/remote_manifests_test.go` | `TestRefreshRemoteManifests_should_UpdateLiveSnapshot_When_FetchSucceeds` | Integration (`httptest` server) | Happy path |
| Story 2.4.2: a failed background fetch (connection refused) leaves `activeSnapshot` and on-disk cache both unchanged, emitting only the `"remote manifest fetch failed"` log line | `session/detection/remote_manifests_test.go` | `TestRefreshRemoteManifests_should_LeaveSnapshotAndCacheUnchanged_When_ServerUnreachable` | Integration | Error path |
| Story 2.4.2: exactly one background goroutine runs across two `InitRemoteManifests(ctx)` calls in the same process (`sync.Once` re-entrancy guard) | `session/detection/remote_manifests_test.go` | `TestInitRemoteManifests_should_LaunchGoroutineExactlyOnce_When_CalledTwice` | Integration (counting `RemoteFetcher` double + `sync.WaitGroup`/channel sync, no `time.Sleep` per `.claude/rules/fix-flaky-tests-dont-defer.md`) | Edge case (re-entrancy) |

#### Epic 2.5 — Trust/pinning + kill switch + full logging — BLOCKED (needs ADR-002 finalized; parallelizable with 2.1–2.4)

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| Story 2.5.1: the default configured source URL embeds a 40-hex-char pinned commit SHA, not `main`/`latest` | `config/config_test.go` (or `session/detection/remote_manifests_test.go` — whichever owns the resolution function per plan.md's Story 2.5.1 Files note) | `TestResolveRemoteManifestSourceURL_should_EmbedPinnedSHA_When_NoOverrideConfigured` | Unit | Happy path |
| Story 2.5.1: an explicit source URL override is honored, and a malformed/non-HTTPS override is rejected rather than silently used | same as above | `TestResolveRemoteManifestSourceURL_should_RejectOverride_When_OverrideURLNotHTTPS` | Unit | Error path |
| Story 2.5.2: the kill switch (`STAPLER_SQUAD_DISABLE_REMOTE_MANIFESTS=1`) makes `InitRemoteManifests` return `nil` immediately | `session/detection/remote_manifests_test.go` | `TestInitRemoteManifests_should_ReturnNilImmediately_When_KillSwitchSet` | Unit | Happy path |
| Story 2.5.2: with the kill switch set, `RemoteCacheDir()` is never created and no goroutine is launched, while `InitPlugins` (Phase 1) is entirely unaffected | `session/detection/remote_manifests_test.go` | `TestInitRemoteManifests_should_NotCreateCacheDirOrLaunchGoroutine_When_KillSwitchSet` | Integration | Happy path (isolation from Phase 1) |
| Story 2.5.2: each of the five fetch outcomes (success, network failure, validation rejection, downgrade rejection, content-mismatch rejection) produces exactly one matching log line | `session/detection/remote_manifests_test.go` | `TestRefreshRemoteManifests_should_LogExactlyOneLine_When_EachOutcomeOccurs` (table-driven, 5 subtests) | Integration (log-capture hook, not stdout parsing) | Happy path (observability contract) |
| Story 2.5.2: a fetch failure's log line names the actual error, not a generic message, so a fetch problem is debuggable from logs alone | `session/detection/remote_manifests_test.go` | `TestRefreshRemoteManifests_should_LogUnderlyingError_When_NetworkErrorOccurs` | Unit/Integration | Error path |

## UX Acceptance Tests

N/A — pure infrastructure feature (Go backend/CLI only, no `web-app/` changes in either phase's
Scope per `requirements.md`), no `design/ux.md` exists for this project, and none is warranted.

## Test Stack

- **Unit**: Go stdlib `testing` + `testify` (`require`/`assert`), matching the existing
  convention in `session/detection/*_test.go` (e.g. `registry_test.go`,
  `session/detection/binaries/*_test.go`).
- **Integration**: Go stdlib `testing`, real filesystem via `t.TempDir()` and
  `t.Setenv("STAPLER_SQUAD_TEST_DIR", ...)` for config-dir isolation (Phase 1, reused from
  `detector-plugins`); `httptest.NewTLSServer` for Phase 2's fetch tests, with the fetcher's
  underlying client trusting the test server's cert via `server.Client()` rather than disabling
  TLS verification in shipped code. No `time.Sleep`-based polling anywhere — use
  `require.Eventually`, channels, or `sync.WaitGroup` for async assertions, per
  `.claude/rules/fix-flaky-tests-dont-defer.md` and this repo's existing
  `docs/adr/003-no-static-sleeps-in-tests.md` convention.
- **E2E / UX**: N/A — no UI surface, see above.
- **Process-level verification (Phase 1 only)**: a handful of Story 1.1/1.2 acceptance criteria
  are one-time `git`/`gh`/`grep` checks and repo-wide gates (`make lint`, `make quick-check`),
  not permanent automated tests — encoding "commit X is not an ancestor of `main`" as a lasting
  `Test...` function would go stale (false) the moment Phase 1 merges. These are marked
  "Verification (manual/CI gate)" in the table above rather than given `Test...` names.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |

- **Phase 1**: coverage target is met by reusing `detector-plugins`' own already-passing suite
  unchanged (see `project_plans/detector-plugins/implementation/validation.md` for its own
  coverage accounting) — Task 1.2.3 requires the full suite to pass with zero weakened
  assertions, not just a coverage percentage.
- **Phase 2 (once unblocked)**: all public/exported functions (`RemoteCacheDir`, `RemoteFetcher.
  Fetch`, `InitRemoteManifests`) and all unexported-but-independently-testable functions
  (`compareManifestVersion`, `shouldAcceptManifest`, `refreshRemoteManifests`, the two-dir
  `rebuildSnapshot`) need happy-path and error-path coverage per the mapping table above before
  Phase 2 is considered done, matching plan.md's own "Summary of what 'done' means" section.
- `go test -race ./session/detection/... -count=1` is mandatory for both phases — Phase 1
  because it introduces the first concurrent write to `activeSnapshot`; Phase 2 because it adds
  a *third* concurrent writer (the network-triggered goroutine) to the same guarded pointer.
- `make nil-safety` is mandatory for both phases, per `detector-plugins`' own precedent
  (`lookupBinaryDetector`'s `activeSnapshot.Load()` nil-guard is exactly the shape NilAway
  flags) and because Phase 2's `RemoteFetcher`/cache-read paths introduce new nil-return
  surfaces of the same shape.
