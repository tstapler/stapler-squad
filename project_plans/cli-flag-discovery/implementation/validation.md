# Validation Plan: cli-flag-discovery

**Date**: 2026-09-21

Derived from `plan.md` (task IDs in brackets), `../requirements.md` (AC1-AC10) and `../design/ux.md` (UX-1..UX-42 plus the 5 S6 vocabulary criteria). Type names follow the plan's Domain Glossary. No fake `CommandExecutor` and no sleeping-script binaries: `Prober` tests use injected function fakes (`lookPath`, `run`, `stat`); fakes include an injected `readHead` for the native-vs-script gate; runner tests re-exec the test binary through the unexported `runWith` seam (tasks 1.1.2b-c). No `time.Sleep`; use `require.Eventually` and a fake clock.

## Happy Path Scenario
Given Program Config with a saved-form command field and a server whose PATH contains `claude` (baseline: today a typo surfaces only when the tmux session fails), when the user types `claude`, blurs the field, then types `--mo` in Default CLI Flags, then presses Down and Enter, then the badge reads "Found: /usr/bin/claude", the listbox offers `--model` with its description, and the value becomes `--model `.

## Requirement → Test Mapping

Go tests live in `config/clihelp/*_test.go` (package `clihelp`), `server/services/defaults_service_probe_test.go`, `server/middleware/probeguard_test.go`, `server/server_probeguard_test.go`. Jest tests live beside their components. Integration = real `runWith` child process, real `Prober` wired into a real handler, or real Connect handler behind the guard.

### AC1: `ProbeProgram(command)` RPC returns found, resolved_path, flags

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-AC1 | defaults_service_probe_test.go | TestProbeProgram_should_MapFoundPathAndFlags_When_ProberReturnsFoundParsed | Unit | Happy path |
| REQ-AC1 | defaults_service_probe_test.go | TestProbeProgram_should_ReturnInvalidArgument_When_CommandEmpty | Unit | Error path (only RPC error) |
| REQ-AC1 | defaults_service_probe_test.go | TestProbeResultToProto_should_DeriveFoundFromStatus_When_EachProbeStatus | Unit | Table: FOUND_PARSED/FOUND_NO_FLAGS/TIMEOUT/NEEDS_CONFIRM true; NOT_FOUND/ERROR/BUSY false |
| REQ-AC1 | defaults_service_probe_test.go | TestProbeProgram_should_MapAliasesWrapperAndTruncated_When_ProberReturnsThem | Unit | Field mapping |
| REQ-AC1 | defaults_service_probe_test.go | TestProbeProgram_should_ReturnAiderFlagShape_When_RealProberWithFakeRunAndAiderFixture | Integration | Real `Prober` + real handler; asserts `--model`, `takes_value`, description (task 1.2.2e) |

### AC2: LookPath on first token, `~` expanded, missing is `found=false` not an RPC error

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-AC2 | resolve_test.go | TestResolve_should_SkipEnvAssignmentsAndExpandTilde_When_EnvPrefixAndTildePath | Unit | Happy path (`FOO=1 "~/my bin/x" --y` -> `/h/my bin/x`) |
| REQ-AC2 | resolve_test.go | TestResolve_should_ReturnResolveError_When_EmptyOnlyEnvUnbalancedOrLeadingDash | Unit | Error path table |
| REQ-AC2 | prober_test.go | TestProbe_should_ReturnNotFoundAndNeverRun_When_LookPathFails | Unit | Error path; nothing cached |
| REQ-AC2 | prober_test.go | TestProbe_should_CallLookPathWithExpandedHome_When_TildeCommandAndWithHome | Unit | `WithHome("/home/tyler")` |
| REQ-AC2 | prober_test.go | TestProbe_should_TreatErrDotAsNotFound_When_LookPathReturnsErrDot | Unit | Edge |
| REQ-AC2 | loginpath_test.go | TestDeriveLoginPath_should_PutShellDirsFirstAndDedupe_When_StubZshEchoesPath | Unit | Login-shell PATH (tasks 1.1.4d-e) |
| REQ-AC2 | loginpath_test.go | TestDeriveLoginPath_should_NotCacheFailureAndRetryAfter30s_When_ShellTimesOut | Unit | Error path, fake clock |
| REQ-AC2 | loginpath_test.go | TestDeriveLoginPath_should_UseServerPathPlusFallbackDirs_When_ShellUnsetOrBashReturnsEarly | Unit | Fallback |
| REQ-AC2 | prober_test.go | TestProbe_should_FindBinary_When_OnlyInLoginShellPathAndReportNotFoundWhen_AliasOnly | Unit | PATH semantics |
| REQ-AC2 | loginpath_test.go | TestDeriveLoginPath_should_ReturnWithinTimeoutAndKillGroup_When_ShellBackgroundsSleep | Integration | Real child (stub shell); background helper cannot hold the pipe |
| REQ-AC2 | loginpath_test.go | TestDeriveLoginPath_should_UseOnlySentinelSpan_When_RcPrintsBannerToStdout | Unit | Stub shell prints banner/p10k text before the markers; only the span's dirs are used |
| REQ-AC2 | loginpath_test.go | TestDeriveLoginPath_should_TreatMissingMarkersAsFailureAndNotCache_When_StubPrintsNoSentinels | Unit | Error path |
| REQ-AC2 | defaults_service_probe_test.go | TestNewDefaultsService_should_SpawnNoProcessAndNoGoroutine_When_Constructed | Unit | `goleak`; only `StartLoginPathDerivation()` starts derivation (pre-mortem #4) |
| REQ-AC2 | defaults_service_probe_test.go | TestProbeProgram_should_ReturnNotFoundWithNilError_When_TildePathMissing | Integration | Real `Prober` + handler: `FOO=1 ~/bin/nope --x` -> `found=false`, err nil |

### AC3: `--help` with 3s timeout, stdin closed, 256KB cap, no shell, cache by path+mtime

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-AC3 | capwriter_test.go | TestCapWriter_should_KeepFirstNBytesAndReturnFullLen_When_WriteExceedsCap | Unit | Happy path |
| REQ-AC3 | capwriter_test.go | TestCapWriter_should_CallOnOverflowOnce_When_ManyWritesOverflow | Unit | Error path |
| REQ-AC3 | runner_test.go | TestDefaultLimits_should_Be3sAnd256KiB_When_Constructed | Unit | Happy path (spec'd constants) |
| REQ-AC3 | runner_test.go | TestHelpSpec_should_YieldExactlyHelpArgAndNilExtraEnv_When_Built | Unit | No shell, only `--help` |
| REQ-AC3 | prober_test.go | TestProbe_should_CallRunOnce_When_SecondProbeHasUnchangedMtime | Unit | Cache hit |
| REQ-AC3 | prober_test.go | TestProbe_should_CallRunAgain_When_StatMtimeChanged | Unit | Cache invalidation |
| REQ-AC3 | prober_test.go | TestProbe_should_ExpireTimeoutResultAfter60sAndParsedAfter10min_When_ClockAdvances | Unit | TTLs (TIMEOUT lengthened to 60s) |
| REQ-AC3 | prober_test.go | TestProbe_should_BypassCachedTimeout_When_ConfirmExecuteCheck | Unit | Explicit Check retries a cached TIMEOUT |
| REQ-AC3 | prober_test.go | TestProbe_should_ApplySlowToolLimits_When_BasenameInSlowToolsOnlyIfAdded | Unit | `limitsFor` override; other tools get 3s (present only if Task 1.1.3a adds `slowTools` entries) |
| REQ-AC3 | prober_realbin_test.go | TestProbe_should_ReturnFoundParsed_When_RealClaudeAiderOrGeminiInstalled | Integration | Real binary with `ConfirmExecute`; `t.Skip` when none installed; guards the measured-timeout decision (Task 1.1.3a) |
| REQ-AC3 | prober_test.go | TestProbe_should_NeverCacheNotFoundErrorOrBusy_When_Probed | Unit | Error path |
| REQ-AC3 | prober_test.go | TestProbe_should_PassConfiguredLimitsToRun_When_WithLimits | Unit | Limits plumbing |
| REQ-AC3 | prober_test.go | TestProbe_should_CoalesceToOneRunAndReturnRealResultToAll_When_10ConcurrentSameBinaryUnderRace | Unit | Singleflight, leader-only slot |
| REQ-AC3 | prober_test.go | TestProbe_should_ReturnBusyAndNotCacheIt_When_TwoSlotsHeldByDistinctBinaries | Unit | Semaphore; retry succeeds after release |
| REQ-AC3 | prober_test.go | TestProbe_should_NotFreeSlot_When_CallersCancelButFlightStillRunning | Unit | Cancelled callers keep the slot |
| REQ-AC3 | prober_test.go | TestProbe_should_ReturnRealResultOnReprobe_When_FirstCallerCancelledMidFlight | Unit | Cancel-then-reprobe, follower-of-cancelled-leader |
| REQ-AC3 | prober_test.go | TestProbe_should_ExitFlightGoroutine_When_AllCallersCancelled | Unit | goleak / Eventually, no sleeps |
| REQ-AC3 | prober_test.go | TestProbe_should_SkipStoringInCache_When_MtimeChangedDuringRun | Unit | TOCTOU re-stat |
| REQ-AC3 | runner_test.go | TestRunWith_should_TruncateTo262144AndReturnPromptly_When_Helper_bigout | Integration | Real child, 1MB to stdout and stderr |
| REQ-AC3 | runner_test.go | TestRunWith_should_KillEarlyUnder1s_When_Helper_flood_Timeout2s | Integration | Overflow kill |
| REQ-AC3 | runner_test.go | TestRunWith_should_SetTimedOutWithin500msAndGroupGone_When_Helper_hang_Timeout80ms | Integration | Hard timeout (ignores SIGTERM) |
| REQ-AC3 | runner_test.go | TestRunWith_should_KillGroupAfterWait_When_Helper_orphan_LeavesGrandchild | Integration | `require.Eventually` on `Kill(-pid,0)==ESRCH` |
| REQ-AC3 | runner_test.go | TestRunWith_should_ReturnErrorNotSuccess_When_ContextCancelled | Integration | Cancel is not partial success |
| REQ-AC3 | runner_test.go | TestRun_should_ReachRealProcess_When_ExportedRunAgainstTestBinaryWithHelp | Integration | Production `Run` smoke; output contains `-test.run` |
| REQ-AC3 | runner_test.go | TestRunWith_should_RunStdinClosedAndOwnSession_When_Helper_sid | Integration | `Getsid(0)==Getpid()`, no controlling tty |

### AC4: Parser handles GNU/argparse/cobra/clap; unparseable yields empty flags, never an error

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-AC4 | parser_test.go | TestParseHelp_should_ReturnAssigneeShortAndValue_When_GhPrListFixture | Unit | Happy path (cobra) |
| REQ-AC4 | parser_test.go | TestParseHelp_should_ReturnAliasOnOneEntry_When_ClaudeFixtureAllowedTools | Unit | `--allowedTools` alias `--allowed-tools` (commander) |
| REQ-AC4 | parser_test.go | TestParseHelp_should_ParseModelTakesValue_When_AiderFixture | Unit | argparse |
| REQ-AC4 | parser_test.go | TestParseHelp_should_MeetMinCountAndNameRegex_When_RgUvGeminiAgyFixtures | Unit | clap, yargs, Go flag |
| REQ-AC4 | parser_test.go | TestParseHelp_should_StripAnsiAndOverstrike_When_ColouredVariantFixture | Unit | ANSI variant |
| REQ-AC4 | parser_test.go | TestParseHelp_should_ReturnEmptySliceAndNoError_When_TmuxUsageErrorOrGitFixture | Unit | Error path (negatives) |
| REQ-AC4 | parser_test.go | TestParseHelp_should_ExpandNoBracketForm_When_LongFlagHasBracketNo | Unit | `--[no-]long` |
| REQ-AC4 | parser_test.go | TestParseHelp_should_CapFlagsAt500AndDescriptionAt300_When_HugeInput | Unit | Caps |
| REQ-AC4 | parser_fuzz_test.go | FuzzParseHelp_should_NotPanicAndKeepNameInvariant_When_ArbitraryBytes | Unit | Fuzz error path |
| REQ-AC4 | parser_fuzz_test.go | TestParseHelp_should_ReturnEmptyWithin100ms_When_256KBSingleLineOfDashes | Unit | Adversarial |
| REQ-AC4 | parser_fuzz_test.go | TestParseHelp_should_ReturnWithin100ms_When_10kTinyLines | Unit | Adversarial |
| REQ-AC4 | defaults_service_probe_test.go | TestProbeProgram_should_ReturnFoundNoFlagsAndEmptyFlags_When_RealProberAndUnparseableHelp | Integration | Real prober; status `FOUND_NO_FLAGS`, no error |

### AC5: Program Config "binary found / not found" on command blur

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-AC5 | useProbeProgram.test.ts | useProbeProgram_should_SetFoundState_When_CheckResolvesFound | Unit | Happy path |
| REQ-AC5 | useProbeProgram.test.ts | useProbeProgram_should_IgnoreStaleResponse_When_FirstProbeResolvesLast | Unit | Deferred promises out of order |
| REQ-AC5 | useProbeProgram.test.ts | useProbeProgram_should_MapToTransportError_When_RpcRejectsOrPermissionDenied | Unit | Error path (`Code.PermissionDenied`, 403-shaped) |
| REQ-AC5 | useProbeProgram.test.ts | useProbeProgram_should_MapToBusyOrError_When_StatusErrorOrBusy | Unit | Error path, never `notFound` |
| REQ-AC5 | useProbeProgram.test.ts | useProbeProgram_should_AbortAndSkip_When_UnmountOrEmptyCommand | Unit | Lifecycle |
| REQ-AC5 | useProbeProgram.test.ts | useProbeProgram_should_MemoizeFoundButNotErrors_When_SameCommandRechecked | Unit | Memo |
| REQ-AC5 | ProbeStatusBadge.test.tsx | ProbeStatusBadge_should_RenderFoundPathAndCheckedOnServerOnly_When_Found | Unit | Copy (D2) |
| REQ-AC5 | ProbeStatusBadge.test.tsx | ProbeStatusBadge_should_RenderAliasAwareNotFoundCopy_When_NotFound | Unit | Error path |
| REQ-AC5 | ProbeStatusBadge.test.tsx | ProbeStatusBadge_should_RenderCouldntCheckWithRetry_When_TransportOrBusyAndNeverNotFound | Unit | Error path |
| REQ-AC5 | ProbeStatusBadge.test.tsx | ProbeStatusBadge_should_RenderNoFlagsTimeoutAndWrapperVariants_When_Given | Unit | Variants; TIMEOUT reads "Timed out reading flags — try Check again", distinct from the no-flags copy |
| REQ-AC5 | ProbeStatusBadge.test.tsx | ProbeStatusBadge_should_RenderNotRunYetCopyAndCheckButton_When_NeedsConfirm | Unit | `needsConfirm` variant, 44px `Check` calls `onConfirm` |
| REQ-AC5 | ProgramsManager.test.tsx | ProgramsManager_should_ShowFoundStatus_When_CommandBlurs | Unit | Happy path |
| REQ-AC5 | ProgramsManager.test.tsx | ProgramsManager_should_RunCheckAndNotSubmit_When_EnterPressedInCommandField | Unit | D1 |
| REQ-AC5 | ProgramsManager.test.tsx | ProgramsManager_should_KeepSaveEnabled_When_NotFoundOrTransportError | Unit | Error path |
| REQ-AC5 | cli-flag-discovery.spec.ts | programs_settings_should_ShowNotRunYetOnBlurThenFlagsAfterCheckAndNotFoundForMissingPath_When_FixtureScript | Integration | Playwright vs isolated server, real RPC, real fixture script (shebang): blur -> "Not run yet", marker file absent; Check -> marker present, flags listed; missing path -> "Not found" |

### AC6: Flag autocomplete, non-blocking unknown-flag warning, description per flag

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-AC6 | flagTokens.test.ts | tokenAtCaret_should_ReturnTokenBounds_When_CaretMidString | Unit | Happy path |
| REQ-AC6 | flagTokens.test.ts | applyCompletion_should_AppendTrailingSpaceNoEquals_When_FlagTakesValue | Unit | Happy path |
| REQ-AC6 | flagTokens.test.ts | filterFlags_should_MatchNameShortAndAliasesPrefixThenSubstring_When_TextGiven | Unit | Happy path |
| REQ-AC6 | flagTokens.test.ts | tokenAtCaret_should_ReturnNull_When_TokenNotDashOrAfterDoubleDash | Unit | Error path |
| REQ-AC6 | validateFlags.test.ts | validateFlags_should_ReturnOnlyVerbos_When_MixedKnownEqualsBundledNegationAndTerminator | Unit | Happy path |
| REQ-AC6 | validateFlags.test.ts | validateFlags_should_ReturnEmpty_When_ZeroKnownFlagsOrWrapper | Unit | Error path (suppression), Proxy entry |
| REQ-AC6 | validateFlags.test.ts | validateFlags_should_AcceptAlias_When_AllowedToolsKebabForm | Unit | Alias |
| REQ-AC6 | useListboxNav.test.ts | useListboxNav_should_WrapOnDownUpAndCloseOnEscape_When_KeysPressed | Unit | Reducer |
| REQ-AC6 | FlagCombobox.test.tsx | FlagCombobox_should_SetAriaAttributesAndActiveDescendant_When_DownPressed | Unit | Happy path |
| REQ-AC6 | FlagCombobox.test.tsx | FlagCombobox_should_InsertModelAndShowInlineDescription_When_DownThenEnter | Unit | Happy path |
| REQ-AC6 | FlagCombobox.test.tsx | FlagCombobox_should_RenderPlainInputNoListbox_When_FlagsEmpty | Unit | Error path |
| REQ-AC6 | FlagCombobox.test.tsx | FlagCombobox_should_KeepSameInputNodeAndFocus_When_FlagsArriveLate | Unit | No remount |
| REQ-AC6 | FlagCombobox.test.tsx | FlagCombobox_should_CloseWithoutClearing_When_EscapePressed | Unit | Exit path |
| REQ-AC6 | ProgramsManager.test.tsx | ProgramsManager_should_ShowWarningDescribedByAndNotInvalid_When_VerbosTyped | Unit | Happy path (warning) |
| REQ-AC6 | ProgramsManager.test.tsx | ProgramsManager_should_ShowNoSuggestionsOrWarning_When_NotFoundZeroFlagsOrWrapper | Unit | Error path |
| REQ-AC6 | ProgramsManager.test.tsx | ProgramsManager_should_ShowOnlySoftWarningAndKeepSaveEnabled_When_KnownValidFlagNotInParsedSet | Unit | Parser false positive (pre-mortem #5): flag absent from probe output yields only the "is not listed ... may still work" text, no `role=alert`, no `aria-invalid`, Save enabled |
| REQ-AC6 | cli-flag-discovery.spec.ts | programs_settings_should_ListAlphaSuggestion_When_TypingAlpInFlagsAfterFoundFixture | Integration | Playwright, real probe of `probe-fixture.sh` |

### AC7: Session creation shows the same badge and flag validation (scoped to saved `cli_flags`)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-AC7 | useAvailablePrograms.test.ts | useAvailablePrograms_should_CarryCommandAndCliFlags_When_ListProgramsConfigMapped | Unit | Happy path (task 2.3.1a) |
| REQ-AC7 | useAvailablePrograms.test.ts | useAvailablePrograms_should_KeepStaticProgramsWorking_When_CommandFieldsAbsent | Unit | Error/compat path |
| REQ-AC7 | ProgramProbeSection.test.tsx | ProgramProbeSection_should_ShowResolvedPathBadge_When_ProgramFound | Unit | Happy path |
| REQ-AC7 | ProgramProbeSection.test.tsx | ProgramProbeSection_should_ListOnlyBogus_When_SavedFlagsYesAlwaysAndBogus | Unit | Saved-flag warning |
| REQ-AC7 | ProgramProbeSection.test.tsx | ProgramProbeSection_should_ShowNotFoundBadgeWithoutDuplicateWarning_When_BinaryMissing | Unit | Error path, `preset-program-warning` testid kept |
| REQ-AC7 | ProgramProbeSection.test.tsx | ProgramProbeSection_should_ShowNothing_When_TransportErrorOrWrapperOrBusy | Unit | Error path (transport/busy show the "Couldn't check right now." badge, no flag warning) |
| REQ-AC7 | ProgramProbeSection.test.tsx | ProgramProbeSection_should_SendResolveOnlyOnSelectionAndConfirmOnlyOnCheck_When_SelectionChangesThenCheckClicked | Unit | Picker never executes on selection (`resolve_only:true`, no `confirm_execute`); `Check` sends `confirm_execute:true` |
| REQ-AC7 | OmnibarCreationPanel.test.tsx | OmnibarCreationPanel_should_RenderProgramProbeSectionAndNoOldSpan_When_ProgramSelected | Integration | Panel with real section, mocked RPC; diff gains one element |

### AC8: Security (only command being configured, sanitized env, only `--help`, timeout and oversize tests)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-AC8 | resolve_test.go | TestResolve_should_KeepLiteralClaudeSemicolonAndNoArgs_When_ShellMetacharacters | Unit | Happy path (no shell) |
| REQ-AC8 | resolve_test.go | TestResolve_should_ReturnRelativePath_When_DotSlashBinSlashDotDotOrTildeUser | Unit | Error path |
| REQ-AC8 | resolve_test.go | TestResolve_should_FlagWrapper_When_EnvNpxOrBuiltinProxyCommand | Unit | Wrapper detection |
| REQ-AC8 | prober_test.go | TestProbe_should_ReturnNotFound_When_DirectoryNonExecutableOrWorldWritable | Unit | Error path |
| REQ-AC8 | prober_test.go | TestProbe_should_NeverRun_When_WrapperCommand | Unit | `env ... claude`, `npx` |
| REQ-AC8 | prober_test.go | TestProbe_should_RunHelpOnlyOnFirstToken_When_CommandHasDangerousArgs | Unit | First token only |
| REQ-AC8 | prober_test.go | TestProbe_should_EmitExactlyOneProgramProbeLog_When_EachOutcome | Unit | slog capture; no env values/args |
| REQ-AC8 | runner_test.go | TestProbeEnv_should_DropGithubTokenAndAnthropicKey_When_ParentHasThem | Unit | Sanitized env |
| REQ-AC8 | runner_test.go | TestRunWith_should_DropExtraEnvWithoutTestPrefix_When_Given | Unit | Seam confined |
| REQ-AC8 | probeguard_test.go | TestProbeGuard_should_Return405_When_MethodGet | Unit | Guard error path |
| REQ-AC8 | probeguard_test.go | TestProbeGuard_should_Return403_When_HostOrOriginNotLoopbackOrAllowed | Unit | Guard error path |
| REQ-AC8 | probeguard_test.go | TestProbeGuard_should_Pass_When_LoopbackHostV4V6AndAllowedOrOmittedOrigin | Unit | Guard happy path |
| REQ-AC8 | probeguard_test.go | TestProbeGuard_should_HonorLateSetOriginsAndHostnames_When_ReadLazily | Unit | Lazy config |
| REQ-AC8 | probeguard_test.go | TestProbeGuard_should_Return403ForEveryRequest_When_NonLoopbackBind | Unit | Bind guard |
| REQ-AC8 | probeguard_test.go | TestProbeGuard_should_Pass_When_HostPublishedViaSetHostnames | Unit | LAN/Tailscale hostname on `:8543` allowed (pre-mortem #3); UI shows "Couldn't check right now." (never "not found") when the guard refuses, per the AC5 `PermissionDenied` hook row |
| REQ-AC8 | prober_test.go | TestProbe_should_ReturnNeedsConfirmAndNeverRun_When_ShebangScriptImplicitOrResolveOnly | Unit | Execution gate; `run` not called on blur or `ResolveOnly` (task 1.1.4f-g) |
| REQ-AC8 | prober_test.go | TestProbe_should_RunOnceAndRemember_When_ConfirmExecuteThenImplicitProbeOfSameKey | Unit | Confirmed set; changed mtime returns `NEEDS_CONFIRM` again |
| REQ-AC8 | prober_test.go | TestProbe_should_RunNativeImplicitly_When_ElfOrMachoHead | Unit | Native magic table (ELF, Mach-O, fat) |
| REQ-AC8 | prober_confirm_test.go | TestProbe_should_NotCreateMarker_When_ScriptProbedImplicitlyAndCreateItAfterConfirm | Integration | Real `Run` and script that touches a marker file (task 1.1.4g) |
| REQ-AC8 | defaults_service_probe_test.go | TestProbeProgram_should_PassConfirmExecuteAndResolveOnlyToProber_When_SetOnRequest | Unit | Field plumbing; `NEEDS_CONFIRM` maps to `found=true` in the `probeResultToProto` table |
| REQ-AC8 | probeguard_test.go | TestProbeGuard_should_LeaveOtherProceduresUntouched_When_DifferentPath | Unit | Scope |
| REQ-AC8 | runner_test.go | TestRunWith_should_ReportNoLeakedSecretsAndEmptyTempCwd_When_Helper_printenv_PoisonedParentEnv | Integration | Real child; control asserts poison present in `parentEnv` and `HOME` arrived |
| REQ-AC8 | runner_test.go | TestRunWith_should_RemoveTempCwd_When_RunReturns | Integration | Cwd cleanup |
| REQ-AC8 | probeguard_test.go | TestProbeGuard_should_NotReachService_When_WrongHostPostAgainstRealConnectHandler | Integration | Real Connect handler behind guard |
| REQ-AC8 | server_probeguard_test.go | TestStartChain_should_403WrongHostAnd_Pass_LocalhostHost_When_AuthMiddlewareNil | Integration | `Start()` chain |
| REQ-AC8 | server_probeguard_test.go | TestRemoteChain_should_NotContainGuard_When_HostIsOnyxAt8444WithAuth | Integration | `StartRemote()` chain |
| REQ-AC8 | server_probeguard_test.go | TestStartChain_should_HonorOriginsSetAfterConstruction_When_SetOriginsCalledLater | Integration | Lazy read |
| REQ-AC8 | cli-flag-discovery.spec.ts | probe_endpoint_should_Return403_When_PostWithHostEvilExample | Integration | Playwright `request.post` |

### AC9: Works on mobile (tap-to-show, no hover dependency)

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-AC9 | FlagInfoButton.test.tsx | FlagInfoButton_should_ExpandInlineWithAriaExpandedTrue_When_Clicked | Unit | Happy path, no `mouseover` |
| REQ-AC9 | FlagInfoButton.test.tsx | FlagInfoButton_should_CollapseAndRenderNothing_When_SecondClickOrNoDescription | Unit | Error/edge path |
| REQ-AC9 | FlagInfoButton.test.tsx | FlagInfoButton_should_UseMinSize44Tokens_When_Rendered | Unit | 44px via style tokens |
| REQ-AC9 | FlagCombobox.test.tsx | FlagCombobox_should_SelectOnMouseDownWithoutBlurReprobe_When_OptionTapped | Unit | Touch selection |
| REQ-AC9 | cli-flag-discovery.spec.ts | mobile_should_TapInfoButtonAndShowDescription44px_When_Viewport375x667HasTouch | Integration | AC9 proof: `aria-expanded="true"`, bounding box >= 44x44 |

### AC10: Go parser fixtures (claude/aider/git-style) and jest component tests

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-AC10 | parser_test.go | (AC4 rows: claude, aider, gh, gh-pr-list, rg, uv, gemini, agy, negatives git, tmux) | Unit | Fixtures committed under `config/clihelp/testdata/help/` |
| REQ-AC10 | (gate) | go test ./config/clihelp ./server/services ./server/middleware -race | Integration | Exit 0 (task 5.2.1a) |
| REQ-AC10 | (gate) | pnpm exec jest --testPathPatterns="useProbeProgram\|ProbeStatusBadge\|ProgramProbeSection\|FlagCombobox\|ProgramsManager\|FlagInfoButton\|flagTokens\|validateFlags" --no-coverage | Integration | Exit 0, run in `web-app/` |

## UX Acceptance Tests

Every criterion in `design/ux.md` has one row (47 = UX-1..UX-42 plus S6-1..S6-5; UX-21 is withdrawn and keeps a negative test). "Jest" means React Testing Library in the named file; "Playwright" means `tests/e2e/cli-flag-discovery.spec.ts` (data-testid/ARIA locators only, no `waitForTimeout`); "Manual" is a numbered checklist item on the dev instance (`PORT=62871`, `STAPLER_SQUAD_INSTANCE=claude-manual-test`, never the live `:8543`).

Plan decisions applied: D4 dropped, so UX-21 is withdrawn (a negative test remains) and UX-30/UX-31 are validated on what remains. `design/ux.md` S6/S7 were aligned to the plan: UX-34 now states Info for every outcome and Warn for BUSY, and the test asserts exactly that.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| S6-1: each `ProbeUiState` maps to exactly one non-empty row | ProbeStatusBadge.test.tsx | ProbeStatusBadge_should_RenderNonEmptyDistinctText_When_EachUiStateVariant | Jest | Render all variants (idle, checking, found, no-flags, needsConfirm, timeout, wrapper, busyOrError, notFound, transportError); assert one text each, and busyOrError/transportError both start "Couldn't check right now." |
| S6-2: transport failure never uses NOT_FOUND text | ProbeStatusBadge.test.tsx | ProbeStatusBadge_should_NotContainNotFoundText_When_TransportError | Jest | Render `transportError`; assert no "Not found" |
| S6-3: icon shapes differ, `aria-hidden`, text carries meaning | ProbeStatusBadge.test.tsx | ProbeStatusBadge_should_UseDistinctAriaHiddenIcons_When_SuccessWarningNeutral | Jest | Assert three icon ids, all `aria-hidden` |
| S6-4: 4.5:1 contrast light and dark | cli-flag-discovery.spec.ts | badge_should_PassAxeColorContrast_When_LightAndDarkThemes | Playwright | Toggle theme; run Axe `color-contrast` on `prog-command-status` |
| S6-5: no "invalid"/"error"/"failed" in warning-tone copy | ProbeStatusBadge.test.tsx | ProbeStatusBadge_should_AvoidBannedWords_When_WarningToneVariants | Jest | Regex over not-found and transport copy |
| UX-1: learn existence in one action | cli-flag-discovery.spec.ts | programs_should_ShowStatusAfterOneBlurEnterOrCheckTap_When_CommandTyped | Playwright | Fill, blur; repeat with Enter and `prog-command-check` |
| UX-2: `role=status` + `aria-describedby`, never `aria-invalid` | ProgramsManager.test.tsx | ProgramsManager_should_LinkStatusViaDescribedByAndNeverSetInvalid_When_ProbeSettles | Jest | Assert `role=status`, `aria-live=polite`, describedby id, no `aria-invalid` |
| UX-3: Save enabled in every probe state, no modal | ProgramsManager.test.tsx | ProgramsManager_should_KeepSaveEnabledWithoutModal_When_CheckingNotFoundOrTransportError | Jest | Iterate states; click Save; assert saved |
| UX-4: stale response dropped | useProbeProgram.test.ts | useProbeProgram_should_IgnoreStaleResponse_When_FirstProbeResolvesLast | Jest | (same test as AC5 stale row) |
| UX-5: Check/Retry >= 44x44 and Tab order after input | cli-flag-discovery.spec.ts | check_and_retry_should_Be44pxAndTabbableAfterInput_When_Rendered | Playwright | Bounding boxes; Tab from `prog-command-input` lands on Check |
| UX-6: probe never moves focus or scrolls | ProgramsManager.test.tsx | ProgramsManager_should_KeepActiveElementAndScroll_When_ProbeSettles | Jest | Assert `document.activeElement` unchanged; `scrollIntoView` not called |
| UX-7: 375px wraps, no horizontal scroll, 44px path toggle | cli-flag-discovery.spec.ts | badge_should_WrapWithoutHorizontalScrollAndOffer44pxPathToggle_When_Viewport375 | Playwright | `scrollWidth<=clientWidth`; toggle box >= 44 |
| UX-8: reduced-motion static spinner, text remains | ProbeStatusBadge.test.tsx | ProbeStatusBadge_should_ShowStaticSpinnerAndCheckingText_When_ReducedMotion | Jest | Mock `matchMedia`; assert no animation class, "Checking..." present |
| UX-9: `--model` in Down+Enter or one tap | FlagCombobox.test.tsx | FlagCombobox_should_InsertModelWithTwoKeystrokesOrOneTap_When_MoTyped | Jest | Down, Enter; separately one mouseDown on option |
| UX-10: full ARIA 1.2 attributes and option ids/`aria-selected` | FlagCombobox.test.tsx | FlagCombobox_should_ExposeComboboxListboxOptionAttributes_When_Open | Jest | Assert role, autocomplete, expanded, controls, activedescendant, option ids |
| UX-11: option announces name, takes-value, description | FlagCombobox.test.tsx | FlagCombobox_should_ExposeAccessibleNameWithTakesValueAndDescription_When_OptionRendered | Jest | Assert accessible name/description text; manual VoiceOver/NVDA spot check on the dev instance |
| UX-12: Escape keeps text; typing never prevented | FlagCombobox.test.tsx | FlagCombobox_should_NeverPreventDefaultOnTypingAndKeepText_When_Escape | Jest | Type, Escape; assert value and `defaultPrevented` false |
| UX-13: rows >= 44px; in-flow list under 640px | cli-flag-discovery.spec.ts | listbox_should_RenderInFlowWith44pxRows_When_Viewport375 | Playwright | Computed `position` not absolute/fixed; row heights >= 44 |
| UX-14: option tap triggers no probe | FlagCombobox.test.tsx | FlagCombobox_should_NotBlurOrReprobe_When_OptionSelectedByPointer | Jest | Spy `probeProgram`; mouseDown option; not called |
| UX-15: no flags means identical plain input, no error styling | FlagCombobox.test.tsx | FlagCombobox_should_MatchPlainInputSnapshotWithoutErrorStyling_When_NoFlags | Jest | Snapshot/attribute comparison |
| UX-16: warning wording, no "invalid"/"error" | ProgramsManager.test.tsx | ProgramsManager_should_RenderSpecWordingWithoutBannedWords_When_UnknownFlag | Jest | Exact string match and regex |
| UX-17: no `role=alert`, no `aria-invalid`, warning tokens + triangle | ProgramsManager.test.tsx | ProgramsManager_should_UseWarningTokensTriangleNoAlertNoInvalid_When_UnknownFlag | Jest | Assert role absent, class/token, icon |
| UX-18: input `aria-describedby` includes warning id while visible | ProgramsManager.test.tsx | ProgramsManager_should_IncludeWarningIdInDescribedBy_When_WarningVisible | Jest | Add then fix flag; id present then absent |
| UX-19: Save enabled with warning | ProgramsManager.test.tsx | ProgramsManager_should_KeepSaveEnabled_When_WarningVisible | Jest | Click Save with `--verbos` |
| UX-20: no warning when zero flags | validateFlags.test.ts | validateFlags_should_ReturnEmpty_When_ZeroKnownFlags | Jest | (same as AC6 suppression) plus ProgramsManager render check |
| UX-21: withdrawn (D4 dropped) | ProgramsManager.test.tsx | ProgramsManager_should_RenderOnlyPlainWarningTextWithNoActionControl_When_UnknownFlag | Jest | Assert the warning contains no button (negative test) |
| UX-22: read any description with one tap | cli-flag-discovery.spec.ts | mobile_should_TapInfoButtonAndShowDescription44px_When_Viewport375x667HasTouch | Playwright | (same as AC9 proof) |
| UX-23: real `<button type=button>`, `aria-expanded`, `aria-controls`, named | FlagInfoButton.test.tsx | FlagInfoButton_should_BeNamedButtonWithExpandedAndControls_When_Rendered | Jest | Assert tag, type, attrs, label "Show description for --model" |
| UX-24: tap `[i]` changes nothing else | FlagInfoButton.test.tsx | FlagInfoButton_should_NotChangeValueOrTriggerProbe_When_TappedInAvailableFlagsDisclosure | Jest | In the "Available flags (N)" disclosure per 4.2.2a (no button lives in the listbox): input value and RPC spy unchanged |
| UX-25: active option description visible without gesture | FlagCombobox.test.tsx | FlagCombobox_should_ShowActiveOptionDescriptionInline_When_ArrowNavigation | Jest | Down; description text in DOM; `aria-describedby` on input |
| UX-26: no description means no `[i]` | FlagInfoButton.test.tsx | FlagInfoButton_should_RenderNoControl_When_DescriptionEmpty | Jest | Render with empty description |
| UX-27: missing binary shows NOT_FOUND badge, single message | ProgramProbeSection.test.tsx | ProgramProbeSection_should_ShowSingleNotFoundMessage_When_ProgramMissing | Jest | Assert one badge, no second `preset-program-warning` |
| UX-28: badge `role=status`; select references it | ProgramProbeSection.test.tsx | ProgramProbeSection_should_DescribeSelectByStatusBadge_When_Rendered | Jest | Assert `aria-describedby` on `omnibar-program` |
| UX-29: create never disabled or delayed | OmnibarCreationPanel.test.tsx | OmnibarCreationPanel_should_KeepCreateEnabled_When_CheckingNotFoundOrTransportError | Jest | Iterate states; assert button enabled |
| UX-30: `omnibar-flags-warning` lists only absent saved flags | ProgramProbeSection.test.tsx | ProgramProbeSection_should_ListOnlyAbsentSavedFlagsWithSpecWording_When_Probed | Jest | Reuse `--yes-always --bogus` case; wording equals UX-16 |
| UX-31: 375px layout under picker, wraps; link dropped (D4) | cli-flag-discovery.spec.ts | omnibar_should_ShowBadgeDirectlyUnderPickerWithoutHScroll_When_Viewport375 | Playwright | Create-session dialog at 375px; no horizontal scroll; no "Edit in Program Config" control |
| UX-32: no per-keystroke or closed-section probe | ProgramProbeSection.test.tsx | ProgramProbeSection_should_ProbeOnlyOnSelectionChangeWhenVisible_When_SectionClosedOrTyping | Jest | Spy RPC: 0 calls closed or on typing; 1 per change |
| UX-33: audit line fields | prober_test.go | TestProbe_should_LogResolvedPathStatusFlagsDurationCacheHitTruncated_When_Probed | Go unit | slog capture; `program_probe` with all six fields present (plus `confirmed`, `resolve_only`) |
| UX-34: log levels (Info all, BUSY Warn) | prober_test.go | TestProbe_should_LogInfoForNotFoundAndWarnForBusy_When_Probed | Go unit | Info for NOT_FOUND, cache hit and NEEDS_CONFIRM; Warn for BUSY; msg is `program_probe` |
| UX-35: no args or env values in log | prober_test.go | TestProbe_should_NotLogArgsOrEnvValues_When_CommandHasArgsAndAssignments | Go unit | `FOO=secret claude --x tok`; line contains only `claude` |
| UX-36: keyboard-only operation in order | (manual) | Keyboard walkthrough | Manual | Tab: command input, Check, badge toggle, flags input (arrows), "Available flags" disclosure; confirm every control operable and order logical |
| UX-37: Axe AA, all states, testid/ARIA locators | cli-flag-discovery.spec.ts | forms_should_PassAxeWcagAA_When_FoundNotFoundCheckingAndWarningStates | Playwright | Axe on Program Config and omnibar; no `nested-interactive` |
| UX-38: color paired with icon and text | ProbeStatusBadge.test.tsx | ProbeStatusBadge_should_PairColorWithIconAndText_When_AllTones | Jest | Each tone has icon + text (contrast covered by S6-4) |
| UX-39: touch targets >= 44px (Check, Retry, `[i]`, rows, path toggle) | cli-flag-discovery.spec.ts | touch_targets_should_Be44pxMinimum_When_Viewport375HasTouch | Playwright | Bounding boxes of each control (Check, Retry, `[i]`, option rows, path toggle) |
| UX-40: no autocapitalize/autocorrect/spellcheck | ProgramsManager.test.tsx | ProgramsManager_should_DisableAutoCapAutoCorrectSpellcheck_When_CommandAndFlagsInputs | Jest | Assert three attributes on both inputs |
| UX-42: script never run on blur or picker selection | cli-flag-discovery.spec.ts | programs_should_NotRunScriptOnBlurAndRunAfterCheck_When_FixtureIsShebang | Playwright | Blur -> marker file absent and "Not run yet"; Check (and separately Enter) -> marker present |
| UX-42 (unit) | ProgramsManager.test.tsx | ProgramsManager_should_SendConfirmExecuteOnlyForCheckOrEnter_When_BlurThenCheckThenEnter | Jest | Request-shape spy: blur sends neither flag |
| UX-41: no added required step | cli-flag-discovery.spec.ts | programs_should_SaveInSameStepsAsBefore_When_ProbeAndWarningVisible | Playwright | Fill, Save immediately (no Check, warning present); count actions equals baseline |

## Test Stack
- **Unit (Go)**: `testing` + `testify` (`require`), table-driven, `-race`; `goleak` or `require.Eventually` for the flight-exit test; fake clock; `slog` capture handler; `FuzzParseHelp`.
- **Integration (Go)**: re-exec helper in `runner_helper_test.go` (modes `bigout`, `flood`, `hang`, `orphan`, `printenv`, `sid`; `CLIHELP_TEST_HELPER` marker passed through the permitted `CLIHELP_TEST_` env prefix); `httptest` for `ProbeGuard` and per-listener chains; real Connect handler behind the guard.
- **Unit and component (web-app)**: Jest + React Testing Library + `userEvent`, shared mock `web-app/src/lib/hooks/__mocks__/probeProgramMock.ts` (keeps jscpd at or below 0.12%); `jest-axe` only if already a dependency.
- **E2E / UX**: Playwright + Allure in `tests/e2e/cli-flag-discovery.spec.ts` (first line `// @feature program_config:probe, settings-programs`), page helper `tests/e2e/pages/ProgramsSettingsPage.ts`, fixture `tests/e2e/fixtures/probe-fixture.sh` (chmod +x, not world-writable); global-setup provisions the isolated server. Manual checklist for UX-36 and the screen-reader clause of UX-11 only.
- **Migration**: N/A (plan: no schema or data changes; additive proto). No `migration_should_be_reversible` test.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./config/clihelp ./server/services ./server/middleware -race -coverprofile=coverage.out && go tool cover -func=coverage.out` | >=80% line on `config/clihelp` and `server/middleware/probeguard.go` |
| TypeScript/Jest | `cd web-app && pnpm exec jest --coverage --coverageThreshold='{"global":{"lines":80}}'` scoped to the new files | >=80% line on new files |
| E2E | `cd tests/e2e && npx playwright test cli-flag-discovery.spec.ts` | All pass |
| Gates | `make ready`, `make ready-complexity-gate`, `pnpm run lint:duplicates` in `web-app/` | Green; jscpd <= 0.12% |

- All public service methods: happy path + error paths covered (`Probe`, `Resolve`, `ParseHelp`, `Run`/`runWith`, `ProbeProgram`, `ProbeGuard`).
- All external integrations (child process, HTTP guard, Connect handler): unit with fakes plus at least one integration test (see Integration rows above).
- UX acceptance criteria: each of the 47 criteria in `design/ux.md` has a row above.

## Known gaps and dependencies (recorded, not silently dropped)
- ADR-001 owner sign-off and the AC7 scope-down (saved `cli_flags` only; alias `extraFlags` not validated) are PENDING human acceptance (plan Unresolved Questions); AC7 tests validate the scoped-down behavior only.
- Fixture-capture unknowns (plan 1.1.3a): `git --help` may spawn `man`; whether claude/aider/gemini `--help` writes to `$HOME`. Outcomes may change the `git` negative fixture and the child `HOME` in `probeEnv`.
- Measured `--help` timings (plan Flagged Choice 11) are UNMEASURED until Task 1.1.3a; the `slowTools` row above exists only if the measurement adds entries, and the real-binary smoke test skips where the binary is absent.
- ADR-001 mitigation for scripts (`NEEDS_CONFIRM`) and its first-click cost for claude/gemini/aider are PENDING human acceptance along with the ADR.
- `LoopbackBound` accessor is unresolved (plan Unresolved Q #7); the guard tests use an injected `ProbeGuardConfig` function, so they do not depend on it.
- Plan residual concerns (`remoteChain` helper, `localhost` as loopback, Host parsing with `net.SplitHostPort`) are covered by `TestRemoteChain_*` and `TestProbeGuard_should_Pass_*` rows.

## Pre-mortem (risks and where covered)
- Hanging or noisy binaries: `hang`/`flood`/`orphan` runner tests, early kill, `WaitDelay=200ms`.
- Cancelled or busy probe poisoning the cache: cancel-then-reprobe, follower-of-cancelled-leader, BUSY-not-cached, flight-exit tests.
- False "not found" from PATH mismatch: login-PATH tests (failure not cached, `$SHELL` unset, bash early-return, background-helper shell); alias-only NOT_FOUND copy; guard 403 rendered as "Couldn't check".
- Wrapper false warnings: wrapper skip tests and the built-in Proxy entry case.
- Timeout too short for slow CLIs (pre-mortem #1): timing go/no-go in Task 1.1.3a, TIMEOUT copy/TTL/Check-bypass tests, real-binary smoke test.
- Implicit execution of scripts (pre-mortem #2): `NEEDS_CONFIRM` gate, request-shape tests, real-process marker test, e2e marker check.
- Guard vs real hostnames (pre-mortem #3): `SetHostnames` pass test plus "Couldn't check" mapping.
- Login-PATH goroutine and rc noise (pre-mortem #4): hermetic-constructor test and sentinel-pollution tests.
- Parser partial coverage (pre-mortem #5): soft-warning-only row.
- Parser false positives: warn-only, false-negative bias, zero-flags suppression.
- Unauthenticated listener abuse: target rules plus `ProbeGuard` plus per-listener chain tests.
- Focus loss in the combobox: same-node/focus test.
- `gen/` output not committed; jscpd ratchet (shared mock, `useListboxNav` extraction).
