# W0e audit: file-to-package manifest, raw-field audit, startup order (2026-11-13)

Read-only audit by an agent against `origin/main` at `bf6b0a093`. Machine-readable outputs were `w0e-manifest.csv` (266 rows: file,loc,package,note) and `w0e-rawfield.tsv` (297 rows); they are scratch files, regenerate from the method in section 2 if needed. Numbers are the agent's; nothing was compiled.

**Key findings:** (1) the `instance` core is 10,508 LOC at file level, over the cap before shims, so method-level extraction out of `instance.go` (2,902 LOC) is required; (2) `session/lifecycle` already exists and `tmux`/`tymux` import it, so the satellite must be renamed (e.g. `session/sessionhealth`) to avoid a cycle; (3) `storage_backlog.go` (1,617 LOC) is 48 `*EntRepository` methods and belongs in `backlogrepo` whole; (4) three plain-field hooks (`itemChangePublisher`, `callbackDispatcher`, `ChainFirer`) are set after the backlog sync loop starts: a possible data race, fixed by a shared settable hook type; (5) 297 raw Instance-field accesses (38 writes, 44 with no `InstanceSnapshot` cover) live outside the Instance family.

---

## 1. File-to-package manifest (266 non-test files; basis: `go run ./tools/bzl/scoping t9` = 71,111 on linux `GoFiles`; 71,219 with the 3 `*_windows.go` files = 108 LOC)

Per-package file-level totals vs the plan's figure (tests follow their subject file; white-box tests touching unexported Instance state go with `instance`; test LOC not enumerated):

| Package | Files | LOC | Plan |
|---|---|---|---|
| `instance` core | 33 | **10,508** | 8,250 |
| `instance/tmuxctl` | 14 | 4,147 | 3,987 |
| `instance/workspace` | 5 | 1,860 | 1,750 |
| `instance/claudecap` | 7 | 1,345 | 1,300 |
| `instance/prset` | 4 | 1,235 | 908 |
| `instance/shells` | 1 | 510 | 510 |
| `instance/tagging` | 2 | 279 | 412 |
| `backlogrepo` | 7 | 5,573 | 6,072 |
| `instancerepo` (insight ~500 and dbkit ~360 split out) | 4 | 5,245 | ~1,900 + 450 |
| `workflowengine` | 20 | 5,321 | 5,823 |
| `tmuxproc` | 15 | 4,679 | 5,090 |
| `driver` | 18 | 4,452 | 4,634 |
| `historyadapters` | 11 | 2,835 | 3,194 |
| `prrepo` | 8 | 2,527 | ~1,900 |
| `reviewqueue` | 11 | 2,445 | 2,445 |
| `workflowrepo` | 10 | 2,244 | 1,893 |
| `backlogcore` | 10 | 2,041 | 1,974 |
| `lifecycle` satellite (**rename**) | 6 | 2,003 | 2,003 |
| `messagingrepo` | 10 | 1,859 | 1,761 |
| `domain` additions | 12 | 1,795 | 2,673 |
| `gitwt` additions | 2 | 1,209 | 1,209 |
| `flowrepo` | 8 | 1,094 | 1,204 |
| `webhookrepo` | 4 | 845 | 658 |
| `claudeleaf` | 3 | 841 | 781 |
| `prtrack` | 5 | 823 | 984 |
| `dbkit` | 6 | 793 (+360) | ~600 |
| `procbackend` | 6 | 591 | 604 |
| `histcontract` / `scrollleaf` | 2 / 7 | 403 / 402 | 457 / 211 |
| `sessiondata` | 3 | 401 (+~247 from `storage.go`) | not measured |
| `prompts` (merge into existing) | 2 | 275 | unknown |
| `rowtx`, `prleaf`, `pendingguidance` | 3, 1, 2 | 177, 138, 135 | ~200, 138, ~130 |

Only one package reaches 9,000: **`instance` core at 10,508** (excludes ~417 LOC of planned shims and ~60 of host code). 7,327 is `instance*.go` + `live_instance.go` after controllers leave; Ring B files add 1,362 (`actor`, `state_machine`, `prompt_delivery`, `review_state`, `execution_target`, `session_event_recorder`, `types`). Trim candidates: `registry` 319, `retry_state` 455, `instance_controller` + `controller_manager` 541, `instance_hibernate`/`instance_crash` 310. Even after all of these the core stays above 9k at file level: method-level extraction out of `instance.go` (2,902 LOC) is required.

**Reviewer items resolved:**
- Ring B: the 12 named files other than `pty_discovery` sum to 2,173; with `backend_factory` (140) = 2,313 (matches the plan's figure; inferred). `pty_discovery.go` (921) is the external-session PTY discovery manager; the only Instance symbol in it is `const stopJoinTimeout` (`:29`), used at `instance_hibernate.go:164` and `session_driver.go:372`: move the constant, the file goes to `tmuxproc`. `process_capabilities` goes to `procbackend`.
- `storage_backlog.go` (1,617 LOC): 48 methods, **all `*EntRepository`**, no `*Storage` methods: it belongs in `backlogrepo` whole. Storage-facade equivalents are in `storage.go` (184 `*Storage` methods, 59 backlog/workflow-named, ~320 LOC) and disappear when `Storage` is dissolved. `ClaimDiagnoseNudgeAttempt` could go to `driver`.
- Store-residual gap: `ent_*` 12,089 + `storage*` 4,488 = 16,577; the remaining 3,453 sit in `workflow_stage_seed`, `session_pr_backfill`, `backlog_item_prs`, `trigger_fire_event_repository`, etc. Repo-level total from this assignment: 20,492 (vs the plan's 20,030).
- Instance methods defined OUTSIDE `instance*.go` (cannot move above `instance` unless converted to functions): `pr_tracking` (15), `retry_state` (9), `turn_end` (4), `claude_trust`/`session_activity`/`session_driver` (2 each), `backlog`, `session`, `repo_path`, `history_adapter`, `hot_restore`, `backend_resolution`, `pane_submit`, `claude_version_check`, `ratelimit_lease_accessor`, `testing`, `actor`/`prompt_delivery`. `backend_resolution.go` contradicts Part A (`pinResolvedBackend` is an Instance method): left in the core.
- Not reproduced: the plan's 5,823 for `workflowengine` (this audit: 5,321), the `domain` lowering total of 2,427 (about 1.3k nameable), `prrepo` ~1,900 (this audit: 2,527).

**Name collisions:** `session/prompts` exists (`store.go`, stdlib-only `PromptStore`, 209 LOC): merge `prompt_sections.go` + `steertext.go` (275) into it, no new dependencies. `session/lifecycle` exists (`Reason`, timeouts) and is imported by `session/tmux/control_mode.go` and `session/tymux/stream.go`: merging the 2,003-LOC satellite into it would make `tmux` import Instance and create a cycle: **rename the satellite** (e.g. `session/sessionhealth`).

**Splits inside files:** `storage.go` (~247 lines of `InstanceData` types to `sessiondata`), `ent_repository.go` (~360 to `dbkit`, ~500 to `insightrepo`; name-regex estimates), `transition_gate.go` (pure verdict part to `workflowengine`, Storage-bound part with 19 Storage refs to services), `session.go` (246 to `domain`, 148 deleted).

Not verified: compilation of any package; placement of `registry.go`, `consent_enrichment.go`, `pi_status_source.go` (guesses); test LOC per package.

## 2. Raw-field audit (per `.claude/rules/instance-lock-free-reads.md`)

Method: a go/types program over `./session` (non-test), selectors whose receiver is `Instance`/`*Instance`, field list from the struct (`instance.go:209`), cover checked against `InstanceSnapshot` and its nested `GitHub`/`Autonomous` structs. **297 exported or promoted field accesses in 29 files outside the `instance*.go` family; 38 writes; 44 with no snapshot field ("missing").** Counting only `Path|Branch|Title|Status|WorkingDir`: 158 hits (the earlier review's 70 undercounted). Server and other packages were not scanned.

Largest: `session.go` 70 accesses (20+ writes; `SessionToInstance`, deleted by plan item 1.5), `session_driver.go` 60 (7 writes), `pr_tracking.go` 33, `review_queue_poller.go` 21 (`Status` at :567, :605, :636, :662), `retry_state.go` 16 (2 writes), `health.go` 17, `startup_scanner.go` 12, `hibernation_sweeper.go` 9, `external_approval.go` 7, `autonomous_driver.go` 6, `pty_discovery.go` 5, ~19 more files with 1-4 each.

Writes outside `session.go`: `session_driver.go:1219` `Status` (under `inst.mu.Lock`); `session_driver.go:383,1176-1178,1192` and `retry_state.go:310,450` write `RetryMaxAttempts`, `RetryAttempt`, `LastFailureReason`, `NextRetryAt` (**not in `InstanceSnapshot`**); `turn_end.go:57`, `session_activity.go:9`, `backend_resolution.go:56` (inside Instance methods: they move with the core); `storage.go:515` `Artifacts`.

Snapshot-missing fields used outside the family: `RetryAttempt`, `RetryMaxAttempts`, `NextRetryAt`, `LastFailureReason`, `RetryHistory`, `RetryState`, `RetryPolicyOverride`, `LastViewed`, `LastAcknowledged`, `LastTerminalUpdate`, `LastMeaningfulOutput`, `LastOutputSignature`, `LastAddedToQueue`, plus `GitManager` (`health.go:581,584`, `review_queue_poller.go:246`) and `ExistingWorktree`. **Adding these to `InstanceSnapshot` is a prerequisite of the move.**

Unexported Instance API used outside the family (524 hits, 25 files): the actor API (`instanceState`, `sendCtx`, `sendSync*`) is used outside by `review_queue_poller.go:556,603,634,661` (603/634/661 are status-guarded transitions to `Active`; 556 not fully read): `reviewqueue` needs one exported method such as `TransitionToActiveIf(ctx, from Status) error`. `hot_restore.go:94`, `turn_end.go:55`, `session_activity.go:15`, `state_machine.go:97` are Instance methods/Ring B and move with the core. Other unexported members reached into: `session_driver.go` (51 hits: `mu`, `driverMux`, `driverGoroutines`, `driverWG`, `driverRunning`, `driverStopper`, `driverDestroyed`, `clearNextRetryAt`: needs a driver-state API), history adapters (`claudeSession`, `claudeSessionMu`, `piSession`, `tryExtractConversationUUID`, ~45 hits: need accessors), `hibernation_sweeper.go` (`hibernationIdleDuration`, `hibernateAutomatically`), `storage.go` (`prPersistMu`, `clearPRPersistPending`, `terminalClearPending`).

Not verified: whether each write holds the right lock; raw-field use in server/other packages (compiles either way).

## 3. Startup order of the late-bound hooks (`server/dependencies.go`)

| Setter | Storage | Set at | Earliest reader | Race |
|---|---|---|---|---|
| `SetItemChangePublisher` | plain field on `EntRepository` | `dependencies.go:1064` | `publishItemChanged` (`ent_repository_backlog.go:2242`), any backlog mutation | nil-checked reads, but a plain field with no happens-before to goroutines already running; whether the instance-start `go func` at `:1123` reaches it is UNVERIFIED |
| `SetCallbackDispatcher` | plain field | `dependencies.go:1306` | `dispatchCallback`, status transitions | same, plus the sync loop |
| `SetChainFirer` | plain field | `dependencies.go:2079` (via `WireChainFirer`) | `dispatchChainFire` on a "done" transition | **latest-set hook**: `BacklogController.Enable` runs at `:1623` (`go sl.Start(ctx)`), ~450 lines earlier, so the sync loop and listener can run concurrently with a plain-field write: possible data race |
| `SetTransitionGate` | `atomic.Pointer` | `dependencies.go:1696` (via `InjectTransitionGate`) | `GatedTransition` (`transition_gate.go:210`) via `backlog_sync.go:315` | no data race; a window `1623..1696` returns the legacy Off answer; the sync loop's first run is one `sl.interval` tick after start (inferred) |
| `SetWakeChangeHook` | `atomic.Pointer` | `server/wakes_module.go:216` (from `server.go:886`) | `notifyWakeChanged` | no race; changes before the set are missed |
| `SetClaimRecorder`, `SetForeignClaimLookup`, `SetPRProvenanceSource` | `atomic.Pointer` | `main.go:1578-1583` inside `startHostGossip`, only with remote access on | `CreateBacklogItem` (`storage.go:1252`), `SyncOne` | no data race; `srv.Start` (`main.go:545`) precedes `startRemoteAccess` (`:554`), so an early item may lack `ClaimingHostID`; behind the off-by-default `cross_host_claim_dedup` flag |

`dbkit.Hook[T]`: unnecessary for the five atomic setters (already race-free; the reader-before-set window remains regardless), **necessary for the three plain fields** (`itemChangePublisher`, `callbackDispatcher`, `ChainFirer`; `ChainFirer` depends on `workflowScheduler`, built after `backlogSvc`, so constructor injection cannot reorder). Once `Storage` is dissolved every repo package needs a settable hook anyway.

Not verified: no `-race` run of `BuildDependencies`; whether any instance-start goroutine reaches the plain-field hooks; the sync loop's `interval`; whether `main.go:1578` is the only claim-setter caller (non-test grep only).
