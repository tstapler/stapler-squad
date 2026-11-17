# Roadmap: top-level `session` from ~71k to under 10,000 LOC (2026-11-12)

Produced by the `session-under-10k-roadmap` workflow (3 analysts, 1 synthesizer, 2 adversarial reviewers). Owner requirement (2026-11-12): split the four held clusters (workflow engine, history adapters, `ent_*`, feature flags) and make the central `session` package under 10,000 LOC. The roadmap below is the synthesizer's output; the two reviews that follow **found defects in its sequencing** (D1-D8) and in two figures; they are part of this document and take precedence where they conflict. Nothing here was re-measured by the synthesizer; the reviewers spot-checked the arithmetic. Baseline measured on main `ce726e73c`: `session` = 71,111, `server/services` = 70,021 (`go run ./tools/bzl/scoping t9`).

---

## Synthesized roadmap

# Roadmap: `session` from ~71k to under 10,000 LOC

All LOC figures are non-test, from `go run ./tools/bzl/scoping t9` unless noted. Lens 1 is the only exhaustive partition, so the ledger uses its numbers. Lens 3 numbers are in brackets where they differ. Nothing here was re-measured by me.

## 1. Verdict

**Leaf moves alone cannot reach 10k.** The evidence is consistent across the lenses:
- Lens 1 peeled the file graph without any redesign and moved 22,851 LOC (32%). That leaves a tangled core of 48,368 LOC over 129 files.
- Lens 3 found the four held clusters total about 19.8k LOC and take `session` to about 51k. FLAGS-1 removes 0 LOC, because it lives in `server/services`.
- Lens 1's full partition of every other cluster still leaves 17,036 LOC. All of it is the `Instance` aggregate: 14,014 LOC in files with at least 50 LOC of `Instance` methods, plus 3,022 LOC of support files.

**Under 10k is reachable for the top-level `session` package without redesigning the hub types, if `Instance` is relocated whole.**
- Lens 2 Option 1 moves `Instance` and its snapshot, actor and setters to `session/instance`. This is a package move, not a method redesign.
- It must keep `InstanceSnapshot`, `Snapshot()` and the actor setters in the same package as `Instance`, per `instance-lock-free-reads.md`.
- `EntRepository` and `Storage` need no redesign beyond DTO lowering (ADR-011). Lens 2 and Lens 1 agree on the evidence: `ent_*` files reference no `Instance` symbol, and only about 15-18 symbols cross the boundary.
- Top-level `session` then holds just the `Storage` facade, at most 4,488 LOC.

**Relocation moves the size problem; it does not solve it.**
- `session/instance` would be 17,036 LOC.
- If the owner's 10k intent covers every package, `Instance` must be decomposed. That is a real redesign:
  - Lens 1's split is a 9,557 LOC core plus 7,479 LOC of capability files (tmux, claude, shells, PR set, and others).
  - The capability files would become controller structs with delegating shims.
  - The shims would need 400 LOC or less.
  - The core would land at about 9,950, with no headroom. `instance.go` (2,902) and `instance_actor_setters.go` (1,200) must also shrink.
- This is wave 7. It is conditional and needs design first.

**Who is right**
- Lens 2 said the 13-15k remainder was unverified. Lens 1's partition answers it: the remainder is the `Instance` family.
- Lens 3 is right that the held clusters are cheaper than assumed:
  - Workflow engine (WE): its closure touches `Storage`, `EntRepository` and `Instance` only through `transition_gate.go`.
  - History (HIST): the contract package is feasible with a `Session` port.
  - FLAGS-1: no mutable global is needed.
- Lens 1 and Lens 3 disagree on whether WE needs the store types lowered first. Lens 1 counts 76 symbols from WE into `store` and `backlog`; Lens 3 counts four back-pointers into `session`. They likely measure different edges. I treat WE as gated on lowering only the types it names, to be confirmed with `go list -deps` before the wave.
- Lens 1 and Lens 3 disagree on whether sub-stores should be peeled before or after the `EntRepository` move. I follow Lens 2: move whole first, then peel, because there are few importers (about 190 references in 8 packages). The peel is then a cheap move inside the new package.

**Cluster sizes: owner figures vs lenses**

| Cluster | Owner | Lens 1 | Lens 3 |
|---|---|---|---|
| Workflow engine | 4,371 LOC / 14 files | 5,823 / 21 | 4,491 / 20 |
| History | 3,061 / 11 | 4,122 / 17 | about 2.9k out |
| `ent_*` | 12,089 / 31 | 12,089 / 31 | matches |
| FLAGS-1 | up to 2,115 / 10 | n/a | 1,585 / 7 non-test, 0 effect on `session` |

## 2. Target end-state

Cluster rows use Lens 1 numbers. "Storage" is the facade kept in `session`.

| Package | Layer | LOC | Notes |
|---|---|---|---|
| `session` (top level) | facade (Storage) | at most 4,488 | `storage*.go`, 8 files |
| `session/domain` additions | domain model | 2,427 | 17 files; value types lowered |
| `session/entstore` core | repository / data mapper | about 7,421 | after peels below |
| `session/entstore/backlogstore` (or similar) | repository | 3,460 | peeled to keep the core under 10k |
| `session/workflowstore` | repository | 1,261 | 567+325+180+189 (Lens 3) |
| `session/<wake, webhook, guidance, flow, ...>` stores | repository | about 3,400 | Lens 3: 694+553+950+1,010 plus extras |
| `session/workflowengine` | service | 5,823 | [Lens 3: 4,491] |
| `session/convhistory` + `/adapters` | gateway / adapter | 4,122 | [Lens 3: about 3.2k]; adapters leaf imported only by the composition root |
| `session/tmuxproc` | infrastructure | 6,197 (6,089 on the t9 basis) | includes 108 LOC of `*_windows.go` that t9 excludes |
| `session/lifecycle` | service | 2,246 | |
| `session/reviewqueue` | service | 2,428 | |
| `session/prtrack` | service | 1,449 | |
| `session/backlog` (merge) | service | 1,974 | merge target unchecked |
| `session/driver` | service | 4,784 | |
| `session/claudeint` | adapter | 737 | |
| `session/gitwt` (merge) | adapter | 1,552 | merge target unchecked |
| `session/scroll` | infrastructure | 414 | |
| `session/instance` | aggregate | 17,036 | 24 method-heavy files (14,014) + 21 support files (3,022) |

**Ledger.** The core entstore row is 15,542 minus 1,261, 3,400 and 3,460.
- Rows: 4,488 + 2,427 + 15,542 (entstore family) + 5,823 + 4,122 + 6,197 + 2,246 + 2,428 + 1,449 + 1,974 + 4,784 + 737 + 1,552 + 414 + 17,036 = **71,219**.
- That is 71,111 (t9) plus 108 Windows LOC. It matches Lens 1's ledger.
- The 15,542 is the `ent_*` 12,089 plus 3,453 of other `EntRepository`-method and persistence files. Lens 3's "about 7.5k" for the core excluded those 3,453, so entstore alone would be about 10.9k. That is why the backlog peel is in the plan.

**Gaps**
- Tests were not partitioned. White-box tests must move with their code, and that cost is unmeasured.
- Whether sibling packages (`session/backlog`, `lifecycle`, `hibernation`, `flow`, `prpoller`) are valid merge targets was not checked.

## 3. Waves

Within a wave, PRs touch disjoint files and can run in parallel worktrees, but they land serially because `budget.txt` and `boundaries.txt` conflict. Cost classes: small, medium or large agent waves. "One pass" means one worktree agent can do it; "design first" means a design step precedes it. Session LOC is on the t9 basis, starting at 71,111.

**Wave 0: prep and independents (4 PRs, small)**
- Move: HIST registry injection (H0).
  - `defaultHistoryRegistry` becomes injected from `DefaultHistoryAdapters()` in `server/dependencies.go`. It fails loudly if empty.
  - One pass.
- Move: `transition_gate.go` split.
  - The pure part (about 250 LOC) moves to the WE leaf. The `Storage.Gated*` methods (about 280 LOC [Lens 1: 181]) stay.
  - Add a `StageRepoFactory` seam and move `PipelineStageExecutor` and `StageRole` (42 LOC).
  - One pass.
- Move: FLAGS-1 stage 1 into `server/services/featureflags`.
  - About 450 LOC. It has 4 non-test callers, including receiver-less `wsconn`, which imports the leaf.
  - One pass.
- Move: measure white-box test LOC per cluster.
- Session after: 71,111.

**Wave 1: value-type lowering to `session/domain` (large, serial PRs)**
- Moves:
  - `InstanceData`, `ClaudeSessionData`, `PiSessionData`, `ShellData`, `GitWorktreeData`.
  - The `PRRef` family.
  - `BacklogStatus`, `AcCriterion`, `BacklogItemChange`.
  - The `repository.go`, stage and gate DTOs.
  - `FlowMarker*`, `LoadOptions`, and the status enums.
- Seam: none; no new interface. No aliases.
- Importers: `GetEntClient()` appears in 78 files, and `session.EntRepository` in 29 non-test and 90 test files outside `session` (Lens 3). Per-type counts are unmeasured.
- Needs design first (move order and unexported-helper handling), then codemod PRs.
- Session after: about 68,684 (minus 2,427).

**Wave 2: leaves with no Instance edges (parallel, medium)**
- `workflowengine`, 4,491 to 5,823 LOC.
  - Importers: 40 files outside `session`; 13 non-test and 28 test files inside.
  - One pass after the Wave 0 gate split, and once the lowered types it names are in `domain`.
- `convhistory` + `adapters` (H1), with a `Session` port.
  - It needs `ClaudeSessionData` and `PiSessionData` lowered first. `isValidUUID` and the Pi event reader move too.
  - The port has two implementations, `*Instance` plus the adapters' fakes, so it is justified.
  - Importers: 16 files outside `session`; 7 non-test and 18 test files inside.
  - Design first for the `Session` port. Setters must go through the actor path.
- FLAGS-1 stage 2 (`FeatureFlagService` and related) with a consumer-side `FlagAuditSink`. Zero effect on `session`. One pass.
- Session after: 58,739.

**Wave 3: infrastructure and adapters with back-edges (parallel, medium)**
- `tmuxproc`: 117 symbols in from `session`, 18 back out.
- `claudeint`: 9 symbols in, 4 back.
- `gitwt` remainder: 9 in, 6 back.
- `scroll`: 18 in, 4 back.
- Seam: consumer-side ports for the back-edges.
- Each needs design first for its back-edges. After that it is one pass.
- Session after: 49,947 (minus 8,792).

**Wave 4: `entstore` (large)**
- 4a: move the whole `EntRepository` family, 15,542 LOC, to `session/entstore`.
  - That includes the `EntRepository` methods in `storage_backlog.go`.
  - Importers: about 190 references in 8 packages, plus 63 straddling test files.
  - Needs design first: the ownership of `ent_client_instrumented.go` and `slow_op_watchdog.go`, and migration registration at the composition root.
  - `Storage.Repo` becomes `*entstore.EntRepository`.
- 4b, parallel after 4a: peel `workflowstore`, the backlog store, wake, webhook, guidance and flow stores. Each is one pass and file-disjoint.
- `Storage` stays in `session` as the facade; this is the Lens 3 position, not Lens 2's split.
- Session after: 34,405.

**Wave 5: service-up satellites (parallel, medium to large)**
- `lifecycle`, `reviewqueue`, `prtrack`, `backlog`, `driver`.
- Residual-to-cluster edges that must be inverted by injection: lifecycle 11, reviewqueue 4, prtrack 6, backlog 2, driver 17.
- Unexported symbols to export or hide behind interfaces: 36, 33 and 73 for lifecycle, reviewqueue and driver. Counts for the others were not reported.
- Wired at `server/dependencies.go`.
- Lifecycle, reviewqueue and driver are the three that need design first.
- Session after: 21,524, which is `Instance` 17,036 plus `Storage` 4,488.

**Wave 6: relocate `Instance` to `session/instance` (large, design first)**
- Seam: `instance` must not import `session`. Lens 2's candidate blockers: `HistoryAdapterRegistry`, `ClaudeSessionManager`, `Checkpoint`, `WorktreeEnricher`, `ReviewQueue`/`ReviewItem`, `Notifier`, `BounceMainBranch`, `StaleResumeDetector`, `repo_path.go`, `ApprovalMetadataProvider` and `RepairCorruptedGitRepo`. None was checked one by one.
- Importers: about 1,100 references in 22 packages, from Lens 2. The six biggest packages sum to 986.
- Also confirm the raw-field reads in `session_driver.go` (20 fields) and `session.go` (33 fields) are all in `InstanceSnapshot`.
- Session after: at most 4,488.

**Wave 7: `Instance` decomposition (conditional on owner question 1)**
- It targets `session/instance` at or under 10k.
- Needs design first; it is a multi-wave epic.

## 4. Checklist every move must pass

- `go run ./tools/bzl/scoping t9` for the size cap (T9d). Add a `scoping-budget-raise:` commit line for any budget raise.
- Update `tools/bzl/scoping/budget.txt` and `boundaries.txt`.
- `make gazelle` must leave a clean diff, and `make gazelle-check` must pass. Do not hand-edit `BUILD.bazel`. Hand-written files need a row in `tools/bzl/hand-build-files.txt`.
- Update `go_sources` filegroups and Bazel `data` and visibility.
- Run the sinkguard classification for any new or moved RPC handler.
- Update path-keyed guards:
  - `server/services/terminal_write_pins_test.go`
  - `server/services/legacy_item_pr_readers_test.go`
  - `server/services/workflow_bypass_inventory_test.go`, `workflow_bypass_strict_test.go` and `configured_workflow_engine_test.go`
  - `session/legacy_item_pr_readers_test.go`, `legacy_pr_readers_test.go` and `pr_state_writer_guard_test.go` (Lens 3)
- Update the `noarchivedrevival` file set (`tools/lint/noarchivedrevival`) when moving `health`, `review_queue_poller`, `session_driver` and the other listed files. Update the `instance*.go` glob in `.claude/rules/instance-lock-free-reads.md` in Wave 6. Run `make actor-field-guard`.
- Update the depguard rules (`.golangci.yml`, the `session/ent` allowlist) and the `no_ent_in_services` glob.
- Run the golangci gate:
  - `dupl` and `gocognit` apply to modified existing functions too.
  - Run `make ready-complexity-gate`.
- Run `make registry-generate` and commit the changed `docs/registry/features/` files when `+api`/`+feature` markers move.
- Move white-box tests with their code. Use a `git mv` plus codemod, with no lasting aliases, one cluster per PR.
- In a fresh worktree, run `make build` first, since `gen/` and `session/ent/*` are gitignored. Do not commit generated ent output.

## 5. Risks and stop rules

- **Test LOC is unmeasured.** Wave 0 measures it. If tests double a wave's cost class, re-plan that wave.
- **Back-edge cycles.** Service-up packages call back into core. Stop a move if breaking a cycle needs more than a consumer-side port or an export. A fix that grows an interface beyond its consumer's real method set is a redesign; escalate it instead.
- **Review-repair loops.** Repeated new blockers in one cluster mean simplify or defer, not patch. Stop after two repair rounds on a move.
- **Lowering codemod collisions.** Wave 1 PRs conflict textually, so they stay serial. Do not parallelize them.
- **Wave 6 cycle check fails.** If more than a handful of Lens 2's blocker types cannot move down or sit behind a consumer-side interface, stop and reconsider before the 1,100-reference codemod.
- **`entstore` over 10k.** Peel the backlog store (3,460) before declaring Wave 4 done.
- **Startup wiring.** Atomic-pointer fields on `Storage` (gate, claim recorder, foreign-claim lookup) are set once at startup. Verify the order in `server/dependencies.go` before moving them. This was unverified.
- **Live service.** Test with the manual-instance recipe, never `make install-service` mid-move. Cap concurrent worktree agents so scratch and Bazel JVMs do not exhaust RAM tmpfs.

## 6. Open owner questions

1. Does the 10,000 cap apply to every package, or only to top-level `session`? If every package, Wave 7 is required and `session/instance` (17,036) is not done at Wave 6.
2. Should `Storage` (at most 4,488) stay as the `session` facade, as Lens 3 proposes? Lens 2 would split it by aggregate, which touches 800+ external uses. Staying leaves 5.5k LOC of headroom.
3. Should `entstore` ship as one large package (15.5k) before being peeled, or must no intermediate package exceed 10k? This sets whether Wave 4a lands as-is or is split first.
4. Are `session/backlog` and `session/gitwt` valid merge targets, or should the satellites get new packages? Their import direction was not checked against the existing siblings.
5. Do test files count toward the cap, or does t9's non-test basis stand?
---


## Review 1: arithmetic and evidence

The roadmap's arithmetic holds, but the partition has one double-count and several figures I could not check. I ran everything in the read-only checkout (`.../stapler-squad.worktrees/bazel-epic0`).

**Verified**
- `go run ./tools/bzl/scoping t9` reports `T9d: services=70021 session=71111`. That matches the 71,111 baseline.
- The ledger rows sum to 71,219. That is 71,111 plus 108, and the 108 is correct: the three `session/*_windows.go` files total 108 lines (`cat session/*_windows.go | wc -l`).
- `ent_*` non-test is 31 files and 12,089 lines (`cat $(ls session/ent_*.go | grep -v _test) | wc -l`). The roadmap's 12,089 / 31 is right, and 12,089 + 3,453 = 15,542.
- `storage*.go` non-test is 8 files and 4,488 lines. Both numbers are right.
- The 17,036 `Instance` row is 14,014 + 3,022, and 9,557 + 7,479 also equals 17,036.
- 71,219 − 22,851 = 48,368. 22,851 is 32.1% of 71,111.
- The per-wave "session after" figures recompute from the stated deltas:

  | Wave | Session LOC after | Delta |
  |---|---|---|
  | 1 | 68,684 | −2,427 |
  | 2 | 58,739 | −(5,823 + 4,122) |
  | 3 | 49,947 | −8,792 (= 6,089 + 737 + 1,552 + 414) |
  | 4 | 34,405 | −15,542 |
  | 5 | 21,524 | −12,881 (= 2,246 + 2,428 + 1,449 + 1,974 + 4,784) |

  The Wave 5 result equals 17,036 + 4,488, and 15,542 − 1,261 − 3,400 − 3,460 = 7,421.
- `transition_gate.go` is 584 lines. A roughly 250 pure part plus a roughly 280 `Storage.Gated*` part is plausible, though that sum is only about 530 and I did not check the split itself.
- `ent_client_instrumented.go` is 58 lines and `instance.go` is 2,902, as stated. `instance_actor_setters.go` is 1,200 and `storage_backlog.go` is 1,617.

**Wrong or inconsistent**
1. **Double-count of `storage_backlog.go`.**
   - The 8 `storage*.go` files (4,488) include `storage_backlog.go`, 1,617 lines.
   - Wave 4a says the `EntRepository` methods in that file move to `entstore`, and the 15,542 is described as including them.
   - Both cannot hold. The ledger sums exactly to t9, so the partition is file-exact and each file is counted once.
   - So either "Storage ≤ 4,488" is overstated, or the 3,453 "other `EntRepository` files" excludes `storage_backlog.go` and Wave 4a is mis-described. The roadmap needs a per-file assignment for that file.
   - Under 10k still holds either way, because the Storage row can only shrink.
2. **Wave 2 uses the upper bound for the workflow engine (WE).**
   - The 58,739 uses WE = 5,823 and history = 4,122. The prose gives WE as 4,491–5,823 and history as about 3.2k (Lens 3).
   - On Lens 3 numbers the session figure after Wave 2 would be about 1,100–2,000 higher (68,684 − 4,491 − 3,200 = 60,993).
   - Later waves are pinned to the Lens 1 ledger, so the end state is unaffected. The mid-wave figures are not "exact".
3. **The workflow engine size is unreconciled.**
   - `cat $(ls session/workflow*.go | grep -v _test) | wc -l` gives only 3,061 lines over 18 files.
   - The owner's 4,371 / 14 files, Lens 1's 5,823 / 21 and Lens 3's 4,491 / 20 were not reproduced.
   - The roadmap says "Nothing here was re-measured by me", so the three figures are unsupported and the gap between them (1,300+ LOC) is unexplained.
   - The 3,061 here is a coincidence with the owner's history figure, not a cross-check.
4. **The "≤ 10k" claim depends on every wave.**
   - Top-level `session` ≤ 4,488 is arithmetically sound.
   - It is reached only after Wave 6, where `Instance` (17,036) becomes `session/instance`. That is not under 10k unless Wave 7 happens.
   - The roadmap says this, but the Wave 7 numbers are tight. The core of 9,557 plus shims of 400 or less is about 9,950, with `instance.go` and the actor setters (4,102 together) still to shrink.
5. **`instance*.go` totals 14,567 non-test (my count).** The roadmap's 24 method-heavy files (14,014) plus 21 support files (3,022) are not all `instance*.go` files, so those totals cannot be compared directly. I did not reproduce them.

**Unsupported (not checkable from the roadmap alone)**
- Every symbol and edge count: 117 in / 18 back for `tmuxproc`, 76 versus 4 for WE, about 190 references in 8 packages, about 1,100 references in 22 packages, `GetEntClient()` in 78 files.
- All Lens 3 per-store figures. Their sum (694 + 553 + 950 + 1,010) is 3,207, against the roadmap's "about 3,400 plus extras", so roughly 200 LOC are unexplained.
- All cluster LOC figures other than `ent_*` and `storage*` (history, tmuxproc, lifecycle, reviewqueue, prtrack, driver and the rest).
---


## Review 2: architecture and sequencing (verdict: does not hold as sequenced)

**Verdict: the roadmap does not hold as sequenced. Waves 2, 3 and 5 depend on Wave 6 (the `Instance` move) and on Wave 4 (the `entstore` move), so they cannot land first.**

Method: I used the go/types file-edge tool in `~/.cache/tmp/an/` (`sess_now.json`, `final.json`, non-test files only). I also moved files into a scratch copy at `~/.cache/tmp/rv/cp` and ran `go build -gcflags=-e` on each moved set. The edge counts are by symbol name and may be slightly inflated; the build errors are exact.

**D1. Wave 2 "leaves with no Instance edges" is false for history and scroll.**
- History scratch build (14 files: `history*`, the `*_adapter.go` files, `registry.go`, `canonical.go`, `claude_scroll_adapter.go`). Undefined symbols:
  - `Instance` 22 times and `LiveInstance` 14 times.
  - `Storage` 3, `NewLiveInstance`, `ClaudeSessionData` 6 and `PiSessionData`.
  - `isValidUUID`, `ErrInstanceDataNotFound`, `NewInhibitionEngine` (review queue) and `NewClaudeGestureForwardStrategy`.
  - The scroll types.
- `NewLiveInstance` constructs an `Instance`. A `Session` port covering this is a larger seam than "a port with two implementations". `LiveInstance` and `Storage` are not in Lens 2's Wave 6 blocker list.
- go/types edges from history: 13 symbols into Instance methods, 6 into Instance state files, 11 into store, 2 into review queue.
- **Fix:** add the `LiveInstance` and `Storage` seams to the design-first step. Alternatively, put history after Wave 6.

**D2. History and scroll are mutually dependent, but the roadmap puts them in different waves.**
- Scratch build of the scroll set (`scroll_*`, `gesture_forward_strategy.go`) fails with undefined `Instance`, `isIdleStatus` and `NewClaudeScrollAdapter`. The last of these lives in `claude_scroll_adapter.go`, which belongs to history.
- Edges: scroll to history 6 symbols, history to scroll 2. Scroll to Instance methods is 4 symbols.
- **Fix:** move scroll and history in one PR, or keep `claude_scroll_adapter.go` with scroll and give `ScrollAdapter` a consumer-side seam.

**D3. Workflow engine (WE) is not a clean leaf, and it cycles with `backlog` and `driver`.**
- Scratch build of 12 non-test WE files (`workflow_engine*`, `configured_workflow_engine`, `stage_config_cache`, `workflow_system_edges`, `workflow_slug`, `workflow_stage_seed`): the only undefined symbols are domain-lowerable types (`BacklogStatus` 83 times, `GateStatus`, `BacklogItemTransitionInput`, `GateKind*`) plus the repository interfaces.
- It also names concretes that need wiring:
  - `NewEntStageConfigRepository` is an ent constructor, which a leaf cannot import. It needs a factory injected at the composition root (`server/dependencies.go`).
  - `validTransitions`, `transitionCallers`, `evaluateStructuralCheck` and `ParseGateConfig` have to travel with the Wave 0 `transition_gate` split.
- Cluster edges: WE uses `backlog` 20 symbols, `driver` 18, store 47. The reverse is `backlog` to WE 13, store to WE 49.
- Wave 2 puts WE ahead of `backlog` and `driver` (Wave 5) and `entstore` (Wave 4). Those later packages would have to import WE, and WE their symbols.
- **Fix:** list the exact lowered types as a hard prerequisite of Wave 1. Also schedule `backlog`'s 20 inbound symbols for inversion before the WE move.

**D4. Wave 3 and Wave 5 point the wrong way against `Instance`.**
- Placing these packages before `Instance` moves forces large ports. The OUT counts to Instance files (methods + state), from the go/types edge map:
  - `driver`: 107.
  - `review queue`: 58.
  - `lifecycle`: 50.
  - `tmux`: 39.
  - `pr tracking`: 23.
- The roadmap says tmux has "18 back" edges. The measured figure is 39 into Instance plus 20 into review queue.
- `review queue` to tmux is 6 symbols, so tmux and review queue form a cycle.
- Inbound edges are heavy too: Instance methods use tmux 135 symbols, and `driver` and `lifecycle` also call into it.
- A port covering 86 Instance methods contradicts the stop rule in section 5 ("an interface beyond its consumer's real method set is a redesign").
- The Wave 6 blocker list (`ReviewQueue`, `WorktreeEnricher`, `Notifier`, `StaleResumeDetector`) names Wave 5 packages. So Wave 6 needs them below `Instance`, while Wave 5 needs `Instance` below them.
- **Fix:** run a single cycle-breaking design first, deciding which side owns each of these edges. Moving `Instance` before the satellites would reverse some of the 86-method ports into natural downward imports. This is an owner-visible reordering.

**D5. Wave 4b "cheap peel" is not cheap.**
- Go methods cannot live in another package from their receiver. Peeling the backlog, wake, webhook, guidance and flow stores off `EntRepository` needs new store types, new constructors and rewritten call sites. The roadmap counts about 190 references for the whole move.
- The peeled files also share the `EntRepository` struct and `ent_repository.go`, so "file-disjoint" is false for 4b.
- The `EntRepository` methods in `storage_backlog.go` make the 15,542 LOC figure contingent on the same rewrite.
- **Fix:** treat the peel as a Storage/EntRepository decomposition (new hub-type design), not a move. Also state that `Storage` (in `session`) imports `entstore`, so `entstore` must never import back.

**D6. Wave 1 is a hidden precondition and needs ordering detail.**
- Waves 2 and 3 compile only after the domain lowering (D3 shows 83 `BacklogStatus` references). The `ClaudeSessionData` and `PiSessionData` types are needed by history (D1).
- The "serial" PRs cover 2,427 LOC but touch every cluster's files. They also conflict with Wave 0's `transition_gate.go` edit. Wave 0 PR 2 and Wave 1 are therefore not file-disjoint.
- **Fix:** put the `transition_gate` split after the first lowering PR, or merge them.

**D7. Rule and composition-root risks.**
- `instance-lock-free-reads.md`: moving `driver` (`session_driver.go`, 20 raw fields) and `session.go` (33 raw fields) before `Instance` means cross-package reads of exported fields (`Path`, `Branch` and similar). These bypass `Snapshot()`, and this is not just something to "confirm" in Wave 6. It is a pre-Wave-5 check.
- The `noarchivedrevival` file list names `session/*.go` paths; moves to subpackages break its path-keyed matching. This item is already in the checklist but is not scheduled per wave.
- A `Session` port plus injected factories add composition-root wiring in `server/dependencies.go` for WE, history, `lifecycle` and `driver`. The startup order of the atomic-pointer fields is flagged as unverified; make it a Wave 0 task.

**D8. Owner decisions the roadmap does not list.**
- Whether `Storage` may import `entstore` concretely, or must hold a consumer-side interface.
- The `session.Instance` to `instance.Instance` rename and stutter across about 1,100 references, with no alias allowed.
- Whether relocating `Instance` meets the "can't see why it would need to be bigger" intent, given that `session/instance` stays at 17,036 LOC. Question 1 touches this but does not state the consequence.
- Whether wave order may change so `Instance` moves before the satellites (D4).

**Arithmetic.** The wave totals recompute correctly (68,684 → 58,739 → 49,947 → 34,405 → 21,524 → 4,488). Wave 2 assumes the lower WE figure while the ledger uses 5,823, so the post-Wave-2 number is inconsistent with the ledger.

Scratch copies are under `~/.cache/tmp/rv/cp`. I changed nothing in the checkout.