# Validation Plan: alias

**Date**: 2026-06-20

---

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| FR-1: AliasConfig struct in config | `config/config_alias_test.go` | `TestAliasConfig_LoadsFromJSON_WhenAliasesFieldPresent` | Unit | Happy path: config.json with aliases array populates `Config.Aliases` correctly |
| FR-1: AliasConfig struct in config | `config/config_alias_test.go` | `TestAliasConfig_InitializesEmpty_WhenAliasesFieldAbsent` | Unit | Error path: config.json without `"aliases"` key → `Aliases` is `[]AliasConfig{}` (not nil) |
| FR-1: AliasConfig struct in config | `config/config_alias_test.go` | `TestAliasConfig_RoundTrip_WhenWrittenAndReloaded` | Integration | Write config with aliases to disk; reload via `LoadConfigFromPath`; verify all fields survive round-trip |
| FR-2: Omnibar `@` trigger / AliasDetector | `web-app/src/lib/omnibar/detectors/AliasDetector.test.ts` | `AliasDetector_should_returnAlias_When_knownAliasWithTrailingSpace` | Unit | Happy path: `@myproj ` with alias list containing `myproj` → `InputType.Alias` |
| FR-2: Omnibar `@` trigger / AliasDetector | `web-app/src/lib/omnibar/detectors/AliasDetector.test.ts` | `AliasDetector_should_returnAliasNotFound_When_unknownSlugWithSpace` | Unit | Error path: `@nonexistent ` → `InputType.AliasNotFound` (never falls through to null) |
| FR-2: Omnibar `@` trigger / AliasDetector | `web-app/src/lib/omnibar/detectors/AliasDetector.test.ts` | `AliasDetector_should_returnAliasBrowse_When_bareAtSign` | Unit | Edge: `@` alone → `InputType.AliasBrowse` (browse mode, not null) |
| FR-2: Omnibar `@` trigger / AliasDetector | `web-app/src/lib/omnibar/detectors/AliasDetector.test.ts` | `AliasDetector_should_returnAliasBrowse_When_partialNameNoSpace` | Unit | Edge: `@myp` (no space) → `InputType.AliasBrowse` (completion mode, not null) |
| FR-2: Grammar parsing (branch/label/flags) | `web-app/src/lib/omnibar/detectors/AliasDetector.test.ts` | `AliasDetector_should_parseFullGrammar_When_allPartsPresent` | Unit | `@myproj:feature/auth working on auth --model haiku` → metadata `{aliasName, branch, label, extraFlags}` all correctly extracted |
| FR-2: Grammar parsing (branch/label/flags) | `web-app/src/lib/omnibar/detectors/AliasDetector.test.ts` | `AliasDetector_should_parseEmptyBranchAndLabel_When_onlyNameWithSpace` | Unit | `@myproj ` → metadata `{aliasName: "myproj", branch: undefined, label: undefined, extraFlags: undefined}` |
| FR-3: Alias palette browse mode | `web-app/src/components/ui/AliasPalette.test.tsx` | `AliasPalette_should_renderGroupedSections_When_browseModeAndAliasesPresent` | Unit | Happy path: render with `input="@"` and aliases with groups → group section headers visible, rows present |
| FR-3: Alias palette browse mode | `web-app/src/components/ui/AliasPalette.test.tsx` | `AliasPalette_should_renderEmptyState_When_browseModeAndNoAliases` | Unit | Error path: render with `input="@"` and empty alias list → empty state element rendered (`data-testid="alias-palette-empty"`) |
| FR-3: Alias palette fuzzy filter | `web-app/src/components/ui/AliasPalette.test.tsx` | `AliasPalette_should_renderFlatFilteredList_When_partialNameTyped` | Unit | `input="@myp"` → group headers absent, only matching aliases shown |
| FR-4: Environment variable `${VAR}` expansion | `config/defaults_test.go` | `TestExpandEnvVars_ExpandsSetVar_WhenVarExistsInEnvironment` | Unit | Happy path: `${MY_VAR}` with `MY_VAR=hello` in env → value becomes `"hello"` |
| FR-4: Environment variable expansion — unset | `config/defaults_test.go` | `TestExpandEnvVars_OmitsKey_WhenVarNotSetInEnvironment` | Unit | Error path: `${UNSET_VAR}` with no env var → key omitted from returned map |
| FR-4: Environment variable expansion — literal | `config/defaults_test.go` | `TestExpandEnvVars_PassesThroughLiteral_WhenNoVarSyntax` | Unit | Literal value `"hello"` → unchanged in returned map |
| FR-4: EnvVars integration through CreateSession | `server/services/session_service_alias_test.go` | `TestCreateSession_AppliesAliasEnvVars_WhenAliasNameProvided` | Integration | `CreateSession` with `alias_name="myproj"` where alias has `env_vars: {"FOO":"bar"}` → `instanceOpts.EnvVars["FOO"] == "bar"` |
| FR-5: CLI flags — static alias flags | `config/defaults_test.go` | `TestResolveAlias_IncludesStaticCLIFlags_WhenAliasDefinesThem` | Unit | Happy path: alias with `cli_flags: "--model haiku"` → `ResolvedDefaults.CLIFlags == "--model haiku"` |
| FR-5: CLI flags — invocation-time append | `config/defaults_test.go` | `TestResolveAlias_AppendsExtraFlags_WhenInvocationFlagsProvided` | Unit | `ResolveAlias(cfg, "myproj", "", "", "--verbose")` with alias `cli_flags="--model haiku"` → `CLIFlags == "--model haiku --verbose"` |
| FR-5: CLI flags — no static flags | `config/defaults_test.go` | `TestResolveAlias_UsesOnlyExtraFlags_WhenAliasCLIFlagsEmpty` | Unit | Error path (degenerate): alias with no `cli_flags` + `extraFlags="--foo"` → `CLIFlags == "--foo"` (no leading space) |
| FR-5: CLI flags wire-up in session create | `server/services/session_service_alias_test.go` | `TestCreateSession_AppendsCLIFlagsFromAlias_WhenAliasNameProvided` | Integration | `CreateSession` with alias `cli_flags="--model haiku"` → `instanceOpts.CLIFlags` contains `"--model haiku"` |
| FR-6: Default resolution order | `config/defaults_test.go` | `TestResolveAlias_FollowsResolutionOrder_WhenGlobalDirProfileAliasAllDiffer` | Unit | Happy path: global program=`"base"`, dir rule program=`"dir"`, profile program=`"profile"`, alias program=`"alias"` → resolved program=`"alias"` (alias wins) |
| FR-6: Default resolution order — profile inherits dir | `config/defaults_test.go` | `TestResolveAlias_DirectoryRuleAppliesBeforeProfile_WhenAliasPathMatchesRule` | Unit | Error path (missing layer): alias with profile but no dir rule match → only global + profile in chain; dir-layer absent |
| FR-6: Resolution skips ResolveDefaults double-apply | `server/services/session_service_alias_test.go` | `TestCreateSession_SkipsDoubleResolveDefaults_WhenAliasNameProvided` | Integration | Verify `ResolveDefaults` is NOT called when `alias_name` is set; only `ResolveAlias` route executes (check by injecting a config with conflicting global/alias values and asserting alias wins) |
| FR-7: Wire gap fix — EnvVars in InstanceOptions | `session/instance_test.go` | `TestNewInstance_PopulatesEnvVars_WhenPassedInOptions` | Unit | Happy path: `NewInstance` with `EnvVars: map[string]string{"X":"1"}` → `instance.EnvVars["X"] == "1"` |
| FR-7: Wire gap fix — CLIFlags in InstanceOptions | `session/instance_test.go` | `TestNewInstance_PopulatesCLIFlags_WhenPassedInOptions` | Unit | Happy path: `NewInstance` with `CLIFlags: "--foo"` → `instance.CLIFlags == "--foo"` |
| FR-7: Wire gap fix — empty map produces no env entries | `session/instance_tmux_test.go` | `TestInitTmuxSession_AddsNoExtraEnv_WhenEnvVarsMapEmpty` | Unit | Error path: `EnvVars` is empty map → `session.ExtraEnv` count unchanged from baseline |
| FR-7: Wire gap fix — EnvVars applied at tmux session time | `server/services/session_service_envvars_test.go` | `TestCreateSession_ThreadsEnvVarsIntoInstanceOpts_WhenProvidedInRequest` | Integration | `CreateSession` with `env_vars: {"FOO":"bar"}` in request → `instanceOpts.EnvVars["FOO"] == "bar"` |
| FR-7: Wire gap fix — no alias path guard 400 | `server/services/session_service_alias_test.go` | `TestCreateSession_DoesNotReturn400_WhenAliasNameProvidedWithoutPath` | Integration | `CreateSession` with `alias_name="quick"` and no `path` field → does not return `CodeInvalidArgument` |
| FR-7: ListAliases RPC | `server/services/defaults_service_test.go` | `TestListAliases_ReturnsAllAliases_WhenConfigHasAliases` | Integration | `ListAliases({})` with two aliases in config → response contains both with correct field mapping |
| FR-7: ListAliases RPC — empty config | `server/services/defaults_service_test.go` | `TestListAliases_ReturnsEmptyList_WhenNoAliasesConfigured` | Integration | `ListAliases({})` with empty alias list → `response.Aliases` is empty slice (not null/error) |
| FR-7: `create_alias_session` dispatch | `web-app/src/lib/omnibar/actions/dispatch.test.ts` | `dispatchOmnibarAction_should_callCreateSession_When_createAliasSessionAction` | Unit | Happy path: `{type:"create_alias_session", aliasName:"myproj", branch:"feat", label:"work"}` → `deps.createSession` called with `aliasName:"myproj"` |
| FR-7: `create_alias_session` dispatch — label as title | `web-app/src/lib/omnibar/actions/dispatch.test.ts` | `dispatchOmnibarAction_should_passLabelAsTitle_When_createAliasSessionHasLabel` | Unit | `{type:"create_alias_session", label:"working on auth"}` → `createSession` called with `title:"working on auth"` |
| FR-7: `create_alias_session` — TypeScript compile guard | `web-app/src/lib/omnibar/actions/dispatch.test.ts` | `dispatchOmnibarAction_should_notThrow_When_createAliasSessionActionDispatched` | Unit | Error path: dispatch with valid `create_alias_session` action succeeds without throwing |

---

## UX Acceptance Tests

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| AC-01: Alias invocable in ≤2 keystrokes after `@` | `tests/e2e/alias.spec.ts` | `alias:invoke_should_createSession_When_shortNameAndEnterPressed` | Playwright | 1. Navigate to `BASE_URL`. 2. Click omnibar. 3. Type `@fe `. 4. Press Cmd+Enter. 5. Assert RPC fired with `aliasName:"fe"`. Total after `@`: 3 chars + Cmd+Enter |
| AC-02: Browse palette appears within 100ms of `@` | `tests/e2e/alias.spec.ts` | `alias:browse_should_showPaletteWithin100ms_When_atSignTyped` | Playwright | 1. Navigate to `BASE_URL`. 2. Click omnibar. 3. Type `@`. 4. Assert `[data-testid="alias-palette"]` is visible (default 1000ms timeout; real check notes start time before type, expects palette visible immediately) |
| AC-03: Launch session in ≤5 key presses from browse | `tests/e2e/alias.spec.ts` | `alias:browse_should_launchSession_When_arrowDownEnterCmdEnterUsed` | Playwright | 1. Navigate to `BASE_URL`. 2. Click omnibar. 3. Type `@`. 4. Press `ArrowDown`. 5. Press `Enter` (selects alias). 6. Press `Meta+Enter` (creates session). 7. Assert RPC fired with `aliasName` set. 5 presses total. |
| AC-04: Cmd+Enter submits alias form without mouse | `tests/e2e/alias.spec.ts` | `alias:invoke_should_submitWithCmdEnter_When_aliasResolved` | Playwright | 1. Navigate to `BASE_URL`. 2. Click omnibar. 3. Type `@myproj `. 4. Press `Meta+Enter`. 5. Assert `CreateSession` RPC intercepted with `aliasName:"myproj"`. No mouse click on submit button. |
| AC-05: Resolution chip appears before Enter when single match | `tests/e2e/alias.spec.ts` | `alias:chip_should_appearLive_When_singleAliasMatchRemains` | Playwright | 1. Type `@myp` in omnibar. 2. Assert `[data-testid="alias-resolution-chip"]` visible (before pressing Enter/space) while exactly one alias matches the partial |
| AC-06: Chip displays alias name + path + program | `tests/e2e/alias.spec.ts` | `alias:chip_should_showNamePathProgram_When_aliasResolved` | Playwright | 1. Type `@myproj `. 2. Assert chip contains alias name text `"myproj"`, path text `"~/code/myproj"`, program text `"claude"` |
| AC-07: Extra flags shown in chip as "appended flags" | `tests/e2e/alias.spec.ts` | `alias:chip_should_showAppendedFlags_When_extraFlagsTyped` | Playwright | 1. Type `@myproj --model haiku`. 2. Assert chip contains text matching `/appended/i` and `"--model haiku"` |
| AC-08: Chip updates live when label/flags edited | `tests/e2e/alias.spec.ts` | `alias:chip_should_updateLive_When_labelTextEdited` | Playwright | 1. Type `@myproj working on`. 2. Assert chip shows label `"working on"`. 3. Continue typing ` auth`. 4. Assert chip updates to show label `"working on auth"` |
| AC-09: Not-found state shown instead of session search | `tests/e2e/alias.spec.ts` | `alias:notFound_should_showNotFoundState_When_unknownAlias` | Playwright | 1. Type `@nonexistent `. 2. Assert `[data-testid="alias-not-found"]` is visible. 3. Assert session search results (`[data-testid="session-result"]`) are NOT visible |
| AC-10: Fuzzy nearest-match suggestion when distance ≤2 | `tests/e2e/alias.spec.ts` | `alias:notFound_should_showDidYouMean_When_levenshteinDistanceIsTwo` | Playwright | 1. Type `@myrojc` (typo with Levenshtein distance 2 from `myproj`). 2. Assert "Did you mean" link visible. 3. Assert link text contains `"@myproj"` |
| AC-11: Not-found state has Clear and Esc exits | `tests/e2e/alias.spec.ts` | `alias:notFound_should_clearOnEsc_When_unknownAlias` | Playwright | 1. Type `@nonexistent `. 2. Press `Escape`. 3. Assert omnibar input value is empty or `""`. No stuck state. |
| AC-12: Config parse error shows specific error message | `tests/e2e/alias.spec.ts` | `alias:configError_should_showErrorDetails_When_configMalformed` | Playwright | Setup: Mock `ListAliases` RPC to return an error with message `"line 42: unexpected token"`. 1. Type `@`. 2. Assert `[data-testid="alias-config-error"]` visible. 3. Assert error text contains line reference |
| AC-13: Config parse error provides path-copy and Dismiss actions | `tests/e2e/alias.spec.ts` | `alias:configError_should_hasCopyAndDismiss_When_configError` | Playwright | 1. Trigger config error state (mock RPC error). 2. Assert `[aria-label*="Copy"]` button visible. 3. Assert `[aria-label*="Dismiss"]` button visible. 4. Click Dismiss. 5. Assert error panel no longer visible |
| AC-14: Config parse error does not block other omnibar modes | `tests/e2e/alias.spec.ts` | `alias:configError_should_allowPathInput_When_configError` | Playwright | 1. Trigger config error state. 2. Clear `@`. 3. Type `/tmp`. 4. Assert path detection result shown (not blocked by alias error) |
| AC-15: Empty state shows correct copy and code example | `tests/e2e/alias.spec.ts` | `alias:emptyState_should_showCopyAndExample_When_noAliasesConfigured` | Playwright | 1. Mock `ListAliases` to return empty list. 2. Type `@`. 3. Assert `[data-testid="alias-palette-empty"]` visible. 4. Assert text contains `"No aliases yet"`. 5. Assert JSON code example block visible |
| AC-16: Empty state provides "Copy config.json path" action | `tests/e2e/alias.spec.ts` | `alias:emptyState_should_haveCopyPathButton_When_noAliasesConfigured` | Playwright | 1. Mock `ListAliases` empty. 2. Type `@`. 3. Assert `[data-testid="copy-config-path"]` visible. 4. Click it. 5. Assert toast visible |
| AC-17: Empty state is not a dead end — Esc closes palette | `tests/e2e/alias.spec.ts` | `alias:emptyState_should_closeOnEsc_When_noAliasesConfigured` | Playwright | 1. Mock `ListAliases` empty. 2. Type `@`. 3. Assert empty state visible. 4. Press `Escape`. 5. Assert `[data-testid="alias-palette"]` not visible. 6. Assert omnibar input is empty |
| AC-18: Aliases appear in grouped sections when browsing | `tests/e2e/alias.spec.ts` | `alias:browse_should_showGroupHeaders_When_aliasesHaveGroups` | Playwright | 1. Mock `ListAliases` with aliases in groups "work" and "tools". 2. Type `@`. 3. Assert `[data-testid="alias-group-header"]` elements visible with text "WORK" and "TOOLS" |
| AC-19: Ungrouped aliases render above groups with no header | `tests/e2e/alias.spec.ts` | `alias:browse_should_renderUngroupedFirst_When_mixedGrouping` | Playwright | 1. Mock aliases: one ungrouped, one in group "work". 2. Type `@`. 3. Assert ungrouped alias row appears before any group header in the DOM |
| AC-20: Group headers disappear when filtering is active | `tests/e2e/alias.spec.ts` | `alias:filter_should_hideGroupHeaders_When_partialNameTyped` | Playwright | 1. Mock aliases with groups. 2. Type `@w`. 3. Assert `[data-testid="alias-group-header"]` not visible (flat list during filter) |
| AC-21: Filtering is fuzzy, case-insensitive, across name/group/description | `tests/e2e/alias.spec.ts` | `alias:filter_should_matchCaseInsensitively_When_filterTextMixedCase` | Playwright | 1. Mock alias named `"MyProj"` with description `"Frontend"`. 2. Type `@front`. 3. Assert alias row for `"MyProj"` visible (description match). 4. Type `@MYPR`. 5. Assert same row visible (name match, case-insensitive) |
| AC-22: Tab completes to longest unambiguous prefix | `tests/e2e/alias.spec.ts` | `alias:completion_should_completeLongestPrefix_When_multipleMatchesOnTab` | Playwright | 1. Mock aliases `"foo"` and `"foobar"`. 2. Type `@fo`. 3. Press `Tab`. 4. Assert input value is `"@foo"` (longest common prefix, not `"@foobar"` since ambiguous) |
| AC-23: Text before first `--` is session label | `tests/e2e/alias.spec.ts` | `alias:grammar_should_setLabelFromTextBeforeFlags_When_invoked` | Playwright | 1. Type `@myproj working on auth --model haiku`. 2. Assert chip shows label field = `"working on auth"`. 3. Assert flags shown separately = `"--model haiku"` |
| AC-24: `:branch` suffix overrides worktree branch | `tests/e2e/alias.spec.ts` | `alias:grammar_should_parseBranch_When_colonBranchSuffixTyped` | Playwright | 1. Type `@myproj:feature/auth `. 2. Assert chip shows branch `"feature/auth"`. 3. Press Cmd+Enter. 4. Assert RPC body contains `branch: "feature/auth"` |
| AC-25: `--extra-flags` appended to alias static flags | `tests/e2e/alias.spec.ts` | `alias:grammar_should_appendExtraFlags_When_invocationFlagsProvided` | Playwright | 1. Type `@myproj --verbose`. 2. Press Cmd+Enter. 3. Intercept RPC. 4. Assert `cliFlags` in request body appends `"--verbose"` to alias's static flags |
| AC-26: Case-insensitive alias matching | `tests/e2e/alias.spec.ts` | `alias:invoke_should_resolveAlias_When_nameTypedInUpperCase` | Playwright | 1. Type `@MYPROJ `. 2. Assert resolution chip shows (alias found). 3. Assert chip displays name `"myproj"` (normalized) |
| AC-27: All palette interactions navigable by keyboard | `tests/e2e/alias.spec.ts` | `alias:keyboard_should_navigateEntireFlow_When_noMouseUsed` | Playwright | Full keyboard-only flow: 1. Focus omnibar with keyboard shortcut. 2. Type `@`. 3. `ArrowDown` twice. 4. `Enter`. 5. `Meta+Enter`. 6. Assert session created. Zero mouse events. |
| AC-28: First Esc in filtered state clears to `@` not close | `tests/e2e/alias.spec.ts` | `alias:keyboard_should_clearFilterOnFirstEsc_When_filterActive` | Playwright | 1. Type `@myp`. 2. Press `Escape`. 3. Assert input value is `"@"` (not empty). 4. Assert palette still visible in browse mode |
| AC-29: First Esc in browse palette clears `@` to empty | `tests/e2e/alias.spec.ts` | `alias:keyboard_should_clearAtSignOnFirstEsc_When_browsePaletteOpen` | Playwright | 1. Type `@`. 2. Assert browse palette visible. 3. Press `Escape`. 4. Assert input is empty. 5. Assert palette closed. 6. Assert standard discovery mode visible |
| AC-30: Second Esc from discovery mode closes omnibar | `tests/e2e/alias.spec.ts` | `alias:keyboard_should_closeOmnibarOnSecondEsc_When_inDiscoveryMode` | Playwright | 1. Type `@`. 2. Press `Escape` (back to discovery). 3. Press `Escape` again. 4. Assert omnibar closed / not visible |
| AC-31: Alias palette has `role="listbox"` and `aria-label` | `tests/e2e/alias.spec.ts` | `alias:a11y_should_haveListboxRole_When_browsePaletteOpen` | Playwright | 1. Type `@`. 2. Assert element with `role="listbox"` and `aria-label="Alias palette"` exists in DOM |
| AC-32: Each alias row has `role="option"` with descriptive `aria-label` | `tests/e2e/alias.spec.ts` | `alias:a11y_should_haveOptionRole_When_aliasRowRendered` | Playwright | 1. Type `@`. 2. Assert `role="option"` elements present. 3. Assert at least one has `aria-label` matching pattern `@<name>` |
| AC-33: Resolution chip uses `role="status"` and `aria-live="polite"` | `tests/e2e/alias.spec.ts` | `alias:a11y_should_haveStatusRole_When_aliasResolutionChipShown` | Playwright | 1. Type `@myproj `. 2. Assert element with `role="status"` and `aria-live="polite"` exists |
| AC-34: Not-found state uses `role="alert"` and `aria-live="assertive"` | `tests/e2e/alias.spec.ts` | `alias:a11y_should_haveAlertRole_When_notFoundState` | Playwright | 1. Type `@nonexistent `. 2. Assert `[role="alert"][aria-live="assertive"]` element exists |
| AC-35: Config error state uses `role="alert"` and `aria-live="assertive"` | `tests/e2e/alias.spec.ts` | `alias:a11y_should_haveAlertRole_When_configErrorState` | Playwright | 1. Mock config error. 2. Type `@`. 3. Assert `[role="alert"][aria-live="assertive"]` exists |
| AC-36: Action buttons have descriptive `aria-label` | `tests/e2e/alias.spec.ts` | `alias:a11y_should_haveDescriptiveAriaLabels_When_notFoundStateShown` | Playwright | 1. Type `@nonexistent `. 2. Assert Clear button has `aria-label` containing `"clear"` or similar. 3. Assert "Did you mean" link has `aria-label` describing the suggested alias |
| AC-37/38: WCAG AA color contrast (visual, not-found, focus) | `tests/e2e/alias.spec.ts` | `alias:a11y_should_passAxeAudit_When_browsePaletteOpen` | Playwright (Axe) | 1. Type `@`. 2. Run `await new AxeBuilder({ page }).analyze()`. 3. Assert `violations` array is empty (blocks on WCAG AA) |
| AC-39: Resolution chip "Alias resolved" text label (not color alone) | `web-app/src/components/ui/AliasPalette.test.tsx` | `AliasResolutionChip_should_renderTextLabel_When_resolved` | Unit | Render `<AliasResolutionChip resolved={true} .../>`. Assert text "Alias resolved" is present (not just CSS class) |
| AC-40: Error states use icon + text | `web-app/src/components/ui/AliasPalette.test.tsx` | `AliasNotFound_should_renderIconAndText_When_rendered` | Unit | Render `<AliasNotFound slug="foo" .../>`. Assert both icon element and error text are present |
| AC-41: Omnibar placeholder mentions `@alias` | `tests/e2e/alias.spec.ts` | `alias:placeholder_should_includeAtAlias_When_discoveryModeActive` | Playwright | 1. Navigate to `BASE_URL`. 2. Assert omnibar input has `placeholder` attribute matching `/@alias/i` |

---

## Detector Unit Tests (AliasDetector)

These extend the FR-2 mapping above. Test IDs start at `T-UNIT-TS-030` continuing from WorkflowDetector's `T-UNIT-TS-101` series.

| Test ID | Test Name | Scenario |
|---|---|---|
| T-UNIT-TS-030 | `AliasDetector_should_returnAlias_When_knownAliasWithTrailingSpace` | `@myproj ` (exact match + space) → `InputType.Alias` |
| T-UNIT-TS-031 | `AliasDetector_should_returnAlias_When_knownAliasWithBranch` | `@myproj:feature/auth ` → `InputType.Alias`, `metadata.branch == "feature/auth"` |
| T-UNIT-TS-032 | `AliasDetector_should_returnAlias_When_knownAliasWithLabel` | `@myproj working on auth ` → `InputType.Alias`, `metadata.label == "working on auth"` |
| T-UNIT-TS-033 | `AliasDetector_should_returnAlias_When_knownAliasWithExtraFlags` | `@myproj --model haiku` → `InputType.Alias`, `metadata.extraFlags == "--model haiku"` |
| T-UNIT-TS-034 | `AliasDetector_should_returnAlias_When_fullGrammarAllParts` | `@myproj:feat/auth working on auth --model haiku` → all four metadata fields populated |
| T-UNIT-TS-035 | `AliasDetector_should_returnAliasNotFound_When_unknownSlugWithSpace` | `@nope ` → `InputType.AliasNotFound`, `metadata.slug == "nope"` |
| T-UNIT-TS-036 | `AliasDetector_should_returnAliasBrowse_When_bareAtSign` | `@` → `InputType.AliasBrowse` (not null) |
| T-UNIT-TS-037 | `AliasDetector_should_returnAliasBrowse_When_partialNameNoSpace` | `@myp` → `InputType.AliasBrowse` with `metadata.partial == "myp"` |
| T-UNIT-TS-038 | `AliasDetector_should_matchCaseInsensitive_When_upperCaseAliasName` | `@MYPROJ ` → `InputType.Alias` (resolved to `myproj`) |
| T-UNIT-TS-039 | `AliasDetector_should_havePriority36` | `detector.priority === 36` |
| T-UNIT-TS-040 | `AliasDetector_should_neverReturnNull_When_inputStartsWithAt` | Any `^@.*` input → result is non-null |

---

## Go Unit Tests: FindAlias, GetAliasesByGroup, expandEnvVars, ResolveAlias

All in `config/defaults_test.go` following the existing `TestFuncName_ExpectedBehavior_WhenCondition` naming convention.

| Test Name | Function | Scenario |
|---|---|---|
| `TestFindAlias_ReturnsAlias_WhenNameMatches` | `FindAlias` | Happy path: alias list contains `"myproj"`, call with `"myproj"` → non-nil pointer |
| `TestFindAlias_ReturnsNil_WhenNameNotFound` | `FindAlias` | Error path: alias not in list → `nil` |
| `TestFindAlias_IsCaseInsensitive_WhenUpperCaseProvided` | `FindAlias` | `"MYPROJ"` finds alias stored as `"myproj"` |
| `TestGetAliasesByGroup_GroupsCorrectly_WhenMixedGroups` | `GetAliasesByGroup` | Two groups + one ungrouped → map has two named keys and one `""` key |
| `TestGetAliasesByGroup_ReturnsEmpty_WhenNoAliases` | `GetAliasesByGroup` | Empty alias list → empty map |
| `TestGetAliasesByGroup_UngroupedUsesEmptyKey_WhenNoGroup` | `GetAliasesByGroup` | Aliases without `group` field → stored under key `""` |
| `TestExpandEnvVars_ExpandsSetVar_WhenVarExistsInEnvironment` | `expandEnvVars` | `${MY_VAR}` with env var set → expanded value |
| `TestExpandEnvVars_OmitsKey_WhenVarNotSetInEnvironment` | `expandEnvVars` | Unset var → key absent from returned map |
| `TestExpandEnvVars_PassesThroughLiteral_WhenNoVarSyntax` | `expandEnvVars` | Literal string unchanged |
| `TestResolveAlias_ReturnsError_WhenAliasNotFound` | `ResolveAlias` | Unknown alias name → `error` returned |
| `TestResolveAlias_FollowsResolutionOrder_WhenGlobalDirProfileAliasAllDiffer` | `ResolveAlias` | Four-layer stack: alias inline fields win |
| `TestResolveAlias_PromotesPathIntoResolvedDefaults` | `ResolveAlias` | Alias with `path: "~/code/myproj"` → `result.Path == "~/code/myproj"` |
| `TestResolveAlias_IncludesStaticCLIFlags_WhenAliasDefinesThem` | `ResolveAlias` | Static flags from alias config present in result |
| `TestResolveAlias_AppendsExtraFlags_WhenInvocationFlagsProvided` | `ResolveAlias` | `extraFlags` arg appended to static flags with a space separator |
| `TestResolveAlias_UsesOnlyExtraFlags_WhenAliasCLIFlagsEmpty` | `ResolveAlias` | Empty static flags + extra flags → no leading space in result |
| `TestResolveAlias_DirectoryRuleAppliesBeforeProfile_WhenAliasPathMatchesRule` | `ResolveAlias` | Dir rule for alias path → dir values in resolution chain |
| `TestResolveAlias_PropagatesBranchAndLabel_WhenPassedAsArgs` | `ResolveAlias` | `branch` and `label` args → in returned `ResolvedDefaults` |

---

## Test Stack

- **Unit (Go)**: `go test ./config/... ./session/... ./server/services/...` with `testify/assert` (or stdlib `t.Errorf` matching existing style in `defaults_test.go`)
- **Unit (TypeScript — detector)**: `cd web-app && npx jest --no-coverage --testPathPatterns="AliasDetector.test"`
- **Unit (TypeScript — dispatch)**: `cd web-app && npx jest --no-coverage --testPathPatterns="dispatch.test"`
- **Unit (TypeScript — component)**: `cd web-app && npx jest --no-coverage --testPathPatterns="AliasPalette.test"`
- **Integration (Go)**: Go tests with real `Config` struct; no mocks for the config layer. Session service tests may use a test double only for the tmux transport layer (not the config/resolver layer).
- **E2E / UX**: Playwright via `cd tests/e2e && npx playwright test alias.spec.ts`. Server must be running: `STAPLER_SQUAD_INSTANCE=e2e-local ./stapler-squad --tmux-keep-server &`. Where full alias RPC is not available, use `page.route()` to mock `ListAliases` and `CreateSession` network calls.

---

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line coverage on `config/defaults.go` (new functions), `session/instance.go` changes, and `server/services/session_service.go` alias branch |
| TypeScript/Jest | `cd web-app && npx jest --coverage` | ≥80% line coverage on `AliasDetector.ts`, `dispatch.ts` (new case), and `AliasPalette.tsx` |
| E2E | Playwright run completes without `test.skip` on alias-critical paths | All alias.spec.ts tests passing; no skipped tests in the AC-01 through AC-30 happy-path set |

---

## File Index: New Test Files Required

| File | Test Type | Phase |
|---|---|---|
| `config/config_alias_test.go` | Go unit | Phase 2 |
| `config/defaults_test.go` | Go unit (extend existing) | Phase 2 |
| `session/instance_test.go` | Go unit (extend or create) | Phase 1 |
| `session/instance_tmux_test.go` | Go unit | Phase 1 |
| `server/services/session_service_envvars_test.go` | Go integration | Phase 1 |
| `server/services/session_service_alias_test.go` | Go integration | Phase 5 |
| `server/services/defaults_service_test.go` | Go integration (extend or create) | Phase 2 |
| `web-app/src/lib/omnibar/detectors/AliasDetector.test.ts` | TS unit | Phase 3 |
| `web-app/src/lib/omnibar/actions/dispatch.test.ts` | TS unit (extend existing) | Phase 5 |
| `web-app/src/components/ui/AliasPalette.test.tsx` | TS unit | Phase 4 |
| `tests/e2e/alias.spec.ts` | Playwright E2E | Phase 5 |

---

## Pitfall Tests (Guard against known blockers)

These directly address the resolved blockers documented in the plan's "Flagged Risks" section:

| Blocker | Test Name | File |
|---|---|---|
| Fallthrough bug: AliasDetector returns null for `@foo` (no space) | `AliasDetector_should_neverReturnNull_When_inputStartsWithAt` | `AliasDetector.test.ts` |
| Path guard 400 on pathless alias | `TestCreateSession_DoesNotReturn400_WhenAliasNameProvidedWithoutPath` | `session_service_alias_test.go` |
| CLIFlags replace vs append ambiguity | `TestResolveAlias_AppendsExtraFlags_WhenInvocationFlagsProvided` + `TestResolveAlias_UsesOnlyExtraFlags_WhenAliasCLIFlagsEmpty` | `defaults_test.go` |
| Double-apply ResolveDefaults when alias_name set | `TestCreateSession_SkipsDoubleResolveDefaults_WhenAliasNameProvided` | `session_service_alias_test.go` |
| activeDropdown impossible state (two dropdowns active) | `alias:keyboard_should_navigateEntireFlow_When_noMouseUsed` | `alias.spec.ts` |

---

## Migration Test

N/A — the alias feature adds new config fields and new RPC methods. No data migration is required. Backward compatibility is tested by `TestAliasConfig_InitializesEmpty_WhenAliasesFieldAbsent` (config.json without `"aliases"` loads cleanly).
