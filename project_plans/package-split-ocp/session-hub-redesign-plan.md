# Revised wave plan: hub-type redesign (2026-11-12)

Output of the `session-hub-redesign` workflow (4 design agents, 1 synthesizer, 2 adversarial reviewers), produced after the owner decisions of 2026-11-12: every package under 10,000 non-test LOC; `Instance` moves first; `Storage` is split by aggregate; the four held clusters are in scope. This **supersedes the wave order** of `session-under-10k-roadmap.md`. The cycles review below reproduced six defects in W2/W3 with scratch builds (verdict: NO-GO on W2 and W3 as written); the arithmetic review follows. W0 and W1 were not refuted. The design reports (A: Instance relocation, B: Instance decomposition, C: Storage/EntRepository by aggregate, D: satellites) are in the workflow journal; the synthesized plan carries their numbers.

---

# Revised wave plan: session and server/services under 10k

Baseline: main `ce726e73c`, session = 71,111 non-test LOC (t9 basis). All LOC below comes from the designers (Parts A–D). I re-ran no builds. The only commands I ran were `git show 9ef0bdede:...session-under-10k-roadmap.md | sed -n 265,330p`, to read D1–D8 (Part D read the roadmap from the same commit).

## 1. Verdict and changes from the prior roadmap

**Verdict: GO, in this order.** Lower values, then peel the zero-back-edge leaves, then move Instance, then peel satellites as plain downward imports. Instance decomposition and the per-aggregate repository peel run as parallel tracks.

The move PR (W3) is NO-GO until W0–W2 land. This matches Part A's conditional GO.

**What changed:**
- **Instance moves before the satellites.** This reverses the prior wave order and follows owner decision 2.
- **Ring A leaves still go below Instance.** Part D measured them as having no back-edges, so they are plain imports with no ports.
- **Ports only where a back-edge exists.** These are the driver loop, the process factory, the history registry and the store. Part A found the closure bounded at about 6 ports.
- **Instance is decomposed after it moves**, into controller subpackages (Part B).
- **Storage and EntRepository are dissolved into per-aggregate repositories** (Part C).
- **A new DTO leaf, `session/sessiondata`.** Part A says the DTOs cannot go into `session/domain` (purity test; the PR types import `github`). Parts C and D say lower them. This is my resolution, not designer-verified: pure values go to `domain`, and `InstanceData`/PR vocabulary go to `sessiondata`. That keeps `backlogrepo` and `prrepo` from importing `instance`.

**Prior defects D1–D8** (text read from `9ef0bdede`):

| # | Status | Resolution |
|---|---|---|
| D1 | Fixed, one open check | History splits at the Instance boundary. Contract files (`history_detector`, `history_fork`, `scroll_adapter`) go below as leaves. Adapters and registry go above (W7). `LiveInstance` (79 LOC) moves with Instance, so no Session port. Open: the scratch build reported 3 undefined `Storage` symbols in history, but Part C's grep found zero. W2's compile loop settles it. |
| D2 | Fixed | `claude_scroll_adapter` and history land in one PR (W7). Scroll leaves go below. |
| D3 | Fixed, with new work | WE has 0 edges to Instance, so it sits before `backlogcore` and `driver`. Cuts: lower the types, extract a `prompts` leaf (18 symbols), and add a `StageRepoFactory` seam (one call site, `workflow_engine_replay.go:308`). I use 5,823 LOC for the 21-file cluster. The 4,371 and 4,491 figures did not reproduce (Part D), so I use neither. |
| D4 | Fixed | Owner decision. The Instance↔driver cut is `DriverRunner`, with the Start/Stop wrappers (about 150 LOC) moving into Instance. The tmux↔review cut is moving `startup_scanner.go` (98 LOC) into `reviewqueue`. |
| D5 | Fixed by design, with a caveat | The peel is a hub redesign (Part C). Repos never import Storage, and Storage is dissolved. PC-3 to PC-8 all edit the `ent_repository.go` struct and `dependencies.go`, so they are not file-disjoint. They can be authored in parallel but land serially. |
| D6 | Fixed | `transition_gate` pure-part split lands after the first lowering PR (W1.2). |
| D7 | Partly fixed | A startup-order test and a Snapshot audit of `session_driver.go` (20 raw fields) and `session.go` (33) are W0 tasks. The `noarchivedrevival` path edits are in the per-move checklist. Open: whether the plain-field hooks race. Part C marks that UNVERIFIED, and `dbkit.Hook[T]` removes the question. |
| D8 | Answered by owner decisions 1–4 | New real questions are in §7. |

The prior arithmetic inconsistency (low WE figure in the wave total) is fixed by using 5,823 throughout.

## 2. Target package table

LOC is file-level arithmetic from the designers, not post-move t9. T9d (the size cap) is the gate.

**Layer 0 (leaves)**

| Package | Contents | LOC | Headroom |
|---|---|---|---|
| `session/domain` | Lowered values (2,427 across clusters), plus `Session` (246) | 2,673 | ample |
| `session/sessiondata` (new) | `InstanceData` family, PR vocabulary (`PRRef`, `PRView`, …) | not measured | – |
| `session/prompts` (new) | Driver `Sec*` constants, etc. | size unknown | – |
| `session/procbackend` | `process_manager`, `process_capabilities`, `shell*`, `backend_registry` | 604 | 9.4k |
| `session/histcontract`, `scrollleaf` | `history_detector`, `history_fork`, `scroll_adapter`; scroll lease, canary, metrics | 457, 211 | ample |
| `session/claudeleaf` | `claude_session_manager`; `pi_status_source` (530, pending design) | 251 + 530 | ample |
| `session/gitwt` (existing 913) | `worktree_enricher` (432), `repo_path` (777), git leaf | about 2,122 | ample |
| `session/prleaf` | `pr_poll_metrics` | 138 | ample |
| `session/dbkit` | Open, options, migrations, seed, `Hook[T]` | about 600 (estimate) | ample |
| `session/rowtx`, `pendingguidance` | PR link rows; guidance cache | about 200, about 130 | ample |

**Layer 1 (Instance and its controllers)**

| Package | LOC |
|---|---|
| `session/instance` core | about 8,250 (Part B: 7,775 + 417 shims + about 60 host) |
| `instance/tmuxctl` | 3,987 |
| `instance/workspace` | 1,750 |
| `instance/claudecap` | 1,300 |
| `instance/prset` | 908 |
| `instance/shells` | 510 |
| `instance/tagging` | 412 |

Part B's table sums to 17,036 including 246 to `domain` and 148 deleted. I verified that arithmetic.

**Layer 2 (satellites)**

| Package | LOC |
|---|---|
| `workflowengine` | 5,823 |
| `tmuxproc` | 5,090 (t9; 5,198 on the ledger basis) |
| `driver` | 4,634 |
| `historyadapters` | 3,194 (2,955 + 83 + 156) |
| `reviewqueue` | 2,445 |
| `lifecycle` | 2,003 |
| `backlogcore` | 1,974 |
| `prtrack` | 984 |

**Layer 3 (repositories, Part C)**

| Package | LOC |
|---|---|
| `backlogrepo` | 6,072 |
| `workflowrepo` | 1,893 |
| `messagingrepo` | 1,761 |
| `prrepo` | about 1,900 |
| `instancerepo` | about 1,900 (estimate) |
| `flowrepo` | 1,204 |
| `webhookrepo` | 658 |
| `insightrepo` | about 450 (estimate) |

`server/services` and `server/dependencies.go` sit above all of these.

**Ledger (sums to 71,111; I recomputed it):**

| Item | LOC |
|---|---|
| Lowering | 2,427 |
| Ring A | 2,704 |
| Instance family | 19,499 |
| WE, prtrack, gitwt | 7,111 |
| backlogcore, tmuxproc | 7,064 |
| reviewqueue | 2,445 |
| history | 3,194 |
| driver | 4,634 |
| lifecycle | 2,003 |
| Store cluster (residual) | 20,030 |

**Gaps, flagged:**
- **108 Windows LOC.** Part D's file partition sums to 71,219, which is t9 plus 108 Windows LOC. I did not re-measure them.
- **Store residual.** Part C's repo files total about 16.7k plus about 4.8k of Storage logic, which gives about 21.5k against D's 20,030. Not reconciled. The W0 file-to-package manifest must assign each file.
- **Ring A double counts Pi metrics.** Ring A is 2,704 excluding the 60 LOC `pi_status_source_metrics`.
- **Instance family basis.** The 19,499 is Part D's basis (17,036 + 2,313 Ring B + 150). Part A measured 15.6k to 21.2k depending on what is pulled in, mostly on `wc -l` rather than t9.
- **Core headroom.** Ring B (2,313) is not in Part B's decomposition. Minus `repo_path` (777), it adds 1,536; with the 150 wrappers, the core could reach about 9,936 (my arithmetic). That is under the cap by about 64 LOC. W3/B7 must place Ring B files into controllers (for example `claude_trust` into `claudecap`, `scroll_gate` into `tmuxctl`), or the core fails the cap.
- **Part A / Part D disagreement on `backend_factory` and `backend_resolution`.** Part D put them in Ring B (they travel with Instance). Part A keeps them behind with `ProcessManagerFactory` injected. Part A is right. Inside `instance` they would import the concrete backends, creating an instance↔tmuxproc cycle. They stay with `tmuxproc` (W5), and Ring B shrinks by their LOC (not measured).
- **Part A / Part D disagreement on `review_queue.go`.** Part A found it is only aliases of `session/queue`. Replace the aliases with direct imports and delete the file; no port is needed.

**Packages still over 10k that this plan does not fix:**

| Package | LOC / cap |
|---|---|
| `session/backlog` | 13,427 |
| `server/services/backlogsvc` | 15,327 (budget cap 15,400) |
| `server/services` | 70,113 |
| `server/services/wsconn` | cap 9,895, no headroom |
| `session/instance` | interim 18.7k, until the B track |

Owner decision 1 covers these, so they need a follow-on design. See §7.

## 3. Dependency DAG (A imports B)

```
domain, prompts, sessiondata                        <- everything
procbackend, histcontract, scrollleaf, claudeleaf,
  prleaf, gitwt, dbkit, rowtx, pendingguidance       -> domain, sessiondata
instance                                             -> domain, sessiondata, procbackend, histcontract,
                                                        scrollleaf, claudeleaf, gitwt, prleaf, session/queue
instance/{tmuxctl,workspace,claudecap,prset,
  shells,tagging}                                    -> domain (Host interface; never instance)
workflowengine                                       -> domain, prompts
prtrack                                              -> instance
tmuxproc                                             -> instance, procbackend
backlogcore                                          -> instance, workflowengine
reviewqueue                                          -> instance, tmuxproc
historyadapters                                      -> instance, reviewqueue, histcontract
driver                                               -> instance, backlogcore, workflowengine, gitwt, tmuxproc,
                                                        reviewqueue, historyadapters, prompts
lifecycle                                            -> instance, driver, tmuxproc, reviewqueue
workflowrepo, flowrepo, webhookrepo, insightrepo,
  messagingrepo, prrepo, backlogrepo                 -> dbkit, rowtx, pendingguidance, domain, sessiondata, ent
instancerepo                                         -> instance, dbkit, rowtx
server/dependencies.go                               -> all
```

Repos never import each other (depguard). Hooks are injected through `dbkit.Hook[T]`.

**Topological order:** W0 → W1 → W2 → W3. After W3, the satellite chain (W4 → W9) runs alongside Track B (B0 → B7). Track C (PC-3 to PC-8) can start after W1. PC-9 follows W5. PC-10 follows W3 and W9.

## 4. Waves

Within a wave, PRs are file-disjoint where stated and land serially.

### W0: small, directly executable now (5 PRs, design done unless noted)
- **0a: Part A's P1.**
  - Replace `ReviewQueue` aliases with `session/queue` (3 and 4 refs).
  - Replace `*ent.Shell` in `instance_shells.go:421` with `ShellData` and a `ShellRepository` (ent plus fake).
  - Redeclare `ApprovalMetadataProvider`, `Notifier` and the `BounceMainBranch` constant at their consumers.
- **0b: `prompts` leaf.** It needs a short enumeration of the 18 WE→driver constants first; size is unknown. Land after 0c to avoid trivial conflicts.
- **0c: `StageRepoFactory` seam** at `workflow_engine_replay.go:308`, wired at `dependencies.go:943`.
- **0d: FLAGS-1 stage 1** (about 450 LOC, UNVERIFIED; plan in `roadmap.md:168-172`). It removes 0 LOC from session and is independent.
- **0e: audits and manifest.**
  - A composition-root hook startup-order test.
  - A Snapshot audit of the raw field reads in `session_driver.go` and `session.go`.
  - The file-to-package manifest that closes the ledger gaps.

### W1: lowering, large, serial codemod chain, one cluster per PR
1. **1.1:** `NormalizeHost` to a stdlib leaf, then value lowering to `domain` (the 2,427 LOC; the 83 `BacklogStatus` refs, `GateStatus`, the AC types, etc.).
2. **1.2:** `transition_gate` pure-part split (D6).
3. **1.3:** `sessiondata`. The `InstanceData` family (179 refs in 54 files) and the PR vocabulary with the three `Err*` and `PRLink`/`PRInitial`/`PRLinkResult` (289 refs in 58 files). `Status`, `SessionType` and `ProcessManagerBackend` go to `domain`. The purity test must pass.
4. **1.4:** small DTOs, four file-disjoint PRs: wake, guidance (including the create-input type with the unexported field), flow marker and limiter, hook types and workflow definitions.
5. **1.5:** Part A's P2.
   - Convert `WorkspaceKey` and the stray methods: `ToSession`, `SetProcessManagerForTest`, `IsBacklogOriginatedSession`, `historyRegistry`, and Part B's 10 strays.
   - Delete the `InstanceToSession`/`SessionToInstance` converters (148 LOC; only caller `ToSession`, no non-test callers).
   - Extract `repo_path` git helpers to `gitwt`.
6. **1.6:** PC-1 `dbkit` (makes hooks atomic) and PC-2 `rowtx`/`pendingguidance`.

Needs a design step: only the exact lowered-type list (derive from the 1.3 compile loop). Everything else is design done.

### W2: leaves and ports, medium (about 6 file-disjoint PRs)
- Leaves: `procbackend`, `histcontract`+`scrollleaf`, `claudeleaf`, `prleaf`, and `worktree_enricher`/`repo_path` into `gitwt`.
- **Pi status (needs a design step).** `pi_status_source` uses 18 symbols from `pi_adapter.go`, which imports Instance. Either extract pure helpers, or inject a `PiStatus` port of 5 methods.
- **History/conversation port (needs a design step).** `CanonicalTurn` lives in `canonical.go`, which stays above. Lower the type, or keep the port at 5 methods or fewer. Anything larger is a redesign.
- **Ports (design done).** `HistoryPorter` (`Handles`, `CrossesAdapters`, `Port`), `DriverRunner`, `ProcessManagerFactory`, and a `store` interface for `live_instance.go`. Real implementation plus fake each.
- **Wiring decision.** Functional options on `NewInstance`/`FromInstanceData` (10 non-test and 39 test sites) rather than package globals, so tests stay `t.Parallel`. If the cost is too high, a one-time `Configure(Deps)` is the fallback.
- **Guard prep (Part A's P6):** `noarchivedrevival` paths, the `actor-field-guard` file list, the lock-free-reads glob (`session/instance/**/*.go`), Bazel lists and `budget.txt`.

### W3: Instance relocation, large
- One `scoping move --lower` of about 47–55 files plus 50 white-box tests (13,247 LOC).
- **Importers:**
  - 929 non-test refs in 178 files and 1,890 test refs in 234 files for the 171 exported names.
  - `session.Instance` alone: 502 refs in 134 files; with `InstanceSnapshot`, 1,502 in 331 files across 36 packages.
  - The largest is `server/services` with 1,447 refs.
  - The `Status` constants are a plain qualifier change (720 refs, 163 files).
- **Prerequisite:** a scratch `go build -gcflags=-e` on the exact move set before opening the PR.
- Declare a ratcheted interim `budget.txt` row for `session/instance`.

### W4–W9: satellites (each `git mv` plus codemod, one cluster per PR)

| Wave | PRs | Cost | Notes |
|---|---|---|---|
| W4 | `workflowengine` (5,823); `prtrack` (984); `gitwt` residual (304) | medium × 3 | Disjoint. Path-keyed `workflow_bypass_inventory_test.go`. |
| W5 | `backlogcore` (1,974); `tmuxproc` (5,090), with `backend_factory` and `backend_resolution` | medium × 2 | `session/backlog` cannot absorb backlogcore (13,427). |
| W6 | `reviewqueue` (2,445), including `startup_scanner.go` | small | Breaks the tmux↔review cycle. |
| W7 | `historyadapters` (+ scroll and claude residuals) | medium | One PR (D2). Needs W6 (`InhibitionEngine`, 2 symbols). |
| W8 | `driver` (4,634), with Start/Stop wrappers staying in Instance | medium | `noarchivedrevival` files `session_driver.go`, `health.go`. |
| W9 | `lifecycle` (2,003) | small | Last, because driver→lifecycle edges are 0. |

### Track B: Instance decomposition (after W3, parallel with W4–W9, all within `session/instance/**`)

| PR | Content | Cost |
|---|---|---|
| B0 | Remove the dead-method candidates (71 methods, 496 LOC; this is an upper bound that still includes interface-satisfying methods) after compile and interface checks | small |
| B1 | `Host` seam plus `tagging` pilot (0 host methods). The `InstanceSnapshot` question is decided here (§7) | small |
| B2, B3 | `shells` (1 host method); `prset` (3 host methods) | small, medium |
| B4 | `workspace` plus checkpoint (8) | medium |
| B5 | `claudecap` (12) | medium to large |
| B6a, B6b | `tmuxctl` leaves (about 1.5k), then main (136 methods) | medium, large |
| B7 | Core trim and shim sweep. The core must be at or under 9k, with Ring B files placed | medium |

B2 and B3 are file-disjoint. The shim budget is about 417 LOC.

### Track C: repositories

| PR | Content | Cost |
|---|---|---|
| PC-3 | `workflowrepo` (pure move) | small |
| PC-4 | `webhookrepo` | small |
| PC-5 | `flowrepo` (engine injected) | small |
| PC-6 | `messagingrepo` (wake + guidance + operator wake) | medium |
| PC-7 | `insightrepo` | small |
| PC-8 | `prrepo` (`pr-state-write-guard` allow-list changes) | medium |
| PC-9 | `backlogrepo`; claim recorder, remediation and `transitiongate` move to services. Needs W5 and an approved freeze on `storage_backlog.go` | large |
| PC-10 | `instancerepo`; delete `EntRepository`, `Storage`, `GetEntClient` (78 files), `NewStorageWithRepository` (74), `NewTestEntRepository` (97 files; replaced by `repotest`) | large |

PC-3 to PC-8 are independent of each other but not file-disjoint (shared struct); land them serially, leaves first. PC-10 needs W3 and W9.

## 5. Per-move checklist

Each of these was caught only by CI on earlier extractions. Run all of them before pushing.
- **Sinkguard tests.** New RPC classification, and path-keyed tags.
- **Path-keyed guards.** `server/services/terminal_write_pins_test.go`, `session/legacy_item_pr_readers_test.go` (lines 31–32 key on two ent files), `workflow_bypass_inventory_test.go`, `workflow_bypass_strict_test.go`, `legacy_pr_readers_test.go`, `pr_state_writer_guard_test.go`, `Makefile:1330`, `.golangci.yml:472`.
- **Bazel.** `make gazelle` and `go_sources`/`data` for moved files.
- **Budget.** `budget.txt` and `boundaries.txt`, with a `scoping-budget-raise:` commit line for any raise.
- **`noarchivedrevival`.** The file set at `tools/lint/noarchivedrevival/analyzer.go:41-44` (Part A: three files move with Instance, `session_driver.go` stays).
- **`actor-field-guard`.** `Makefile:1301`: its own file list and the "outside `session/instance*.go`" exemption.
- **`instance-lock-free-reads.md`.** Update the glob. Satellites must use `Snapshot()` or `ActiveDir()`, not raw exported fields.
- **Registry.** `make registry-generate`.
- **Gates.** `make ready` runs the golangci/`dupl` gates, so check them before pushing.
- **Other.** depguard rule `scoped_packages_off_ent`, `tools/lint/nowrongdbentity`, and `t.Parallel()` on moved tests.

## 6. Risks and stop rules

- **Two repair rounds, then simplify.** Repeated new blockers in one subsystem means delete or simplify, not patch.
- **Instance floor.** If a decomposed core cannot get under 10k, Part B's fallback keeps `claudecap` and `prset` in the core, at about 10.4k, which still fails. Stop and re-plan before B7 if B6b leaves more than about 1k of orchestration in the core.
- **Port size.** A port over 5 methods is a redesign. Every new interface needs two implementations or a named fault-injection test in the same PR.
- **No repo-to-repo imports.** Move the shared piece to `rowtx` or `dbkit`, or inject a hook.
- **`backlogrepo` cap.** Stop at 9k and split item sessions, verdicts and AC (about 2.1k).
- **Codemod hand edits.** A PR needing hand edits in more than about 10% of its files gets split.
- **Lowering imports.** A lowering that needs `domain` to import `github`, `tmux` or `config` stops. Build a stdlib leaf first.
- **Concurrent state.** Check `uptime` and `free -g` before launching agent waves, since tmpfs `/tmp` and parallel Bazel JVMs have OOM-restarted the service before.
- **Unverified items that block specific waves:**
  - The 71-method dead set.
  - The 1,568 LOC "pure" set compiling standalone.
  - The 125 LOC struct-slimming and 60 LOC `Host` estimates.
  - `Host` surface including non-snapshot fields (`processManager`, `pmMu`, `GitManager`, `started`, `claudeSession`/`piSession`).
  - The `pty_discovery.go` symbol Instance uses.
  - The git→driver 2-symbol edge.
  - White-box test LOC per package.

## 7. Open owner questions

1. **Interim cap exception.** Is `session/instance` at about 18.7k allowed from W3 until B7, with a ratcheted `budget.txt` row? Decision 1 says "end under 10k", and I assumed end state.
2. **`InstanceSnapshot` in `domain`.** Part B recommends moving the struct (not `Snapshot()` or the setters) to `domain` so controllers can read it. This amends a sentence of `instance-lock-free-reads.md`. The alternative is per-controller read views and more mapping code.
3. **Other packages over 10k.** Does the cap apply to `session/backlog` (13,427), `backlogsvc` (15,327) and `server/services` (70,113)? Decision 1 says every package, and no designer covered them. If yes, commission a separate design before W5, because PC-9 and `backlogcore` add code to that area.
---

## Review 1: import cycles and ordering (scratch builds)

Verdict: NO-GO on W2 and W3 as written. I found six defects. All were reproduced in a scratch copy of `ce726e73c` (`~/.cache/tmp/cycrev/`, now deleted). I used `go build -gcflags=-e` per package. The scratch copy built `./session` cleanly first.

**D-A. `backend_registry.go` cannot live in `procbackend` (W2).**
- Command: copied `process_manager`, `process_capabilities`, `backend_registry` into `session/procbackend`.
- Result: `undefined: newTmuxBackendFromOpts, NewNativeProcessManager, newTymuxBackendFromOpts, TmuxBackend, TymuxBackend, NativeProcessManager, ErrUnrecognizedBackend`.
- Cause: the registry literal names the concrete backends, and `BackendKey()` methods are declared on them.
- Control: with the registry removed and `shell.go` and `shell_registry.go` added, `procbackend` builds clean.
- Fix: `procbackend` gets only the interfaces, `ProcessManagerBackend`, and the shell files. `backend_registry.go` goes to `tmuxproc` with `backend_factory.go`, which owns `ErrUnrecognizedBackend` at line 21. `Instance` reaches it through the `ProcessManagerFactory` port.

**D-B. `histcontract` and `scrollleaf` form a cycle, and the scroll adapter is placed above `Instance` (W2/W7).**
- Evidence:
  - `scroll_adapter.go` uses `ScrollForwardCapability` (in `scroll_forward_capability.go`) and `NewClaudeScrollAdapter`.
  - `scroll_forward_metrics.go:59` calls `resolveScrollAdapter`.
  - So if the contract sits in `histcontract` and the metrics in `scrollleaf`, each needs the other.
- Scratch build: `histcontract` reports `undefined: ScrollForwardCapability, NewClaudeScrollAdapter`; `scrollleaf` reports `undefined: resolveScrollAdapter`.
- The plan also puts `claude_scroll_adapter` in W7, above `Instance`. But `instance_scroll_forward.go:97,279` and `scroll_gate.go:65` call `resolveScrollAdapter` from the `Instance` side. That is a seventh port, which the plan does not list, or a cycle.
- Fix: put `scroll_adapter`, `claude_scroll_adapter` (54 LOC), capability, lease, canary and metrics together in `scrollleaf`. `histcontract` holds only `history_detector` and `history_fork`.
- Verified: with `scroll_adapter.go` and `claude_scroll_adapter.go` moved into `scrollleaf`, the cycle error disappears. Three small gaps remain:
  - `NewClaudeGestureForwardStrategy` and `IsClaude` are missing from `scrollleaf`.
  - `isValidUUID` is missing from `histcontract`; it is defined at `claude_command_builder.go:93`.
  - Fix for these: add them to W2's leaf list.

**D-C. The Instance file set is under-specified (W3).**
- The plan says "47–55 files" and never lists them. I copied the 31 `instance*.go` files plus `live_instance.go` (14,753 LOC) into `session/instance`.
- Result: 180 distinct undefined symbols. The biggest groups:

| Symbol group | Defined in | Refs |
|---|---|---|
| `instanceState`, `sendSyncValue`, `sendCtxValue` | `actor.go` | 147 |
| `capabilityOf`, `TmuxSessionAccessor`, `PriorityCapturer`, `ExitStatusReporter`, `InStreamResyncer` | `process_capabilities.go` | 25+ |
| `promptChannel*` | `prompt_delivery.go` | 10 |
| `transitionKey`, `lookupTransition` | `state_machine.go` | 6 |
| `ReviewState` | `review_state.go` | 4 |
| `NewTagManager` | `tag_manager.go` | 3 |
| `Checkpoint`, `CheckpointList` | `checkpoint.go` | 9 |
| `ExecutionTarget`, `LocalTarget` | `execution_target.go` | 8 |
| `AttachSource`, `ProjectLegacyPRMirror`, `TurnEnded` | `pr_set.go`, `pr_set_rules.go`, `session_event_recorder.go` | 9 |
| `stopJoinTimeout` | `pty_discovery.go` (921 LOC) | 2 |
| `InstancePermissions`, `ErrInvalidTransition` | `types.go` | 11 |

- None of `actor.go`, `state_machine.go`, `prompt_delivery.go` or `review_state.go` is named in the plan. This is the unlisted Ring B.
- `pty_discovery.go` is 921 LOC, which the "unverified symbol" bullet understates.
- Fix: before W3, the W0 manifest must list every file. Add a W2 prerequisite: re-run this build until the only undefined symbols are the six named ports and the lowered types.

**D-D. The actor API is unexported, but satellites above `Instance` call it (W3/W6).**
- Command: `grep instanceState` outside `instance_*` and `actor.go` finds `hot_restore.go`, `turn_end.go`, `session_activity.go`, `state_machine.go`, and `review_queue_poller.go`. The poller has four `inst.sendCtx(ctx2s, func(s *instanceState){...})` calls at lines 556, 603, 634 and 661.
- Once `Instance` moves, `reviewqueue` (W6) cannot compile these.
- Fix: add exported intent methods on `Instance` (for example `MarkReviewX(ctx, ...)`) in W1.5, and place `hot_restore`, `turn_end` and `session_activity` explicitly in the plan. `hot_restore.go` is in the `noarchivedrevival` file set.

**D-E. A raw field write exists in a satellite (rule violation).**
- Evidence: `session_driver.go:1216` reads `inst.Status` and `:1219` writes `inst.Status = PermanentlyFailed`. The same pattern appears in 70 raw `.Path|Branch|Title|Status|WorkingDir` references across `session_driver.go`, `review_queue_poller.go` and `health.go`.
- After the move these still compile, because the fields are exported, and so they silently break `instance-lock-free-reads.md`.
- The plan's W0e audit lists only `session_driver.go` and `session.go`. It misses `review_queue_poller.go` and `health.go`.
- Fix: widen the audit to every satellite file, and make the W3 definition of done "no exported mutable field assigned outside `instance/`". Add a lint or `rg` gate to enforce it.

**D-F. `sessiondata` is not a light leaf (W1.3).**
- Evidence: `pr_set.go` imports `github`. The `InstanceData` block (`storage.go:31`) is already tagged with the types `GitWorktreeData`, `DiffStatsData` and `ClaudeSessionData`. The file also imports `session/git`, `hostclaim` and `tokens`.
- `go list -deps ./github` shows `config`, `executor`, `telemetry` and `session/git/backend/cli` in its closure. I found no cycle back to `session`.
- Impact: every repository package inherits that closure. Add a depguard row for `sessiondata` and size it with t9; the plan lists it as "not measured".

**Checks that held:**
- W0a's `instance_shells.go:421` `*ent.Shell` reference is real.
- W0c's single call site at `workflow_engine_replay.go:308` is real.
- `domain` is stdlib-only; `queue` imports only `log`, `detection` and `git`.
- `noarchivedrevival` lists its files at `analyzer.go:38-50`.

**Not verified:** I did not build the whole 47-file set with the missing Ring B files added, so the true port count is unknown. I did not run `go vet` or measure the lock-free reads inside the Track B controllers. Machine load was about 56 during the builds.
---

## Review 2: size arithmetic and executability

**Verdict: the ledger adds up, but the per-package LOC and the Wave 1 file lists are not yet concrete enough to hand to worktree agents.** I recomputed the checkout at `ce726e73c` with `wc` and `go run ./tools/bzl/scoping t9`. I could not check the designers' per-file claims, because the Parts A–D file lists are not in the checkout.

**What holds**
- Baseline: T9d reports `services=70021 session=71111`.
- Cap: T9d `max=72000` passes, and `budget.txt` has `session loc=71500`.
- Ledger: 2,427+2,704+19,499+7,111+7,064+2,445+3,194+4,634+2,003+20,030 = 71,111. I recomputed it, and no row is repeated.
- Instance family: 17,036+2,313+150 = 19,499.
- WE/prtrack/gitwt: 5,823+984+304 = 7,111.
- Gitwt: 913+432+777 = 2,122.
- Part B controllers: 3,987+1,750+1,300+908+510+412 = 8,867. Adding the 8,250 core gives 17,117. That matches 17,036−246−148+417+60 = 17,119, so Part B's own arithmetic is consistent.
- The 108 Windows LOC: `session/*_windows.go` totals exactly 108. `cat session/*.go` (non-test) gives 71,219, which is 71,111+108.

**Wrong or vague claims**

1. **Two packages already exist.**
   - `session/prompts` already exists with `store.go`, a `BUILD.bazel` and 388 lines including tests. The plan calls it "new" (W0b).
   - `session/lifecycle` already exists (`await`, `reason`, `timeout`, `observability`; 537 lines including tests). The plan's `lifecycle` (2,003 LOC, W9) would collide with it.
   - The plan needs a merge-or-rename decision for both. Neither is a leaf the plan can simply add.

2. **The Store ledger row has no destination.**
   - `session/ent_*.go` (non-test) is 31 files and 12,089 lines, which matches the owner's figure.
   - `session/storage*.go` (non-test) is 4,488 lines. The plan says "about 4.8k of Storage logic", so that figure is unreconciled.
   - The repo packages in §2 total 6,072+1,893+1,761+1,900+1,900+1,204+658+450 = 15,838. Three of those are estimates ("about"), so the true total is unknown.
   - The 20,030-LOC Store residual is therefore only partly assigned: roughly 4–5k has no named package. Dissolving `Storage` is not a package until its logic is placed. `session/migrations` (572, already exists), `dbpool` and `dbmaint` exist too, and the plan does not say how `dbkit` relates to them.

3. **The Instance core has about 1% headroom, not "ample".**
   - By the plan's own arithmetic the core reaches about 9,936 LOC before Ring B is placed, a 64-LOC margin.
   - Part A's `backend_factory` and `backend_resolution` correction shrinks Ring B by an unmeasured amount, so the 9,936 figure is itself unverified.
   - The core can only pass if B7 moves specific Ring B files. The plan names two (`claude_trust`, `scroll_gate`) but gives no LOC for them.
   - Before W3, `session/instance` has no per-file allocation. The 47–55 files and 50 white-box tests (13,247 LOC) are counts, not a list.
   - Test LOC does not count toward T9, but the 13,247 figure is not labeled as such.

4. **LOC bases are mixed.**
   - `tmuxproc` is 5,090 on the t9 basis and 5,198 on the ledger basis, and the ledger row 7,064 uses 5,090. Yet the plan still prints both figures.
   - `session/backlog` is 13,427 by `wc`, but its `budget.txt` row says `loc=13802`.
   - `server/services` is 70,113 by `wc`, but T9d says 70,021, a gap of 92. The plan quotes both (70,113 in §2 and 70,021 in the baseline) with no explanation.
   - `backlogsvc` is quoted as 15,327 against a cap of 15,400. Neither number was measured here.
   - Several rows are estimates presented in the same table as measured values: `sessiondata` ("not measured"), `prompts`, `dbkit` (about 600), `prrepo`, `instancerepo` and `insightrepo`. They should be marked as estimates.
   - `domain` is "2,673" (2,427+246) with no check against what is already in the package. That gives no per-package T9d row.

5. **Other packages stay over 10k, which breaks owner decision 1.**
   - The plan admits `session/backlog` (13,427), `backlogsvc` (15,327) and `server/services` (70,113) stay over the cap.
   - `server/services` is not just over 10k. It is 7× the cap and has no decomposition at all. So the headline "session and server/services under 10k" is not delivered, and `server/services` alone is not scoped. This needs a go/no-go from the owner, not an open question in §7.
   - `wsconn` is capped at 9,895 in `budget.txt`, which leaves 105 LOC of headroom. Any W-wave touching its callers can breach it.
   - `session/instance` is 18.7k from W3 until B7. `budget.txt` ratchets are lower-only, so that interim row needs a `scoping-budget-raise:` commit line. The plan treats this as an owner question but schedules W3 without waiting for the answer.

6. **Wave 1 is not ready to hand to agents today.**
   - **1.1 and 1.3:**
     - 1.1 has no list of the lowered types. The plan says to "derive from the 1.3 compile loop", but 1.1 runs before 1.3, so the dependency is backwards.
     - 1.3 gives ref counts (179 in 54 files, 289 in 58) but no symbol list and no target file mapping.
     - The purity test for `domain` is named but not where it lives.
   - **1.4:** it names four PRs without saying which files each touches. Disjointness is asserted, not shown.
   - **1.5:** it mentions "Part B's 10 strays" without naming them.
   - **1.6:** `dbkit` is "about 600 (estimate)". `Hook[T]` has no signature or call-site list. It is not stated how it replaces the existing plain-field hooks.
   - **Codemod:** no tool or command is given. The plan assumes `scoping move --lower`, but it never shows the invocation or how it handles test files, Bazel `go_sources`/`data`, or the path-keyed guards.
   - **Guards:** the plan lists several path-keyed tests (§5) but gives no per-move table of which test lines change. W2 (guard prep) therefore has to be done before W1 moves a file, which the plan does not say.

7. **Open verification gaps that block waves:**
   - the 71-method dead set
   - the 1,568-LOC "pure" set
   - whether history compiles (the 3 undefined `Storage` symbols)
   - the `Host` surface

**What would make it executable:**
1. Add a per-file manifest (file, target package, LOC) summing to 71,219.
2. Reconcile the Store residual so every file has a package.
3. Resolve the two name collisions.
4. Give 1.1 and 1.3 an explicit type list.
5. State the codemod command.
6. Add a table of the guard edits per move.

Checkout used: `/home/tstapler/code/github.com/tstapler/stapler-squad.worktrees/bazel-epic0` at `ce726e73c`. I did not edit it.