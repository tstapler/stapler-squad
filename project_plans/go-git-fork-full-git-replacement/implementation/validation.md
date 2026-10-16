# Validation Plan: go-git-fork-full-git-replacement

**Date**: 2026-10-08
**Inputs**: `implementation/plan.md` (Revision 5), `requirements.md`. No `design/ux.md` exists (infrastructure change, no user-facing surface), so there is no UX section.

## Happy Path Scenario
Given a `session` test package and a live server whose `git` on `PATH` is a recording shim, with every cohort set to `gogit` and a repository with no hooks, signing, LFS or remote host, when a session is created (linked worktree add), edited, committed, paused (worktree removed), resumed (existing-branch add) and cleaned up through the ConnectRPC API, then every call succeeds, the repository passes `git fsck --strict` and `git worktree prune --dry-run` is empty, and the shim log holds no `git` hit outside the allow-list. The same flow also passes in a container with no `git` binary.

## Metric wording used below
`requirements.md` Success Metrics (SM) and the plan's reworded forms (plan Open Question O-9, ADR-003 tiers):

| ID | `requirements.md` wording | Plan wording (what is actually tested) |
|---|---|---|
| SM-1 | Zero `git` subprocess spawns in a full session cycle and in the `session` test package, measured with a PATH shim | Zero `git` spawns in `ServerProcess` **except allow-listed carve-outs** keyed on (operation, reason): `gh`/`agent_tool`, `remote_host`, `capability_*`, `torn_read`/`object_missing`, `unsafe_worktree_write`, `capability_local_transport`. Two layers (PATH shim over `./...`, plus counters). Story 5.3.1 |
| SM-2 | Every existing `session` and `server/services` test passes with the CLI fallbacks removed, none deleted or weakened | The `cli` backend stays permanently (ADR-003/006). Parity is checked by: pass count unchanged per batch, no removed `func Test`, and the no-git-PATH run passing the same tests (skip-set ratchet). Stories 5.1.2, 5.3.1 |
| SM-3 | `session` wall time and summed mutex delay do not regress vs post-#955 (about 72 to 92 s) | Baseline recorded in Story 0.1.2; wall time checked per batch in Story 5.1.2. **Mutex delay has no checking story** (gap G-1) |
| SM-4 | Runtime works on a machine with no `git` binary, verified in a minimal container | Server-process tier only; git-using agents, `gh`, hooked/signed/LFS repos and remote hosts still need git (Story 5.3.2) |

Where the two wordings differ, the tests are written against the stricter reading (the original), and the plan's carve-outs are asserted as an explicit allow-list so a spawn outside it fails.

## Requirement → Test Mapping

Test files follow the plan's `Files:` entries. Names use `methodName_should_ExpectedBehavior_When_Condition`. Go subtests use `Test<Name>/<case>`; the underscore form is the Go function name.

### Success metrics

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| SM-1 zero spawns (session cycle) | `executor/safeexec/spawn_gate_test.go` | `TestSpawnGate_should_FailRun_When_ShimHitOutsideAllowList` | Integration | Happy path: `go test -count=1 ./...` with shim stubs for `git`, `git-upload-pack`, `git-receive-pack`, `git-upload-archive`, `git-lfs`, `gh`; zero un-allow-listed hits |
| SM-1 | `executor/safeexec/spawn_gate_test.go` | `TestSpawnGate_should_RefuseToRun_When_CountFlagMissingOrCacheable` | Unit | Error path: gate script rejects a run without `-count=1` |
| SM-1 | `executor/safeexec/spawncount_test.go` | `TestBackstopInequality_should_Fail_When_UnroutedLocalSpawn` | Unit | Error path: direct `safeexec.CommandContext(ctx,"git","status")` makes `sum(backstop) > sum(backend, reason != remote_host)` |
| SM-1 | `executor/safeexec/spawncount_test.go` | `TestBackstopInequality_should_Pass_When_LocalRunnerAndRemoteRunnerBothUsed` | Unit | Happy path: Local call increments both counters; remote call increments only `remote_host` |
| SM-1 | `executor/safeexec/spawncount_test.go` | `TestBackstopInequality_should_Fail_When_RemoteSpawnMasksLocalBypass` | Unit | Synthetic run: one remote spawn plus one unrouted local spawn must fail |
| SM-1 | `session/git/backend/cli/spawn_test.go` | `TestCountingRunner_should_CountOncePerRun_When_MethodRunsTwoGitCommands` | Unit | `git_backend_cli_spawn_total{operation,reason}` per `Runner.Run` |
| SM-1 | `executor/safeexec/spawn_gate_test.go` | `TestSpawnGate_should_AttributeToTest_When_CwdUnderTempDir` | Unit | Report names test parsed from `t.TempDir()`; otherwise `(package,cwd,argv)` only |
| SM-1 | `executor/safeexec/spawn_gate_test.go` | `TestAllowList_should_RejectShimHit_When_ParentIsServerNotTestChild` | Unit | Fake agent child that runs `git` is allowed; same argv from server process fails |
| SM-1 | `executor/safeexec/spawn_gate_test.go` | `TestSpawnAllowlistRatchet_should_Fail_When_EntryAddedWithoutBaselineEdit` | Unit | Allow-lists cannot launder spawns |
| SM-1 | `tools/lint/norawgitpath/analyzer_test.go` | `Analyzer_should_Flag_When_TestSetsPathOutsideSpawngate` | Unit | `t.Setenv("PATH")`, `os.Setenv("PATH")`, literal `/usr/bin/git` flagged; `circuit_breaker_test.go:141` carries `nolint` |
| SM-1 | `session/git/backend/gogit/transport_tripwire_test.go` | `TestFileTripwire_should_ReturnErrLocalTransport_When_GogitClonesLocalBareRepo` | Integration | Zero shim hits from go-git's `file` transport with `PATH` holding only stubs |
| SM-1 | `tests/no-git-container/run.sh` | `no_git_container_should_ReportZeroBackendSpawns_When_SessionCycleRuns` | E2E | Create, edit, pause, cleanup, local clone via API; backend spawn counters 0 |
| SM-1 | `session/git/backend/routed_test.go` | `TestRouted_should_ServeAllCohortsInProcess_When_AllGogitAndCleanRepo` | Integration | Happy-path scenario above against a fixture repo, asserting zero `cli` implementation labels |
| SM-2 parity | `scripts/spawn-gate.sh` + `scripts/no-git-skips.txt` | `no_git_run_should_PassSameTests_When_PathHasNoGit` | Integration | `go test -count=1 -json ./...` with no `git` on `PATH`; `S_nogit \ S_normal` subset of ratcheted skip file |
| SM-2 | `scripts/no-git-skips.txt` (CI compare) | `skipFileRatchet_should_Fail_When_EntriesGainedVersusMain` | Unit | Error path: skip list grew |
| SM-2 | `testutil/spawngate/spawngate_test.go` | `RequireGit_should_SkipWithRequiresGitPrefix_When_GitMissing` | Unit | Only sanctioned skip reason is recognised |
| SM-2 | `session/*_test.go` (per batch PR) | `batchMigration_should_KeepPassCountAndFuncTests_When_FixturesMigrated` (CI script: `go test -json` pass count plus `git diff --stat` no removed `func Test`) | Integration | Pass count unchanged, no deletion or skip, per batch |
| SM-2 | `testutil/gitfixture/gitfixture_test.go` | `NewRepo_should_CommitWithoutExec_When_FixtureUsed` | Unit | Happy path: shim count 0, `git fsck` clean |
| SM-2 | `testutil/gitfixture/gitfixture_test.go` | `Identity_should_UseSetConfigNotCLI_When_AuthorConfigured` | Unit | Replaces `git config --local` in `identity.go` |
| SM-2 | `session/git/backend/gogit/*_oracle_test.go` (tag `gitoracle`) | `Oracle_should_AgreeWithRealGit_When_InProcessStateWritten` | Integration | `make test-oracle` runs in CI; oracle excluded from gate by tag |
| SM-3 wall time | `scripts/test-session-timing.sh` (new, Story 5.1.2) | `sessionPackageWallTime_should_NotExceedBaseline_When_BatchMerged` | Integration | `go test ./session` wall time at or below post-#955 baseline (72 to 92 s) |
| SM-3 | `scripts/test-session-timing.sh` | `sessionPackageWallTime_should_FailBatch_When_AboveBaselineBand` | Unit | Error path: gate fails above the recorded band |
| SM-3 mutex delay | `scripts/test-session-timing.sh` | `sessionMutexDelay_should_NotExceedBaseline_When_BatchMerged` (`go test -mutexprofile`, summed delay vs `audit.md`) | Integration | **Proposed; no plan story checks this (gap G-1)** |
| SM-4 no git binary | `tests/no-git-container/run.sh` | `no_git_container_should_StartAndServe_When_NoGitBinaryInstalled` | E2E | `debian:stable-slim` by digest, no `git`/`gh`; session create, edit, pause, cleanup all succeed |
| SM-4 | `session/vcs/detect_test.go` | `Detect_should_FindGitRepo_When_NoGitOnPath` | Unit | Task 5.3.2a: `exec.LookPath("git")` pre-check removed |
| SM-4 | `session/vcs/detect_test.go` | `Detect_should_ReturnNotARepo_When_DirectoryIsNotGit` | Unit | Error path, no git on `PATH` |
| SM-4 | `tests/no-git-container/run.sh` | `no_git_container_should_ListZeroHitMethods_When_CoverageTableWritten` | E2E | Every `Backend` method with hit count; zero-hit methods embedded in `what-no-git-means.md` |

### Scope: linked-worktree support (Epics 0.2/4.1, native code already in `session/git/native`)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| In-scope: worktree add, existing branch | `session/git/native/worktree_add_existing_test.go` | `AddWorktreeForExistingBranch_should_AttachWithoutMovingRef_When_BranchHasNoWorktree` | Unit | Happy path: `.git` file, `HEAD` ref, ref still at `abc1234`, oracle `worktree list --porcelain` and `fsck` agree |
| worktree add | same | `AddWorktreeForExistingBranch_should_ReturnErrBranchInUse_When_CheckedOutElsewhere` | Unit | Error path, message contains "already used by worktree" |
| worktree add | same | `AddWorktreeForExistingBranch_should_LeaveNoOrphanAdminDir_When_TwoGoroutinesRace` | Integration | Exactly one succeeds; `worktree prune --dry-run` empty |
| worktree remove/list/prune | `session/git/native/worktree_remove_test.go` | `SelfHealRemove_should_LeaveNoAdminDir_When_AdminDirPartialAndLocked` | Integration | Missing `commondir`, `locked` file; `refs/heads/*` untouched |
| worktree list | `session/git/native/worktree_list_test.go` | `ListWorktrees_should_EqualPorcelain_When_LockedAndPrunableWorktreesExist` | Integration | 3 worktrees incl. locked and deleted dir |
| worktree list | same | `ListWorktrees_should_ReturnError_When_RepoPathInvalid` | Unit | Error path |
| per-worktree HEAD/index/config, `commondir` | `session/git/backend/gogit/refs_test.go` | `CurrentBranch_should_ReturnWorktreeBranch_When_LinkedWorktreeOnFeatureBranch` | Integration | Returns `feat/x`, not `main`; equals CLI |
| per-worktree semantics | same | `GitDir_should_DifferFromCommonDir_When_LinkedWorktree` | Unit | Distinct newtypes resolve to admin dir vs shared `.git` |
| per-worktree semantics | same | `OpenRepo_should_NotCacheHandle_When_CalledTwice` | Unit | Fresh `*git.Repository` per call, `EnableDotGitCommonDir` true |
| linked-worktree HEAD failure | `session/git/backend/gogit/refsspike_test.go` | `RefsSpike_should_RecordClass1to3Counts_When_RacingWorktreeAddAndPackRefs` | Integration | S7: 3 configs x 60 s plus one 10-minute run; outcome A, B or C recorded in `gates.md` |
| linked-worktree HEAD failure | `session/git/backend/gogit/refs_test.go` | `ResolveRef_should_ReturnObjectMissing_When_HeadShaHasNoObject` | Unit | Error path: predicate routes to CLI |
| linked-worktree HEAD failure | same | `ResolveRef_should_ReturnErrUnborn_When_RepoHasNoCommits` | Unit | Sentinel parity |
| `getHeadCommitSHA` mitigation | `session/git/util_test.go` | `getHeadCommitSHA_should_KeepCLIFallback_When_G8NotFixedAtRoot` | Unit | Mitigation not deleted without proof (Story 6.2.1) |

### Scope: in-process replacements for remaining CLI sites (Epics 2.1 to 2.3)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| rev-parse family | `session/git/backend/gogit/refs_test.go` | `RepoRoot_should_ReturnRealpath_When_RepoUnderSymlinkedTmp` | Unit | Matches `git rev-parse --path-format=absolute` |
| rev-parse family | same | `RefExists_should_Work_When_ServerCwdIsRoot` | Integration | Replaces no-`-C` call sites (`backlog_lifecycle.go:1767`, `backlog_service_lifecycle.go:658`) |
| rev-parse family | same | `CurrentBranch_should_ReturnLiteralHEAD_When_Detached` | Unit | Sentinel parity |
| rev-parse family | same | `RefExists_should_ReturnFalse_When_BranchMissing` | Unit | Error path |
| rev-parse (classifier) | `pkg/classifier/classifier_test.go` | `Classifier_should_MatchCLI_When_CwdIsSubdirOfLinkedWorktree` | Integration | `RepoRoot`, `IsGitRepo`, `IsWorktree` equal for gogit and CLI adapters |
| rev-parse (classifier) | same | `Classifier_should_ReportNotRepoWithoutSpawn_When_DirNotGit` | Unit | Error path, zero spawn |
| merge-base / rev-list / log | `session/git/backend/gogit/refs_test.go` | `MergeBase_should_MatchCLIExit1_When_NoCommonAncestor` | Unit | No-result mapped to CLI exit-1 error |
| merge-base / rev-list / log | same | `RunBoth_should_AgreeOnLogSubjectsAndRevListCount_When_PackedRefsAndShallowClone` | Integration | Differential; shallow-clone mismatch listed (shadow) |
| for-each-ref / symbolic-ref | same | `ListRefs_should_EqualForEachRef_When_PackedAndLooseRefsMixed` | Integration | Prefix filter parity |
| for-each-ref / symbolic-ref | same | `IsSymbolic_should_ReturnError_When_RefNotSymbolic` | Unit | Error path |
| remote / config | same | `RemoteURL_should_ReturnConfiguredURL_When_OriginSet` | Unit | Happy path; includes `refs/remotes/origin/HEAD` absent case |
| remote / config | same | `Config_should_ReturnNotFound_When_KeyUnset` | Unit | Error path |
| diff / status | `session/git/backend/gogit/status_test.go` | `IsDirty_should_ReturnFalse_When_CleanWorktreeWithCRLFFiles` | Integration | Via `worktree_dirty_fast.go`; matches empty porcelain |
| diff / status | same | `IsDirty_should_ReturnTrue_When_TrackedFileModified` | Unit | Happy path |
| diff / status | same | `Status_should_RouteToCLI_When_PorcelainGoldenDiffers` | Unit | Error path: rename+modify, untracked dir, staged and unstaged same file; unsupported case recorded |
| diff / status | `session/git/backend/gogit/diff_test.go` | `DiffNumstat_should_MatchCLI_When_BinaryAndRenameFilesChanged` | Integration | Binary rows `-`/`-` |
| diff / status | same | `Diff_should_StayCLI_When_HeaderOrNoNewlineLinesDiffer` | Unit | Per-op route decision recorded |
| diff / status | same | `Diff_should_UseCLIMergeBaseChoice_When_ThreeDotRangeHasMultipleBases` | Integration | |
| local write (add/commit) | `session/git/backend/gogit/write_test.go` | `Commit_should_BuildTreeFromUpdatedIndex_When_CommitAllWithModifiedAndDeletedFiles` | Integration | Tree equals `git commit -a` |
| local write | same | `Commit_should_RouteToCLI_When_PreCommitHookExecutable` | Integration | Reason `capability_hooks`, hook executes |
| local write | same | `Commit_should_RunInProcessWithConfiguredAuthor_When_NoHooksNoSigning` | Integration | Oracle `git log -1 --format=%an` |
| local write | same | `SetUpstream_should_WriteRemoteAndMergeKeys_When_PushDashU` | Unit | `branch.feat/x.remote`, `.merge` |
| local write | same | `BranchRename_should_MatchCLIReflog_When_RenamedInWorktree` | Integration | Gated on reflog parity |
| local write | same | `Checkout_should_StayOnCLI_When_FaultInjectionTestNotGreen` | Unit | `unsafe_worktree_write` allow-listed |
| locking | `session/git/backend/gogit/clilock_test.go` | `WithIndexLock_should_KeepAll400Entries_When_200CLIAddsRaceGogitAdds` | Integration | No lost writes, no stale lock |
| locking | same | `WithIndexLock_should_ReturnErrLockedAndKeepForeignLock_When_FreshLockPreexists` | Unit | Error path |
| locking | same | `WithIndexLock_should_NeverExposePartialIndex_When_CLIReaderLoopsDuringWrites` | Integration | 5,000 entries, 200 writes |
| locking | same | `WithIndexLock_should_RemoveOwnLock_When_ErrorOrPanicInScope` | Unit | |
| locking | same | `WithIndexLock_should_NotDeadlock_When_24GoroutinesUnderRepoWorktreeLock` | Integration | `-race`, 60 s |
| locking | same | `WithIndexLock_should_MatchCLITwin_When_CompositeSequencesRun` | Integration | Read-your-writes matrix (Commit(All), Amend, Remove then Status, Move, Reset, Restore(Staged), AddWithOptions(All)) |
| locking | `session/git/backend/gogit/censustest_test.go` | `TestIndexCallSiteCensus_should_Fail_When_GogitAddsOrRemovesIndexCallSite` | Unit | Census pinned to `gates.md`, runs on every rebase |
| locking | `session/git/backend/gogit/clilock_test.go` | `ScopeAbort_should_RestoreRefAndIndexBytes_When_ErrorBeforeCommitPhase` | Integration | HEAD, ref, index pre-state; `Wrote()` false |
| locking | same | `ScopeAbort_should_SetWroteBeforeRename_When_ErrorAtFirstCommitRename` | Unit | No CLI replay |
| locking | same | `Router_should_NotFallBack_When_WroteTrueAfterRefRename` | Unit | Error path, `git_backend_error_total` increments |
| locking | `session/git/backend/gogit/lockjournal_test.go` | `Recovery_should_RemoveOnlyOwnLock_When_ServerSigkilledAndAllFiveConditionsMet` | Integration | Fake clock, 60 s rule |
| locking | same | `Recovery_should_LeaveLockUntouched_When_NoTokenYoungLiveSessionOrOwnerAlive` | Unit | Error path, outcome table counters |
| locking | same | `Recovery_should_ScanAllJournalDirs_When_InstanceDirChanged` | Integration | `instances/*`, `workspaces/*` |
| locking | same | `Recovery_should_Disable_When_FilesystemLacksXattr` | Unit | `recovery_unsupported_fs` |
| refs | `session/git/backend/gogit/refwriter_test.go` | `RefWriter_should_TakeRefLockThenPackedRefsLock_When_DeletingBranch` | Integration | Order, CAS under lock |
| refs | same | `RefWriter_should_StayConsistent_When_PackRefsAndBranchDeleteRace60s` | Integration | `for-each-ref` equals expected, `fsck` clean |
| reflog | `session/git/backend/gogit/reflog_test.go` | `Reflog_should_MatchCLITwin_When_CommitAmendRenameResetCheckout` | Integration | Entry count and messages |
| repack | `session/git/backend/gogit/repackspike_test.go` | `ObjectLookup_should_RetryOnFreshHandle_When_RepackRunsConcurrently` | Integration | Zero `ErrObjectNotFound` after retry (S6) |
| repack | same | `ObjectLookup_should_RouteToCLI_When_RetryStillFails` | Unit | Reason `object_not_found` |

### Scope: network (fetch/push/clone, credential, ssh-fallback parity; Epics 3.1/3.2)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| credential helper | `session/git/backend/gogit/credential/credential_test.go` | `Client_should_ReturnUsernamePassword_When_HelperExecedWithGet` | Integration | osxkeychain and `gh auth git-credential`, zero `git` spawns |
| credential helper | `.../credential/provider_test.go` | `CredentialFor_should_ReturnGHEToken_When_HostIsGHE` | Unit | Host scoping; never the github.com token |
| credential helper | same | `CredentialFor_should_ExecConfiguredHelper_When_NoKeychainEntry` | Integration | Shim records 0 `git` execs |
| credential helper | same | `Provider_should_KillProcessGroup_When_HelperTimesOut` | Unit | Error path; `approve`/`reject` on result |
| GHE hosts | `.../credential/credential_test.go` | `Fetch_should_Succeed_When_PrivateRepoOnGitHubAndGHE` | Integration | Manual-gated live repos (needs credentials) |
| credential handling not weakened | `.../credential/transport_test.go` | `Fetch_should_DropAuthorization_When_CrossHostRedirect` | Integration | `httptest` A to B, redacted error |
| credential handling not weakened | `session/git/redact/redact_test.go` | `Git_should_StripUserinfoAndTokens_When_ErrorContainsTokenURL` | Unit | `ghp_` absent |
| credential handling | same | `Git_should_StripAuthorizationHeader_When_HeaderLineLogged` | Unit | Table incl. `Authorization:` |
| credential handling | `session/git/native/rollout_test.go` | `withOperationSpan_should_RedactBeforeRecordError_When_OperationFailsWithTokenURL` | Integration | In-memory exporter |
| credential handling | `session/git/backend/gogit/network_test.go` | `Clone_should_LeaveNoUserinfoInConfig_When_FailsAfterCloneBeforeCleanup` | Integration | No token in `remote.origin.url` or argv |
| ssh-fallback (HTTPS to SSH) | `.../credential/urlrewrite_test.go` | `Fetch_should_UseSSHURL_When_InsteadOfRewritesHTTPS` | Unit | Resolver over `gitconfig` `EffectiveConfig` |
| ssh-fallback | same | `Fetch_should_NotRetryOverSSH_When_FallbackSettingFalse` | Unit | Default off; error returned |
| ssh-fallback | same | `Fetch_should_RetryOnceViaSSH_When_FallbackSettingTrue` | Unit | Counted |
| ssh-fallback | same | `Fetch_should_RouteToCLI_When_HostHasProxyJump` | Unit | Reason `capability_ssh_proxy` |
| ssh-fallback | `.../credential/ssh_probe_test.go` | `SSHProbe_should_ListUnsupportedDirectives_When_ProxyCommandIdentityFileInclude` | Integration | G5 input |
| fetch/push/clone | `.../network_test.go` | `FetchAll_should_PruneRemoteTrackingRef_When_UpstreamBranchDeleted` | Integration | Equals CLI |
| fetch/push/clone | same | `FetchBranch_should_RejectRefspec_When_NameHasColonOrLeadingDash` | Unit | Error path |
| fetch/push/clone | same | `Push_should_ReturnErrNonFastForward_When_RemoteAhead` | Integration | Same typed error on both backends |
| fetch/push/clone | same | `Pull_should_RouteToCLI_When_RebaseConfiguredOrDiverged` | Unit | go-git only fast-forwards |
| fetch/push/clone | same | `Push_should_RouteToCLI_When_PrePushHookPresent` | Unit | ADR-006 |
| fetch/push/clone | same | `Router_should_RouteToCLI_When_FetchOrPushURLIsFileAfterInsteadOf` | Unit | `capability_local_transport` on both URLs |
| fetch/push/clone | `.../network_oracle_test.go` (tag `gitoracle`) | `Network_should_AgreeWithCLI_When_ServedByGitDaemon` | Integration | `git://127.0.0.1:<port>`: clone, fetch, push, delete-ref; `fsck` agrees |

### Scope: test fixtures, rollback path

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| fixtures use fork/go-git | `testutil/gitfixture/gitfixture_test.go` | `Fixture_should_BuildBareRemoteInProcess_When_TestNotAboutCLI` | Unit | `PlainInit` plus object writes, not a `file` clone |
| fixtures | same | `Fixture_should_PassFsck_When_WorktreeAddedInProcess` | Integration | Oracle check |
| rollback: cohort switch | `session/gitwiring/parse_test.go` | `ParseCohortMap_should_DefaultToCLI_When_KeyAbsent` | Unit | Happy path |
| rollback: cohort switch | same | `ParseCohortMap_should_WarnAndUseCLI_When_ModeInvalid` | Unit | Error path: `network=bogus` |
| rollback: cohort switch | same | `ParseCohortMap_should_ResolveLocalwriteToCLI_When_ShadowRequested` | Unit | No shadow for writes |
| rollback: env override | same | `ParseCohortMap_should_ForceAllCLI_When_EnvOverrideIsCLI` | Unit | `STAPLER_SQUAD_GIT_BACKEND=cli` |
| rollback: router | `session/git/backend/routed_test.go` | `ResolveRef_should_ReturnCLIResultAndCountFallback_When_GogitReturnsObjectNotFound` | Unit | `fallback_total{reason="object_not_found"}` +1 |
| rollback: router | same | `Router_should_ReturnErrNoRemoteRunner_When_RemoteRunnerNil` | Unit | Local runner count 0 |
| rollback: router | same | `Router_should_RunRemoteViaSSHRunnerWithoutLocalAccess_When_AllCohortsGogit` | Unit | Gogit double fails test if invoked |
| rollback: router | same | `Router_should_ClassifyMismatchRacyOrReal_When_ShadowResultsDiffer` | Unit | Pinned re-read |
| rollback: router | same | `Router_should_OrWroteAcrossScopes_When_MethodOpensTwoScopes` | Unit | No CLI replay |
| rollback: remote wiring | `session/git/backend/locate_test.go` | `Locate_should_BuildRemoteWithSameRunner_When_TargetIsSSH` | Unit | Same runner value |
| rollback: remote wiring | `session/instance_worktree_test.go` | `InstanceWorktree_should_RouteEveryGitCallThroughLocation_When_RemoteTarget` | Integration | Includes `:140` direct runner call |
| rollback: lint | `tools/lint/norawgitcli/analyzer_test.go` | `Analyzer_should_Flag_When_RawGitCallOutsideSanctionedPackages` | Unit | `testdata` covers `safeexec`, `runner.Run`, `commandRunner().Run` |
| rollback: lint | same | `Analyzer_should_Pass_When_CallInsideCLIBackendOrGitwiring` | Unit | |
| rollback: package topology | `session/git/backend/topology_test.go` | `ImportGraph_should_BeAcyclicAndRespectDepguard_When_NativeBackendGogitBuilt` | Unit | `go list` graph plus depguard allow-lists |
| rollback: gitconfig | `session/git/native/gitconfig/oracle_test.go` (tag `gitoracle`) | `Resolver_should_EqualGitConfigList_When_IncludeIfHasconfigAndSymlinkedParent` | Integration | Matrix vs `git config --list --show-origin --show-scope --includes -z` |
| rollback: gitconfig | `session/git/native/gitconfig/resolve_test.go` | `Resolver_should_FailClosed_When_IncludeUnresolvable` | Unit | `capability_detect_error` |
| rollback: capability | `session/git/native/capability_test.go` | `Capabilities_should_RouteCLI_When_ExecutableNonSampleHookExists` | Unit | Re-evaluated per mutating call |
| rollback: capability | same | `Capabilities_should_NotUseStaleCache_When_HookChmodedOrEdited` | Unit | Content-hash cache |
| rollback: capability | same | `Capabilities_should_RouteCLI_When_GpgsignOrLFSDriverOrSplitIndexPresent` | Unit | Reasons `capability_gpgsign`, `_lfs`, `_unsupported_index` |
| rollback: capability | same | `DepguardRule_should_BlockImportOfGogitConfigDecoder_When_InRoutingPackage` | Unit | |
| fork rollback | `scripts/fork-rollback-check.sh` | `forkRollback_should_PassMakeCI_When_ReplaceLineDeleted` | Integration | `go mod tidy && make ci` on upstream `v5.19.3`, every fork tag |
| fork wiring | `.github/workflows/build.yml` job | `replaceWiring_should_PassCIAndShowSingleGogit_When_ForkTagUsed` | Integration | `go mod graph` one go-git; `GOPRIVATE` plumbing |
| fork rebase | `docs/how-to/rebase-go-git-fork.md` rehearsal | `rebaseRunbook_should_ProduceTaggedForkAndGreenCI_When_NewUpstreamTag` | Integration | Includes census, oracle and soak |
| fork upstream watch | `.github/workflows/go-git-upstream-watch.yml` | `upstreamWatch_should_OpenIssue_When_NewTagOrAdvisory` | Integration | Dry-run via `workflow_dispatch` |

### Non-functional requirements

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Performance SLO (no slower than CLI) | `session/git/backend/gogit/bench_test.go` (tag `spike`) | `BenchmarkBackend/<op>/{cli,gogit}` | Integration | 30 iterations on 27 GB and 5 GB repos; p50 per op; G3 per-op flip |
| Performance SLO | same | `GateG3_should_KeepOpOnCLI_When_GogitP50WorseThanCLI` | Unit | Error path: slow op stays `cli` |
| Performance SLO | `session/git/backend/gogit/probe_test.go` | `Probe_should_ListDivergences_When_RenameCRLFShallowLFSFixtures` | Integration | Feeds golden tests and preflight |
| Performance SLO (ongoing) | `.github/workflows/soak.yml` | `perfRegression_should_FailNightly_When_P50ExceedsRecordedG3Baseline` | Integration | **Proposed; plan only measures at the spike (gap G-2)** |
| Scalability (dozens of sessions) | `session/git/backend/soak_test.go` (tag `soak`) | `Soak_should_FinishCleanly_When_24SessionsRunWithRepackAndCLIWriters5Min` | Integration | `fsck --strict` clean, prune empty, no `.lock`, RSS growth under 500 MB |
| Scalability | same | `Soak_should_Fail_When_TornReadObjectMissingRatioAboveG8Ceiling` | Unit | Error path |
| Scalability | same | `Soak_should_Fail_When_DetectErrorAbove1PercentOrLockOutlivesScope` | Unit | |
| Scalability | same | `Soak_should_RecoverOthers_When_OneWriterSigkilled` | Integration | Other 23 sessions complete with no lock error |
| Security (credentials, internal) | covered above (redactor, redirect, no-token-in-config, helper exec) | | | See network section |
| Security: supply chain | `.github/workflows/go-git-upstream-watch.yml` | see fork upstream watch row | Integration | GHSA feed |

### Constraints

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Existing auth: keychain, helper | see network section (`provider_test.go`, `credential_test.go`) | | | |
| ssh-fallback behaviour | see ssh-fallback rows | | | |
| GitHub Enterprise hosts | `.../credential/provider_test.go` | `CredentialFor_should_ReturnGHEToken_When_HostIsGHE` | Unit | |
| Fork rebased on upstream releases | see fork rebase and upstream watch rows; `session/git/backend/gogit/censustest_test.go` | `TestIndexCallSiteCensus_should_Fail_...` | Unit | Fails a rebase that changes index call sites |
| macOS and Linux | `.github/workflows/build.yml` matrix | `oracleAndLockTests_should_PassOnLinuxAndMacOS_When_Run` | Integration | **Proposed matrix addition**: xattr behaviour (ext4, tmpfs, APFS) differs |
| Linked-worktree semantics match CLI | see worktree scope rows | | | |

### Observability and risk control

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Count and log every CLI fallback with operation | `session/git/backend/routed_test.go` | `Router_should_IncrementFallbackCounterWithOperationCohortReason_When_FallingBack` | Unit | `git_backend_fallback_total` |
| Count every fork-path error with operation name | same | `Router_should_IncrementErrorCounter_When_GogitFailsWithoutFallback` | Unit | `git_backend_error_total{operation,implementation}` |
| Counters on existing metrics pipeline | `session/git/native/rollout_test.go` | `Histogram_should_RecordAllOpsOver100ms_When_OperationRuns` | Unit | Existing `git_operation_duration_ms`, no second histogram |
| Logs redacted, with required fields | `session/git/backend/routed_test.go` | `FallbackLog_should_ContainOpCohortReasonAndPassRedactor_When_Logged` | Unit | |
| Capability detect ratio observable | `session/git/native/capability_test.go` | `Capabilities_should_EmitDetectTotalWithOutcome_When_Called` | Unit | `git_capability_detect_total` |
| Lock recovery counters | `session/git/backend/gogit/lockjournal_test.go` | `Recovery_should_EmitOutcomeCounter_When_LockExamined` | Unit | `git_lock_stale_recovered_total` |
| Staged rollout, `shadow` soak | `docs/how-to/flip-git-backend-cohort.md` checklist | `refsShadowWindow_should_BlockFlip_When_RealMismatchOrDetectErrorOver1Percent` | Integration (manual, counters) | 7 days, day-one capability check |
| Promotion gate | `config/git_backend_test.go` | `Promote_should_Refuse_When_CriteriaUnmetOrFallbackErrorNonZero` | Unit | Mechanical checklist |
| Default flip order | `config/git_backend_test.go` | `Defaults_should_FlipInOrderRefsDiffstatusLocalwriteNetworkWorktree_When_Released` | Unit | `cli` stays selectable |
| CLI not deleted until a release cycle | `session/git/backend/routed_test.go` | `Retirement_should_KeepCLIBackendAndCarveOutRoutes_When_FlagRemoved` | Unit | Story 6.2.1 |

### Migration (Step 5)

The plan has no database schema. Its Migration Plan has two reversible additive changes: the `git_backend_cohorts` config key and the `go.mod` `replace` line. One reversal test per change, named for the required `migration_should_be_reversible`:

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Config migration reversible | `config/config_test.go` | `migration_should_be_reversible_When_GitBackendCohortsKeyAddedThenRemoved` | Migration | Up: key added, parsed cohorts apply. Down: key removed, every cohort `cli`; existing config not rewritten on load |
| `go.mod` migration reversible | `scripts/fork-rollback-check.sh` | `migration_should_be_reversible_When_ReplaceLineAddedThenRemoved` | Migration | Up: `replace` to fork tag, build passes. Down: line deleted, `go mod tidy && make ci` pass on upstream tag |
| On-disk repo state unchanged | `session/git/backend/gogit/*_oracle_test.go` | `Oracle_should_ReadStateWithRealGit_When_WrittenInProcess` | Migration | `git fsck`, `git worktree list`; no format change |

## UX Acceptance Tests
Omitted: no user-facing surface (no `design/ux.md`; session behaviour is explicitly out of scope for change).

## Test Stack
- **Unit**: Go `testing` plus `testify`; table-driven; `-race` on `session/git/backend/...`. Fake gogit and cli backends for the router; analyzers tested with `analysistest` (`tools/lint` is its own module).
- **Integration**: real git repos built in `t.TempDir()`, real CLI used only as `OracleTest` (tag `gitoracle`), `httptest` for credential and redirect tests, `git daemon` for network oracle, billy filesystem decorator for fault injection, injected fake clock for lock recovery.
- **Soak/perf**: tag `soak` (nightly, `make test-soak`), tag `spike` for `BenchmarkBackend`, `go test -mutexprofile` for SM-3.
- **E2E**: `tests/no-git-container` (Docker, `debian:stable-slim` by digest) driving the ConnectRPC API; PATH-shim run `scripts/spawn-gate.sh` over `./...`.
- **Manual**: live-GitHub and GHE fetch (credentials), 7-day shadow window, maintainer first-day dogfood checklist.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./session/git/... ./session/gitwiring/... ./executor/safeexec/... ./testutil/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | at least 80% line on new packages (`backend`, `backend/gogit`, `backend/cli`, `native/gitconfig`, `redact`, `gitwiring`); no drop elsewhere |
| Gates | `make ready` plus `scripts/spawn-gate.sh` | zero un-allow-listed shim hits; allow-list sizes at or below baseline |

- All public `Backend` methods: happy path, error path, and a `RunBoth` differential case.
- All external integrations (credential helper, HTTPS transport, SSH, `git daemon`, Docker): unit with doubles plus at least one integration test.
- Every operation that can write has a CLI-twin parity test before it leaves `cli` (the plan's promotion criterion).

## Coverage Summary and Gaps

Every `requirements.md` success metric, in-scope item, constraint, NFR, observability and risk-control requirement has at least one mapped test above, against both the original and the plan's reworded metrics. Gaps and weaknesses found:

- **G-1 (SM-3 mutex delay)**: Story 0.1.2 records baseline "summed mutex delay", but no later story compares it. Story 5.1.2 checks wall time only. Added proposed test `sessionMutexDelay_should_NotExceedBaseline_When_BatchMerged`; needs a plan task (not edited, per instructions).
- **G-2 (performance SLO after flip)**: the SLO is checked once, at spike S3 (Story 0.2.3). Nothing re-checks per-op p50 on real large repos after cohorts flip or on a fork rebase. Proposed nightly regression test above.
- **G-3 (SM-1 and SM-2 wording)**: the original "zero spawns" and "CLI fallbacks removed" are not testable as written, because the plan keeps the `cli` backend permanently and allow-lists carve-outs (`gh pr create`, remote hosts, hooked/signed/LFS repos, `unsafe_worktree_write` ops, local-path remotes, `torn_read`). The tests prove "zero outside the allow-list, and the allow-list only shrinks". If Tyler wants the original wording, O-1/O-9/O-10 must be decided and many of these carve-outs retired (ADR-003 narrowing noted in R3-6).
- **G-4 (live-credential tests)**: GitHub and GHE fetch with real keychain credentials cannot run in CI; stays manual (G5 spike evidence).
- **G-5 (platform)**: xattr-based lock-token recovery behaves differently on APFS, ext4 and tmpfs; plan asserts `ENOTSUP` handling but does not add a macOS plus Linux CI matrix. Proposed above.
- **G-6 (no-git container coverage)**: container flow drives a hand-picked path and not Playwright; methods with zero hits are reported, not tested. SM-4 is therefore only proven for the exercised methods plus the shim layer.
- **G-7 (test names)**: `plan.md` names only `TestIndexCallSiteCensus` and `TestSpawnAllowlistRatchet`; every other name above is proposed here and needs adopting at implementation time.
