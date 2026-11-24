# Plan: the SessionService hub and `server/services` under 10k (2026-11-23)

Owner decision 2026-11-23: "every package under 10k" still applies to `server/services`. This plan measures the hub and the rest of the package, designs the target layout, and orders the PRs. **Nothing here was compiled or moved.** Labels: **V** = verified by a command named in the row; **I** = inferred, with the reason. It builds on [other-packages-under-10k-plan.md](other-packages-under-10k-plan.md) Part 3 (35 packages, 41 PRs, no adversarial review, its file lists are not in the checkout) and on the 2026-11-22 leaf ranking. Where this plan disagrees with them, section 7 says so.

## 0. Findings that change the plan

1. **The hub is thin; the package is not the hub.** `*SessionService` has 407 methods (299 exported) and 98 fields, but only 7,718 lines of method code (V, section 1). 121 of its 181 RPC methods are short delegators (at most 12 lines) to 27 sub-service structs that already live in the same package. The 65.8k lines are mostly those sub-services, HTTP handlers, workers and helpers.
2. **The "403 back-edge symbols" point downward.** Going file by file: the 31 core files (the ranking's CORE definition) use 397 symbols defined in 90 other files (V, graph). That edge is legal once the leaves are packages, because the hub imports them. The edge that blocks a move is the other direction: the rest of `server/services` needs **82 symbols** (63 package-level names and 19 methods) defined in hub files (V, graph). 82 of 82 are classified in section 3 (86 by the type-based review; 3.3 absorbs the extra classes); none is genuine hub state except the `SessionService` receiver itself.
3. **One proto service, so the Remote Facade splits in Go, not in proto.** `SessionService` is a single proto service of 181 RPCs (V). Splitting it in proto would change every wire path (`/session.v1.SessionService/X`) that the web app, mobile app, MCP proxy and interceptors use. Go-level composition keeps the paths: a facade struct embeds one handler struct per RPC group, each in its own package. Section 2 shows that compiles, and that an uncovered or doubly-covered RPC is a compile error.
4. **The 10k goal needs stages 0 to 5 plus two hub extractions** (top level 8,645 after 6b); the full OCP hub redesign (stage 6c) is not needed for the size goal (section 6).
5. **The 10k decision is not gated anywhere, and the gate is not a largest-package check.** `tools/bzl/scoping/t9.go:68` sets `LargestPkgLOCMax: 72000`, but T9d (`t9.go:90`) compares only `server/services` and `session` against it (V). Stage 7 splits the gate (section 6).
6. **The "kernels" and the L1 layer were not DAGs in the first draft** (an independent type-based review found 77 kernel-to-leaf symbols and an L1 cycle; I reproduced the 77). Section 3.3 now lists the symbol cuts and file moves; the assignment is committed as `session-service-hub-assignment.csv`.
7. **Seven other packages are over 10k today** (section 8a); the owner's scope for them is open question 5.

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
| Pure delegates (at most 12 lines, at most one service field, no receiver-method call; heuristic, I). The review counted 127 (727 lines) with the 12-line limit alone | 121 | 665 | Mostly in `session_service_delegates.go` (111 methods, 101 of them RPCs, 1,021-line file) and `files_delegates.go` |
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
| Core files (31 files, 12,726 lines: `session_service*`, `session_streamer`, `terminal_access`, `terminal_service`, `workspace_service`, `external_*`, `tmux_input_send`, `human_activity`; I: that regex is my reconstruction of the ranking's list, which is not published) use symbols in 90 non-core files | 397 (207 method names, 115 exported, 75 unexported); I, not independently re-runnable without the ranking's list | Hub to leaf. Legal after the split. I read the ranking's "403" as this edge (I: 397 now, and the graph predates the federation move; I did not recount the 403 itself). Largest targets: `defaults_service.go` 19, `github_user_nudge.go` 18, `rules_service.go` 17, `feature_flag_service.go` 16, `autonomous_orchestration_service.go` 16. |
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

### 3.3 Kernel leaks and L1 cycles, and the cut lists (revised 2026-11-23 after an independent type-based review)

The first draft called five packages "L0 kernels" and an L1 layer without cycles. Both were wrong. Under the first assignment, 77 distinct symbols in kernel files used files assigned to L1 or hub packages (V: the reviewer's count, reproduced by my re-run), and the L1 packages formed one strongly connected component (reviewer: {adminrpc, attention, guidance, notify, providers, rules, workflows} about 31.2k lines, plus {fileops, vcssearch}). The reviewer also found leaf-to-hub symbols my graph missed: 86 in total, not 82 (adds `file_metrics.go` using `mustInt64Counter`/`mustFloat64Histogram` from `session_creation_metrics.go`, the sweepers using creation metrics and `commitTerminalStatus`, and `guidance_request_hook_mirror.go` using `RedactSecrets`). I have not reproduced the 86 (I have no type information); I fold those three classes into the cuts below.

**What I re-ran (I, file-level graph; assignment committed as [session-service-hub-assignment.csv](session-service-hub-assignment.csv), 268 rows, sums to 65,807).** The first-draft assignment reproduces the reviewer's 77 kernel-to-higher symbols exactly (V). After the file moves in 3.3.1 and the symbol cuts in 3.3.2 and 3.3.3, the kernel graph is acyclic and the L1 graph is a DAG.

#### 3.3.1 File moves versus the first draft

| Change | Why |
|---|---|
| `guards_flag`, `lease_flag`, `steer_authorization`, `session_service_guarded_steer` leave `access` for the hub (`sessionrpc`) | All four are `*SessionService` receiver files. Their value types (`SteerOutcome`, `Steer*`, `ErrSteerPaneOwnership`, `NudgeRecorder`, `steerRequestFacts`) are cut out as symbols into `access` (3.3.2). |
| `tmux_input_send`, `external_pane_resize` join `access` | `terminal_access.go` needs `sendInputToTmuxWithRetry`, `ExternalPaneTarget`, `resizeExternalCapturePaneSession`. |
| `hidden_reply_audit.go` becomes kernel `auditkit` | `flagsvc`, `access` and `notify` all use `AuditSink`/`AuditLine`/`ErrAuditFailed`/`flagOutcome*`. |
| `approval_store.go` becomes kernel `approvalkit` | `ApprovalStore`/`PendingApproval` are used by `workflows`, `adminrpc`, `notify`, `attention`. |
| `path_resolver.go` moves into `fsops` | The file backends use `ResolvedPath`, `WorktreeRoot`, `OpenWorktreeRoot`, `linkProber`. |
| `secret_scanner.go` becomes kernel `secretscan` (94 lines) | `guidance_request_hook_mirror.go` uses `RedactSecrets`. |
| `credentials.go`, `provider_limits.go` become kernel `credkit` | `anthropicwire` uses `Credential`, `CredentialChain`, `ProviderLimits`, `parseIntHeader`, `parseTimeHeader`. |
| `notification_actions.go`, `notification_priority.go`, `rate_limiter.go` move to `notify` | They build and rate-limit notifications; `attention` and `autonomous` call them (one way). |
| `domain_checker.go` moves to `attention`; `window_activator.go` to `adminrpc`; `guidance_ack_wiring.go`, `guidance_runtime_wiring.go` to the hub | Receiver or consumer-owned. |
| `fileops` and `vcssearch` merge into `filesvc` (8,053 lines) | They call each other (7 and 4 symbols). Merge chosen over two port sets; the alternative is a consumer-side `FileOps`/`SearchHandler` port pair. |

#### 3.3.2 Symbol cuts into the kernels (explicit list; sources are the first-draft assignment's defining files)

| Kernel | Symbols to define inside it | Defined today in |
|---|---|---|
| `access` | `AuthModeRequired`, `ListenerLocal`, `ListenerRemote`, `RequestRecordFrom` | `capture_tap_service.go` (stays in `adminrpc` as the RPC adapter) |
| `access` | `SteerOutcome` + 8 constants, `ErrSteerPaneOwnership`, `NudgeRecorder`, `steerRequestFacts`, `requestFactsOf`, `BacklogLinkResolver`, `BacklogReviewLink` | `session_service_guarded_steer.go`, `steer_authorization.go`, `hidden_review_steer.go` |
| `flagsvc` | none from L1 once the 7 leaf-owned flag consts and their `Spec` rows (`hiddenSessionReplyFlagName`, `hiddenSessionReadonlyGuardsFlagName`, `terminalWriteLeaseFlagName`, `completionReviewerFlagName`, `githubIssueProgressCommentsFlagName`, `ChangelogFlagName`, `UpdateCheckFlagName`) move to the file that owns each (in-package pre-step, stage 0; the header of `feature_flag_specs.go` already prescribes this) | `feature_flag_specs.go`, `hidden_question_reply.go`, `completion_reviewer.go`, `backlog_github_progress_sync.go`, `update_check_service.go` |
| `fsops` | `knownTextExtensions`, `readFull`, `truncateSize`, `internalIOError` | `file_service.go`, `file_ops.go` |
| `fsops` | `mustInt64Counter`, `mustFloat64Histogram` (copy into the existing `server/services/internal/streammetrics`, which holds `watch_stream_metrics.go`; contents not read, I) | `session_creation_metrics.go` |
| `fsops` | `WorkspaceProvider`, `.GetWorkspace` (consumer-side interface, defined where used) | `workspace_service.go` |
| `fsops` | `expandTilde`, `parseJSONLogHeader`, `deployID`, `validateTargetDirectory` (used by `adminrpc` and `workflows`) | `path_completion_service.go`, `log_header_scan.go`, `deploy_id.go`, `workflow_service.go` |
| `questionkit` | `ActionResult`, `applied`, `notWired`, `outcome`, `replyOutcomeResult`, `StaleReasonAnswerPartial`, `tabbedQuestions`, `itemSessionLookup`(+`GetItemSessionBySessionUUID`), `metaGuidanceRequestID`, `metaQuestionID`, `metaReplyUnavailable`, `sanitizeNotificationText`; consumer-side ports replacing `AnswerRouter`, `answerRows`, `deliverySink` (7 symbols) | `notification_action_service.go`, `pane_reply_core.go`, `question_answer_sequence.go`, `hidden_review_steer.go`, `approval_question.go`, `approval_handler.go`, `question_answer_router.go` |
| `anthropicwire` | `RuleSpec` (lower to `session/domain`, as the DTO-lowering plans do for value types) | `rules_store.go` |
| `approvalkit` | `PendingApproval.Decisions()` (accessor for the unexported `decisionCh`, read by the approval handler wait loop and one rules test) | `approval_store.go` |

Deferred from the `fsops` kernel PR (#1797): the `fsops` row for `expandTilde`, `parseJSONLogHeader`, `deployID` and `validateTargetDirectory` is owned by the Stage 5 `adminrpc`/`filesvc`/`workflows` PRs, which create the edges it cuts.

Added by the `auditkit` kernel PR: the in-memory `fsops.FSOps` test double (`memFS`) moves from `server/services/fsops_test.go` to `server/services/fsops/memfs` so both `auditkit` and the staying `server/services` tests share it; it adds one tiny test-support leaf beyond the plan, and `fsopstest` could not host it because `fsops`'s own tests import `fsopstest`.

#### 3.3.3 L1 cycle cuts (each edge below is a symbol set that becomes a kernel symbol, a move, or a consumer-side port)

| Edge cut | Symbols | Cut |
|---|---|---|
| `notify` to `attention` | 8: `buildApprovalMessage`, `matchesIDData`, `stableIDForData`, `sanitizeNotificationText` (approval_handler.go), `meta*` (approval_question.go), `steerRequestFacts` | text builders move into `notify`; `meta*` to `questionkit`; `steerRequestFacts` to `access` |
| `notify` to `adminrpc` | 3: capture-tap auth | `access` (3.3.2) |
| `notify` to `workflows` | 1 | consumer-side port in `notify` (symbol not inspected, I) |
| `workflows` to `attention` | 5: `ApprovalStore`(+1), `metaGuidanceRequestID`, `ActionResult`, `applied`, `notWired`, `outcome` | `approvalkit`, `questionkit` |
| `workflows` to `guidance` | 3: `CancelPeerHelp`, `ErrNotPeerHelp`, `GuidanceRequestService` | port in `workflows` |
| `adminrpc` to `workflows` | 2: `deployID`, `validateTargetDirectory` | `fsops` |
| `adminrpc` to `filesvc` | 2: `expandTilde`, `parseJSONLogHeader` | `fsops` |
| `rules` to `adminrpc` | 2: `ClaudeSettingsWatcher.Reload` | port in `rules` |
| `attention` to `rules` | 3: `AnalyticsStore.RecordFromResult`, `riskLevelString` | port in `attention` |
| `filesvc` to `adminrpc` (1), `filesvc`/`workflows` (1 each way) | 3 | ports (symbols not inspected, I) |

Total cuts: 32 L1 symbols plus the kernel symbol lists in 3.3.2. Re-run result (I): **no kernel cycle** (order: `fsops`, `secretscan`, `credkit`, `access` < `auditkit`, `approvalkit` < `flagsvc` < `questionkit`, `anthropicwire`) and **L1 acyclic** after the cuts; the remaining one-way L1 edges are `attention` to `notify` (26), `attention` to `guidance` (4), `prs` to `backlogsync` (4), `notify` to `gatestats` (3), `attention` to `prs` (2) and singletons. `attention` therefore imports `notify` and `guidance`; neither imports it.

Leaf-to-hub symbols still left after the moves: 94 (I, re-run). The big groups are `SessionService` receivers and methods (`sessionrpc`, `sessioncreate`, `sessionstream` to the facade 11, 11 and 8), `attention` to the hub 14 + 5, `providers` to the hub 12 (the `Steer*` set, which 3.3.2 moves to `access`), `filesvc` to the hub 5 (`WorkspaceProvider` and three conversion helpers, `vcsStatusToProto`, `listPRFilter`, `prRefFromProto`) and `workflows` to the hub 5 (`ReactiveQueueManager`, `ScrollbackSequencer`, defined where consumed). They are ports, kernel symbols or hub-adapter calls, as in 3.2.

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

### 5.1 Packages (V: lines are `wc -l` sums of the committed assignment, which is a design, I)

The per-file assignment is [session-service-hub-assignment.csv](session-service-hub-assignment.csv) (columns file, package, layer, stage, lines; 268 rows; sums to 65,807). It supersedes the first draft's table; the reviewer's per-package recounts differed from it because the first draft's partition was not published and the file moves in 3.3.1 had not been made. Package sizes below recount from the CSV with `awk -F, 'NR>1{p[$2]+=$5} END{for(k in p) print k,p[k]}'`.

| Layer | Package under `server/services/` | Lines | Contents |
|---|---|---|---|
| L0 kernels (import only domain, config, `session*`, and lower kernels) | `fsops` | 1,695 | `fsops`, `file_atomic`, `file_backend*`, `file_inode_*`, `file_codec`, `file_metrics`, `files_telemetry`, `dup_fd_*`, `exec_*`, `settings_file_atomic_write`, `path_resolver` |
| | `secretscan` | 107 | `secret_scanner`, `redaction_sentinels` (the sentinels moved in from `ai_interfaces.go`, #1798) |
| | `credkit` | 589 | `credentials`, `provider_limits` |
| | `access` | 1,128 | `terminal_access`, `human_activity`, `local_write_verdict`, `terminal_input_metrics`, `session_nudge_guard`, `tmux_input_send`, `external_pane_resize`, plus the symbols in 3.3.2 |
| | `auditkit` | 468 | `hidden_reply_audit` (imports `fsops`; `AuditKind*`, `AuditPhase*`, `FlagOutcome*` and `Fallback` are exported because the staying callers cross the package boundary, and `AuditFileName`, `AuditQueueSize`, `AuditModeQueued` and `AuditModeFallbackLog` only because the staying tests read them; the audit-focused tests of the sink moved with it, the flag-flip tests stay in `feature_flag_audit_test.go`) |
| | `approvalkit` | 460 | `approval_store` |
| | `flagsvc` | 1,695 | `feature_flag_*`, `feature_flags_*` (imports `auditkit`, `access`) |
| | `questionkit` | 1,716 | `pending_question_store`, `askuserquestion_parser`, `question_delivery_track`, `answer_listener`, `guidance_request_hook_mirror`, `guidance_request_mirror_proto`, `guidance_dialog_*`, `notification_quality_metrics`, `reply_idempotency` (imports `flagsvc`, `secretscan`) |
| | `anthropicwire` | 824 | `anthropic_client`, `anthropic_limits_client`, `gemini_limits_client`, `cli_ai_client`, `selecting_ai_client`, `ai_interfaces` (imports `credkit`) |
| L1 leaves (import kernels and `session*`, never the hub; the L1 graph is a DAG after 3.3.3) | `filesvc` | 8,053 | merge of the old `fileops` and `vcssearch` (82% of the cap) |
| | `attention` (ships split per Part 3 into `approvals`, `qareply`, `notifyactions`; not re-measured) | 7,908 | `approval_*` (not the store), `hook_*`, `question_*`, `pane_reply_*`, `notification_action*`, `hidden_question_reply`, `hidden_review_steer`, `pi_extension_*`, `claim_checker`, `mcp_proof`, `domain_checker` |
| | `providers` | 6,014 | `github_*`, `capacity_*`, `quota_*`, `jules_*`, `callback_*`, `webhook_*`, `update_check`, `trigger_rate_limiter` |
| | `adminrpc` (split by role at author time) | 5,888 | `database_service`, `defaults_service`, `config_service`, `utility_service`, `project_service`, `launcher_*`, `remote_service`, `claude_settings_*`, `slash_command`, `client_log`, `log_level`, `circuit_breaker`, `debug_snapshot`, `error_registry`, `diagnostic_service`, `tymux_rollout`, `stream_hub_rollout`, `tmux_version`, `tcc_enrichment`, `capture_tap`, `window_activator` |
| | `rules` | 4,536 | `rules_*`, `analytics_*`, `tagging_*`, `escape_code_handler` |
| | `notify` | 2,569 | `notification_*`, `slack_*`, `backlog_notifier`, `stale_session_notifier`, `memory_pressure_notifier`, `rate_limiter` |
| | `backlogsync` | 2,524 | `backlog_*`, `completion_reviewer*`, summaries, `headless`, `llm_backend` |
| | `workflows` | 2,053 | `workflow_*`, `review_queue_service`, `checkpoint`, `wake_trigger_*` |
| | `autonomous` | 1,956 | `autonomous_*`, `lease_wedge` |
| | `guidance` | 1,248 | `guidance_*` outside the kernel and the hub wiring |
| | `prs` | 626 | `pr_*`, `session_pr_holds`, `active_pr_status` |
| | `gatestats` | 608 | `gatestats_*` |
| L2 hub handlers | `sessionrpc` | 5,698 | lifecycle, crud, update, shells, PR refs, liveness, workspace, the four steer/guard receiver files, the two guidance wiring files |
| | `sessioncreate` | 2,112 | create, `session_creation_*`, sweepers |
| | `sessionstream` | 2,394 | stream, watch, events, diagnose, terminal service, external websocket |
| L3 facade | `server/services` | 2,947 | `session_service.go` struct, embeds, wiring, delegators; about 2k after delegator removal and constructors moving to the root (I) |

Largest package is `filesvc` at 8,053, then `attention` before its split at 7,908 (the first draft's 9,148 included files that moved to kernels and `notify`). `sessionkit` (the ten shared helpers, about 200 lines) is carved out of the hub files in stage 6a and is not a separate row. `backlogsvc` (15,641) and `wsconn` (9,767) are in section 8a.

### 5.2 Dependency rule and wiring

- Leaves never import the hub. The hub imports leaves and kernels. A leaf imported by a sibling needs an `isolate_<leaf>` depguard row listing it (`.claude/rules/leaf-packages.md` rule 4).
- Ports are consumer-side (rule 3). `LiveInstanceFinder` and `SessionDeleter` already exist (`sessionretention` precedent: `package-layout.md:47`).
- **The facade declares no RPC method itself.** A method declared on `SessionService` silently wins over an ambiguous embed and would hide a duplicate, so every one of the 181 RPCs must come from an embedded handler and `var _ sessionv1connect.SessionServiceHandler = (*SessionService)(nil)` (`session_service.go:56`) stays. The reviewer verified: two embeds of 90 and 91 methods satisfy the 181-RPC interface; overlap fails only through the interface assertion; a missing embed fails; interceptors are unaffected. (VERIFIED by the reviewer with real type information.)
- **`Set*` setters.** `server/dependencies*.go` builds each handler's `Deps`. The 54 setters cannot all become plain `Deps` fields: the ones that run after construction (the Jules and keyring swaps, `SetSlackNotifier`, `SetStatusDetailProvider`, `SetWorkflowService`, and similar) need getter closures (`func() T`) per `.claude/rules/leaf-packages.md` rule 3 (as `AfterLinkHook` and `DbPath` do). This overlaps backend-di-refactor Story 1.3 (owner Q4, 2026-11-20); stage 6c waits for or builds on it.
- **Hub group calls (I, committed assignment re-run).** `sessionrpc` to `sessionstream` 6, `sessionstream` to `sessionrpc` 3, `sessionrpc` to `sessioncreate` 4, `sessioncreate` to `sessionrpc` 4, `sessioncreate` to `sessionstream` 2, and each handler to the facade (`SessionService` receiver methods) 8 to 11; the reviewer's type-based counts differ (rpc to stream 10, rpc to create 2, create to rpc 4, create to stream 3 including `ArchiveSession`, stream to rpc 3, facade and stream 11/6 for the `wire*Callback` family). Both say the same thing: the hub groups call each other, so these must be `sessionkit` members (`FindLiveInstance`, `findInstance`, `loadInstancesWithWiring`, `wireCallbacks` and the `wire*Callback` family, `removeFromAllPollers`, `ArchiveSession` entry) or consumer-side ports; none can be a package import.
- Open-closed: a new RPC group is one new package with a `Handler`, one embed line in `services.SessionService`, one constructor call in the root; forgetting the embed is a compile error. A new proto service is one package and one registration in `server.go`.

## 6. Staged PR sequence

Cost classes: **S** = one author wave plus CI; **M** = author plus two review lenses, port design; **L** = characterization tests first, multi-wave. Sizes are agent work, not time. "After" is the top-level non-test line count of `server/services` (`wc -l`), computed from the `stage` column of the committed CSV: 65,807 at the start (V arithmetic; assignment I). The CSV's `stage` column is the single source for which file moves when.

| Stage | PRs | Content | Moves out | Top level after | Ports and risk | Test plan | Rollback |
|---|---|---|---|---|---|---|---|
| 0 | 2 S + 1 M | (a) Guard hardening (section 7): `noliveinstanceraw` multi-package, analyzer and Makefile path-existence checks, `terminal_input_sites_scan_test` walking subdirectories, `TestMain` requirement. (b) Per-stage `budget.txt` rows. (c) In-place pre-step: the 7 leaf-owned flag consts and `Spec` rows move to their owning files, composition root appends (M). | 0 | 65,807 | none | `make ready`, analyzer tests | revert |
| 1 | 4 S + 3 M + 2 L | Kernels, in layer order: `fsops`, `secretscan`, `credkit`, `access` (M), `auditkit`, `approvalkit` (S each), `anthropicwire` (M), `flagsvc` (L), `questionkit` (L). The symbol cuts of 3.3.2 are part of these PRs (about 26 relocated symbols, 14 of them into `questionkit`). | 8,673 | 57,134 | Highest symbol-relocation count per PR; exported names become API. Stage 1 was S/M with 6,607 lines in the first draft; it is now 9 PRs and 8,673 lines because the kernels had to absorb their dependencies. | `go test ./server/...`, `make test-delivery-guards`, mutation check per moved branch | revert per PR, no alias to unwind |
| 2 | 11 S | READY clusters from the ranking (uploads, remote, claude settings, misc handlers, summaries, hook receivers, completion reviewer, forward sync with 1 port, search, 2 singles); federation is done. | 6,715 | 50,419 | `search` at the SC1 cap; `backlog_debug_mutate_handler.go` is path-keyed in 3 places (section 7) | per cluster, mutation checks | revert |
| 3 | 6 M | Small NEEDS-PORTS: webhooks, quota core, gatestats, tagging rules, rollouts, sweepers' non-receiver files. | 4,258 | 46,161 | 10 test files share approval fixtures; 1 to 4 hub methods per cluster (ranking) | as above | revert |
| 4 | 4 L + 1 M | Attention cluster after kernels: `approvals`, `qareply`, `notifyactions`, `notify` (L each, `notify` takes the text builders and descriptor files), `guidance` (M). 9 receiver files convert. Stage total is 10,457: `attention` 6,996 + `notify` 2,213 + `guidance` 1,248; the first draft's 12,580 for these three packages included 1,045 lines that stage 2 (689) and stage 3 (356) now take. | 10,457 | 35,704 | The 32 L1 cuts of 3.3.3 land here and in stage 5. AskUserQuestion routing (`docs/explanation/askuserquestion-answer-routing.md`) and the `noarchivedrevival` file `question_answer_router.go`. The attention split is not re-measured. | router characterization tests before moving; `make test-delivery-guards` | revert; router has a guard test |
| 5 | 11 M + 3 L | Remaining L1: `providers` (2 M), `rules` (2 M), `adminrpc` (3 M), `filesvc` (3 L, merge of two old packages), `workflows`/`backlogsync`/`autonomous`/`prs` (4 M). 12 receiver files. By package: `filesvc` 6,041, `adminrpc` 4,052, `rules` 3,786, `providers` 3,588, `workflows` 2,053, `autonomous` 1,956, `prs` 626, `backlogsync` 451. | 22,553 | 13,151 | `providers` needs the `access` steer symbols; the ports of 3.3.3 for `rules`, `workflows`, `filesvc`, `adminrpc` | whole-tree golangci, `make test-delivery-guards` | revert |
| 6a | 1 L | `sessionkit` plus `sessionstream`. 51 receiver methods; `StreamTerminal` is 543 lines in one method. | 2,394 | 10,757 | `terminal_input_sites_scan_test.go` globs the top level only | stream integration tests, e2e terminal specs | revert |
| 6b | 1 L | `sessioncreate`: the `CreateSession` closure is 38 fields. | 2,112 | **8,645** | `sessioncreate` and `sessionrpc` call each other (5.2) | `session_service_create_test.go`, the 57 constructor tests, `make test-race` | revert |
| 6c | 3 L | `sessionrpc` handler, facade becomes embeds only, delegators and constructors removed, `Set*` setters to `Deps` plus getter closures. Optional for the size goal. | 5,698 and delegator removal | 2,947, about 2k (I) after trimming | `noliveinstanceraw` monitored methods move; Story 1.3 overlap | full suite | revert in 3 steps |
| 7 | 1 S | Gate (rewritten, below). | 0 | 8,645 | see below | `go run ./tools/bzl/scoping t9` | revert |

PR count 3+9+11+6+5+14+1+1+3+1 = **54** (S 18, M 22, L 14), against Part 3's 41 for the same area (the two counts are not reconciled per PR).

**Minimum path to `server/services` under 10k:** stages 0 to 5, 6a and 6b: 13,151 after stage 5, 10,757 after 6a (still over), **8,645** after 6b. Stage 6c is for the open-closed property and the thin facade, not the size cap.

**Stage 7, the gate (rewritten).** The first draft said to lower `LargestPkgLOCMax` (`tools/bzl/scoping/t9.go:68`, 72,000) to 10,000. That is wrong. T9d (`t9.go:90`, `main.go:130`) checks only `LOC["server/services"]` and `LOC["session"]`, both against the one `LargestPkgLOCMax`; it does not look at the largest package. Setting 10,000 would fail `session` (13,981, V) and would say nothing about `backlogsvc`, which T9d does not measure (my earlier "fails `backlogsvc`" was false). So stage 7 is: add a separate `T9dServicesLOCMax` (10,000) next to the session cap, set it in the PR that lands stage 6b, and leave the session cap to the session split (the owner-approved tmux and process-backend plan, PR #1770 per the review; I did not read it). A repo-wide "no package over 10k" gate would be a new check over `go list` rows (I), only possible after section 8a is decided.

### 6.1 Estimation and wall-clock blockers

- **Agent cost:** stages 2 and 3 are the cheap parallel waves (federation `94acd8ec7`: 37 files, 231 added lines). Stages 1, 4, 5 (3 L) and 6 hold the L work (14 L PRs; each needs a characterization wave before the move and a review wave after). Totals by class: 18 S, 22 M, 14 L, no XL.
- **Wall-clock blockers (named, not hours of agent work):** CI restarts on every push and the make-only run takes about 2 hours (`.claude/rules/leaf-packages.md`); at most 3 open split PRs (`roadmap.md` "Rules for executing", ADR-010 decision 4) and shared rows (`budget.txt`, `boundaries.txt`, `.golangci.yml`, `tools/bzl/scandata/BUILD.bazel`) force serial rebases, so 54 PRs need at least 18 serial CI rounds. Owner answers to Q1 to Q8 gate stages 1, 4, 5 and 6c. Disk: btrfs metadata and Go cache regrowth need `df` and `btrfs filesystem usage` before each wave (today: 133 GB free, metadata 84%).

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
| 9 | Scoping | `tools/bzl/scoping/budget.txt:46` (services row), `boundaries.txt` (12 `server/services/*` rows), `t9.go:68,90` | package | Loud for budget/boundaries; **T9d never fails for 10k** and checks only `server/services` and `session` | New rows per package, ratchet the services row each stage, split the gate in stage 7. |
| 10 | Bazel | `server/services/BUILD.bazel:773-792` (`data` globs of `go_sources`), `tools/bzl/scandata`, root and `server/BUILD.bazel` | generated | Loud (`bazel build --nobuild //...` is blocking) | `make gazelle`, `go run ./tools/bzl/scandata -fix .`; the federation PR touched `BUILD.bazel`, `server/BUILD.bazel`, `tools/bzl/scandata/BUILD.bazel` plus per-leaf BUILD. |
| 11 | Interceptors and WebSocket bridge | `server/server.go:600-603` (`NewScopedFeatureFlagInterceptor`, `services.ProgramCLIFlagProbeGatedMethod`), `:617-619` | procedure name | Not affected by Go-level facade (one `NewSessionServiceHandler`, `:608`). The constant's import path changes if `ProgramCLIFlagProbeGatedMethod` moves: loud | None beyond the import. |
| 12 | `svctest` driver-leak check | `server/services/internal/svctest/main.go:19` (`Main`), `driver_leak.go` (matches goroutine stacks containing `/session/driver.StartSessionDriver.func`) | stack text, package independent | Silent per package: only `server/services/backlogsvc/testmain_test.go` and the top-level `testmain_test.go` call `svctest.Main` (`ls server/services/*/testmain_test.go`). A new package that starts drivers without it leaks unseen. | Stage 0: every new package gets `testmain_test.go`; a guard test lists packages importing `session/driver`. |
| 14 | `make actor-field-guard` | `Makefile:1317-1328`: a hard-coded file list including `server/services/session_service.go` and `server/services/workflow_service.go`, `grep ... 2>/dev/null` (V) | file path | **Silent**: a moved or deleted file is dropped by `2>/dev/null` and the guard passes | Replace the list with a directory walk or add the path-existence check (stage 0). |
| 15 | Bazel test sizing | `tools/bzl/go_test.bzl:5-14`: `server/services` is (`enormous`, 4 shards) and `exec-host-binary`; new packages get defaults (V) | package | Silent: timeouts or host-dependent cached results (review: approval `socat` relay tests) | A row per new package in the same PR (open question 9). |
| 16 | `compare-affected` | `tools/ci/compare-affected/compare.go:94,221` (V): `sel.hasTargetIn("//server/mcp", "//server/services")` | Bazel target | Silent: whether sub-package targets count is not verified (I); a change moved to a new target may not select the mcp-integration job | Add the new targets or confirm prefix matching (stage 0). |
| 13 | `server/mcp` import set | `server/mcp/import_set_guard_test.go` (exact equality) | import path | Loud if `server/mcp` imports a new kernel | Adjust only if a kernel is used there. |

## 8. Reconciliation with the earlier Part 3 ledger

- Part 3's "112 upward symbols" and this plan's "82 leaf-to-hub symbols" count different sets (Part 3: cut by in-package pre-moves; here: symbols defined in 54 hub files). I did not reproduce 112; its ledger is not in the checkout.
- Part 3 chose `svcbase` for `fsOps`; this plan calls it `fsops` and keeps it narrow (1,392 lines of file I/O) rather than a general base package (`svcbase` risk in Part 3 section 6).
- Part 3's `sessionlife`/`terminal` packages correspond to `sessionrpc`/`sessionstream` here. The stage order differs: this plan moves all leaves before the hub so that hub handlers import finished leaves.
- Part 3's cross-part gap (`DerivePriority`, `SessionStopper`, `SessionSteerer`, claim types, `Steer*` consts, `ReasonName*`) is partly closed here: the `Steer*` constants and `NudgeRecorder` go to the steer kernel (stage 1); `DerivePriority` goes to `notify`; the rest are unresolved.

## 8a. Every package over 10k today (V: `ls <dir>/*.go | grep -v _test.go | xargs cat | wc -l`, run 2026-11-23; matches the reviewer's table)

| Package | Non-test lines | Plan |
|---|---|---|
| `session/instance` | 23,485 | `budget.txt:5` carries an INTERIM owner-approved cap exception; decomposition is a later track (`session-hub-redesign-plan.md`). |
| `server/services` (top level) | 65,807 | This plan. |
| `server/services/backlogsvc` | 15,641 | `backlogsvc-ocp-plan.md` and Part 3 (BS-1 to BS-6). |
| `session` | 13,981 | Session-tree plans; the tmux and process-backend split is PR #1770 (per review, not read by me). |
| `server/mcp` | 13,322 | ADR-010 rejected splitting it (SC2/SC4); `mcp-single-root-plan.md` shrinks its edges, not its size. No plan reaches 10k. |
| `server` | 10,476 | Root `server` package; `roadmap.md` calls it already a leaf (no size plan). |
| `session/tmux` | 10,272 | tmux/process-backend plan. |
| `session/unfinished` | 10,186 | No plan found by me. |

`server/services/wsconn` (9,767) stays under. Whether "every package under 10k" covers the other seven is open question 5; the cost per package is rough: `session/unfinished` and `server` are within 5% of the cap (S to M each, I); `server/mcp` needs a design that ADR-010 rejected (L); `session`, `session/tmux`, `session/instance` and `backlogsvc` have plans.

## 9. Open questions that need the owner

1. **Go-level facade or proto split?** This plan keeps one proto `SessionService` and splits in Go (verified by the reviewer). A proto split would change wire paths for the web app, mobile app and MCP proxy. Confirm.
2. **Is stage 6c in scope, or is the size goal enough?** Stages 0 to 6b reach 8,645; 6c only makes the facade thin.
3. **Kernel packages exported or `internal`?** `server`, `server/mcp` and `wsconn` need `access` and `flagsvc` types, so `internal/` would block them. Default here: exported.
4. **`attention` split.** Three packages per Part 3 (default) or one at 7,908? The split is not re-measured.
5. **Scope of "every package under 10k".** The owner said it applies to `server/services`. Does it also cover `session`, `session/instance`, `backlogsvc`, `server/mcp`, `server`, `session/tmux`, `session/unfinished` (section 8a)? This decides whether stage 7 is a services-only threshold or a repo-wide check, and whether `server/mcp` (rejected by ADR-010) and `server` need new plans.
6. **Budget basis.** Which base is authoritative: `wc` (65,807), the `budget.txt` row (65,715, GOOS-tagged files excluded), or t9?
7. **Does Story 1.3 (backend-di) land before 6c?** Both rewrite the 54 `Set*` calls.
8. **Who owns cutting the L1 cycles (3.3.3) and the kernel symbol relocations (3.3.2)?** They are the riskiest part of stages 1, 4 and 5; the plan assumes one author wave per PR with a review wave, but a decision on one owner per kernel avoids two PRs relocating the same symbol.
9. **Per-package Bazel test settings.** `tools/bzl/go_test.bzl:5-14` gives only `server/services` and `session` a size and shard count (`enormous`, 4) and a host-coupled tag (`exec-host-binary`). New leaf packages get defaults, so approval and hook tests that need `socat` relays or the host may time out or cache host-dependent results. Add one `_SIZES`/`_HOST_COUPLED` row per new package in the same PR, or accept defaults?
10. **Mechanical path checks.** Stage 0 adds existence checks for the `Makefile` `actor-field-guard` file list, `compare-affected` target keys, and the analyzer path lists. Approve adding them to `make ready`?

## 10. Self-review and limits

- Not run: `go build`, `go vet`, `go test`, golangci, Bazel, `scoping criteria`, any move. `gen/` and `session/ent` were not generated in this worktree, so no type information was used. My own measurements are syntax-level (go/parser plus regex): field use via `s.<field>` selectors, so a field passed through a helper or an embedded call is missed (the method-call closure covers receiver helpers only).
- The file graph is the ranking's go/types artifact, not regenerated; it predates the federation move. 82 vs 397 depend on it.
- The package assignment (section 5, committed CSV) is a regex partition over file names plus the file moves of 3.3.1. It balances LOC and keeps the cycle edges named in 3.3, but it was not run through `scoping move --dry-run`. Per-package sizes can shift by a few hundred lines when symbols are cut inside files (`capture_tap_service`, `workspace_service`, `notification_action_service`, `hidden_review_steer`, `steer_authorization`).
- The "pure delegate" count (121) is a size-and-shape heuristic (at most 12 lines, one service field, no receiver call). The stage 6c delegator removal depends on it; the compile-time assertion in 2.2 is the real check.
- Receiver-bearing files outside the hub packages under the first-draft assignment (the revised one moves 6 of them into the hub): 21 files, 77 methods, 1,409 lines (V, go/parser over the receiver methods): attention 7 files, guidance 2, access 4, adminrpc 3, rules 2, fileops 1, autonomous 1, backlogsync 1. They convert in stages 4 and 5 and are the main source of per-stage port work.
- Citing the previous ranking: the READY and NEEDS-PORTS cluster lists in stages 2 and 3 are from `leaf-candidates.md` (2026-11-22, scratch artifact, not in the repo); only their line counts were re-added (6,715 and 4,258, from the `stage` column of the CSV).
- Revision 2026-11-23: the independent review verified (type-based, by the reviewer) the embedding design, the hub counts and the stage arithmetic; its three MAJOR findings (kernels not leaf-level, L1 not a DAG, wrong gate) are addressed in 3.3, 5 and 6. The 3.3 re-run is a file-level graph analysis (I); the symbol cuts are not compiled, so the first kernel PR may find more (the review's 86 vs my 82 shows the graph undercounts). Symbols marked "not inspected" in 3.3.3 are counts only.
