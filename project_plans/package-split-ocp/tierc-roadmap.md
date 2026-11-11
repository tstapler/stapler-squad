# Tier C roadmap: dependency inversion for the hub types (2026-11-11)

Output of the `package-split-tierc` workflow (102 agents: one analyst per hub area, four review lenses per proposal, one synthesizer) over `Instance`, persistence (`EntRepository`/`Storage`), session coordination, `SessionService` and the remaining `server/services` handlers. Companion to `roadmap.md` (waves 1-4, merged). Claims are the reviewers' and were not all reproduced; the plan labels them.

## Assessment

By the strict rule (no reject verdict, no blocker), only SVC-5-edge-relocations is accepted, as two PRs in wave 1. Every other package proposal had at least one reject, and most had a verified blocker. The same two causes keep appearing. First, a new package would still import the hub types (Instance, Storage, EntRepository, SessionService), so it is a lower dependency of the core, not an open-closed leaf, and any core change still rebuilds it. Second, several ports were wrong or polluting: HistoryHost and UsageReader do not match the real code, the Classifier and StatusDeterminer ports do not match the real flow, and several ports have one implementation. The roadmap's own SC6 figures were reconfirmed for session clusters, with no new evidence offered. Waves 2 and 3 are not accepted proposals. They are in-place fixes that the reviewers named as fallbacks for OCP violations (global history registry, parallel yolo map, hard-coded determiner), and each needs its own short design check before starting. The real unlock for any further physical leaf split is lowering the Storage/EntRepository DTOs (Tier C, ADR-011). That is why the plan stops short of more package moves. No claim here has been re-run by me; all counts and simulation results come from the reviewers' reports and are labelled unreproduced where they matter."

## Wave 1

Only SVC-5-edge-relocations survived the acceptance rule (all four lenses accepted, no blocker). It is split into two independent PRs, as the reviewers required. Both are pure relocations of code with no back-edge into server/services, were simulated in a throwaway worktree (go build ./server/... ./cmd/... and go vet clean, moved tests passed), and neither needs a new port. Neither is an OCP leaf and neither claims to be. They remove the server/services hub from the import closure of server/push and cmd/ssq-hooks.

### SVC-5a-push: server/services -> server/push (absorbs PushService/PushNotification/PushHandler; no new directory)

- Cost: S (one PR, mechanical git mv plus import rewrite)
- Payoff: Closure change, not LOC: module closure of server/push drops by about 28 packages (160 to about 132), measured by reviewers via go list -deps and not yet reproduced. Churn on these files is 12 commits per 180d, so edits to push stop invalidating server/services and its roughly 378 test files. No SC6 gain is claimed; the replay result must be recorded in the PR body.
- Files moved:
  - server/services/push_service.go
  - server/services/push_handler.go
  - server/services/push_service_test.go
- Exports:
  - PushService and PushNotification keep their current names (rename to push.Service is explicitly deferred; decide up front and do not mix it into this PR)
  - No new exported identifiers; none of the three files has init() or build tags
- Extension points:
  - server/push already has the Notifier interface and the DeliveryOption variadic. A new delivery channel is a new file in server/push implementing Notifier plus one registration line in server.go; the stable core (server/services) is not edited.
  - Closed for modification: PushService, which now has no inbound edge from the services hub.
- Tests to move:
  - push_service_test.go (helpers newTestPushService and newValidSubscription are used only there; nothing straddles)
- Guard changes:
  - golangci depguard: add a rule that server/push must not import server/services (and server/push not import server)
  - tools/bzl/scoping/budget.txt: lower server/services caps by the moved LOC, add a server/push row, via `go run ./tools/bzl/scoping budget --init`
  - BUILD.bazel: hand-edit or run Gazelle. Move the three srcs out of server/services/BUILD.bazel, move the webpush-go dep to server/push, and drop //server/services from server/push
  - Update stale path references in docs/tasks/web-push-notifications.md and docs/gap-analysis.md
- Verify:
  - go build ./server/... ./cmd/...
  - go vet ./server/push ./server/services ./server  (the server/web embed error in a fresh worktree is a known artifact and not a regression)
  - go test ./server/push ./server/services -count=1
  - go list -deps ./server/push | grep stapler-squad/server/services  (must print nothing)
  - go run ./tools/bzl/scoping replay  and  go run ./tools/bzl/scoping deps  before and after, paste both deltas
  - Gazelle diff clean; make lint-custom; make registry-generate only if a marker moved
- Composition-root wiring: server/server.go:466-467 constructs the service as push.NewPushService(...) instead of services.NewPushService(...); server/push/notifier.go:32,36,43 and subscriber.go:141 drop the services. qualifier. No aliases are left behind (only these call sites exist). Verify with grep before the move.

### SVC-5b-antigravityexport: server/services -> server/services/antigravityexport

- Cost: S (one PR; independent of SVC-5a)
- Payoff: Module closure of cmd/ssq-hooks drops by about 26 packages (reviewers measured 160 to about 134, not yet reproduced), so a hooks binary build and its test no longer depend on server/services. Churn on the file is 14 commits per 180d together with push.
- Files moved:
  - server/services/antigravity_exporter.go
  - server/services/antigravity_exporter*_test.go (the two test files)
- Exports:
  - ExportAntigravityRulesFromDB (already exported; name kept)
  - exportRulesLocked stays unexported. Do NOT add ExportRulesLocked. The rules_store.go:253 edge in the original proposal was false: that is RulesStore.exportRulesLocked, an unrelated method.
  - Helpers (permissionsFileMu, writeJSONAtomic, readSettingsObject, toSet, sameSet, stringList, read/writeManagedState) have no use elsewhere in services and move unexported
- Extension points:
  - None claimed. The exporter has one target. Say so in the PR body: this is a hub-closure reduction, not an OCP seam. A second export target (another agent's settings file) would be a new file in antigravityexport or a sibling package, invoked from the existing two call sites, which is the point at which a small ExportTarget slice becomes justified.
- Tests to move:
  - Both antigravity_exporter test files (no shared helpers with other services tests). Check the settingsPath-typed helpers and claude_settings_parser.go for shadowing before the move.
- Guard changes:
  - golangci depguard: antigravityexport may import only log, session/* and stdlib; it must not import server/services
  - Confirm the new directory is covered by the no_ent_in_services glob (**/server/services/**)
  - tools/bzl/scoping/budget.txt row for the new package and lowered services caps
  - BUILD.bazel: create server/services/antigravityexport/BUILD.bazel, remove the srcs from server/services/BUILD.bazel
- Verify:
  - go build ./server/... ./cmd/...
  - go vet ./server/services ./server/services/antigravityexport ./cmd/ssq-hooks
  - go test ./server/services/antigravityexport ./server/services -count=1
  - go list -deps ./cmd/ssq-hooks | grep stapler-squad/server/services  (must print nothing after this PR; anything that remains must be named)
  - go run ./tools/bzl/scoping replay and deps before and after; Gazelle diff clean
- Composition-root wiring: Rewrite the four callers in the same PR with no forwarding alias: server/services/rules_service.go:128, server/services/hook_status_service.go:183, cmd/ssq-hooks/main.go:1079 and :1106. The reviewers' simulation needed no aliases. Do not keep a services forwarder, or cmd/ssq-hooks keeps its services import and the payoff is lost.

## Wave 2

In-place, no-new-package OCP fixes. EVERY reviewer who rejected a package split on payoff or hub grounds named the same fallback: fix the OCP violation inside session, with no forwarders and no new directory. These are not accepted proposals under the strict rule (their parent proposals carried reject verdicts), so each is flagged as a reviewer-endorsed fallback that still needs its own short design check before it starts. They are sequenced here because they are the prerequisites that make any later physical move honest: the stable core must stop importing concrete variants first. At most 3 land in parallel.

### INST-history-registry-injection: session -> none (in place in session)

- Cost: M
- Payoff: Removes the 'constructors that import every concrete implementation' OCP violation and a process-global. No Bazel invalidation gain is claimed (ledger SC6 stands); this is an OCP and test-seam change.
- Ports:
  - HistoryAdapterRegistry (already exists as the registry type). No new interface. Instance receives the registry through a constructor option or a field set by the composition root.
- Extension points:
  - A new agent history adapter is a new file implementing HistoryAdapter plus one entry in the registry slice built at the composition root. session core is not edited.
  - Closed for modification: the registry, Instance's checkpoint/transfer paths.
- Guard changes:
  - Add a startup check that the injected registry is non-empty (fail loudly on a nil/empty registry rather than silently returning no adapters)
  - Bare &Instance{} in tests must still resolve a registry: provide a test helper in session, not an init()
- Prerequisites:
  - Re-derive the call-site list with grep before starting: reviewers cite about 6 sites (instance_checkpoint.go:49,253; instance_program.go:77,87; history_transfer.go:20-21) and Instance.historyRegistry at history_adapter.go:72
  - Owner decision Q1 below (this reverses the roadmap's 'SC6 near zero' deferral only for the in-place injection, not for the physical move)
- Verify:
  - go build ./... && go vet ./session/... ./server/...
  - go test ./session -race -count=1 -run 'History|Checkpoint|Transfer'
  - grep proves no non-test session file other than the registry wiring names NewClaudeAdapter/NewPiAdapter/etc.
  - behaviour-preserving: golden test of the adapter list order at startup
- Composition-root wiring: server/dependencies.go builds the registry from DefaultHistoryAdapters() (used today only in history_adapter.go and tests) and passes it to the session constructor / Storage load path. defaultHistoryRegistry and the hard-coded constructor calls leave the core.

### INST-launch-yolo-in-place: session -> none (in place in session)

- Cost: S
- Payoff: OCP and testability; reads become lock-free and race-free. Explicitly not a CI/Bazel win.
- Ports:
  - Optional AutoApproveFlag() on the existing launchCommandBuilder (consumer-side already, session/instance_tmux.go:60-90). No new interface.
- Extension points:
  - A sixth agent is one new builder file (Matches/Build/StderrLog/PromptChannel plus AutoApproveFlag) and one slice entry. yoloFlagByAgent (instance_tmux.go:292) is deleted.
- Guard changes:
  - None. Do not add DefaultBuilders()/WithLaunchBuilders until a second consumer exists.
- Prerequisites:
  - Add ExtraArgs and the pi session id to InstanceSnapshot (or define the exact race-free read) before converting the builders' raw field reads to Snapshot(), per .claude/rules/instance-lock-free-reads.md
  - Commit the golden argv characterization matrix first, pinning builder ordering
- Verify:
  - go test ./session -race -count=1 -run 'Launch|Build|Program'
  - argv golden test passes byte-for-byte before and after
  - go vet ./session
- Composition-root wiring: None. The launchBuilders slice stays in session; the point is that adding a sixth agent touches one builder file and no map in another file.

### INST-review-hidden-snapshot-fix: session -> none (in place in session)

- Cost: S
- Payoff: Removes a possible data race. Ship as its own tiny PR; the reviewers asked for it to be decoupled from any package split.
- Extension points:
  - None. Closes a lock-free-read rule violation (session/review_queue_determiner.go:196, plus raw inst.Title reads at :102,110,115,263 per the attention reviewer).
- Verify:
  - go test ./session -race -count=1 -run 'ReviewQueue|Determiner'
- Composition-root wiring: None.

## Wave 3

More reviewer-endorsed in-place OCP refactors that the reviewers said to do instead of a package move. Each needs a design check first, since the original proposals were rejected and the fallbacks were not independently reviewed. Order after wave 2 because the wiring pattern (slice injected at the composition root, non-empty startup check) is proven there.

### ATTN-determiner-injection: session -> none (in place)

- Cost: M
- Payoff: OCP and test seam only; no new package, no Bazel claim.
- Ports:
  - StatusDeterminer (already exists in session; keep Determine(*Instance,...) unchanged). No AttentionSubject, no new type.
- Extension points:
  - An alternative determiner is a new type implementing StatusDeterminer, injected at the root. Suppression stays a poller-internal ordered slice (suppressionRules already exists).
- Guard changes:
  - Add a ReviewQueuePollerConfig field or option; no depguard change
- Prerequisites:
  - Choose the constructor strategy (option vs required argument vs default supplied by the composition root) and count the roughly 77 NewReviewQueuePoller* call lines; svctest (liveness_consolidation.go:17) is a non-test constructor
  - State loud-failure behaviour for a nil determiner
- Verify:
  - go test ./session ./server/reviewqueue ./server/services/internal/svctest -race -count=1
  - go vet ./...
- Composition-root wiring: server/dependencies.go passes the determiner; tests use a session test helper that supplies the default.

### PERS-2-wake-unexport: session -> none (in place)

- Cost: S-M
- Payoff: Smaller Storage method surface; wake DTO/error relocation to session/domain is the only prerequisite for any later leaf.
- Ports:
  - server/wakes.Store (already exists, consumer-defined)
- Extension points:
  - None added.
- Guard changes:
  - wake_polling_guard_test stays in session; no re-point
- Prerequisites:
  - List the full surface first: timerStore methods (NextWakeFireAt, ListDueWakeTriggers, ClaimWakeTrigger, MarkWakeMissed), ListWakeTriggersByStatus, SetWakeChangeHook and its Storage forwarder at storage_wake.go:16
  - Keep the SetWakeChangeHook name; do not rename to SetOnChange
- Verify:
  - go build ./... && go test ./server/wakes ./server/mcp ./session -race -count=1 -run Wake
- Composition-root wiring: server/wakes_module.go:216-236 and the listed test callers use the repo directly instead of deps.Storage.SetWakeChangeHook.

## Deferred or rejected

- **INST-history (physical convhistory package)**: payoff:reject and 3 other revise-major verdicts. Roadmap.md:327 and :55 record SC6 near zero and no new evidence was brought. Port does not match the code (callback is func(), ExtractConversationUUID does not exist), ClaudeSettings omitted, persistence structs would leak into the package, and the core would still import every adapter. Only the in-place registry injection (wave 2) is kept. Reopen the physical split only after `scoping replay` shows SC4/SC6 movement, and then as two packages (a stable contract plus a volatile adapters leaf).
- **INST-agent-launch (session/agentlaunch package)**: Cycle (simulate_split feasible=false), instance_program_extension.go must not move (takes *Instance, no second implementation), prompt_delivery.go and instance_tmux_mcp.go omitted, ExtraArgs absent from InstanceSnapshot, and core would import the volatile package. Only the in-place yolo/predicate fix (wave 2) is kept.
- **INST-procmgr-backends**: open-closed:reject and payoff:reject. The registry already shipped in backend_registry.go, so the remaining violation is three constructor lines. 36 in-package tests build backends, a bare &Instance{} needs a tmux default, ledger p4 failed SC1/SC6 (universal -0.41 pp), and *tmux.TmuxSession leaks into the contract. Role-interface splitting is interface pollution without 2+ real consumers.
- **INST-review-status (session/reviewstatus)**: Payoff reject plus a new import cycle (InstanceStatusInfo carries session.Status, ReviewState is embedded in Instance). review_queue.go is already an alias shim over session/queue. Only the Hidden snapshot fix is kept (wave 2).
- **INST-prset (session/prset, session/prgate)**: Payoff reject: 1 commit per 180d. Stage 1 compiles (verified) but aliases keep session as the hub and the 42 consumer references stay. prgate stage is blocked on the Storage DTO move (InstanceData, ItemSessionSummary, ItemGuardView, InstanceSnapshot) and on pr_poll_metrics.go being wrongly listed. Revisit only with Storage Tier C.
- **PERS-1 (session/webhookstore)**: All verdicts revise with major dependency and feasibility findings: ErrNotFound/ErrConflict sentinels create a leaf-to-session cycle, AuthenticateIntegrationToken is missing from the port, ent types leak through WebhookReconcileResult, and services still import the DTOs so it is not a leaf. Payoff 2 commits per 180d. Rank below any split that removes a session import from a high-fan-in package.
- **PERS-2 physical wakestore**: payoff:reject, DTOs and sentinels (ErrWakeCapExceeded, ErrNotFound) must first move to session/domain; the consumer port would otherwise import the adapter. Only the in-place Storage-forwarder reduction is kept (wave 3).
- **PERS-3 (session/rulestore)**: Four unrelated domains in one package; Projects write the Session table and depend on Storage.saved.reset(); DTOs would be shared by about 8 non-root packages so it is not a leaf; no real OCP seam. Do not create the package.
- **PERS-4 (session/workflowstore)**: Ports return *ent.* types so there is no OCP gain; BuildWorkflowEngineReplay has only test callers and the Makefile runs go test ./session; 15 straddling session tests; the leaf would import session. Only reconsider inside the full ent_* Tier C extraction, where the ledger shows an SC6 pass.
- **PERS-5 (backlogsvc ports / session/backlogstore)**: backlogsvc already exists, so stages 1-3 are consumer-side ports with zero Bazel payoff. The port lists were not derived from code (SetBacklogItemPRAndTransition does not exist), PendingGuidanceSource is a single-implementation interface, and the final leaf is blocked on the ADR-011 DTO move. Not scheduled.
- **COORD-1 worktreeconsistency**: payoff:reject (SC4 fails: 4 commits per 180d). The Classifier port does not match the real flow and repair is hard-wired to one case. In-place PR1/PR2 (Classifier slice, EntRepository.RepairWorktreeRow) is a possible later fallback but only flag-only checks gain OCP.
- **COORD-2 prcleanup**: open-closed:reject with a blocker. The file must first be split because outcome types stay in core, the mirror reconciler binds to unexported Instance state, importers include session/backlog, backlogsvc and server/mcp so it is not a leaf, SC4 has zero commits. The in-place ordered []BlockRule (PR2) is a possible later behaviour-change PR with a characterization test.
- **COORD-3 health**: payoff:reject. The Instance surface is about 23 members including exported fields, so the HealthTarget port is cosmetic, InstanceStore has no second consumer, a field-vs-method collision (started/Started) blocks the capitalisation, and it needs a new exported mutator. SC4 has 0 of the last 500 commits.
- **COORD-4 sweepers**: payoff:reject; ledger already skipped by SC6. Needs unexported Instance members (hibernateAutomatically, hibernationIdleDuration, shells) exported. The BackgroundTask runner in server.go (a hot file, 147 commits per 180d) does not remove the per-sweeper gate edits, so it is a speculative registry.
- **COORD-5 attention (session/attention)**: payoff:reject. suppressionPolicy is called by the staying poller, so the core would import the leaf; the AttentionSubject port is mis-specified; review_diff_budget.go is imported by session/backlog; 6 in-package tests cannot import the leaf. Only determiner injection (wave 3) is kept.
- **SVC-1 sessionjobs**: payoff:reject. The sessionArchiver and sessionDeleter ports were wrong (connect types in the port), svckit does not exist yet, FindSupersededSessions is shared with backlogsvc, and the Job interface is a cosmetic registry. Possible later: the in-place ports plus a table-driven server.go job list.
- **SVC-2 fileops**: payoff:reject. Touch gain 0.00 pp measured with scoping touch (prod_changes=248, gain=0). Not a leaf (SessionService.fileSvc, GetFileService), files_delegates.go mixes SessionService and FileService methods, FileBackend is not implementable outside wtpath without exporting linkProber, ContentSearcher has one implementation. Reopen only if the owner wants a test-share gain.
- **SVC-3 insights**: payoff:reject; ledger row 40 already skipped by SC1/SC6 (test -0.020 pp). One real finding worth keeping as its own item if wanted: peel watch_stream_metrics.go into server/services/internal/streammetrics, which removes the services import from wsconn and backlogsvc (8 callers). That needs its own SC check and was not accepted here.
- **SVC-4 slack**: payoff:reject and a verified cycle. SlackConfigService holds the concrete *SlackNotifier and calls unexported postToSlack, so the core still imports slackchan. approvalNotifier has one implementation. Revisit only inside a notification-channel strangler with a second real channel.
- **SVC-webhooks**: payoff:reject and prior ledger verdict (SC4 fails: 2 of the last 500 commits touch it, neither cluster-only). The sha256Hex back-edge is a method-name false positive. Defer until a second webhook provider exists. If wanted, the flag-name constant could move to config on its own merits.
- **SVC-providers**: payoff:revise but the UsageReader port is wrong (drops cost, idle-wait and spend-rate guardrails because it omits tokens.ParseResult), julesKeyringTokenSource must be exported, and provider construction still lives in NewSessionService. Stage 1/2 in-place cleanup (deduplicate the provider if-chain, lock-safe Bind/LimitsFor) is a candidate for a later wave once the port is redesigned.
- **SVC-policy**: payoff:reject and a hard dependency on SVC-providers, which does not exist. The knownCLIAgents move is a 15-line in-place change that needs no package.
- **SVC-autonomous**: open-closed:reject, payoff:reject. DriverHost would be about 14-15 methods (interface pollution), consumer-owned interfaces would move into the producer, and NewSessionService would keep the concrete import. Record in roadmap deferred list.
- **Rejected clusters carried from the input (scroll forwarding, write lease, serialization, worktree/git, process/terminal methods, state machine/actor, shells, claude controller, tags/checkpoints, pane reply, Jules poller, summary generators, tag poller, nudge/diagnose, hooks/wake/guidance, workflow_engine)**: Rejected upstream by the original analyses for the reasons given there (single implementation, cold code, receiver-bound methods, or Instance-state entanglement). Nothing in this synthesis changes those verdicts.

## Owner questions

- Q1: The roadmap deferred session P6-historyadapters with 'SC6 near zero'. Your 2026-11-10 note says open-closed shape alone justifies a physical move. Do you accept the in-place registry injection (wave 2, no new package) as sufficient, or do you want the physical convhistory split (as a stable contract plus an adapters leaf) even with no measurable Bazel gain?
- Q2: Do you want a package-count goal or an OCP-shape goal? Nearly every proposed leaf failed because it still imports the hub types (Instance, Storage, EntRepository, SessionService), so it is a lower dependency of the core, not a leaf. Real leaves need the Storage/EntRepository DTO lowering (ItemSessionSummary, BacklogStatusEventData, ItemGuardView, InstanceData into a lower package, ADR-011) first. Should that Tier C DTO move be the next scheduled project before any further physical splits?
- Q3: For waves 2-3 (in-place refactors derived from rejected proposals), do you want them run as ordinary SDD fix PRs, or held until the DTO lowering lands? They carry no Bazel claim.
- Q4: SC6 waiver policy. Q1 of 2026-11-10 waives SC4/SC6 for OCP justification. Reviewers read that as applying only when the split actually opens an extension seam. Confirm that pure file moves with no new seam (such as SVC-5) still need a replay number in the PR body rather than a blanket waiver.
- Q5: May the cmd/ssq-hooks and server/push closure reductions in wave 1 be accepted on the strength of go list -deps counts (about 160 to 132-134) rather than a scoping replay SC6 pass, given that replay showed zero gain for comparable moves?
- Q6: Should SVC-5a also rename PushService to push.Service (stutter removal)? The reviewers asked for an up-front decision. Default here is to keep names and defer the rename.