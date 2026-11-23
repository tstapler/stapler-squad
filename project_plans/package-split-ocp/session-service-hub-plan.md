# Plan: the SessionService hub and `server/services` under 10k (2026-11-23)

Owner decision 2026-11-23: "every package under 10k" still applies to `server/services`. This plan measures the hub and the rest of the package, designs the target layout, and orders the PRs. **Nothing here was compiled or moved.** Labels: **V** = verified by a command named in the row; **I** = inferred, with the reason. It builds on [other-packages-under-10k-plan.md](other-packages-under-10k-plan.md) Part 3 (35 packages, 41 PRs, no adversarial review, its file lists are not in the checkout) and on the 2026-11-22 leaf ranking. Where this plan disagrees with them, section 7 says so.

## 0. Findings that change the plan

1. **The hub is thin; the package is not the hub.** `*SessionService` has 407 methods (299 exported) and 98 fields, but only 7,718 lines of method code (V, section 1). 121 of its 181 RPC methods are short delegators (at most 12 lines) to 27 sub-service structs that already live in the same package. The 65.8k lines are mostly those sub-services, HTTP handlers, workers and helpers.
2. **The "403 back-edge symbols" point downward.** Going file by file: the 31 core files (the ranking's CORE definition) use 397 symbols defined in 90 other files (V, graph). That edge is legal once the leaves are packages, because the hub imports them. The edge that blocks a move is the other direction: the rest of `server/services` needs **82 symbols** (63 package-level names and 19 methods) defined in hub files (V, graph). 82 of 82 are classified in section 3; none is genuine hub state except the `SessionService` receiver itself.
3. **One proto service, so the Remote Facade splits in Go, not in proto.** `SessionService` is a single proto service of 181 RPCs (V). Splitting it in proto would change every wire path (`/session.v1.SessionService/X`) that the web app, mobile app, MCP proxy and interceptors use. Go-level composition keeps the paths: a facade struct embeds one handler struct per RPC group, each in its own package. Section 2 shows that compiles, and that an uncovered or doubly-covered RPC is a compile error.
4. **The 10k goal needs stages 1 to 5 plus two hub extractions**; the full OCP hub redesign (stage 6c) is not needed for the size goal (section 6).
5. **The 10k decision is not gated anywhere.** `tools/bzl/scoping/t9.go:68` sets `LargestPkgLOCMax: 72000` (V). The plan ends with a gate change (stage 7).

## 1. Evidence base

Commands (V unless noted). Scripts were scratch files under `~/.cache/svc-hub-plan/` and are not committed; the method is reproducible from these lines.

| Fact | Command / source | Result |
|---|---|---|
| Non-test top-level lines and files | `ls server/services/*.go \| grep -v _test.go \| xargs cat \| wc -l`; file count | 65,807 lines, 268 files. `tools/bzl/scoping/budget.txt:46` says 65,715 / 262 files (GOOS-tagged files are excluded there). |
| Tests | `ls server/services/*_test.go \| wc -l`; `cat \| wc -l` | 363 files, 87,513 lines. 57 construct a `SessionService` (`grep -l 'NewSessionService\|&SessionService{'`). |
| Struct and methods | `go/parser` over `server/services/*.go`, receiver `*SessionService`, selectors `s.<field>` | 98 fields, 407 methods (299 exported, 108 not), 7,718 method lines, 54 `Set*` setters (218 lines, 31 in `session_service_wiring.go`). |
| Proto | `grep -E '^\s*rpc '` per `proto/session/v1/*.proto` | 340 RPCs in 18 services. `SessionService` has 181 (`session.proto`). |
| Registered handlers | `grep 'sessionv1connect.New.*Handler('` in `server/` | 18, one per proto service (`server/server.go:608,639,657,666,674,689,697,717,753,772,798,817,872,922,931,940,975`, `server/wake_trigger_wiring.go:40`). |
| File graph (I for staleness) | `services-graph.json` from the 2026-11-22 ranking (go/types, per-file defs and uses) | Built before the federation move: it lacks the 6 GOOS files (no edges) and still lists the 5 federation files, which I dropped. All 267 other file sets match the current tree. |
| Who imports the hub | `grep -rl '"github.com/tstapler/stapler-squad/server/services"'`, non-test | root 2 files, `server` 28, `server/services/backlogsvc` 18, `server/services/wsconn` 8, `tools/lint/noliveinstanceraw` 1. 18 test files outside the package name `services.SessionService`. |
| `server/mcp` | same grep over `server/mcp/*.go` | 0 files; it reaches the hub through the consumer-side ports of `mcp-single-root-plan.md` M4b-5. |

### 1.1 File map (V sizes by `wc -l`; the first-pass families are my grouping by file-name prefix, I; they sum to 65,807 and 268 files)

| Family | Files | Lines | What it serves |
|---|---|---|---|
| Hub (`session_service*`, `session_creation_*`, sweepers, `hidden_*`, `steer_authorization`, `terminal_*`, `workspace_service`, `external_*`, `*_wiring`, `capture_tap`, `tcc_enrichment`, `guards_flag`) | 48 | 15,789 | The 181 `SessionService` RPCs, 4 constructors, 54 setters, terminal and stream paths. The wider hub (every file with a `*SessionService` method plus these) is 54 files, 17,085 lines. |
| Files, VCS, search (`file_*`, `files_*`, `fsops`, `path_*`, `vcs_*`, `content_search*`, `search_*`, uploads) | 46 | 9,223 | `FileService` (8 RPCs), search, VCS status |
| GitHub, quota, providers (`github_*`, `credentials`, `capacity_*`, `quota_*`, `jules_*`, `callback_*`, `webhook_*`, `pr_*`) | 31 | 7,819 | `GitHubUserService` (15 RPCs), quota, Jules, webhook registration |
| Misc handlers (`defaults_service`, `utility_service`, `database_service`, `remote_service`, `diagnostic_service`, small RPC and HTTP handlers) | 32 | 7,072 | Settings, diagnostics, small RPCs |
| Approvals, hooks, questions (`approval_*`, `hook_*`, `question_*`, `pane_reply_*`, `pending_question_store`, `pi_extension_*`) | 26 | 6,684 | Approval and AskUserQuestion pipelines, HTTP hook receivers |
| Rules, analytics, tagging, AI clients | 13 | 4,802 | `RulesService` (12 RPCs), tagging, escape analytics |
| Workflow, backlog helpers (`workflow_*`, `backlog_*`, `completion_reviewer*`, `wake_trigger_*`, `review_queue_service`) | 19 | 4,001 | `WorkflowService` (9), `WakeTriggerService` (4), backlog helpers |
| Notification, Slack | 18 | 3,964 | Notification store, Slack notifier, interactive handler, notification actions |
| Autonomous, lease, claim checker | 7 | 2,244 | Autonomous orchestration |
| Guidance | 12 | 2,237 | `GuidanceRequestService` (6), hook mirror, ack detector |
| Feature flags | 7 | 1,364 | `FeatureFlagService` (2) |
| Gate stats (GOOS-tagged) | 9 | 608 | Gate stats store and writer |

Names in parentheses are examples; the catch-all row ("misc handlers") is whatever matched no other prefix. The exact partition used for the plan is section 5.

## 2. The ConnectRPC structure and the Remote Facade

### 2.1 Which Go struct serves which proto service (V: regex over `func (x *T) Name(... connect.Request[...] ...)` signatures)

349 connect-typed handler methods in 38 receiver types. `SessionService` holds 183 (181 proto RPCs plus 2 non-proto). The other 17 proto services each already have their own struct: 11 in the top level (`GitHubUserService` 15 RPCs, `GuidanceRequestService` 6, `RemoteService` 6, `TymuxRolloutService` 5, `WakeTriggerService` 4, `DiagnosticService` 2, `HandoffSummaryService` 2, `LLMBackendService` 2, `QuotaSettingsService` 2, `SessionSummaryService` 2, `HeadlessService` 1) and 6 in leaf packages (`backlogsvc`, `backlogflow`, `importsvc`, `insights`, `sessionbundle`, `unfinishedwork`). So the Remote Facade is already one struct per proto service except the 181-RPC one.

Inside the 181, 27 sub-service structs (`RulesService` 12 RPCs, `DefaultsService` 14, `WorkflowService` 9, `FileService` 8, `NotificationService` 6, ...) already implement the connect-typed methods; `SessionService` re-exports them through delegators.

| `SessionService` RPC methods | Count | Method lines | Source |
|---|---|---|---|
| Pure delegates (at most 12 lines, at most one field, no receiver calls; heuristic, I) | 121 | 665 | Mostly in `session_service_delegates.go` (111 methods, 101 of them RPCs, 1,021-line file) and `files_delegates.go` |
| Substantive | 60 | 3,896 | `StreamTerminal` 543, `CreateSession` 399, `UpdateSession` 242, `DeleteSession` 154, `WatchSessions` 148, `BatchCreateSessions` 123, escape analytics 289 in 3 RPCs, `RunOneShot` 111, ... |

### 2.2 Can the facade split into handler structs, one per concern?

Yes, at the Go level. `server/server.go:608` builds the one handler with `sessionv1connect.NewSessionServiceHandler(deps.SessionService, sessionOpts...)`; the interceptors (`NewScopedFeatureFlagInterceptor`, `NewFederatedSessionGuard`, `server/server.go:600-603`) and the three WebSocket bridge paths (`server/server.go:617-619`) are keyed by procedure name or path, which a Go-level split does not change.

The mechanism is struct embedding with disjoint method sets. I checked the Go semantics with two toy programs (scratch, not committed):

- `struct{ *PkgA; *PkgB }` where each embeds half of an interface compiles and satisfies it with `var _ Handler = (*Facade)(nil)`. **V.**
- Embedding `Unimplemented` next to `*PkgA` at the same depth fails: `*Facade does not implement Handler (ambiguous selector *Facade.A)`. **V.** So the facade must not embed `UnimplementedSessionServiceHandler`; the compile-time assertion `var _ sessionv1connect.SessionServiceHandler = (*SessionService)(nil)` (`server/services/session_service.go:56`) then fails if any of the 181 RPCs is uncovered or served twice. That is a stronger guard than today's.

Tests already embed `sessionv1connect.UnimplementedSessionServiceHandler` (`server/ws_unary_chain_test.go:73`, `cmd/commands/commands_test.go:21`), so the generated type exists.

**Target facade:** `services.SessionService` stays the public name (about 20 non-test files outside the package name it, some in comments only, and 18 test files, so no codemod), but its struct becomes embeds only:

```go
type SessionService struct {
    *sessionrpc.Handler     // lifecycle, crud, update, shells, PR refs
    *sessioncreate.Handler  // CreateSession, BatchCreateSessions, FanOutSession, background pipeline
    *sessionstream.Handler  // WatchSessions, StreamTerminal, events, detection
    *rules.Service; *settings.Service; ... // one embed per sub-service group; 121 delegators disappear
}
```

Each handler struct holds a `Deps` struct of consumer-side ports, not `*SessionService`. Constructors move to the composition root (`server/dependencies*.go`), which already names concrete types.

### 2.3 Dependency matrix: method by field (V counts; clustering is I)

Direct field use (top of 98): `eventBus` 34 methods, `storage` 31, `ReviewQueuePoller` 21, `rulesSvc` 15, `AutonomousSvc` 16, `defaultsSvc` 17, `statusManager` 10. The rest are used by 1 to 12 methods. I treat `storage`, `eventBus`, `concStorage`, `statusManager`, `ReviewQueuePoller` as ubiquitous (`sessionkit.Core`, below).

After removing the five ubiquitous fields:

| Measure | Result |
|---|---|
| RPCs whose transitive field closure (through receiver-method calls) has exactly 1 non-ubiquitous field | 138 of 181 |
| RPCs with 0 | 14 |
| RPCs with 2 to 12 | 22 |
| RPCs with 14 to 33 | 7 (the lifecycle core: `CreateSession`, `BatchCreateSessions`, `FanOutSession` at 33, or 38 counting the ubiquitous five) |
| Connected components of methods that share a non-ubiquitous field | 93; one giant component of 235 methods (85 RPCs, 5,685 lines, 70 fields); 73 singletons |

The giant component is created by a few shared helpers, not by RPC logic. Receiver-method in-degree (V): `FindLiveInstance` 28, `findInstance` 11, `workflowNames` 11, `loadInstancesWithWiring` 9 (closure of 13 fields), `resolveMCPServerURL` 8, `replyState` 7, `indexSessionForDelivery` 6, `removeFromAllPollers` 5, `livePRInstance` 5 (closure 13), `wireCallbacks` 4 (closure 12). These ten move into `sessionkit` (about 200 method lines, I) and the giant component falls apart into the three hub handlers below.

**Candidate cohesive sub-services of the 181 RPCs:**

| Handler | RPCs and files (I) | Own fields beyond `sessionkit.Core` |
|---|---|---|
| `sessioncreate` | `CreateSession`, `BatchCreateSessions` (in `crud`), `FanOutSession`, `PickFanoutWinner`, `PreviewDestinationPath`, `RetrySessionCreation`; `session_creation_*`, stale/superseded sweepers | `inFlightWorktreeSpawns`, `inFlightConversationClaims`, `githubResolver`, `historyLinker`, `taggingEngine`, `mcpServerURLFn`, `registry`, `quotaGate`, backlog listener |
| `sessionstream` | `StreamTerminal`, `WatchSessions`, `GetSessionDiff`, `GetDetectionEvents`, `ListErrors`, `ArchiveSession`, terminal service, `tmux_input_send`, external streamers | `deliveryGate`, `tapRegistry`, `externalDiscovery`, `tmuxStreamerManager`, `errorRegistry` |
| `sessionrpc` | lifecycle (delete, restart, hibernate, retry, resume, crash, unarchive), crud (get, list, update), shells, PR refs, liveness, workspace | `deleteCleanup*`, `testTmuxServerSocket`, `prRefs`, `crashes` |
| Per-concern services | 27 sub-service structs with their own file(s) | one field each |

## 3. The back-edge symbols, classified

### 3.1 Direction and size (V, graph)

| Direction | Distinct symbols | Meaning |
|---|---|---|
| Core files (31 files, 12,726 lines, the ranking's definition) use symbols in 90 non-core files | 397 (207 method names, 115 exported, 75 unexported) | Hub to leaf. Legal after the split. I read the ranking's "403" as this edge (I: 397 now, and the graph predates the federation move; I did not recount the 403 itself). Largest targets: `defaults_service.go` 19, `github_user_nudge.go` 18, `rules_service.go` 17, `feature_flag_service.go` 16, `autonomous_orchestration_service.go` 16. |
| Non-hub files use symbols defined in the wider hub (54 files, 17,085 lines: every file with a `*SessionService` method plus the core) | 82 (63 package-level names, 19 methods) | Leaf to hub. This is what blocks a leaf. |

### 3.2 The 82 (V list; categories are my reading, I)

| Category | Count | Symbols (examples) | Fix |
|---|---|---|---|
| Pane-reply kernel | 21 | `PaneReplyCore` and its `Run/appendRequest/appendResult`, `replyStage*`, `replyInput`, `replyOutcomeLabel`, `replyWriteError`, `replyFunc`, `replyLimiters.reserve`, `truncateRunes` | Defined in `pane_reply_core.go`, whose `SessionService` methods are the only hub part. Move the file to the question-and-reply package; leave 1 receiver adapter. |
| Steer kernel | 16 | `SteerOutcome` and 8 constants, `ErrSteerPaneOwnership`, `NudgeRecorder`, `steerRequestFacts`, `requestFactsOf`, `resolveTargetPR` | Value types in `session_service_guarded_steer.go` and `steer_authorization.go`; extract to `steer` kernel. |
| Terminal-access kernel | 15 | `AccessFor`, `AnswerTarget` (+2 methods), `AnswerTargetVisible`, `AuthModeRequired`, `HumanActiveGuard`, `ListenerLocal/Remote`, `LiveInstanceFinder`, `RequestRecordFrom`, `TerminalReadOnly/ReadWrite` | `terminal_access.go`, `human_activity.go`; already interfaces. Kernel package `access`. |
| Action-result kernel | 12 | `ActionResult`, `QuestionAnswer`, `actionFlags(.Answer)`, `applied`, `notWired`, `outcome`, `mapReplyOutcome*`, `optionDigit`, `pendingQuestionPeeker(.Peek)` | `notification_action_service.go`; belongs with the actions package. |
| Interfaces defined in the hub but consumed elsewhere | 12 | `ReactiveQueueManager`, `ScrollbackSequencer`, `WorkspaceProvider`, `BacklogDecider`, `BacklogDecision`, `NotificationResolver`, `itemSessionLookup` | Move to the consuming package (rule: define where consumed). |
| Flag-name constants | 3 | `hiddenSessionReadonlyGuardsFlagName`, `hiddenSessionReplyFlagName`, `terminalWriteLeaseFlagName` | Flag package. |
| Hub state | 2 | `SessionService`, `SessionService.findInstance` | `LiveInstanceFinder` port (already exists). |
| Pure helper | 1 | `vcsStatusToProto` | Move beside VCS status. |

So **no category is genuine hub state**. A shared `server/rpcutil` is not the unblocker: the shared pieces are domain kernels (access, steer, pane reply, action result, flags, fsops), not error mapping or auth extraction, which live in `server/adapters`, `server/interceptors` and `server/auth` already (I: none of the 82 is an error-mapping, DTO-conversion or event-bus helper; the one DTO helper is `vcsStatusToProto`).

### 3.3 Cycles among the leaves (I, file-granularity simulation)

Assigning files to a first-pass, coarser partition than section 5 (12 families) and running SCC over the package graph without the hub gives one component holding all 12 families. The edges that keep it tied together (symbols in parentheses; V per file pair via the graph):

| Edge | Symbols | Cut |
|---|---|---|
| `flags` and `attention` (feature_flags_notification_actions.go holds `flagReader` and the notification flag names) | 27 | `flagReader`, flag-name consts, `FlagRegistry` accessors into the `flagsvc` kernel (stage 1). |
| `attention` and `approvals` | 24 | One package, or `ApprovalStore`/`PendingApproval` into the question kernel. |
| `guidance` and `attention` (`PendingQuestionStore`, `GuidanceHookMirror`, `decodeGuidanceOptions`, `tabbedFromRow`) | 24 | `questionkit` kernel (`pending_question_store`, `askuserquestion_parser`, `guidance_request_hook_mirror`, ...). |
| `attention` to `fileops` (`hidden_reply_audit.go` uses `fsOps` and `osFS`, 13 symbols) and `fileops` to `flags` (9), `gatestats` to `fileops` (8) | 30 | `fsops` kernel. |
| `providers` to `rules` (`anthropic_limits_client.go` uses 6 symbols from `anthropic_client.go`) | 6 | `anthropicwire` kernel. |

## 4. Blocked families: why, and what unblocks them

"stay" and "back" are the ranking's SC1 counts (symbols the staying files use, services-internal symbols the cluster needs).

| Family | Lines | Why blocked (ranking) | Cause found here | Unblock |
|---|---|---|---|---|
| Guidance (12 files) | 2,237 | stay 25, back 42, 14 hub methods | Needs `PendingQuestionStore`, `AnswerListener`, `tabbed*` from the question pipeline; 2 files hold `SessionService` receivers (`guidance_ack_wiring`, `guidance_runtime_wiring`). | `questionkit` (stage 1), then move; the 2 wiring files stay as hub adapters. |
| Approvals, hooks, questions, pane reply, hidden (28 files) | 7,530 | stay 117, back 104 | 47 hub symbols (kernels in 3.2) plus the `notification_action_adapters` and `question_answer_router` knot (29 and 23 symbols both ways). | `access`, `steer` kernels; merge questions with notification actions; 7 receiver files convert. |
| Notification, Slack, notification flags (18 files) | 3,882 | stay 71, back 79 | Needs approval store and pending-question types (29 symbols), `DerivePriority`, flag reader. | `questionkit`, `flagsvc`, `DerivePriority` (19 uses by `backlogsvc`, Part 3 gap) to `notify`. |
| File service, vcs, search (36 files) | 6,551 | stay 46-80, back 49-51 | `FileService` takes `flagReader` and `FlagRegistry` (9 symbols), `WorkspaceProvider`; `vcs_*` and `file_*` call each other (28 and 7). | `flagsvc`, `fsops`, consumer-side `WorkspaceProvider`; keep `fileops` and `vcssearch` separate with `vcssearch` above `fileops`. |
| Rules, analytics (6 files) | 3,610 | stay 27+16, back 21 | `anthropic_client.go` types shared with limits clients; 2 receiver files. | `anthropicwire`. |
| Autonomous, lease, claim checker (7 files) | 2,242 | stay 24, back 6 | `lease_wedge.go` receivers (4 methods); roadmap already rejected a driver-host interface (`tierc-roadmap.md` SVC-autonomous). | Stays one package; convert `lease_wedge` receivers to a struct over `LiveInstanceFinder`. |
| GitHub user, limits, credentials, capacity, AI clients, flags (21 files) | 5,398 | stay 14-52, back 7-21 | 15 hub symbols (all steer kernel plus `SessionService`) used by `github_user_nudge.go`; feature-flag file. | `steer` kernel, `flagsvc`. |

## 5. Target architecture

### 5.1 Packages (V LOC from the file assignment; the assignment itself is a design, I)

Sizes recount to 65,807 (sum of per-file `wc -l` over the assignment). Names reuse Part 3 where they match (`approvals`, `qareply`, `notifyactions`, `flags`, `guidance`, `autonomous`, `notify`, `fileops`, `rules`, `sessioncreate`).

| Layer | Package under `server/services/` | Lines | Contents (file prefixes) |
|---|---|---|---|
| L0 kernels (import only domain/config/`session`) | `access` | 1,544 | `terminal_access`, `human_activity`, `guards_flag`, `lease_flag`, `local_write_verdict`, `terminal_input_metrics`, `session_nudge_guard`, steer value types (`steer_authorization`, `session_service_guarded_steer`: 569 lines of these are `SessionService` receivers and move last) |
| | `flagsvc` | 1,695 | `feature_flag_*`, `feature_flags_*` |
| | `questionkit` | 1,716 | `pending_question_store`, `askuserquestion_parser`, `question_delivery_track`, `answer_listener`, `guidance_request_hook_mirror`, `guidance_request_mirror_proto`, `guidance_dialog_*`, `notification_quality_metrics`, `reply_idempotency` |
| | `fsops` | 1,392 | `fsops`, `file_atomic`, `file_backend*`, `file_inode_*`, `file_codec`, `file_metrics`, `files_telemetry`, `dup_fd_*`, `exec_*`, `settings_file_atomic_write` |
| | `anthropicwire` | 829 | `anthropic_client`, `anthropic_limits_client`, `gemini_limits_client`, `cli_ai_client`, `selecting_ai_client`, `ai_interfaces` |
| L1 leaves (import L0 and `session*`, never the hub) | `attention` (split per Part 3 into `approvals` about 5.7k, `qareply` about 2.6k, `notifyactions` about 2.1k, not re-measured) | 9,148 | `approval_*`, `hook_*`, `question_*`, `pane_reply_*`, `notification_action*`, `hidden_*`, `pi_extension_*`, `claim_checker`, `mcp_proof` |
| | `providers` | 6,901 | `github_*`, `credentials`, `capacity_*`, `quota_*`, `provider_limits`, `jules_*`, `callback_*`, `webhook_*`, `domain_checker`, `rate_limiter` |
| | `adminrpc` (split by role at author time, about 2k to 3.5k each) | 5,790 | `database_service`, `defaults_service`, `config_service`, `utility_service`, `project_service`, `launcher_*`, `remote_service`, `claude_settings_*`, `slash_command`, `client_log`, `log_level`, `circuit_breaker`, `debug_snapshot`, `error_registry`, `diagnostic_service`, `tymux_rollout`, `stream_hub_rollout`, `tmux_version`, `tcc_enrichment`, `capture_tap` |
| | `fileops` | 4,931 | `file_*`, `files_*`, `path_*`, `dir_cache`, `gitignore_cache`, `local_file_service`, uploads |
| | `rules` | 4,536 | `rules_*`, `analytics_*`, `tagging_*`, `escape_code_handler` |
| | `vcssearch` | 3,617 | `vcs_*`, `content_search*`, `search_*`, `session_service_artifact_*` |
| | `backlogsync` | 2,524 | `backlog_*`, `completion_reviewer*`, summaries, `headless`, `llm_backend` |
| | `workflows` | 2,053 | `workflow_*`, `review_queue_service`, `checkpoint`, `wake_trigger_*` |
| | `notify` | 1,958 | `notification_*`, `slack_*`, `backlog_notifier`, `stale_session_notifier`, `memory_pressure_notifier` |
| | `autonomous` | 1,956 | `autonomous_*`, `lease_wedge` |
| | `guidance` | 1,474 | `guidance_*` (non-kernel) |
| | `prs` | 626 | `pr_*`, `session_pr_holds`, `active_pr_status` |
| | `gatestats` | 608 | `gatestats_*` |
| L2 hub handlers (import L0 and L1) | `sessionrpc` | 4,903 | lifecycle, crud, update, shells, PR refs, liveness, workspace |
| | `sessioncreate` | 2,112 | create, `session_creation_*`, sweepers |
| | `sessionstream` | 2,372 | stream, watch, events, diagnose, terminal service, external streamers |
| L3 facade (the only package `server`, `backlogsvc` and `wsconn` import) | `server/services` | 2,947 + 175 unassigned (`external_*`) | `session_service.go` struct, embeds, wiring; target after delegator removal and constructors moving to the root: about 2k (I) |

Largest package is `attention` at 9,148 (92% of the cap, over Part 3's own 8k stop rule), so it ships already split. `sessionkit` (the ten shared helpers, about 200 lines) sits beside `access` at L0. The rows above sum to 65,807 including `external_*`.

`backlogsvc` (15,641 lines, `wc` over `server/services/backlogsvc/*.go` non-test) and `wsconn` (9,767) are also near or over 10k. They are out of scope here; `backlogsvc-ocp-plan.md` and Part 3 (BS-1 to BS-6) cover backlogsvc. **V** (`ls`/`cat | wc -l`).

### 5.2 Dependency rule and wiring

- Leaves never import the hub. The hub imports leaves and kernels. Kernels import neither. A leaf imported by a sibling leaf needs an `isolate_<leaf>` depguard row that lists it (`.claude/rules/leaf-packages.md` rule 4).
- Ports are consumer-side (`rules/leaf-packages.md` rule 3); `LiveInstanceFinder`, `SessionDeleter`-style interfaces already exist (`sessionretention` precedent: `package-layout.md:47`).
- `server/dependencies*.go` builds every handler and the `Deps` of each; the 54 `Set*` setters become `Deps` fields. This overlaps backend-di-refactor Story 1.3 (owner Q4, 2026-11-20): stage 6c waits for or builds on it.
- Open-closed: a new RPC group is one new package with a `Handler`, one embed line in `services.SessionService`, one constructor call in the root. Forgetting the embed is a compile error (2.2). A new proto service is one package and one `connect` registration in `server.go`, as today.

## 6. Staged PR sequence

Cost classes: **S** = one author wave plus CI; **M** = author plus two review lenses, port design; **L** = characterization tests first, multi-wave; sizes are agent work, not time. "After" is the top-level non-test line count of `server/services` by `wc` on my assignment (V arithmetic, I for the assignment): 65,807 start.

| Stage | PRs | Content | Moves out | Top level after | Ports and risk | Test plan | Rollback |
|---|---|---|---|---|---|---|---|
| 0 | 2 S | Guard hardening before any move (section 7): `noliveinstanceraw` multi-package; analyzer path-existence test; make `TestMain`/`svctest.Main` mandatory for new packages; sinkguard discovery already auto. Per-stage `budget.txt` ratchet rows. | 0 | 65,807 | none | `make ready`, analyzer tests | revert PR |
| 1 | 5 M | Kernels: `access` (receiver-free files), `flagsvc`, `questionkit`, `fsops`, `anthropicwire`. Move the 82-symbol definitions to their owners. | 6,607 | 59,200 | Exported names become the API; `flagReader`, `fsOps` need exported forms. No new interfaces except `WorkspaceProvider` moved to consumer. Risk: 363 test files; 57 build `SessionService`. | `go test ./server/...`, `make test-delivery-guards`, mutation check per moved branch | `git revert` per PR; no alias left to unwind |
| 2 | 11 S | READY clusters from the ranking (uploads, remote, claude settings, misc handlers, summaries, hook receivers, completion reviewer, forward sync with 1 port, search, 2 singles). Federation is done. | 6,715 | 52,485 | `search` is at the SC1 cap (10); `backlog_debug_mutate_handler.go` is path-keyed in 3 places (section 7) | per-cluster; mutation checks | revert |
| 3 | 6 M | NEEDS-PORTS small: webhooks, quota core, gatestats (needs `fsops`), tagging rules, rollouts, sweepers' non-receiver files | 4,374 | 48,111 | 10 test files share approval fixtures (ranking); 1 to 4 hub methods per cluster (ranking) | as above | revert |
| 4 | 5 L/M | Attention cluster, after stage 1: `approvals`, `qareply`, `notifyactions` (3 L), `guidance` (M), `notify` (M). 9 receiver-bearing files convert from `(s *SessionService)` to a struct over ports. | 11,535 | 36,576 | Highest port count: 51 symbols from `attention` to the hub today; most become kernel imports. Risk: AskUserQuestion routing (ADR-005 `docs/explanation/askuserquestion-answer-routing.md`) and the `noarchivedrevival` file `question_answer_router.go`. | per-package characterization tests of the router before the move | revert; the router has a guard test |
| 5 | 14 M | Remaining L1: providers (2), rules (2), `adminrpc` (3), `fileops` + `vcssearch` (3), `workflows`/`backlogsync`/`autonomous`/`prs`/`access` receivers (4). 12 receiver-bearing files. | 24,067 | 12,509 | `providers` needs the steer kernel (15 symbols) | whole-tree golangci, `make test-delivery-guards` | revert |
| 6a | 1 L | `sessionkit` (10 shared helpers) + `sessionstream` handler. 51 receiver methods; `StreamTerminal` is 543 lines in one method. | 2,372 | 10,137 | `terminal_input_sites_scan_test.go` globs the top level only (section 7) | stream integration tests, e2e terminal specs | revert |
| 6b | 1 L | `sessioncreate` handler: `CreateSession` closure is 38 fields. | 2,112 | **8,025** | Highest cycle risk: `sessioncreate` and `sessionrpc` call each other 4 and 4 times; `sessionstream` and `sessionrpc` 6 and 3 | `session_service_create_test.go` and 57 constructor tests; `make test-race` | revert |
| 6c | 3 L | `sessionrpc` handler, facade becomes embeds only, delegators and constructors removed, `Set*` setters to `Deps`. Optional for size. | 4,903 and delegator removal | about 2.1k (I) | `noliveinstanceraw` monitored funcs move (section 7); Story 1.3 overlap | full suite | revert in 3 steps |
| 7 | 1 S | Set `LargestPkgLOCMax` to 10000 in `t9.go`, update docs (`package-layout.md`, `CLAUDE.md` Status line), final budget ratchet. | 0 | 8,025 or less | none | `go run ./tools/bzl/scoping t9` | revert |

PR count 2+5+11+6+5+14+1+1+3+1 = **49**, against Part 3's 41 for the same area (Part 3 counts 35 packages, this plan 21 plus the split of `attention` and `adminrpc`).

**Minimum path to every package under 10k:** stages 0, 1, 2, 3, 4, 5, 6a and 6b (stage 0's gate is hygiene). Stage 5 ends at 12,509; subtracting `sessionstream` gives 10,137 (still over); subtracting `sessioncreate` as well gives 8,025. Stage 6c is for the open-closed property and the thin facade, not for the size cap.

### 6.1 Estimation and wall-clock blockers

- **Agent cost:** Stage 2 and stage 3 are the cheap, parallel-friendly waves (the federation PR, `94acd8ec7`, was 37 files and 231 added lines and is the template). Stages 4 and 6 hold the L-class work (8 L PRs, each needing a characterization wave before the move and a review wave after). Totals by class: 14 S, 27 M, 8 L, no XL (stage 4's three L and stage 6's five L).
- **Wall-clock blockers (named, not hours of agent work):** CI restarts on every push and the make-only run takes about 2 hours (`.claude/rules/leaf-packages.md`, "Verify before you push"); at most 3 open split PRs (`roadmap.md` "Rules for executing", ADR-010 decision 4) and shared rows (`budget.txt`, `boundaries.txt`, `.golangci.yml`, `tools/bzl/scandata/BUILD.bazel`) force serial rebases, so the 49 PRs need at least 17 serial CI rounds; every PR that shares a row must be rebased after the previous merges. Owner answers to Q1-Q5 below gate stages 4, 5 and 6c. Disk: btrfs metadata and Go cache regrowth (`feedback_clean_worktrees_as_we_go`) need `df` and `btrfs filesystem usage` checks before each wave. (Memory notes; not re-verified today beyond today's `df`: 142 GB free, metadata 84%.)

## 7. Silent-guard inventory (what a move can break without a compile error)

Loud = fails a default `go build`, `go test` or `make lint`. Silent = passes while the guard no longer guards.

| # | Guard | Where (V) | Keyed by | Failure if its file or method moves | Action in the move PR |
|---|---|---|---|---|---|
| 1 | `noarchivedrevival` | `tools/lint/noarchivedrevival/analyzer.go:48-53` (file list), `:59` (`HasSuffix`) | file path suffix | Silent: the moved file is simply unscanned. Entries today: `question_answer_router.go`, `session_service_clear_terminal.go`, 4 federation files | Re-key the path; stage 0 adds a test that every listed path exists. Also `.claude/rules/noarchivedrevival.md` text. |
| 2 | `noliveinstanceraw` | `analyzer.go:58,74` | package path `.../server/services` and monitored methods `IsSessionLive`, `KillTmuxPaneOnly`, `StopSessionByUUID` | Silent: `run` returns nil for any other package, so a moved method is never checked | Stage 0: accept a package set; stage 6c moves the 3 methods and the analyzer rule together. |
| 3 | `nosessiondrivertest`, `norawexec`, `norawdb`, `norawgitopen`, `nocommandpattern` | `tools/lint/*/analyzer.go` (`isDriverPackage` at nosessiondrivertest `:31`; exempt lists in the others) | package exemptions only | New packages inherit the rule; no lapse | None. `norawgitcli` exempts `server/services/internal/svctest` (`:54`): keep that path. |
| 4 | `sinkguard` pins | `server/services/terminal_write_guards_test.go:60-64,522` (file names `terminal_access.go`, `steer_authorization.go`, `hidden_review_steer.go`, `pending_question_store.go`); `terminal_write_ui_exceptions_test.go:18-22` (file plus receiver); `terminal_write_pins_test.go` (keys `server/services:SessionService.X`, per Part 3); `server/deliverygate/sink_guard_test.go:28,66-69` (keys `.../server/services.WatchSessions`, ...) | file name, receiver, `pkgpath.Func` | Loud only under `-tags sinkguard`. The default `go test` skips them (`//go:build sinkguard`). CI runs them in a separate step (`.github/workflows/build.yml:1180`, `Makefile:860-875`, which discovers directories by `grep`, so a new package is picked up) | Run `make test-delivery-guards` in every PR touching stage 1, 4, 6a. |
| 5 | Scanner tests in the package | `home_env_test_helper_test.go:48-98` (`walkSourceDirs(".")`, `CoversEveryLeafDir`); `terminal_input_sites_scan_test.go:25` (`Glob("*.go")`, top level only); `workflow_bypass_inventory_test.go:187` (module `WalkDir`); `no_flag_default_global_test.go:17` (`WalkDir(".")`); `feature_flag_registration_test.go:56` (`servicesSourceDirs`) | directory walk or glob | New leaf dirs are scanned by the walkers. `terminal_input_sites_scan_test` is stale-checked, so moving `StreamTerminal` is loud, but a new input site in a new package is invisible | Make `terminal_input_sites_scan_test` walk subdirectories in stage 0. |
| 6 | Path-keyed registries | `workflow_bypass_inventory_test.go` (6 `server/services/*.go` rows: `backlog_debug_mutate_handler.go` x3, `autonomous_orchestration_service.go` x2, `notification_action_backlog.go`); `session/domain/transition_callers.go:104,109`; `session/legacy_item_pr_readers_test.go` (3 hits) | file path | Loud (ghost/stale checks, constants) | Re-key in the same PR. |
| 7 | Feature registry | `docs/registry/features/backend/*.json`: 130 `handlerFile` values under top-level `server/services` (and 74 under `backlogsvc`, 6 `insights`, 4 `importsvc`, 3 `devtools`, 2 `sessionbundle`, 2 `server/mcp`, 1 `federation`) | file path | Loud (registry-diff CI) | `make registry-generate`; commit the changed entries (up to 130 for top-level files). |
| 8 | `.golangci.yml` | `:604-660` (`no_ent_in_services` exclusions by path: `session_service.go`, `session_service_wiring.go`, `workflow_service.go`, `analytics_escape_service.go`, `error_registry.go`, `session_summary_service.go`, `handoff_summary_service.go` and test files), `:776-794` (forbidigo `LoadInstances` exclusions by regex), `:107-112` (`push_no_services`), `:396` (`isolate_featureflags`) | path globs | Loud, but only in whole-tree CI lint (`.claude/rules/leaf-packages.md`) | Move exclusions with the file; add `isolate_<leaf>` row per new leaf; run `golangci-lint --enable-only=depguard,unused` whole-tree locally. |
| 9 | Scoping | `tools/bzl/scoping/budget.txt:46` (services row), `boundaries.txt` (12 `server/services/*` rows), `t9.go:68` cap 72,000 | package | Loud for budget/boundaries; **T9d never fails for 10k** | New rows per package, ratchet the services row each stage, change the cap in stage 7. |
| 10 | Bazel | `server/services/BUILD.bazel:773-792` (`data` globs of `go_sources`), `tools/bzl/scandata`, root and `server/BUILD.bazel` | generated | Loud (`bazel build --nobuild //...` is blocking) | `make gazelle`, `go run ./tools/bzl/scandata -fix .`; the federation PR touched `BUILD.bazel`, `server/BUILD.bazel`, `tools/bzl/scandata/BUILD.bazel` plus per-leaf BUILD. |
| 11 | Interceptors and WebSocket bridge | `server/server.go:600-603` (`NewScopedFeatureFlagInterceptor`, `services.ProgramCLIFlagProbeGatedMethod`), `:617-619` | procedure name | Not affected by Go-level facade (one `NewSessionServiceHandler`, `:608`). The constant's import path changes if `ProgramCLIFlagProbeGatedMethod` moves: loud | None beyond the import. |
| 12 | `svctest` driver-leak check | `server/services/internal/svctest/main.go:19` (`Main`), `driver_leak.go` (matches goroutine stacks containing `/session/driver.StartSessionDriver.func`) | stack text, package independent | Silent per package: only `server/services/backlogsvc/testmain_test.go` and the top-level `testmain_test.go` call `svctest.Main` (`ls server/services/*/testmain_test.go`). A new package that starts drivers without it leaks unseen. | Stage 0: every new package gets `testmain_test.go`; a guard test lists packages importing `session/driver`. |
| 13 | `server/mcp` import set | `server/mcp/import_set_guard_test.go` (exact equality) | import path | Loud if `server/mcp` imports a new kernel | Adjust only if a kernel is used there. |

## 8. Reconciliation with the earlier Part 3 ledger

- Part 3's "112 upward symbols" and this plan's "82 leaf-to-hub symbols" count different sets (Part 3: cut by in-package pre-moves; here: symbols defined in 54 hub files). I did not reproduce 112; its ledger is not in the checkout.
- Part 3 chose `svcbase` for `fsOps`; this plan calls it `fsops` and keeps it narrow (1,392 lines of file I/O) rather than a general base package (`svcbase` risk in Part 3 section 6).
- Part 3's `sessionlife`/`terminal` packages correspond to `sessionrpc`/`sessionstream` here. The stage order differs: this plan moves all leaves before the hub so that hub handlers import finished leaves.
- Part 3's cross-part gap (`DerivePriority`, `SessionStopper`, `SessionSteerer`, claim types, `Steer*` consts, `ReasonName*`) is partly closed here: the `Steer*` constants and `NudgeRecorder` go to the steer kernel (stage 1); `DerivePriority` goes to `notify`; the rest are unresolved.

## 9. Open questions that need the owner

1. **Go-level facade or proto split?** This plan keeps one proto `SessionService` and splits in Go. A proto split would change wire paths for the web app, mobile app and MCP proxy. Confirm.
2. **Is stage 6c in scope, or is the size goal enough?** Stages 0 to 6b reach 8,025; 6c only makes the facade thin.
3. **Kernel packages exported or `internal`?** `server`, `server/mcp` and `wsconn` need `access` and `flagsvc` types, so `internal/` would block them. Default here: exported.
4. **`attention` at 9,148.** Ship as three packages (`approvals`, `qareply`, `notifyactions`) per Part 3, or accept one package at 92% of the cap? Default here: three.
5. **The 10k gate.** Change `LargestPkgLOCMax` to 10,000 in stage 7 (this also fails `backlogsvc` at 15.6k until its plan lands), or ratchet per package through `budget.txt` only?
6. **Part 3 budget basis.** Which base is authoritative: `wc` (65,807), the budget row (65,715), or t9? Earlier Part 3 asked the same; I used `wc`.
7. **Does Story 1.3 (backend-di) land before 6c?** Both rewrite the 54 `Set*` calls.

## 10. Self-review and limits

- Not run: `go build`, `go vet`, `go test`, golangci, Bazel, `scoping criteria`, any move. `gen/` and `session/ent` were not generated in this worktree, so no type information was used. My own measurements are syntax-level (go/parser plus regex): field use via `s.<field>` selectors, so a field passed through a helper or an embedded call is missed (the method-call closure covers receiver helpers only).
- The file graph is the ranking's go/types artifact, not regenerated; it predates the federation move. 82 vs 397 depend on it.
- The package assignment (section 5) is a regex partition over file names. It balances LOC and keeps the cycle edges named in 3.3, but it was not run through `scoping move --dry-run`. Per-package sizes can shift by a few hundred lines when symbols are cut inside files (`capture_tap_service`, `workspace_service`, `notification_action_service`, `hidden_review_steer`, `steer_authorization`).
- The "pure delegate" count (121) is a size-and-shape heuristic (at most 12 lines, one service field, no receiver call). The stage 6c delegator removal depends on it; the compile-time assertion in 2.2 is the real check.
- Receiver-bearing files outside the hub packages: 21 files, 77 methods, 1,409 lines (V, go/parser over the receiver methods): attention 7 files, guidance 2, access 4, adminrpc 3, rules 2, fileops 1, autonomous 1, backlogsync 1. They convert in stages 4 and 5 and are the main source of per-stage port work.
- Citing the previous ranking: the READY and NEEDS-PORTS cluster lists in stages 2 and 3 are from `leaf-candidates.md` (2026-11-22, scratch artifact, not in the repo); only their line counts were re-added (6,715 and 4,374).
