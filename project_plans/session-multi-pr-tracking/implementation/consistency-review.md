# Cross-artifact consistency review: session-multi-pr-tracking

Phase 4 consistency pass over `requirements.md`, `implementation/plan.md` (1,712 lines; L1-1427 read, the iteration repair logs L1430-1712 only grepped), `design/ux.md`, `decisions/ADR-001..004`. plan.md was not edited. Line numbers below are plan.md unless prefixed.

Counts recomputed by command: 101 `##### Task` headings = 101 rows in the dependency table (diff empty), 30 stories, 11 epics; per-story size mix matches the effort table except Story 1.4 (N2); UX acceptance criteria 58 (S1 5, S2 6, S3 7, S4 4, S5 4, S6 2, S7 8, S8 8, S9 2, S10 5, X 7) match ux.md's summary.

Method: VERIFIED = file or code opened in this pass; INFERRED = reasoned from plan text only.

## Summary

| Severity | Count |
|---|---|
| BLOCKER | 4 |
| CONCERN | 11 |
| NITPICK | 14 |

Coverage result: every In Scope bullet and every Success Metric in requirements.md maps to a story (traceability table, L1253-1261, plus Epics 7-10 for artifacts). The four "Accepted as written" decisions map to Known Risks K1-K4. No requirement is without a story. Plan tasks with weak or no requirement basis are C10, N13 and (benign) T1.3.5, T5.3.0, T5.4.5, which ADR-001/003 justify.

---

## BLOCKERS

### B1. `RunPRStartupMaintenance` in `BuildCoreDepsWithOptions` also runs in the MCP stdio fallback (architecture concern a)
Conflict: plan, ADR-001, Story 1.5 ordering AC vs the code. VERIFIED.
- plan L83, L153, L156 ("Only the service can abort this way"), L420 (T1.5.1), L589, L619, L1692 and ADR-001 L18 and L33 all say maintenance runs "from service bootstrap (`BuildCoreDeps`)" and "never" from hook/CLI processes. Story 1.5 AC L409 asserts the same.
- `main.go:1870-1871` `buildMCPDeps()` calls `server.BuildCoreDeps()`, which is `BuildCoreDepsWithOptions(BuildOptions{})` (`server/dependencies.go:411-452`). `main.go:141-157` shows it is the fallback for the stdio MCP subprocess that is "spawned per Claude Code subagent" when the HTTP server is unreachable. `server/dependencies.go:275` (`BuildDependencies`) and tests (`server/delivery_gate_startup_test.go:48`, `dependencies_workflow_engine_test.go:29`) also call it; the service uses `main.go:384`.
- Consequences: a short-lived MCP process (a) runs the per-boot candidate queries and import, (b) can abort MCP init (`main.go:158`) on SQLite busy, the exact hook failure the plan removed, (c) runs `ResetVerifyCounters` while a service refresher may be writing (breaks the I1 exception's "no concurrent writer" premise, L83), (d) repeats per subagent.
Fix: do not put the call inside `BuildCoreDepsWithOptions`. Add `BuildOptions.RunPRMaintenance bool` (default false) set true only at `main.go:384` and in `BuildDependencies`, or call `Storage.RunPRStartupMaintenance` directly from the `core-deps` phase body at `main.go:384` and from `BuildDependencies`. Reword L83, L153, L156, L420, L589, L619, L1692 and ADR-001 L18/L33 from "`BuildCoreDeps`" to "the service bootstrap call sites". Extend the Story 1.5 AC at L409 with: `buildMCPDeps` (and `ssq-hooks`, CLI) leave counters and links untouched while the lock is held. Update the Known Risk K2 text accordingly.

### B2. With the flag off, the artifact scanner still seeds a `SessionPR`, contradicting ADR-004 and the owner decision
Conflict: requirements.md L92, ADR-004 L9/L16 vs plan T2.2.3 (L497-499), Story 2.2 (L465-468), T2.1.2 (L454). Partly INFERRED (setter behavior from plan text; the call site is VERIFIED at `server/dependencies.go:1874-1888`).
- Owner decision: `session_artifacts.PRURLs` is "display-only and does not seed the relation"; ADR-004 rejects seeding because "phantom PRs get polled and can wedge `pr_pending`".
- T2.2.3 skips the seed only "when `config.SessionMultiPR("reader:poller")` is on"; Story 2.2's AC says "flag on". Today's code calls `storage.UpdateInstancePRNumber` and `inst.SetGitHubPRNumber(ref.PRNumber)`; the plan makes `SetGitHubPRNumber` a thin wrapper over `AttachPR` (L479, L454) and says "with the flag off the writers still go through `AttachPR`, so links accumulate too" (L465). So flag off, a mentioned-only PR becomes a link, and the flag-independent refresher (T3.2.5) and the unflagged gate and sweeper then treat it as a real, polled, cleanup-holding PR. That is the phantom-PR scenario ADR-004 rejected.
- Also ambiguous: "flag on" (ADR-004, Story 2.2) vs scope `reader:poller` (T2.2.3).
Fix: make the skip unconditional (delete the seed block at `dependencies.go:1874-1888`, and drop `UpdateInstancePRNumber` there), update Story 2.2's AC to "regardless of the flag", change T2.2.3 and ADR-004 wording to unconditional, and add a flag-off test asserting no `SessionPR` row is created from `OnScanComplete`. If flag-off legacy behavior must be kept, give it a legacy-scalar-only setter that never calls `AttachPR` and say so in the Glossary ("Legacy mirror").

### B3. Wave-0 ACs name RPCs that exist only in wave 2 (architecture concern b, wave-0 part)
Conflict: wave assignment (L222-267, table L1337, L1357-1378) vs ACs. VERIFIED by reading the ACs and table.
- Story 1.3, Tombstone AC (L333): "a user `AttachSessionPullRequest` clears the tombstone and creates it". The RPC is T4.2.3 (L709, wave 2); wave 0 reaches only the repository (L331 says so for the case-insensitivity AC).
- Story 1.5, AC "Item candidate never re-creates ..." (L412): "`DetachSessionPullRequest(session, #30)` ... When the backfill runs". The RPC is T4.2.3 (wave 2); T1.5.1 is wave 0 and cannot assert it. Other Story 1.5 ACs that need later code carry `[wave 1]`/`[wave 2]` tags (L389); this one does not.
Fix: in L333 and L412 name the repository operations that exist in wave 0 (`DeleteSessionPR` session variant with its tombstone write, plus a `ClearTombstone(owner, ref)` method added to T1.3.2's method list, which today has no named clear operation). Move the RPC-level assertions to T4.2.3's ACs (Story 4.2 L695-698 already has the detach case). Alternatively tag them `[wave 2, T4.2.3]` as L389 does for other ACs.

### B4. Story 1.5 ordering AC names the state refresher, a wave-2 component (architecture concern c)
Conflict: wave placement. VERIFIED.
- L409: ordering AC requires `RunPRStartupMaintenance` to run "before the poller, refresher, sweeper and cleanup ticker are constructed". The refresher is T3.2.5 (wave 2, L1364); T1.5.1 (wave 0) cannot construct or assert it. T1.5.1's text (L420) repeats "before the refresher exists".
- The poller, sweeper and cleanup ticker are existing code built in `BuildServiceDeps`, so only the refresher is the problem.
Fix: restate the wave-0 AC over what exists (poller, sweeper, cleanup ticker; recorded call order in the bootstrap sequence). Add the refresher clause to T3.2.5's tests as a wave-2 AC ("the refresher is constructed after `RunPRStartupMaintenance`, so it never sees pre-reset counters"). Tag the moved clause `[wave 2, T3.2.5]` in Story 1.5.

---

## CONCERNS

### C1. Story 5.1 and Story 5.4 ACs use RPCs without prerequisite edges; Story 5.1 is circular with T4.2.7 (concern b, wave-2 part)
- Story 5.1 ACs L786 (`DetachSessionPullRequest{s1,#30}`), L790 (`DetachItemPullRequest`), L791 (`DetachSessionPullRequest`) are owned by T5.1.2/T5.1.3. Their table prerequisites (L1382-1383) omit T4.2.3 and T4.2.7, and T4.2.7 itself depends on T5.1.2 (L1377), so T5.1.x cannot assert them.
- Story 5.4 AC L916 uses `DetachSessionPullRequest{s1,#30}`; T5.4.1/T5.4.2 (L1392-1393) omit T4.2.3.
Fix: in Story 5.1 and 5.4 ACs use the repository delete-with-tombstone operations (T1.3.2); keep the RPC-level versions in T4.2.3/T4.2.7 tests (T4.2.7's text already says it asserts verdict and gate). If the RPC must be used, add T4.2.3 to T5.4.1 and split Story 5.1's detach ACs into T4.2.7.

### C2. Missing edge T3.2.5 -> T3.2.2 (concern d)
T3.2.5 (L618) "reports newly terminal states through the same `applyPRUpdate` return value and T5.4.3 call site"; the return value is defined by T3.2.2 (L609). Table row L1364 lists T2.3.4, T3.2.1, T3.2.4 only. Fix: add T3.2.2 to T3.2.5's prerequisites in L1364 (no cycle: T3.2.2 depends on T3.2.1, T2.3.3, T1.3.4).

### C3. Missing edges T5.3.1 -> T4.3.0 and T5.4.1 (concern d)
Story 5.3 AC L851 asserts that "the gate and sweeper step 4 treat the item as holding an open #22". The gate is T5.4.1, sweeper step 4 is T4.3.1, and both read `ListItemGuardViewsByItemIDs` from T4.3.0 (L747). Table row L1388 lists T1.3.2, T5.2.3, T5.3.0 only. Fix: add T4.3.0 and T5.4.1 (and T4.3.1 if the sweeper clause stays in this AC) to T5.3.1, or move the gate/sweeper clause of L851 into T5.4.1/T4.3.1. Cycle check: T5.4.1 depends on T1.3.2, T3.2.1, T3.2.5, T4.3.0 and not on T5.3.1, so no cycle.

### C4. Missing edge T2.1.1 -> T1.2.2 (concern d)
T2.1.1 (L450) makes `buildSnapshot` compute the legacy scalars "from `ActivePR`"; `ActivePR` is T1.2.2 (L311). Table row L1345 lists T1.2.1 only; T2.1.2 carries T1.2.2 (L1346). Fix: add T1.2.2 to T2.1.1.

### C5. "Test first (red)" in T4.3.1 and T5.4.1 without the skip convention T1.3.4 uses (concern e)
- T4.3.1 (L751, L755) and T5.4.1 (L922) write failing tests first. T1.3.4 (L358) commits its red tests with `t.Skip("red until T...")` so later `go test ./session` runs stay green.
- T5.4.1's per-path tests drive the real archive RPC, transition, 60s tick, reopen and tombstone sweep, which only pass after T5.4.2 wires the gate (L925); T5.4.1 and T5.4.2 are separate tasks, so T5.4.1 lands red. T4.3.1's red tests and implementation are one task, so the issue there is only commit ordering.
Fix: T5.4.1: commit the per-path tests with `t.Skip("red until T5.4.2")`, record the red output in the commit message, and have T5.4.2 remove the skips (as T3.2.2 does for T1.3.4, L609). T4.3.1: say "red commit not pushed alone; squash with the implementation, or use `t.Skip` as T1.3.4 does". Note the same T1.3.4 sentence in the Epic 1 ordering note (L276).

### C6. ADR-002 title still says "flag-gated projection" (concern f)
`decisions/ADR-002-...md` L1 title: "legacy fields are a flag-gated projection". Its own body (L11) and plan L52, L90, L168, L185 say the projection is NOT gated (flag gates readers only). Fix: retitle to "Active PR is derived (with a user pin); legacy fields are an always-on projection". Also update the ADR file name if it is referenced as a link target (plan L6 links the current name; keep the filename, change only the title and any index text).

### C7. requirements.md Risk Control contradicts the plan's flag semantics
requirements.md L87: "off keeps legacy single-PR behavior and the new relation is written but not read". The plan: flag off still (a) polls and updates `PullRequestState` rows through the always-on refresher (T3.2.5, L616-622), (b) makes the retention sweeper and cleanup gate read link rows (L185), and (c) projects legacy scalars from `ActivePR`, which differs from last-writer-wins when two PRs are open (Story 2.2 L465, "deliberate and pinned"). ADR-002 documents (c) but requirements.md is unchanged. Fix: edit requirements Risk Control to the three-part statement (readers gated; rows refreshed and cleanup-safety readers always on; legacy scalars always the `ActivePR` projection) or record it as an owner-accepted deviation; add it to "Needs the owner" if not yet confirmed.

### C8. UX S2 per-row error states cannot be driven by the persisted fields the plan defines
ux.md L155 and Gap 1 (L513) need row states for auth, rate limit, not-found and network. Plan T4.1.1 (L675) exposes `poll_error` as a free "short string" (ux asks for an enum `none|auth|rate_limited|not_found|network`), and T3.2.4/Story 3.2 (L589-591, L613) specify that non-counted failures (5xx, timeouts, network, host auth, rate-limit streak breaks) "write nothing" to the row and live in in-memory per-state backoff. So a network error or host-level auth failure is invisible to the UI, and `repo_inaccessible` (K5) has no UX copy. Fix: pick one: (1) define `poll_error` as an enum in T4.1.1 with `auth`, `rate_limited`, `not_found`, `repo_inaccessible`, `network`, and carry a non-persisted last-error class from the poller into `PRView` (in-memory, not a column); or (2) trim ux.md S2 to the persisted cases (rate limit via `rate_limit_reset_at`, counted failures via `poll_error`) and drop "network". Add the `repo_inaccessible` message to ux.md either way.

### C9. ADR-003 is "Accepted (owner decision)" but several of its rules are still open owner questions in the plan
ADR-003 L3 and L11/L13 state as decided: attach to a `done` item allowed, superseded-round PRs outside the verdict, MCP/user destroy and force-reset ungated. The plan lists the same items as unconfirmed: Unresolved Questions L213 (archival ungated), L214 (attach to done), L215 (agent-callable destroy), L218-220 (iteration 3 behavior changes, force-reset), "Needs the owner" in the repair logs, and the effort-table blockers L1309. requirements.md "Accepted as written" (L95-99) covers only K1-K4. Fix: split ADR-003 status into "Accepted: done rule (owner 2026-11-03)" and "Proposed, pending owner: cleanup-gate scope and ungated explicit removals", or record the owner's confirmation in requirements.md and delete the matching plan questions.

### C10. T4.2.4 changes PR creation, which requirements.md puts out of scope
requirements.md L60: out of scope is "Changing how PRs are created (`session-pr-creation`) beyond recording the new PR in the relation". T4.2.4 (L717-719) relaxes the `CreatePullRequest` short-circuit and `review_queue_manager.go` auto-create, and Story 4.2 has an AC for it (L693-694). It is needed for "PR B after merged PR A" (success metric 2), so the change is justified but unrecorded. Fix: add one line to requirements In Scope: "relax the create-PR early return so a session with only terminal PRs can open another" and reference T4.2.4 in the traceability table (L1255-1261).

### C11. Role and attach-source vocabulary does not close
Glossary L86-87 and T2.1.3 (L457-458). `PRRole` has `external` and `rework`, but T2.1.3 assigns `rework` "only from a rework session source", and `AttachSource` has no rework value; `external` is never assigned by any task. Story 3.3 (L632) hard-codes `role=followup` for discovery while T2.1.3 derives `followup` only "while an earlier one is merged"; backfill imports use `followup` (L171). Fix: add an `AttachSource` or an explicit `role` argument for rework sessions, say who assigns `external` (or drop it from the sum type and from ux.md S2 "Never ... `external`", L132), and make Story 3.3's AC use T2.1.3's rule.

---

## NITPICKS

- N1. Glossary count is stale: L106 says "Glossary terms: 28"; the table (L78-104) has 27 rows (counted by `sed | grep -c '^| '`). Fix: change to 27 or add the missing term.
- N2. Effort table Story 1.4 (L1272) says 2 tasks "(0/1/1)" = 185k raw; headings T1.4.1 and T1.4.2 are both `(S)` (L379, L383), i.e. 0/2/0 = 110k. All other 29 stories match. Fix: resize T1.4.1 to M if it earned it (prerequisite-order validation, audit policy and a second flag) or change the row to 0/2/0 and recompute totals (8,715k raw, 7,846k CU, 13,727k with verification at present).
- N3. `design/ux.md` "Plan gaps" section (L511-521) is stale: plan now covers Gap 1 (T4.1.1 poll fields, though see C8), Gap 2 (`unscanned_session_count`, T9.2.1/T10.1.1), Gap 3 (nav entry, T10.1.2), Gap 4 (`pr` filter, T4.1.1/T4.2.6), Gap 6 (`transcript_supported`, T7.2.1, a different design from "add `program` to the tab props"), Gap 7 (client-side undo, T4.2.3), Gap 8 (list lookup, T6.2.4), Gap 9 (`#pull-requests`, T6.2.1). Only Gap 5 remains open (N4). Fix: mark each as closed with its task id, keep Gap 5.
- N4. UX-S7-4 (60s "Taking longer than usual" / "Cancel waiting"), the fifth state "scan failed" with retained last-good data, and the "Last scanned N min ago" status line (ux.md L314-322) have no task text. T7.2.1 (L1067) says "four states", T7.2.4 only the Rescan button. Fix: add a client-timer clause and the failed/last-scanned states to T7.2.1 or T7.2.4 (ux.md Gap 5 says a client timer is enough), and list T7.2.4 in S7's plan ref (ux.md L17).
- N5. ux.md S6 (L300-306): chip shows the most recently updated session plus a "+1" popover. Plan T6.2.4 (L1005) and Story 6.2 (L987): one session renders "session: name", two or more render "N sessions" opening a menu. Fix: align one to the other (plan wording is simpler; update ux.md).
- N6. Terminology: ux.md calls the detach action "Not mine" (S2 L156, S3 L182, S5 L285) and plan T4.2.3 says `"not mine" / "stop tracking"` (L710) while T6.2.5 (L1009) and T6.2.6 (L1013) use "Stop tracking" for items and "not-mine" for sessions. Fix: pick one label per owner (session: "Not mine"; item: "Stop tracking") and state it in the Glossary.
- N7. UX behaviors with no task text: merge-method select in the S3 dialog (L177; no `method` field in T4.1.1's request list, L677); S2 header Refresh calling `InvalidateAndRefresh` for open rows (L145; plan has the internal function T3.3.2 but no client-facing RPC for it, only `RefreshPRInfo` in `pr_tracking.go`); S4 select beyond 4 segments (L228); S9 single-match shortcut that navigates straight to the session (L465; T9.2.2 and T10.1.2 only route to `/artifacts?pr=`). Fix: add each to T6.1.3/T6.2.3, T4.1.1 or T10.1.2, or drop it from ux.md.
- N8. Label drift: UX-S1-2 `aria-label="2 more pull requests, open pull request list"` (ux.md L70) vs T6.1.2 `aria-label="2 more pull requests"` (L966). Fix: use the ux.md string in T6.1.2.
- N9. requirements.md is stale against the plan: Decisions L90 gives the active-PR order "newest non-terminal"; ADR-002 L9 and Glossary L88 refine it (`unknown` ranks below open); NFR L45 says "merged and closed PRs are not re-polled" while unverified-terminal rows are polled until verified (L93, Story 3.2) and all-terminal sessions get slow discovery list calls (T3.2.3); Open Questions L101-106 are all resolved in plan L48-57. Fix: add a one-line "see ADR-002 refinement", carve out unverified rows in the NFR, and link the open questions to the plan resolutions.
- N10. Story 11.1 Gates AC (L1234) lists `actor-field-guard` but not the guards this project adds: `pr-field-guard` (T2.2.4), `pr-state-write-guard` (T2.3.4), and the two allow-list tests (`legacy_pr_readers_test.go`, `legacy_item_pr_readers_test.go`). Fix: name them in the AC.
- N11. Proto serialization chain (L252, L257) omits the wave-2 proto edits: T4.2.3 and T4.2.7 add RPCs to `session.proto` and T8.2.1 edits `types.proto`, all concurrent in wave 2, but only T4.2.3 -> T4.2.7 is ordered (L257). Fix: extend the chain to `T7.2.4 -> T4.1.1 -> T4.2.3 -> T4.2.7 -> T8.2.1 -> T9.2.1 -> T10.1.1` or state that types.proto and session.proto are separate chains.
- N12. ADR-003 L16 says `item.pr_url`/`pr_number` mirror is written "in the same atomic UPDATE as the status"; the plan (T5.2.3, L835) also recomputes it at the end of each `ReconcilePRPending` pass for session-link changes and never zeroes it on an empty set. Fix: add both to ADR-003's mirror bullet.
- N13. Stale file references and a quiet scope extension. Story 3.3 Files (L637) and T3.3.3 Files (L649) list `session/instance_actor_setters.go`, but new PR setters live in `session/instance_pr_set.go` (Tech Debt row L142, T2.1.2). T2.2.1 (L490) also tightens terminal-scan detection (repo/host match, push/`gh pr create` lines only), a behavior change with no requirement line. Fix: update the file lists; add the detection tightening to the requirements In Scope or mark it a Known behavior change.
- N14. "Mirror" is overloaded: the session legacy mirror (Glossary L90, ADR-002 title), the item `pr_number`/`pr_url` mirror (L820, L835) and the deleted "mirror links" (L115). A reader of ADR-001 L17 sees "no mirror links" and ADR-002 "legacy mirror" in adjacent files. Fix: use "legacy projection" for sessions and "item scalar mirror" for the backlog item, and keep "mirror links" only in the repair log.

---

## Checks that passed (no finding)

- Deleted iteration-3 names (`abandoned`, `origin_session_uuid`, `AbandonByOrigin`, mirror links, `Superseded ItemSession`): in requirements and ux.md zero hits; in ADR-001/ADR-003 and plan only as explicit "no such thing" statements or repair-log history (grep, pre-L1425). No waive RPC or `WaiveAcceptanceCriterion` appears in the artifacts; plan L710 states detach is the single exit.
- `unverified` / `outcome_unverified`: ux.md L24 (adapter-derived, no stored column), T4.1.1 L675, Glossary L79 and L93 agree; the UX label "Closed or merged (unverified)" matches the backfill seed table (L162).
- Identity: `PRRef` canonical lowercase in requirements L69, ADR-001 L15, Glossary L78, T1.2.1; consistent. `PullRequestState` vs `SessionPR` split used consistently in all four ADRs and the plan.
- Surface coverage: S1-S10 each map to a task (S1 T6.1.2/T6.2.1; S2 T6.1.3/T6.2.2; S3 T6.1.3/T6.2.6; S4 T6.2.3; S5 T6.2.5; S6 T6.2.4; S7 T7.2.x/T8.2.2; S8 T10.1.x; S9 T9.2.2; S10 T6.3.1/T6.1.2), with the gaps listed in N4, N5, N7.
- Dependency table: 101 tasks, no duplicate or missing task ids; no cycle introduced by the edges proposed in C2-C4 (each added edge points to an earlier-or-same-wave task).
