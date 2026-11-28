# The last three packages over 10,000 lines (2026-11-28)

Goal (owner, 2026-11-12): every package under 10,000 non-test lines. Measured on `origin/main` at `e95687f2f`. This replaces the `backlogsvc` numbers in `other-packages-under-10k-plan.md` (15,327 then) and adds the `session/instance` and `server/mcp` splits. `mcp-single-root-plan.md` rules still apply.

## Verdict

| Package | Today | After this plan | Under 10k? |
|---|---|---|---|
| `server/mcp` | 13,451 | about 7,200-7,600 (2 PRs) | Yes. Low design risk. |
| `server/services/backlogsvc` | 16,016 | about 8,550, plus wiring (5 PRs) | Yes, but only with the triage extraction, the hardest PR. |
| `session/instance` | 23,541 | 18,115 (7 file-move PRs), about 15,000 after declaration-level moves | **No.** Needs an owner decision (below). |

Lines are `wc -l` of non-test `.go` files, the basis `scoping budget` uses. None of the three has a build-tagged or GOOS-suffixed non-test file.

```
for p in session/instance server/services/backlogsvc server/mcp; do
  ls $p/*.go | grep -v _test.go | xargs cat | wc -l; done
```

Cost classes (price-weighted, `~/.claude/skills/sdd/skills/ESTIMATION.md`): S = mechanical codemod move; M = codemod plus a new port or guard re-key; L = redesign of shared state with several review rounds.

## 1. `server/mcp` (13,451)

Handlers are already one struct per tool family. Only the backlog family has to move.

| PR | Target | Files | Lines moved | `server/mcp` after | Class |
|---|---|---|---|---|---|
| M1 | `server/mcp/mcpkit` (shared kernel) | `types.go`, `identity_context.go`, `caller_identity_gate.go`, `github_port.go`, plus the declarations below | 525 whole-file, plus about 200-400 relocated | about 12,500-12,700 | M |
| M2 | `server/mcp/backlogtools` | `tools_backlog.go`, `tools_backlog_pr.go`, `tools_backlog_dependencies.go`, `tools_stuck_report.go`, `tools_guidance.go`, `tools_backlog_import_tracker.go`, `guidance_questions_arg.go` | 5,349 | about 7,200-7,600 | M |

**Relocate into `mcpkit` first** (`scoping relocate`), or M2 is an import cycle:
- from `tools_backlog.go`: `WithSessionUUID`, `sessionUUIDFromContext`, `callerSessionUUID`, `callerSessionUUIDForAudit`, `validateUUID`, `featureDisabledResult`, the `Err*` codes, `ReviewTrigger`, `ReviewCompletionSignaler`;
- `TrackerRefParser`, `stuckReportPRChecker`, `ErrPendingCapExceeded`;
- `errResult`, `okResult`, `PRVerification`, `VerifyPRMatchesBranch`, `findInstanceByIDInStore`, `findInstanceByUUIDInStore`, `toolRequestOwnerAction`, and the ports `BacklogRPC`, `AutoReopener`, `GuidanceStore`.

The `CoreOption` in `github_port.go` uses `coreConfig`, so it moves to `server.go` before M1.

**Back-edges that need a port in M2.**
- `tool_modules.go` builds `backlogHandlers{...}` with unexported fields: replace with `backlogtools.New(Deps)` and exported `register*` modules.
- `testsupport_guidance.go` calls `backlogHandlers.resolveItemLink` and reads its `storage`: export one method.
- `server/item_source_plugins.go` uses `TrackerRefParser` and `JiraIssueRef`: re-point (no aliases).

**Guards.**

| Guard | If not changed |
|---|---|
| `TestServerMCP_should_HaveNoNonTestSourceInSubdirectories_When_Scanned` | Fails: it forbids any subpackage until the scan and depguard cover it. Extend it in M1. |
| `.golangci.yml` `isolate_server_mcp` (`**/server/mcp/*.go`) | **Silent**: subpackages are unchecked. |
| `import_set_guard_test.go` (globs one directory) | **Silent** for subpackage imports. |
| `tools_pr_rpc_unreachable_test.go` (`filepath.Glob("*.go")`) | **Silent**: the UI-only PR RPC ban stops covering the moved handlers. |
| `server/services/workflow_bypass_inventory_test.go` rows keyed by path and `Type.method` | Fails; re-key. |
| `tools_backlog_test.go` reads `tools_backlog.go` by name | Fails; moves with the file. |
| Exact per-file maps: `session/wake_polling_guard_test.go` (`tools_backlog.go`), `session/legacy_item_pr_readers_test.go` (`tools_backlog_pr.go`, `tools_stuck_report.go`); also `workflow_bypass_strict_test.go` and the file names in `session/domain/transition_callers.go` | Fail; re-key. The `transition_callers.go` names are enforced by `session/transition_callers_surface_test.go:46` (the `reportBlocked` site in `tools_backlog.go`) and by the registry-versus-inventory check in `workflow_bypass_inventory_test.go`. |
| `session/markerstore/no_mcp_exposure_test.go` (`os.ReadDir` of `server/mcp`, skips directories) | **Silent**: the flow-marker and manual-nudge ban stops covering both new subpackages. Make it walk. |
| `server/services/notify/notification_prune_test.go` `assertNoMCPToolFor` (`filepath.Glob` of `server/mcp/*.go`) | **Silent**: a prune tool added in a subpackage passes. Make it walk. |
| `server/services/terminal_write_pins_test.go` | Unaffected: the pinned writers stay. |

**Tests.** 18 backlog, guidance, stuck and PR test files (about 11,000 lines; chosen by file name, inferred) move or split; `tools_lifecycle_open_pr_test.go` and port tests may stay. `tool_set_golden_test.go` and the registration-order test must pass unchanged. Shared fakes probably need a `mcpkittest` package (inferred). Mutation-check the extended guards by planting a violation in the new subpackage.

## 2. `server/services/backlogsvc` (16,016)

`BacklogService` has 305 methods (10,527 declaration lines), so file moves alone cannot work: Go keeps methods with their type. Each cluster becomes its own handler type in a leaf, **embedded** in `BacklogService`, so ConnectRPC methods and call sites are promoted unchanged. 3,971 lines do not depend on `BacklogService` at all.

All file names below omit the `backlog_service_` prefix unless shown in full.

| PR | Target leaf | Contents | Lines out | Residual | Ports the leaf needs | Class |
|---|---|---|---|---|---|---|
| B1 | `backlogsvc/backlogproto` | the proto converter functions and their helpers (list below) | about 928 | about 15,090 | none, but re-keys a PR-field guard (below) | M |
| B2 | `backlogsvc/workflowcfg` | `transitions`, `stages`, `gates`, `pipeline_mode`, `liveness`, `engine_status`, `workflow_suggestion` | about 1,810 (2,085 whole, 275 already left in B1) | about 13,280 | `HasUnresolvedBlockers`, `isCodeShippedToMain`, `invalidateStageConfigCache`; symbols `invalidateEngineCache`, `planArtifactsPathMissing`, `engineLabelLiveness`, `engineLabelPipeline` | M |
| B3 | `backlogsvc/steer` | `pr_fix_steer`, `verdict_steer`, `verdict_escalation`, `pr_holds`, `ship`, `ship_status` | about 1,300 (1,318 whole) | about 11,980 | `AutoReopenAfterFailedReview`, `resolveLatestWorkCommit`, `notifyRespawnBlockedByActiveSession`, `resolveRespawnBlockedActiveLogged`, `resolveSteerFailedLogged`; symbols `cleanupWiring.ruleLProbe`, `cleanupWiring.scopeOn` | M |
| B4 | `backlogsvc/reads` | `query`, `events`, `ac_audit.go` | about 1,150 (1,254 whole) | about 10,830 | `EnterpriseHosts`, `attachDependencies`, `attachUnresolvedBlockers`, `attachPRSet`, `buildCostLookup`, `checkWorkStageBudget`; symbols `attachUnresolvedBlockersTo`, `maxTriageSessionAge` | M |
| B5 | `backlogsvc/triage` | `trigger_triage`, `triage_ac`, `triage_skip`, `headless_run`, `headless_callers`, `intent`, `chat` | 2,278 | about 8,550 | 14 methods and 8 package symbols (below), 15 shared fields | L |

- **B1 list.** `backlog_service.go`: `allowedTransitionStrings`, `backlogItemSummaryToProto`, `backlogItemToProto`, `duplicateRefPending`, `itemSessionToProto`, `itemSourceToProto`, `triageResultJSON`, `workflowSuggestionsToProto`. `stuck`: `stuckBacklogItemToProto`, `toProtoStuckReason`. `lifecycle`: `dependencyRefsToProto`. `events`: `activityNoteDataToProto`, `backlogItemToProtoOrNil`, `reviewVerdictDataToProto`. `query`: `sourceSyncEventToProto`. `ac_audit.go`: `acCriteriaToProto`. `ship_status`: `fileStatStatusToProto`. `pipeline_mode`: `allStageRoles`, `parseStageExecutorsForProto`, `pipelineModeToProto`, `stageExecutorHashesToProto`, `stageExecutorsFromProto`, `stageExecutorsToProto`. `transitions`: `configMapFromProto`, `configMapToProto`, `stageTransitionToProto`, `transitionGateToProto`. `gates`: `gateSatisfactionToProto`, `gateStatusToProto`. `liveness`: `livenessDefinitionToProto`. `stages`: `stageToProto`. `engine_status`: `engineModeToProto`, `provenanceToProto`, `verdictToProto`.
- **B5 methods.** `CreateBacklogItem`, `SpawnSessionFromItem`, `archiveItemWorkSessions`, `buildCostLookup`, `getScrollbackManager`, `getWorkSessionDiffTruncated`, `isCodeShippedToMain`, `newTriageCleanupContext`, `notifyTransitionFailed`, `resolveCodebaseWorkDir`, `reviewPromptFor`, `stampBaseCommit`, `tombstoneOrphanTriageSessions`, `triagePromptFor`.
- **B5 symbols.** `baseCommitStamp`, `resolveACSnapshot`, `slugify`, `triageCallBudget`, `errReviewInFlight`, `prFixMainBranch`, `backlogWorkBranchSlug`, `logBudgetWarningIfCrossed`.
- **Symbols are not ports.** The core embeds each leaf, so a leaf cannot import `backlogsvc`. Each symbol in the table and the B5 list moves into the leaf if only that leaf uses it; otherwise it must first move **down** into a shared leaf (`backlogsvc/backlogkernel`) or be injected as a func port; keep `errReviewInFlight` one value so `errors.Is` still matches. `cleanupWiring`'s two methods need that type moved down or a two-method port. This shared-leaf step precedes B2; its lines are not counted in the residuals.
- **Cuts between clusters.** `background_models.go` goes to the shared leaf, not B2 (`pinnedStageModel` is used by B2 and B5). `attachPRSet` is used by B4 but sits in B3's set: keep it a core method (move it out of `pr_holds`) before B3.
- **Core calls into leaves** (fine through the embedded field, but each needs an exported name): `buildGuardInput`, `WorkflowEngineStatusSource`; `attachPRSet`, `escalateVerdictToOwner`, `populatePRCounts`, `steerActiveSessionForPRFix`, `PRRunner`, `flaggedForOwner`; `TriggerTriage`, `TriggerReReview`, `recordTriageSkip`; `recordDeniedAcAttempt`, `acActorFromRequest`.
- **Late-set fields.** Anything installed by a `Set*` method after construction needs a getter port, not a value (leaf rule 3).
- **Fallback if B5 stalls.** Moving `epic`, `epic_rpc`, `claim`, `jules` (896) and `sync` plus `backlog_sources_preview.go` (413) from 10,830 gives about 9,520: under the limit with under 500 lines of margin, before wiring. Not recommended as the end state.

**Guards.**

| Guard | If not changed |
|---|---|
| `workflow_bypass_inventory_test.go`: rows for `triage.go`, `headless_run.go`, `trigger_triage.go`, `sync.go`, `lifecycle.go` | Fails on moved paths or receivers (B5: `TriggerTriage`, `TriggerReReview`). Re-key. |
| `backlog_service_base_commit_guard_test.go` (`os.ReadDir(".")`) | **Silent** for any leaf that creates ItemSessions (B5). Extend to walk leaves, and mutation-check. |
| `.golangci.yml` per-file `session/ent` exemptions, and the `ent.IsNotFound` / `LoadInstances` path regexes | Fail (exemption lost). Re-key or remove the ent use. |
| `backlog_service_test.go` reads `backlog_service_trigger_triage.go` by name | Fails; moves with B5. |
| `session/legacy_item_pr_readers_test.go` (`pr_fix_steer`, `ship`, `ship_status`: B3) and `session/wake_polling_guard_test.go` (`verdict_steer`: B3) | Fail on moved paths. Re-key. |
| File names in `session/domain/transition_callers.go` (`headless_run`, `trigger_triage`) | Re-key in B5; enforced by the registry-versus-inventory check in `workflow_bypass_inventory_test.go`. |
| `session/legacy_item_pr_readers_test.go`: B1 moves the only `PrNumber`/`PrURL` uses out of `backlog_service.go` (lines 980-1091) and `backlog_service_stuck.go` (lines 182-183) | Fails twice: both rows go stale and the new `backlogproto` file is unlisted. Re-key in B1. |
| `backlog_transition_denied_recovery_test.go` `denFuncsCalling` (flat `filepath.Glob("*.go")`) | A listed spawn point that moves to a leaf fails; a **new** predicate caller in a leaf is **silent**. Extend to walk leaves. |
| `manual_nudge_recorders_test.go` (walks subdirectories) | Unaffected by this plan (`lifecycle` stays). |

**Tests.** 80 test files (30,444 lines) share `newBacklogService`-style fixtures; each leaf PR needs those in an `internal` test-support package first (inferred: not enumerated per PR). Add one test per PR that asserts `BacklogService` still satisfies the Connect handler interface and that each embedded handler is non-nil on every constructor path.

## 3. `session/instance` (23,541)

### 3a. File moves (verified file lists, do these regardless)

Place leaves as **siblings** (`session/<leaf>`), never under `session/instance/`: `pr-field-guard` excludes `session/instance/*` with a pathspec that also matches subdirectories.

| PR | Leaf | Files | Lines | `instance` after | Back-edge to cut first | Class |
|---|---|---|---|---|---|---|
| I1 | `session/prrules` | `pr_set_rules.go`, `pr_poll_metrics.go`, `pr_set.go` | 369 | 23,172 | none | S |
| I2 | `session/pistatus` | `pi_status_source.go`, `pi_status_source_metrics.go`, `pi_events.go`, `pi_exported.go` | 900 | 22,272 | none | S |
| I3 | `session/claudeproc` | `history_detector.go`, `history_fork.go`, `claude_session_manager.go`, `stale_resume_detector.go` | 1,026 | 21,246 | `stripANSISimple` (move down) | S |
| I4 | `session/repopath` | `repo_path.go`, `worktree_enricher.go`, `github_metadata.go` | 1,223 | 20,023 | `ResolveSessionPath` (move down); one 5-line `Instance` method stays | M |
| I5 | `session/procport` | `process_manager.go`, `process_capabilities.go`, `execution_target.go`, `shell.go`, `shell_registry.go` | 736 | 19,287 | `ShellLaunch` (move down) | M |
| I6 | `session/scrollfwd` | `scroll_adapter.go`, `claude_scroll_adapter.go`, `gesture_forward_strategy.go`, `scroll_forward_capability.go`, `scroll_forward_canary.go`, `scroll_forward_metrics.go`, `scroll_lease.go` | 402 | 18,885 | `IsClaude` (move to `session/launchcmd`) | S |
| I7 | `session/instancevalues` | `canonical.go`, `session_goal.go`, `session_event_recorder.go`, `status_mapping.go`, `tag_manager.go`, `types.go`, `review_state.go` | 770 | 18,115 | none | M |

- **Test hooks move with their types.** `instance_testing.go` declares methods on moved types: `PiStatusSource.SetStatusForTest` (I2) and `WorktreeEnricher`'s `ProbeForTest`, `QueueProcessedForTest`, `QueuePendingForTest`, `ResultCountForTest` (I4). Callers outside the package use some of them, so they stay in non-test files.
- **I7 is not a pure move.** `instance_tags.go` reads the unexported `i.tagManager.tags` (lines 22, 90, 106): add an exported check on `TagManager` or export the field.
- The seven sets have no edges between each other, so they can land in any order. Residuals ignore added import lines.
- External churn is largest for I4 (33 non-test importer files) and I1, I5 (17 each).
- Unexported names that become exported include `WorktreeEnricher.safeProbe`, `activeWorktreeEnricher`, `newProcessManager`, `capabilityOf`, `newShellRegistry`, `scrollLease`, `prMetrics`, `piCommandFactory`, `resolveStaleResumeDetector`.
- `scoping move --dry-run` ran cleanly for I1-I6 (not I7). It does not report *exported* back-edges; the column above comes from a separate go/types pass.
- About 3,100 more lines in the remaining files do not depend on `Instance` (launch-command helpers, MCP proof helpers, constants, errors). Moving them needs `scoping relocate` per declaration and gets to about 15,000 at best.

**Guards.** I6's `scroll_forward_golden_fixture_test.go` and `scroll_forward_canary_test.go` read `testdata/`, whose `BUILD.bazel` is hand-written (`tools/bzl/hand-build-files.txt`): move the fixtures with I6 and update that list. `main_test.go`'s hermetic setup needs an equivalent in any leaf whose tests start processes. `session/legacy_pr_readers_test.go` names `pr_set_rules.go` (I1). `instance_status_gate_test.go` scans only this package and allow-lists writes in `session_goal.go` (I7) and `shell_registry.go` (I5). `server/deliverygate/sink_guard_test.go`, `terminal_write_pins_test.go` and `feature_flag_service_test.go` key on `session/instance` paths or symbols. `noarchivedrevival`'s listed instance files stay; moved declarations cannot call `Instance` methods, so none is a revival site. Many depguard rows allow `session/instance$`, so importers of a new leaf need the leaf added.

### 3b. Why 10k is not reachable by moving files

`Instance` has 516 methods (8,748 declaration lines, 43 files). Counting everything that transitively references `Instance`, about **14,988 lines are bound to it** and 8,553 are not (declaration lines scaled to file length). Even moving every unbound line leaves about 15,000. Comments are 6,824 of the 23,541 lines (blank lines 2,366), so trimming prose does not change the verdict: code alone is 14,351. Reaching 10,000 needs at least 5,000 lines of `Instance` behaviour turned into components with their own types; about 6,000 for a 9,000 target.

| Candidate component | Files | Lines (upper bound) | Methods needed from the rest / fields shared with it |
|---|---|---|---|
| write lease and pane submit | `instance_write_lease.go`, `instance_write_lease_metrics.go`, `pane_submit.go` | 815 | owner UUID, tmux name and snapshot for the ownership check, `MaxInlinePromptBytes`, `*Instance` in `HeldLease`/`WedgedLease` / its own `writeLease` field |
| retry | `retry_state.go` | 513 | not measured; `SetRetryProgress` republishes the snapshot and `RetryNow` revives |
| shells | `instance_shells.go` | 511 | 1 / 5 |
| PR state | `instance_pr_set.go`, `pr_tracking.go`, `instance_pr_edit.go` | 928 | 3 (including `sendSyncErr`) / 13 |
| Claude conversation | `instance_claude.go`, `instance_claude_adopt.go` | 754 | 7 / 9 |
| controller and status | `instance_controller.go`, `instance_approval.go`, `instance_pi_status.go`, `instance_status.go` | 1,170 | 8 / 12 |
| serialization | `instance_serialization.go` | 644 | 8 / 79 |
| worktree and workspace | `instance_worktree.go`, `instance_workspace.go`, `instance_checkpoint.go` | 1,766 | 11 / 25 |
| launch and tmux | `instance_tmux.go`, `instance_tmux_mcp.go`, `instance_program_extension.go`, `prompt_delivery.go` | 2,281 | 14 / 24 |

These overlap the unbound 3,100 lines above and total 9,382 gross. `Instance` keeps state, delegators and actor plumbing, so a 50-70% yield (about 4,700-6,600) is a **guess**, and its low end is below the 5,000 needed. The shared `mu`, actor mailbox and `snapshot` publication make most of these a concurrency redesign, not a move.

### Owner decision D1: what to do about `session/instance`

| Option | End size | Cost | Risk |
|---|---|---|---|
| A. File and declaration moves, keep a lower cap exception | about 15,000-18,000 | S/M | Low. Misses the goal for one package. |
| B. A, then component decomposition (embedded types in leaves) | under 10,000 only if most rows above succeed | several L | Races and lock-order bugs in the most used type. |
| C. Export `Instance` internals and turn lifecycle methods into free functions elsewhere | under 10,000, fastest | M-L | Destroys the encapsulation the actor and `Snapshot()` rules protect. |

**Recommendation: A now, then pilot B on the write lease and pane submit cluster, and decide with a measured yield.** Do I1-I7 and lower the `budget.txt` row to the measured value after each PR (today it is 24,186 against 23,541 real). The lease cluster is larger than shells (815 against 511) and owns its state: one `Instance` field with its own synchronisation. Its costs: it gates every terminal write, about 20 non-test files outside the package name its symbols, `terminal_write_pins_test.go` keys on them, `terminal_write_guards_test.go` names `instance_write_lease.go` as the only file allowed to construct `HeldLease`, and the new leaf needs its own `sendkeysguard` run (the BUG-031 single-write ban). It is also the favourable case, so before extrapolating take a second sample from a component that shares fields (shells or retry). Do not take C.

### Other open decisions

- **D2, `backlogsvc` end state.** Recommend B1-B5 (about 8,550). The alternative is stopping at 10,830 or using the thin fallback.
- **D3, `server/mcp` subpackages.** Recommend subdirectories with the three guards extended. Sibling packages (`server/mcpbacklog`) would sit outside `isolate_server_mcp` and the single-root scan unless added by hand.

## 4. Order and per-PR checks

1. M1, M2 (smallest, closes one package).
2. B1, the shared-leaf step, B2, B3, B4, then B5. B1 must be first; B5 last.
3. I1-I7 in parallel with the above; no shared files except `.golangci.yml`, `budget.txt` and BUILD files, so expect merge conflicts there.
4. D1 pilot.

Each PR: `scoping budget --init` row for every new package plus the lowered origin row; `isolate_<leaf>` depguard and `boundaries.txt` rows; `make gazelle` (and `scandata -fix` for `session/` leaves); `make registry-generate` (77 feature files mention paths in these three packages); `make ent-gen`-dependent whole-tree `unused`, `depguard`, `staticcheck`; `go test ./tools/ci/guardpaths`; complexity gate on moved declarations. CI wall-clock (about 2 hours for a full run) is the main blocker, so batch fixes.

## 5. What is verified and what is not

**Verified (run on `e95687f2f`):** package totals and whole-file line counts; method and receiver counts, cross-file and cross-set references, and importer counts (a go/types pass over `./...` including tests, default build tags); `scoping move --dry-run` for I1-I6, M2's file set and B2's file set (with `background_models.go`, which the table now keeps in the core). Dry runs prove the tool can parse and rewrite the sets, not that the result compiles: methods in moved files still need a new receiver by hand.

**Inferred:** every "about" residual; declaration-level line counts (they exclude blank lines and imports between declarations); relocation sizes; the component yield; test-file movement and fixture work; wiring overhead; that no guard exists beyond those listed (found by grep; an independent fact-check of the first version found three more flat-directory or field-name guards, so assume the list is still incomplete). Nothing was compiled or tested after a move.
