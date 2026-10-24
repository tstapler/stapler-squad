# Research: feature landscape and edge cases (backlog-make-it-true)

Scope: answers requirements.md open questions 1 and 2, plus unstated needs and edge cases for quota settings and `allow_auto_merge`. Evidence is `file:line` against this worktree (branch `stapler-squad-backlog-cohesion`). VERIFIED means I opened the code; UNVERIFIED means inferred or grep-only.

## Headline findings

1. VERIFIED: swapping in `ConfiguredWorkflowEngine` today would silently drop every built-in business gate. `EnsureBuiltInWorkflowStages` seeds 9 stages and one `StageTransition` per `validTransitions` edge but zero `TransitionGate` rows (`session/workflow_stage_seed.go:72-126`; the only `TransitionGate.Create` outside tests is the CRUD path, `session/ent_stage_config_repository.go:429`). `ConfiguredWorkflowEngine.PendingGates` returns nil when an edge has no gates (`session/configured_workflow_engine.go:165-176`). The guards that exist only in `DefaultWorkflowEngine`/`TransitionGuard` are: AC required (idea/refining to ready), plan/blockers/artifacts (ready/queued to in_progress), verdict-clear (review/pr_pending to ready), and PASS verdict plus shipped (to done) (`session/workflow_engine.go:109-124`, `session/domain/backlog.go:652+`). "Seeded defaults equal `validTransitions`" is true for edges and false for gates.
2. VERIFIED: `override_reason` is not honored by the configured engine. `TransitionGuard` short-circuits on `OverrideReason` for to-done and review/pr_pending-to-ready (`session/domain/backlog.go:672-700`). `grep OverrideReason` finds nothing in `session/configured_workflow_engine.go` or `session/gate_*.go`. After a swap, the operator's manual override of a failing gate would stop working.
3. VERIFIED: most status writers never consult the engine. Only 6 sites call it (table below). Everything else calls `storage.TransitionBacklogItemStatus`, which does a SQL compare-and-swap on `ExpectedStatus`/`ExpectedUpdatedAt` but no graph or gate validation (`session/ent_repository_backlog.go:1721-1760`, `session/storage.go:1103-1105`). So even after the swap, config edits only affect operator and RPC-driven transitions; the autonomous pipeline stays hardcoded.

## Q1. Every path that changes status or evaluates transition/gate rules

### A. Paths that consult `s.engine` (will follow the swap)

| # | Site | What it calls | Notes |
|---|---|---|---|
| 1 | `TransitionBacklogItemStatus` RPC, `server/services/backlog_service_lifecycle.go:892` (`CanTransition`), `:933` (`ValidateGates`) | full check, passes `BuildStageConfigSnapshotFallback(item)` | The reference path. Manual override at `:988-996` notes + notifies. |
| 2 | MCP `transition_backlog_item_status`, `server/mcp/tools_backlog_dependencies.go:163` | delegates to #1 via `backlogSvc.TransitionBacklogItemStatus` with actor `agent` | Omits `override_reason` by design (`:140-142`). Follows the engine for free. |
| 3 | `transitionWithGuard`, `server/services/backlog_service_triage.go:683` (`CanTransition`), `:689` (`ValidateGates`) | full check | Callers: `queueBacklogItem` (`:716`), `claimAndSpawnCandidate` (`:839`), Jules dispatch (`jules_dispatch_service.go:361`, via interface at `:87`). |
| 4 | `OverrideVerdict`, `backlog_service_lifecycle.go:1318` | `CanTransition` only, no `ValidateGates` | Then raw storage write at `:1332`. |
| 5 | `AttachSessionToItem`, `backlog_service_sync.go:157` | `CanTransition` only | Then raw storage write at `:158`. |
| 6 | `GetPendingGates` RPC, `backlog_service_gates.go:128` | `PendingGates` (read-only preview) | Preview builds its own guard input (`:140+`), so it must stay in lockstep with #1. |
| 7 | `SyncLoop.SyncOne`, `session/backlog_sync.go:449` | `GuardedTransitionAllowed(sl.workflowEngine, ...)` | `workflowEngine` is `NewDefaultWorkflowEngine()` hardcoded in both constructors (`backlog_sync.go:44-46,57,69`). It is not the service's engine, so it ignores the swap unless injected. |
| 8 | Proto conversion for UI, `allowedTransitionStrings` (`backlog_service.go:1108`) and the `engine` param of `backlogItemToProto`, `snapshotEventForItem` (`backlog_service_events.go:244,273,391`) | `AllowedTransitions` | Display only. After the swap the UI's allowed-transitions list changes with config. |

### B. Paths that write status with no graph or gate check (bypass the engine entirely)

All call `storage.TransitionBacklogItemStatus` (or a sibling raw writer) directly. Rows marked `guarded elsewhere` carry a hand-rolled source-status check, not the engine.

| Site | Transition | Guard that exists instead |
|---|---|---|
| `onAutonomousDriverComplete`, `server/services/autonomous_orchestration_service.go:586` | triage to `ready` (expected `idea`, `:453-454`); work to `review`; others by role (`:429-583`) | `ExpectedStatus` CAS only. Triage-to-ready skips the AC-required gate that `TransitionGuard` would apply to idea-to-ready. Work-to-review runs even when the driver was stuck (`:577-583` comment). **(Corrected in Repair round 4, VERIFIED at `autonomous_orchestration_service.go:432-575`: only the triage role writes a status, `idea->ready`; the work role deliberately does not transition and the review role only closes a stuck row or ends the `ItemSession`.)** |
| MCP `request_review`, `server/mcp/tools_backlog.go:1507,1515` | any valid source to `review` (or `done` if `SkipReviewGate`) | Hand-rolled `unmetAcCriteriaError` (`:1487`); comment `:1494-1497` says "hardcoded built-in targets, not sourced from any stage engine". `SkipReviewGate` sends to `done` with no verdict or shipped check. |
| MCP `report_blocked`, `tools_backlog.go:1682` | in_progress/review to `ready` or `review` | Switch on source status (`:1660-1668`). |
| MCP `resume_work`, `tools_backlog.go:1821` | `ready` to `in_progress` | Source must be `ready` (`:1812`). Skips plan-approved/blockers gate. |
| `OverrideVerdict`, `backlog_service_lifecycle.go:1332` | to a caller-chosen status | Engine `CanTransition` only (row #4 above). |
| `SubmitManualReview`, `backlog_service_lifecycle.go:1446` | to `done` | UNVERIFIED which guard; no engine call in the function. |
| `spawnSessionAfterGates` `backlog_service_triage.go:1268`; `forceResetItem` `:1324` | to `in_progress` | `nil` precondition on `:1268`. |
| `AutoReopenAfterFailedReview` `:2041,2115`; `autoReopenForPRFix` `:2418,2471`; `respawnTriage` `:2637`; `MaybeTriggerTriage` `:2906`; `TriggerReReview` `:3348` | review/pr_pending to `in_progress` or `idea`, rollbacks, to `done` | CAS preconditions only. |
| `TriggerTriage`, `backlog_service_trigger_triage.go:290,772` | triage persistence transitions | CAS only. |
| `pushAndCreatePR` `session/backlog_lifecycle_pr.go:601`; `resolveToPRPending` `:819`; `recoverDriftedPRItem` `:848`; `reconcilePRPendingItem` `:1531`; `closeIfSupersededByMain` `:2183` | review to `pr_pending`; pr_pending to `done` | `IsPRMerged` check before done (`:1519-1531`, `:2183`); no engine. `session/backlog_lifecycle.go:1925-1935` also merged-PR check. |
| `archiveStaleDoneItems`, `session/backlog_lifecycle_archive.go:87` | `done` to `archived` | CAS only. |
| `ReconcileStuckItems`, `session/storage_backlog.go:1006` | in_progress to `review` by raw `tx.BacklogItem.UpdateOne(...).SetStatus` inside a tx | None. Records stage snapshot fields (`:995`) but never asks the engine. |
| `JulesSessionPoller`, `session/jules_session_poller.go:432,518,533` | to `ready`/`review` | CAS only. |
| `ArchiveBacklogItem` (`ent_repository_backlog.go:1484`), `UnarchiveBacklogItem` (`:1559`, to `idea`), `SetBacklogItemPRAndTransition` (`:1977`), `status_corruption_repair.go:82` | raw `SetStatus` | None. |
| `backlog_debug_mutate_handler.go:172` | any | Intentionally no check; comment `:17` and `:146` says it mirrors "no CanTransition/ValidateGates". Gated to the e2e instance only (stuck.go comment cites `STAPLER_SQUAD_INSTANCE=e2e-local`). |
| `CreateBacklogItem` initial status, `ent_repository_backlog.go:~452` | initial `SetStatus(status)` | UNVERIFIED whether the entry-stage flag (`IsEntry`, `workflow_stage_seed.go:55-57`) is enforced on create. |

Implication for workstream A: the config-driven check covers only RPC and MCP-RPC transitions (rows A1-A5). Autonomous completion, lifecycle auto-transitions, and four MCP tools remain hardcoded. This matches the prior-art finding that call-site enumeration is needed (`project_plans/backlog-custom-workflow-stages/research/pitfalls.md:113-139`, "workflow-engine bypass at two confirmed sites"; ADR-013 only says "swap via the same interface", `docs/adr/013-workflow-engine-replaces-valid-transitions.md:58`).

### C. Where `ConfiguredWorkflowEngine` is already live

- `BacklogLifecycleListener.SetWorkflowEngine(stageConfigEngine)` is wired (`server/dependencies.go:1543-1545`). It is used only to resolve automated-review and custom-check gate context (`session/backlog_lifecycle.go:1081-1120`). The comment at `backlog_lifecycle.go:271-274` ("does not wire a ConfiguredWorkflowEngine") is stale.
- `BacklogService.SetStageConfigEngine` receives it only as a cache invalidator (`dependencies.go:1485-1490`, `backlog_service.go:417-465`).
- MCP `h.stageEngine` (`tools_backlog.go:369-374`) is declared as a `stageLister` and used for status-filter validation; `grep stageEngine server/` finds no non-test assignment. UNVERIFIED that it is wired anywhere in production, so MCP status-filter validation likely falls back to the built-in list (`:178-197`).

## Q2. What `ConfiguredWorkflowEngine` does today

All VERIFIED from `session/configured_workflow_engine.go` and `session/stage_config_cache.go`.

| Situation | Behavior | Evidence |
|---|---|---|
| Cache load fails at startup | Logs a warning, returns a non-nil engine with an empty cache (`ptr` nil). `NewConfiguredWorkflowEngine` never returns an error. | `configured_workflow_engine.go:78-85` |
| Empty or never-loaded cache | `Get` returns false; `HasStage` returns false. `CanTransition` therefore returns false for every pair unless the item carries a snapshot fallback that lists the destination. **No fallback to the default graph.** Fail-closed for transitions, a total lockout for items without a snapshot. | `stage_config_cache.go:~150-170`; `configured_workflow_engine.go:102-119` |
| No edge for `(from,to)` in `PendingGates` | Returns `nil, nil` (zero gates, "passes") unless a snapshot fallback proves the edge once existed, in which case one synthetic blocking gate `stage-config-unresolvable`. `CanTransition` is meant to gate legality first. | `configured_workflow_engine.go:165-176,193-215` |
| Stage deleted or disabled while an item sits in it | `HasStage(from)` false: `CanTransition` and `AllowedTransitions` use the item's frozen `StageConfigSnapshot` (built from the latest `BacklogStatusEvent` whose `ToStatus == item.Status`, `ent_repository_backlog.go:145-170`) and log a warning. With no such event (never transitioned into the stage, e.g. seeded or imported rows), fallback is nil, so every transition is denied. | `configured_workflow_engine.go:102-148` |
| Live stage with zero outgoing edges (dead end) | Allowed to exist; the cache seeds an empty entry per enabled stage so it is distinguished from deleted. Live graph wins over the snapshot even if they differ. | `stage_config_cache.go:~118-127` |
| Edge whose endpoint stage is disabled | Skipped at load. `ErrDisableWouldStrandLiveItems` exists to stop disabling a stage with live items (`mapTransitionGraphError`, `backlog_service_transitions.go:108-123`). | `stage_config_cache.go:~96-103` |
| Delete a stage that has live items | Rejected with `FailedPrecondition` unless `force=true`. A forced delete creates the stranded-item case above. | `backlog_service_stages.go:112-150` |
| Gate with unrecognized kind, bad config, missing pipeline mode | Blocks (fail-closed) with a warning. `human_approval` with nil repo degrades to unsatisfied. | `configured_workflow_engine.go:242-345` |
| `pr_green` structural gate | Always fails closed; it is not wired because `BacklogItemTransitionInput` lacks PR/CI data. | `gate_structural.go:19-26` |
| Seed re-run | `EnsureBuiltInWorkflowStages` is create-if-count-zero only. New built-in edges added to `domain.validTransitions` in a later release never reach an existing DB, so seeded config and code drift silently. | `workflow_stage_seed.go:72-87` |
| Seed vs `validTransitions` equivalence | Edges: seeded from `domain.ValidTransitions()`, so equal at seed time. Gates: none seeded, not equal (see headline 1). Entry/terminal flags: only `idea` is entry, `done`/`archived` terminal. | `workflow_stage_seed.go:44-66,97-117` |
| Concurrency | Cache is copy-on-write atomic pointer, writer-serialized; reads are lock-free with no DB round trip, meeting the requirements NFR. | `stage_config_cache.go:52-58,121-176` |

Other gaps for the swap:
- Is there an equivalence test? UNVERIFIED. `session/configured_workflow_engine_test.go` exists; I did not open it to check for a default-vs-configured parity corpus.
- `PendingGates` for built-ins is produced by `DefaultWorkflowEngine` as a single structural gate with ID `builtin:...` (`workflow_engine.go:83-125`), whereas `GetPendingGates` UI semantics for configured gates use persisted gate UUIDs (`gate_status.go:33-40`). The UI must tolerate both.
- Construction order is not circular: `pipelineModeRepo` is built first (`dependencies.go:~700-745`), then `stageConfigEngine` (`:773`), then `NewBacklogService` takes the engine (`:1477`). The swap is a one-line change in terms of wiring, but `stageConfigEngine` can be nil (non-ent storage or construction failure), so the swap needs an explicit nil-fallback to `workflowEngine` and a typed-nil guard (the file already documents this pitfall at `:1483-1489`).

### Needs the plan must add (unstated in requirements)
1. A gate-parity seed (or a composite engine). Either seed `TransitionGate` rows reproducing the five built-in guards, or have `ConfiguredWorkflowEngine` fall through to `TransitionGuard` for edges whose endpoints are both built-in stages. Without one of these, shadow mode would show zero divergence on `CanTransition` while diverging on `ValidateGates` (shadow comparison must compare both, plus `PendingGates`).
2. Decision on `override_reason`: honor it in the configured path (parity) or document its removal. Success metric 1 as written ("allow to deny") does not cover it.
3. Sentinel-error parity: `isFailedPreconditionGuardError` (`backlog_service_lifecycle.go:~855-866`) maps `TransitionGuard` sentinels to `FailedPrecondition`; configured gates return `ErrGateNotSatisfied` wrapping text, so HTTP/Connect codes change from the default engine's behavior. UNVERIFIED that the sentinel list covers `ErrGateNotSatisfied`.
4. Per-item `StageConfigSnapshot` fallback is nil for items with no matching status event; define behavior (recommend: treat nil as "fall back to default graph for built-in slugs").
5. `SyncLoop` and any other holder of `NewDefaultWorkflowEngine()` need the same engine injected or an explicit scope-out.
6. The enforcement lint (success metric 5) should include the bypass inventory above, or the autonomous pipeline stays outside "config changes runtime".

## Edge cases: quota settings

Facts (VERIFIED unless noted):
- `QuotaConfig` fields: `Enabled`, `PauseBelowHeadroomPct`, `ResumeMarginPct`, `ConsecutiveTicksToPause/Resume`, `AssumedWindowTokenBudget`, `RateLimitWindowMinutes`, `ManualOverrideGraceMinutes`, `ForegroundThrottleDelaySeconds` (`config/types.go:736-775`). Defaults applied by `QuotaConfigOrDefault` (`:777-809`); `Enabled=false` and `AssumedWindowTokenBudget=0` stay zero.
- Config is already live: the gate re-reads `config.LoadConfig().Quota` every reconcile tick (`dependencies.go:1429-1436`, `quota_gate.go:92,104,201`). A settings UI needs only a write path, not a reload mechanism.
- No UI or RPC writes `Quota`: proto grep for "quota" matched nothing relevant; `feature_flag_service.go` quota hits are about GitHub API quota (`:264`). Whether a generic config-write RPC exists that could carry `Quota` is UNVERIFIED.
- The "soft cost budget" in requirements item 2 is a different thing from the token headroom budget. `session/budget_warning.go` is a per-item USD threshold, `cost_budget_threshold_usd` (`backlog.proto:289-294`, `session/repository.go:364-368`), unset by default, advisory only: it logs `[BudgetWarning]` and never blocks (`backlog_service.go:1445-1490`). There is no global soft-budget setting to expose; the requirement text needs reinterpretation (global default threshold that new items inherit? or per-item UI only?). The proto comment says 0.0 is a valid threshold, distinct from unset, so a UI cannot use an empty-number input to mean unset without care.

Edge cases:
1. `Enabled=false` while `pausedByQuota=true`: `Reconcile` returns immediately when disabled (`quota_gate.go:201-204`), so nothing re-enables backlog. Disabling the gate from the UI while it has paused backlog can leave backlog paused indefinitely. The UI toggle needs "resume backlog now" semantics or the gate must resume on disable. UNVERIFIED whether `BacklogController` state persists across restart, which compounds this.
2. `Enabled=true` with `AssumedWindowTokenBudget=0` is "reactive-only"; only hard rate-limit events pause (`quota_gate.go:167-168,268-270`). UI must show this as a distinct state; `StatusDetail` already produces the text and says "set ... in config.json", which becomes wrong once a UI exists.
3. Validation: `PauseBelowHeadroomPct + ResumeMarginPct` must stay at or below 100 or the resume branch can never trigger (`quota_gate.go:287`). Zero or negative values are silently replaced with defaults by `QuotaConfigOrDefault`, so a user cannot save 0 meaning "off"; UI must not offer 0 as a value.
4. Hysteresis counters and `consecutiveBelow/Above` are in-memory and reset on a mid-flight config change only incidentally; a changed threshold takes effect on the next tick without resetting streaks (`quota_gate.go:~280-300`). Acceptable but should be tested for "changed value alters behavior" (success metric 2): the test needs a fake `cfgFn`, which the constructor already accepts.
5. Token budget derivation (open question 4): `computeHeadroom` uses `tokenStore.GetAll()` over a trailing 5h window (`quota_headroom.go`, `quota_gate.go:266`); `logUncalibratedUsage` already logs observed usage when uncalibrated (`:418`). Deriving a budget from observed peak usage is plausible but Anthropic publishes no budget (`config/types.go:759-763`), so any derived number is a heuristic and should be shown as a suggestion, not auto-applied. UNVERIFIED beyond reading comments.
6. Two writers to config.json: the file is the persistence layer; a UI write racing a hand edit loses data unless the write is read-modify-write under the config lock. UNVERIFIED how `SaveConfig` locks.
7. Multi-instance: `STAPLER_SQUAD_INSTANCE` and workspace mode give separate configs (`CLAUDE.md` state-isolation), so the UI edits the config of whichever instance serves it; quota is account-wide (`QuotaConfig` doc comment) but config is per-instance, so two instances can disagree.
8. Foreground throttle (`ForegroundThrottleDelaySeconds`) is in the same struct and also gated by `Enabled`; a UI that only exposes "enabled + budget" should say that enabling also turns on foreground-session dispatch throttling (`dependencies.go:1514-1516`, `ShouldThrottleForeground`).
9. Notification spam on flapping: `shouldNotify` cooldown plus `ManualOverrideGraceMinutes` already exist; a UI change of the toggle counts as a "manual override" to the gate (`quota_gate.go:~215-229`) and may suppress notifications for the grace window. Document, do not fix.

## Edge cases: `allow_auto_merge`

Facts (VERIFIED):
- Proto field `optional bool allow_auto_merge = 11`; "unset means not fetched / unknown, not disabled" (`backlog.proto:2137-2144`); handler leaves it unset and defers to a "Phase 4 Story 4.1.4" (`backlog_service_stuck.go:140-143`). The UI renders unknown for unset (`StuckItemDetail.tsx`, per requirements).
- Enable path: `gh pr merge N --auto --squash` (`session/git/worktree_git.go:1134-1148`). On failure it already sends an "Auto-merge not enabled" warning notification (`session/backlog_lifecycle_pr.go:718-726`). So requirement baseline "operator left to discover it" is overstated: a notification exists, but it fires only after the PR is created and the error text is raw `gh` output; the stuck panel and the pre-creation path say nothing.
- The only code that talks about repo-level setting is a comment: this repo itself has `allow_auto_merge` disabled and PRs are merged manually (`session/backlog_lifecycle.go:1925-1935`). That means the first real result of the lookup on this repo will be `false`, and C will immediately surface a warning on every item for stapler-squad itself. Decide whether that is noise.
- No GitHub repo-metadata client exists in `github/` (grep for repo GET found only `RepoRef`, commits, PR GraphQL/REST). A new call must go through `NewConditionalRequest`/`NewConditionalRequestNoCache` (`github/http_client.go:342,367`) per `.claude/rules/norawghrequest.md`. Whether the existing `ETagCache` can be reused for a repo endpoint is UNVERIFIED.

Edge cases:
1. "Known" semantics: repo `allow_auto_merge=true` does not guarantee `gh pr merge --auto` succeeds. GitHub also requires branch protection with required status checks on the base branch, or the call errors with "Pull request is in clean status". Both directions of error exist: field true yet enable fails; field false, so a warn is correct. Surface a tri-state plus reason (`enabled`, `disabled`, `unknown: <why>`), and keep the post-creation failure as the authoritative signal (it already notifies). A new bool cannot carry "unknown due to lookup failure" distinct from "not fetched"; proto optional-bool unset covers both, so add a status enum or a companion error string field.
2. Token scope: repo settings require read permission on the repo; `gh` auth may be a different identity than the token the native client uses, and the same repo can answer differently for GHE hosts (`RepoRef.Host()`, `github/repo_ref.go:41`). Lookup must use the host-aware request builder.
3. Items with no PR repo (local-only repo, `RepoPath` without a GitHub remote, Jules/remote sessions): leave unset and say "not applicable", not "unknown". UNVERIFIED how repo owner/name is resolved from `RepoPath` for this lookup; `NewRepoRef` needs owner/repo.
4. Caching and rate limit: key by repo (not by item) with a TTL; one stuck-panel render with many items from one repo must cost one request. Admission control for background calls exists (`feature_flag_service.go:264` describes priority-aware GitHub admission, default off); the lookup should declare a `CallOrigin` so it is treated as background. UNVERIFIED the exact API.
5. Staleness vs persistence: persisting on the item (rejected alternative) goes stale if the owner flips the repo setting; per-repo TTL cache avoids that and needs no migration. Recommend the cache, with the TTL surfaced as a counter label (success/failure/cached) per Observability Requirements.
6. Warn "before/at PR creation": `pushAndCreatePR` runs unattended; a pre-check warning must not block PR creation (requirements C says additive). Place it adjacent to the `EnablePRAutoMerge` call (`backlog_lifecycle_pr.go:718`) and dedupe with the existing failure notification so the operator gets one message, not two.
7. Branch-protection rule changes after PR creation: merge queue or required reviews can make auto-merge unavailable at merge time even if enabled; ReconcilePRPending's `pr_ready_unmerged` nudge (`backlog_lifecycle_pr.go:~2205`) remains the backstop and is out of scope.
8. Forks and org policy: a repo may allow auto-merge at repo level but the org may disable it; the REST field reflects the effective repo setting, so org policy is covered, but a read-only token returns the field only for users with push access (GitHub docs behavior; UNVERIFIED here).

## Items for other research tracks (not answered here)
- Open question 3 (soak evidence for nudge flags) and question 4 deeper (budget derivation) were not researched beyond reading config docs.
- `SubmitManualReview` (`backlog_service_lifecycle.go:1356-1446`) and `CreateBacklogItem` entry-stage enforcement were not read in full.
- Frontend check of what the stages settings UI currently validates (e.g. whether it blocks deleting built-in stages) was not done.
