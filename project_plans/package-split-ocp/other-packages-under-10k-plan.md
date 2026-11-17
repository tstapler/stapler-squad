# Plan: backlog, backlogsvc and server/services top level under 10k (2026-11-12)

Output of the `other-packages-under-10k` workflow (3 analysts + synthesizer; **no adversarial review was run on this one**). Owner decision 2026-11-12: every package under 10,000 non-test LOC. Companion to `session-hub-redesign-plan.md` (session tree). Nothing was compiled by the analysts; LOC bases differ (wc / non-blank / t9) and are stated in section 0. The synthesizer's mapping of the session-tree waves is marked UNVERIFIED in the plan itself.

---

# Plan: every package under 10,000 non-test LOC (backlog, backlogsvc, services top level)

## 0. Bases and caveats

- **LOC bases.** Part 1 measured session/backlog on three bases: 12,456 non-blank (go/types), 13,427 `wc -l`, and 13,802 in the `budget.txt` row. The 375-line gap between `wc` and the budget row is unexplained.
- **Part 2 and Part 3 basis.** Both use `wc -l` and say so.
- **t9 basis.** t9 reports services at 70,021, against `wc` 70,113. Part 3 attributes the 92 lines to six GOOS-tagged files. t9 has no per-package rows, so "under 10k" is proven on the `wc` and non-blank bases but **not** on a per-package t9 basis. Gate every wave on t9.
- **Session-tree plan.** I did not re-read the plan, so I cannot map its W0-W9, Track B and Track C onto the analysts' waves line by line. Where I name plan waves, the label is UNVERIFIED. The firm facts are the plan's PC-9 move, the W1-W3 lowering waves (Part 1), and the plan's figures of 1,974 (backlogcore) and 6,072 (backlogrepo) LOC.

## 1. Verdict per family

| Family | Today (wc) | Ends under 10k with headroom? | Largest result |
|---|---|---|---|
| session/backlog | 13,427 | Yes. After BL-1 to BL-4 the lifecycle residual is 7,406 non-blank / 7,932 wc. | Budget-basis scaling gives about 8.15k (derived). Still under 10k, but thin against the ~8k target. |
| backlogsvc | 15,327 | Yes. | RPC and hub 7,290; optional `workflowcfg` leaves about 5.2k. |
| server/services top level | 70,113 | Yes. | `approvals` 5,680; facade 2,916 now, about 4.2k after delegators (unverified arithmetic). |
| backlogrepo (plan) | plan says 6,072 | Yes. | Candidate set is 5.5-6.1k. The plan says stop at 9k and split item-sessions, verdicts and AC (~2.1k) if it grows. |
| backlogcore (plan) | plan says 1,974 | Yes, as a sibling package. | Candidate set is 1.3-2.0k. |
| gate, remediation (new) | 1,396 non-blank / 1,536 wc | Yes. | New `session/backlog/transition`. |

Every target is at most 7.4k except the table's own largest rows. Headroom is at least 2k on every package.

## 2. Target packages and ledgers

### 2a. session/backlog (ledger sums to 12,456 non-blank / 13,427 wc)

| Package | Files | Non-blank / wc |
|---|---|---|
| `backlog/stuck` (leaf) | stuck_decisions | 429 / 464 |
| `backlog/liveness` | liveness_definition, liveness_repository, ent_liveness_repository, liveness_cache, liveness_engine | 779 / 856 |
| `backlog/pipeline` | pipeline_engine, backlog_commands, backlog_context, backlog_attached_images, wake_prompt, backlog_review, backlog_triage | 2,282 / 2,514 |
| `backlog/reviewgate` | review_gate, gate_custom_check | 901 / 951 |
| `backlog/chains` | chain_firer, graph_validator | 659 / 710 |
| `session/backlog` (lifecycle residual) | 12 listener files | 7,406 / 7,932 |
| **Sum** | 29 files | **12,456 / 13,427** |

`shadowledger` (621 per the budget row) stays as is.

**Gaps.** The plan's backlogcore (1,974) and backlogrepo (6,072) file lists are not in the checkout.

| Candidate | Part 1 measured (non-blank / wc) | Plan | Unattributed |
|---|---|---|---|
| backlogrepo | 5,459 / 5,884 | 6,072 | about 190-610 |
| backlogcore | 1,335 / 1,501 | 1,974 | about 470-640 |

The session-tree owners must supply the file lists before PC-9 and backlogcore are scheduled.

The new gate package is `transition` (transition_gate, transition_callers, backlog_remediation), 1,396 / 1,536. The claim recorder is 110 / 123.

### 2b. backlogsvc (wc; package totals sum to 15,327)

| Package | LOC |
|---|---|
| `backlogsvc` (RPC and hub) | 7,290 |
| `spawn` (core, holds the shared `Core` deps) | 3,181 |
| `review` | 2,787 |
| `steer` | 964 |
| `dequeue` | 636 |
| `backlogwire` (leaf) | 469 |
| *optional* `workflowcfg`, carved from the RPC row | 2,096 (RPC then about 5.2k) |

**Disagreement within Part 2.** The per-file apportionment of the two split files does not sum to the file totals.
- `backlog_service.go`: 639 + 374 + 411 = 1,424, against 1,557.
- `triage.go`: 1,939 + 1,410 + 218 + 209 = 3,776, against 3,966.

That leaves 323 lines unattributed. Re-deriving each package from the rows gives about 7,238 / 3,043 / 2,716 / 953 / 625 / 429, a sum of 15,004, which is 323 short of 15,327. The package totals were "forced to sum" and are about ±1-4% per package. This does not threaten the 10k goal.

### 2c. server/services top level

The 35 packages plus the facade sum to 70,113. I re-added the table and it matches.

| Group | Packages (LOC) |
|---|---|
| Largest | approvals 5,680; fileops 4,155; sessionlife 3,499; quota 3,229; sessioncreate 3,194; rules 3,178; terminal 3,063; search 2,936 |
| Facade | stays in `server/services`, 2,916 |
| Mid | github 2,821; guidance 2,569; qareply 2,556; autonomous 2,551; notifyactions 2,076; webhooks 1,969; flags 1,898; diag 1,889; notify 1,823; settings 1,713 |
| Small | the remaining 18 packages, all at or under 1,404 |

The t9 adjustment is -42 from `gateaudit` and -50 from `fileops`.

**Caveat.** Several moves are symbol cuts inside files (capture_tap_service, workspace_service, notification_action_service, hidden_review_steer), so per-package LOC can shift by a few hundred.

## 3. Edges, cycles, and chosen breaks

**session/backlog (two cycles).**
- liveness to lifecycle: export the two staleness constants into `liveness`.
- reviewgate to lifecycle: define the two spawner interfaces in `reviewgate`, the consumer. The listener already satisfies them structurally.
- Also export `recordTerminalReviewVerdict` and `closingKeywordWithRepo`.
- The resulting DAG is `stuck ← pipeline ← {chains, reviewgate}`, `liveness ← reviewgate`, and all of them ← lifecycle.

**Gate and remediation (disagreement between Part 1 and Part 3, Part 1 wins).**
- The plan's PC-9 moves transitiongate and remediation into `server/services`.
- Part 1 verified that `session/backlog` lifecycle files, `session/jules/session_poller.go`, `session/pr_conflict_retry.go` and `session/diagnose_nudge.go` call them. A `session/*` package cannot import `server/services`, so the move inverts the dependency rule.
- Part 3 offered `backlogmisc` or `backlogsvc` as the destination. That is **rejected**.
- Decision: the gate and remediation go to a new package `session/backlog/transition`.
- The claim recorder can move to services, because no `session/backlog` file references it. I place it in `backlogmisc` (1,351 to about 1.47k). That placement is my choice, not an analyst's.
- Its `ClaimRecorder` port stays in `backlogrepo`, and `main.go` wires the implementation.

**backlogcore.** It is a sibling package that imports `instance` and `workflowengine`, and `driver` imports it. It cannot be absorbed by session/backlog, because 7,406 + about 2,000 is about 9.4k.

**Ordering constraint.** `backlogrepo` writes must not call the gate. The direction `transition → backlogrepo` holds, because rg found `s.gate` only in `transition_gate.go`.

**backlogsvc (four cycle families).** REVIEW↔SPAWN, DEQ↔SPAWN and STEER↔SPAWN are broken by moving 12 helpers first. Examples are `effectiveReworkCap`, `isCodeShippedToMain`, `IsTriageLive` and the PR-fix reopen group. The `wire` package must not import the RPC package. The layering is `backlogwire → spawn → {review, dequeue, steer} → backlogsvc`, and `steer` also calls `review`. `backlogsvc` imports `services` for about 58 symbols and `services` never imports `backlogsvc`, so no cycle exists.

**services (112 upward symbols).** They are cut by in-package pre-moves, which Part 3 says yields a DAG:
- `fsOps` into `svcbase`.
- Flag-name constants into `flags`.
- `AuditLine`/`AuditSink` into `gateaudit` (no `FlagAuditSink` inversion needed).
- `ActionResult` and related into `qareply`.
- `ActionHooks` and `ApprovalNotifier` as consumer-defined interfaces. These have a real adapter plus a fake, so they are genuine seams.

**Cross-part gap (unresolved).** `backlogsvc` depends on `services` symbols that Part 3 must relocate, but Part 3's ledger does not place them:
- `DerivePriority` (19 uses)
- `HeadlessReReviewUUIDPrefix`
- `SessionStopper` and `SessionSteerer`
- the claim types
- the `Steer*` constants
- the stuck `ReasonName*` values

Until these have homes, `backlogsvc` keeps importing the facade `services`, which is harmless (one-way) but leaves a retarget codemod later. Part 3's W0 must name each destination.

## 4. Ordered waves and interleave

PRs land serially, because `budget.txt` and `boundaries.txt` conflict. Cost classes are small, medium or large. Counts: Part 1 has 5 PRs, Part 2 has 5 plus an optional one, and Part 3 has 41, so about 51 required PRs in all.

| # | Wave | Content | Cost | Design status |
|---|---|---|---|---|
| A1 | BL-1 | `stuck`, `liveness` (staleness constants first) | small | done |
| A2 | BL-3, BL-4 | `reviewgate`, `chains` | small each | done |
| A3 | BL-2 | `pipeline` (two exports) | medium | done |
| A4 | SVC-0 | Part 3 W0 pre-moves (5 PRs, one per layer) | medium x 5 | needs design first for `ActionHooks`, `ApprovalNotifier` and the kernel ports |
| A5 | SVC-1 | L0 leaves (8 PRs) | small x 8 | done |
| B1 | BS-1 | `backlogwire` | small | done |
| B2 | BS-2 | `spawn` (about 60 exported symbols, guard rows) | large | done |
| B3 | BS-3..5 | `review`, `dequeue`, `steer` (each cuts `triage.go`, so serial after `spawn`) | medium-large, small-medium, medium | done |
| B4 | BS-6 | `workflowcfg` | medium | optional |
| C | SVC-2..3 | L1 and L2 (16 PRs) | mostly small, 3 medium | done |
| D | SVC-4..5 | L3-L5 (10 PRs) | medium x 5, large x 3, small x 2 | needs design first for the six kernel ports |
| E | SVC-6 | facade trim, cross-cluster tests, final t9 proof | medium x 2 | done |
| F | BL-5 | `transition` as a `Gate` type | large | needs design first |

**Interleave with the session-tree plan.**
- **Independent now.** A1-A3 (BL-1 to BL-4) and A5 do not depend on the plan's waves, except that their `session.*` imports (207 distinct symbols, with `EntRepository` x74 the heaviest) are retargeted by the plan's lowering waves W1-W3. Land them early to shrink what those waves must codemod.
- **Before the plan's `backlogrepo` and PC-9.** Land `BL-2`/`BL-3` first. Gate callers shrink in number by living in smaller packages.
- **After the plan's `backlogrepo` wave.** BL-5 follows it, because the `Gate` type holds a `backlogrepo` writer. This reorders PC-9: the plan's gate move becomes BL-5 and stays in the session layer. The remediation move merges into BL-5.
- **Callers of the gate that BL-5 must change.**
  - 11+ sites in `session/backlog`.
  - `session/jules/session_poller.go`.
  - 8 files in `backlogsvc`.
  - 2 in `server/mcp`.
  - 3 in `server/services`.
  - 7 in `session`.
  - To touch fewer packages, land BS-2 to BS-5 first. That is my judgement, not an analyst's.
- **Claim recorder.** The plan's claim-recorder move can run in any slot after SVC-2, once `backlogmisc` exists. The `ClaimRecorder` port stays in `backlogrepo`.
- **backlogcore.** It lands with the plan's own wave, as a sibling. No design change here.
- **Freeze.** Part 3 flags a `storage_backlog.go` freeze approval for PC-9. This is an owner blocker.

## 5. Per-move checklist

1. `git mv` plus a codemod, one cluster per PR, no lasting aliases, and `make gazelle` clean (never hand-edit BUILD files).
2. Add a `budget.txt` row for the new package and lower the old cap. Add a `boundaries.txt` row. Carry any raise in a `scoping-budget-raise:` commit line. Check go_sources and data in Bazel for every moved file.
3. Run `-tags sinkguard` tests and rewrite path-keyed rows:
   - `terminal_write_pins_test.go`: pins are keyed as `server/services:SessionService.X`, and `steerActiveSessionForPRFix` and `steerWorkSessionWithVerdict` move.
   - `workflow_bypass_inventory_test.go`: about 25 rows keyed by file plus `BacklogService.<method>`, from line 129 on.
   - `legacy_item_pr_readers_test.go`: rows at lines 38-42.
   - `session/wake_polling_guard_test.go`: row at line 25.
   - `feature_flag_service_test.go`: repo walk and allow-list (lines 664-666).
4. Run `make registry-generate` whenever an RPC or `// +api:` marker moves. Keep the handlers as thin one-line delegators registered at `server.go:858` and in the composition root.
5. Interfaces only where consumed and with two implementations or a genuine seam. This is exactly what makes `Core` a shared struct rather than a new interface.
6. Run t9 (`go run ./tools/bzl/scoping t9`) after each wave, not the ledger.

## 6. Risks, stop rules, open owner questions

**Risks.**
- **Tests.** backlogsvc has 72 test files (28,920 lines) and services has 360 (77,156 lines). 66 of them touch two or more clusters, and the largest is `session_service_test.go` at 4,449 lines. Unexported cross-cluster calls need `export_test.go` shims or must move. Not sized per file.
- **`spawn` exports.** About 60 unexported symbols become exported (review 27, dequeue 14, steer 10, RPC about 9, plus others). The risk is growth into a dumping ground.
- **Lifecycle headroom.** It is thin on the budget basis (about 8.15k). The next cut would be `backlog_lifecycle_pr.go` (2,416 lines, 35 symbol edges), behind a `prmonitor` host interface. Large, not recommended now.
- **`svcbase`** is a deliberate base package and could become a dumping ground.
- **Unverified.**
  - None of the analysts compiled a split, and no tests or builds ran.
  - Part 2 matched the `session/backlog` callers into `backlogsvc` by method name only.
  - The blast-radius count of 346 references is a heuristic.
  - The facade size of about 4.2k is arithmetic.

**Stop rules.**
- Stop and redesign if any new package passes 8k.
- Stop and redesign after more than two new blockers in one subsystem.
- Stop a wave if the t9 total does not move as the ledger predicts.
- Never run a wave while another PR holds the budget rows.

**Open owner questions.**
1. Which budget basis is authoritative (`wc`, non-blank, or t9 per package)?
2. Confirm the gate and remediation stay in `session/backlog/transition`, overriding PC-9 as written.
3. Approve the `ActionHooks`, `ApprovalNotifier` and kernel-port shapes, and the `FlagAuditSink` decision.
4. Is `svcbase` acceptable?
5. Do you want the optional `workflowcfg` package?
6. Approve the `storage_backlog.go` freeze.
7. Supply the plan's backlogcore and backlogrepo file lists.
8. Where do the `services` symbols that `backlogsvc` imports (`DerivePriority` and the others in section 3) live?