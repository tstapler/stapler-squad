# Adversarial Review: backlog-flow-observability

**Date**: 2026-11-05
**Verdict**: CONCERNS

No blocker found. Every claim the caller asked me to test was checked against code (`git show origin/main:<path>`) or the live DB (`sqlite3 -readonly`, queried 2026-11-06 UTC, so counts drift by a few rows from the plan's). Several acceptance criteria and fixture numbers are wrong or unsatisfiable and should be corrected before Phase 1 starts. Each is cheap to fix and none changes the architecture.

## Verified claims (evidence)

- **827 commits behind: VERIFIED.** `git rev-list --count HEAD..origin/main` prints `827`. `git ls-tree -r HEAD` has no `session/dbpool` or `tools/lint/norawdb`, which confirms that research ran on a stale tree.
- **dbpool on origin/main: VERIFIED.**
  - `session/dbpool/open.go` has `writer.SetMaxOpenConns(1)` plus a separate reader pool (`openReader`, `spec.Profile.Readers`).
  - `session/ent_repository.go:182` calls `dbpool.OpenCore(expandedPath, readPoolSize())`.
  - `session/dbsplit/dbsplit.go` routes only a plain `SELECT` to the reader. `WITH`, `RETURNING`, `PRAGMA` and every `Tx` go to the writer.
  - The reader is opened `query_only`.
- **norawdb on origin/main: VERIFIED.** `tools/lint/norawdb/analyzer.go` rejects `sql.Open`, `entsql.Open` and `ent.Open` outside `session/dbpool` and `internal/sqlitedsn`. `_test.go` files are exempt.
- **`NewCore` has 10 parameters on origin/main: VERIFIED.** `registerBacklogTools` sits inside the `backlogEnabled` block (`server/mcp/server.go:90-92`). `server/mcp/tools_stuck_report.go` exists.
- **`MarkStuck` and `ResolveStuck` are single choke points: VERIFIED** (`session/ent_repository_backlog.go:2332` and `:2403`). `MarkStuck` runs inside `tx`.
- **Live shapes match the research.**
  - Events: 257,987 rows. The mixed offsets are real (247,333 `+00:00`, 10,654 `-07:00`).
  - Stuck: 717 rows, 7 unresolved.
  - Verdicts: FAIL 186, PARTIAL 82, PASS 150, UNVERIFIABLE 110.
  - Cohort: 105 distinct items with a `pr_pending>done` event.

## Collapse heuristic test (simulated ADR-002 on the live DB)

I re-implemented the ADR-002 definition in Python. It chains consecutive reverse-direction events on one item, requires the pair in `{ready,queued,pr_pending}<->in_progress` with a gap under 15 minutes, and sets an episode at 6 or more events.

- **Coverage is good.** 255,716 of the 256,128 events in those three shapes land in episodes of at least 6 events. The residual is 412 events, of which 212 are sub-threshold runs counted as real.
- **No misclassification of real review transitions.** `review<->in_progress` and `in_progress>review` never enter a flip pair. Review flips at under 15 minutes (304 and 258) stay real, as intended.
- **Sub-threshold runs on non-thrash items are plausible real flow.** There are 20 runs of length 2 to 5 across 160 items. Twelve are `pr_pending>in_progress`, which is a real PR-fix reopen.

See Concerns 1 to 3 for where the heuristic and its acceptance criteria diverge from live data.

## Blockers

None.

## Concerns

- [ ] **C1. Live thrash is fragmented and far more widespread than the plan's fixtures.**
  - **Evidence:** the simulation yields 103 episodes across 35 of 160 items, not "2 episodes".
    - `a90b99cb` splits into 17 episodes (lengths 6 to 1718) plus 25 two-flip runs.
    - `d91477d5` has 18 episodes and `8a53ce39` has 17.
    - `0791f5b4` splits into 2 episodes (217 and 62), not "exactly 1, Count=278".
  - **Why it matters:** Story 1.2.1 asserts a single episode of 278 and Story 1.2.2 asserts "a90b99cb's 4,325 flips count as 1 thrash episode". Those pass on synthetic one-minute fixtures but are false on live data. A 15-minute hard gap fragments episodes whenever dequeue cadence jitters; 88 `ready>in_progress` pairs and 45 `pr_pending>in_progress` pairs exceed 15 minutes.
  - **Fix:** add episode merging (same item, same shape, inter-episode gap under an hour, or a gap relative to the median), or state in the AC and UI that "episodes" are cadence segments and show per-item totals.
  - **Fix the biggest-offender claim too.** The requirements' "two items ~175k events" are `d91477d5` and `8a53ce39` (about 87k each, both archived, last event 2026-10-05). `0791f5b4` has only 280 events. They fall outside a 30-day window starting 2026-10-06, so Story 6.1.1 should also verify the all-time view.
  - **Banner copy:** the "2 items thrashing" copy is fixture-only. Live shows 35 items all-time.

- [ ] **C2. Collapse runs on window-truncated events.** `Events(ctx, window)` filters in Go to the window, then `Collapse` runs on that slice. An episode that straddles the window start loses its head. If the remainder is under 6 events, those flips are counted as real rework and can break FPY and `PRFix` counts. Run `Collapse` on window plus lookback (at least `ThrashGap`, ideally from the seed event) and clip only for reporting. Add an AC.

- [ ] **C3. FPY can pass thrash-only items.** FPY is "zero real (post-collapse) `pr_pending>in_progress`". An item that thrashed 4,000 times on PR-fix and merged has zero real reopens if every reopen collapsed. Count thrash episodes of cause `PRFixReopen` against FPY, or define explicitly why they do not count. As written, `a90b99cb` is the worst offender and its pipeline flow is hidden by the metric.

- [ ] **C4. Non-judgment counts are inconsistent (three different numbers).**
  - **Evidence:** live data says 98 `no committed changes` FAILs plus 34 `self-check failed` UNVERIFIABLEs, which is 132 of 378 non-PASS verdicts. The 23 `Auto-verified` are PASSes and are not in the 378.
  - **Where the plan goes wrong:** ADR-005 and Story 1.3.1 say "146 of 378". Story 4.2.2 says "102 non-judgment FAILs" in one sentence and "98 and 34 excluded" in the next (98+34 is 132, not 102 or 146).
  - **Fix:** use 132 everywhere and recount via SQL before writing fixtures.
  - **Unclassified non-judgments:** about 11 more UNVERIFIABLE rows are non-judgments ("Review blocked: no diff could be computed" 7, "Degraded to UNVERIFIABLE" 4, "no acceptance criteria" 2). The closed table classifies them as `Judgment`. Add a `ReviewerDegraded` class or flag them in `unrecognized`.

- [ ] **C5. Story 2.1.3 AC is impossible under the schema.** `review_verdicts` has a unique index on `item_session_review_verdict` (live: 528 rows, 528 distinct sessions), so a session has at most one verdict. "3 sessions with 1, 2 and 1 verdicts" cannot exist. Change the AC to 3 sessions with 1 verdict each plus one session with no verdict. The "per-session first, then per item" aggregation is only a join-safety guard, not a dedup step (ADR-005 overstates it).

- [ ] **C6. The deadlock signal does not discriminate.** The requirements' own finding is that the latest progress-note status is `done` on 95.7% of non-PASS criterion verdicts. So `agent_claimed_done` is nearly always true, and the candidate rule reduces to "3 or more consecutive judgment non-PASS verdicts". It is labeled heuristic, but metric (d) of the success criteria is effectively unmeasured. Add a discriminating second signal (for example, the agent re-requested review with no new commit, or verdict-run length versus rework cap) or state the limitation in the Data notes. Define what the 3-run threshold yields on the live data before wiring the UI table.

- [ ] **C7. Requirements gap: rework by cause and verdict mix over time.**
  - **CI versus merge-conflict:** requirements list "PR CI failure, merge conflict" as separate causes. The plan reports one `PRFix` bucket, which is disclosed but is a requirement downgrade. Ask the owner to accept it, or derive the split from stuck reasons (`pr_needs_fix` context).
  - **Verdict outcome mix over time:** in scope per requirements but no story or AC produces a time series. Story 4.1.3's `FlowChart` wrapper ("UI trends") has no consuming story, so it risks becoming dead scaffolding. Either add a trend story (verdict mix, throughput per week) or drop the wrapper.

- [ ] **C8. Episode writer (Story 5.1.2) is under-specified against `MarkStuck`'s real shape.**
  - **No "created or reopened" signal:** `MarkStuck` upserts, then runs a conditional reopen `Update`. Neither returns whether the row was created. The helper needs "ensure one open episode for `(item_id, reason)`" with a select-then-insert inside the same `tx`.
  - **Deadlock risk:** the single writer connection (`SetMaxOpenConns(1)`) means any call on `r.client` inside the open tx deadlocks. Make "helper takes `tx`" an explicit AC, not a note. The "exactly one helper call per function" AC is also too tight: `MarkStuck` needs one insert-if-none call, and `ResolveStuck` needs a second update tied to `n>0`.
  - **Story 0.1.1 does not locate this file:** Story 5.1.2 says the file is "located in Story 0.1.1". It is `session/ent_repository_backlog.go`. Fix the reference.

- [ ] **C9. Effort estimate: size band conflicts with requirements, and costs are flat.**
  - **Arithmetic checks:** 44 tasks (21 M, 20 S, 3 XS), 3,890k raw tokens, and the CU column sums to 10,203k. It matches the plan's 10.2M.
  - **Size mismatch:** the requirements say Medium (0.5 to 3M) and the plan lands at Large (about 10M, 3.4x the upper bound). The plan notes it and proceeds, but the driver is not examined.
  - **Likely overestimate on the read path:** a 30-day window holds only about 10,100 events (`created_at >= '2026-10-06'`), so the SQL reader, index, and perf work (2.1.1, 2.1.5, 0.1.2) are over-sized.
  - **Likely underestimate on the hot-file edits:** the UI sections and Phase 5 (a hot file) are the risk.
  - **Method:** each cost class is a flat token count, with no per-story calibration. The x1.75 verification multiplier is applied uniformly, including to XS tasks. Treat the 10.2M as roughly plus or minus 40% and hold the overrun checkpoint.

- [ ] **C10. The mixed-offset machinery is much smaller than ADR-004 implies.** `-07:00` rows span only 2026-08-06 to 2026-08-17 11:56 local, and all later rows are `+00:00`. A 30-day window from 2026-10-06 never sees a mixed-offset row. The 15h-superset prefilter, rowid tiebreak and Go normalisation matter only for all-time windows. Keep them (they are correct and the superset reasoning holds), but note it so the implementer does not over-test. Also note that the `created_at` index may be unnecessary for the 30-day case: the existing `(item_id, created_at)` index is fine, and the new one trades 258k-row build cost for a window that returns about 4% of rows. Story 0.1.2 should be allowed to conclude "skip the index".

## Minors

- Plan is dated 2026-11-05 but live data runs to 2026-11-06 UTC. Fixture counts (633/425, 526/528) drift; Story 6.1.1 already says "within drift".
- Story 2.1.2 reads `rowid` through the ent `sql.Selector`. `backlog_status_events` has a uuid primary key and is not `WITHOUT ROWID`, so this works, but note that `id` (uuid) is not an ordering key.
- `WITH`, `RETURNING` or window-function SQL from the ent selector goes to the writer connection (`dbsplit.IsReadOnly`). Keep reads to plain `SELECT ... IN (...)` as planned, and assert the pool label in the spike.
- The seed query (last event before window per item) orders by text `created_at`. This is correct on current data (the offset boundary is monotone on 2026-08-17) but is not generally safe. Take the max in Go over a small candidate set.
- `time.Now().UTC()` default for the new episode table is good. The existing `MarkStuck` writes local `time.Now()`, so episode `opened_at` and legacy `first_detected_at` are mixed in the union. Normalise in the reader.
- Dependency Visualization shows 3.1.2 depending on 2.1.5 (perf), which is a gate, not a code dependency; consider letting 3.1.2 proceed in parallel.
