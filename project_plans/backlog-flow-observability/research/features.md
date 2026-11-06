# Research: features and signals for backlog-flow-observability

**Date**: 2026-11-05. **Data**: `~/.stapler-squad/workspaces/6eb0b580fa0331d5/sessions.db`, opened with `sqlite3 -readonly` or Python `mode=ro`. Row counts were taken at query time and drift slightly from requirements.md (for example, 257,987 status events vs 257,973).
**Labels**: VERIFIED = a command was run and its output seen. INFERRED = reasoned from data or code, not directly confirmed.

## Headline findings

1. **Items rarely stall into a dead end; they grind.** Of 161 items, 105 reached `pr_pending>done`. Their median is 2 review bounces, and only 32 of 105 (30%) had zero rework. Archived-without-ever-done items are mostly never-started. VERIFIED.
2. **The reviewer's FAIL and UNVERIFIABLE rates are inflated by non-judgments.** 146 of 378 non-PASS verdicts (38.6%) are infrastructure or missing-input results, not code judgments. 98 FAILs (53% of all FAILs) say "no committed changes". 34 UNVERIFIABLEs are the reviewer's own "codebase-read self-check failed". VERIFIED.
3. **"Agent believes done" is already captured.** `backlog_progress_notes.status='done'` per criterion (from `report_progress`) was the agent's latest claim on 755 of 789 non-PASS criterion verdicts (95.7%) that had a prior note. No new capture is needed for the deadlock heuristic. VERIFIED.
4. **Thrash is a family of persistent-spawn-failure loops, not one bug, and it still occurs.** The dequeue loop ran on 2026-11-05 (278 events), and a PR-fix reopen loop with 4,325 events ended 2026-11-04. Both lack a per-item backoff or circuit breaker. VERIFIED for the data; root cause VERIFIED for the dequeue loop (log) and for the PR-fix loop (log, worktree-create failure).

## 0. Data-source corrections that affect the design (VERIFIED)

- `stage_transitions` is NOT an event log. It is a pipeline-definition table (`from_stage_id`, `to_stage_id`, `enabled`) with 38 rows. `analytics.db` has the same table names, but every backlog table in it has 0 rows. Open Question 1 therefore resolves to: **`backlog_status_events` is the only source of truth for dwell**.
  - `sqlite3 -readonly analytics.db "select count(*) from backlog_items"` returns 0. The same holds for `backlog_status_events`, `backlog_stuck_states` and `review_verdicts`.
  - `analytics.db.analytics_events` holds 100,694 rows (92,842 `rpc`, 5,622 `performance`, 1,179 `navigation`, 1,131 `user_action`). It has no flow semantics.
- **Timestamps are mixed-timezone strings.** Event `created_at` values end in either `+00:00` (247,333) or `-07:00` (10,654, all between 2026-08-06 and 2026-08-17). `backlog_items.created_at` has 93 of 161 rows with `-07:00`. String comparison and `ORDER BY created_at` are wrong across that boundary. Use `datetime(created_at)` in SQL or parse in Go with `time.Parse`. Any window filter must normalize.
- **Performance is not a constraint.** A 30-day count with `note not like` over 258k rows ran in 0.07s: `datetime(created_at)>=datetime('now','-30 day') and note not like 'dequeue%'` took 0.071s and returned 4,755. The query plan is a full scan of the covering index `backlogstatusevent_item_id_created_at`. No created_at-only index exists. Compute-on-read meets the 1s SLO today. Collapsing in SQL (`note not like 'dequeue%'`) removes 92% of the rows anyway. This answers Open Question 5 in part: rollups are not needed for performance yet.
- 54,567 events (21%) carry `allowed_transitions_snapshot` and `stage_name_snapshot`. These columns bloat the table but are not needed for flow metrics.

## 1. Lifecycles: merged/done vs archived-without-merge

Query (Python, read-only). Noise events (`note like 'dequeue%'`) are excluded. "Merged" means the item has a `pr_pending>done` event. Bounce means `review>in_progress`. PR-fix means `pr_pending>in_progress`.

| Cohort | n | Bounces med / mean / max | PR-fix reopens med / mean / max | Sessions med / max | Zero-rework items |
|---|---|---|---|---|---|
| Merged (`pr_pending>done`) | 105 | 2 / 2.44 / 12 | 0 / 42.1 / 4325 | 7 / 29 | 32 |
| Done by another path (done event, no pr_pending>done) | 31 | 2 / 3.87 / 24 | 0 / 0.55 / 3 | 9 / 43 | 7 |
| Never done, any status | 25 | 0 / 2 / 16 | 0 / 0 / 0 | 1 / 29 | 21 |
| Never done and archived | 18 | 0 / 0.72 / 13 | 0 / 0 / 0 | 1 / 29 | 17 |

- The mean of 42.1 PR-fix reopens for merged items is a single outlier. Item `a90b99cb` accounts for 4,325. Medians and distributions are the right aggregates. For merged items, PR-fix reopens are 0 for 73 items, 1 for 16, 2 for 7, and 3 or more for 9.
- **First-pass yield (merged, zero bounces and zero PR-fix) is 32/105 = 30.5%.** Zero review bounces alone is 39/105. The bounce distribution for merged items, with 10 as the cap bucket, is: 0:39, 1:13, 2:17, 3:5, 4:10, 5:7, 6:4, 7:3, 8:1, 9:1, 10+:5.
- The 18 "never done and archived" items were mostly archived from `ready` (14), `idea` (3) or `in_progress` (1). These are cancellations, not review-gate failures. Only 1 had any review round. The "archived without merge" cohort is therefore NOT where review deadlock shows up.
- Where deadlock shows up: highest bounce counts are `5eaaa473` (24 bounces, 43 sessions, archived), `0791f5b4` (17, done), `2846ce8a` (16, still in review), `8a53ce39`, `0e775762`, `61371a09`, `a90b99cb` and `735dcbee` (12-13 each). Many of these were archived after a human override (section 2), so **archive status hides stalled-then-forced items**.
- Session roles in `item_sessions` (1,401 rows): review 608, work 501, triage 288, diagnose 4.
- Edge cases:
  - `done>archived` is 160 events, `archived>done` 25, `done>ready` 8. Reopen and revive paths exist, so "reached done" is not monotonic. Dwell logic must handle repeated visits.
  - 6 items have `done_ev>0` through non-PR paths (`review>done` 35 events).

## 2. Review verdicts

528 verdicts across 139 items, all `review` role. Verdicts per item range from 1 to 24. Query: `review_verdicts v join item_sessions s on s.id=v.item_session_review_verdict`, then join to `s.backlog_item_item_sessions`.

- Outcomes are FAIL 186 (35.2%), PARTIAL 82, PASS 150, UNVERIFIABLE 110 (20.8%). That is 5 more rows than requirements.md because the data moved on. Overrides: 5, all on PASS verdicts by `user`, with reasons "Manual override to done" (4) and "Completed via a different means" (1). VERIFIED.
- **The 4 "Manual override to done" items are `5eaaa473`, `1828108a`, `bd337ac9`, `61371a09`.** They are the longest non-converging sequences (for example `5eaaa473`: `FFFFFFFFFRUFRFFFFFFFFUFP`, 24 verdicts, final PASS is the override). A human override on top of a long non-PASS streak is the best validated deadlock signal available.
- **Sequence shape:**
  - 45 items have one verdict, 25 have two, 15 have three, and 54 have four or more.
  - 50 items have a run of 3 or more consecutive non-PASS verdicts; 26 have 5 or more.
  - 29 items never received a PASS (so deadlock candidates are plentiful and not rare).
  - The final verdict is PASS for 100 items, FAIL for 19, UNVERIFIABLE for 13 and PARTIAL for 7.
  - The first verdict is PASS for only 63 of 139 (45%).
- **Reviewer non-judgments** (regex on `summary`, VERIFIED counts):
  - "no committed changes / nothing to review": 98, all FAIL. This is the work session ending with no commits, not a strictness signal.
  - "codebase-read self-check failed": 34, all UNVERIFIABLE. This is a reviewer-environment fault.
  - "no acceptance criteria": 21 (7 UNVERIFIABLE, 5 PARTIAL, 4 FAIL, 5 PASS). The item itself is underspecified.
  - Summary mentions a truncated diff: 100 (31 UNVERIFIABLE, 33 PARTIAL, 11 FAIL, 25 PASS). **`review_verdicts.diff_truncated` is true for 0 rows**, so the column does not reflect what the reviewer reports. It is unreliable as a signal.
  - 23 of 150 PASS verdicts are synthetic "Auto-verified by reconcileBouncingItems" rows, not reviewer judgments. They should be excluded from reviewer-quality rates.
  - Combined: 146 of 378 non-PASS verdicts are infra or missing-input (FAIL 102, UNVERIFIABLE 41, PARTIAL 3).
- **Per-criterion** (`per_criterion` JSON parsed for all rows, 0 unparseable): PASS 1,503, UNVERIFIABLE 609, FAIL 295, PARTIAL 141.
  - UNVERIFIABLE evidence text is dominated by "not in the visible diff" (157 of 609 matched a not-shown/no-evidence pattern, 62 a test-not-run pattern, 28 outside-the-diff, 36 runtime/browser/live, 10 commit-order/history). The reviewer sees only a diff, so criteria about behavior outside it, runtime, or history are structurally unverifiable.
  - 26 items have at least one criterion that was non-passing (FAIL or UNVERIFIABLE) in every one of 3 or more verdicts. These are the clearest "reviewer and agent cannot agree" cases.
  - Repeat review of the identical `diff_hash`: 25 items, 47 repeat verdicts. This signals reviewer instability or flakiness, which also appears as the `likely_flaky` stuck reason.
- Worked example, `0791f5b4` (sequence `FRUUURURRURRRP`, 17 bounces): AC1 asks for a commit ordering that is already fixed in git history. The agent's note says "Known deviation on AC1: the terminal proxy commit predates the aggregator commit." Another note says "this round closes the reviewer's evidence gaps and resolves no code gaps, because the code already matched." The reviewer cannot pass a criterion the agent cannot change. This is a true agent-believes-done vs reviewer-rejects deadlock. The item eventually passed through `Auto-verified by reconcileBouncingItems` once PR #1442 merged.

## 3. stuck_states (`backlog_stuck_states`)

717 rows, 145 distinct items, 7 currently unresolved, first detected 2026-08-10. Unique key is `(item_id, reason)`, so there is **one row per item per reason**. A recurrence of the same reason on the same item overwrites the row. Historical episode counts and durations for repeat stalls are therefore lost. Durations below are for the latest episode only.

| Reason | n | open | median h | p90 h | max h | avg remediation attempts |
|---|---|---|---|---|---|---|
| autonomous_stuck | 97 | 1 | 2.17 | 20.07 | 168 | 1.7 |
| multiple_reasons | 90 | 1 | 0.97 | 13.27 | 945.5 | 0 |
| respawn_blocked_active | 80 | 0 | 0.98 | 10.37 | 50.1 | 0 |
| pr_needs_fix | 79 | 0 | 0.27 | 5.12 | 35.1 | 1.9 |
| bouncing | 71 | 1 | 1.43 | 34.99 | 945.5 | 2.1 |
| abandoned_review | 65 | 1 | 0.02 | 2.79 | 530.2 | 1.7 |
| orphaned_triage | 55 | 1 | 0.78 | 17.49 | 565.9 | 2.4 |
| stale_work | 55 | 0 | 0.04 | 0.84 | 6.0 | 1.7 |
| pr_ready_unmerged | 50 | 0 | 0.30 | 8.33 | 70.6 | 0 |
| bounce_cap_exhausted | 7 | 0 | 287.7 | 815.1 | 887.7 | 0 |
| likely_flaky | 12 | 1 | 6.87 | 514.6 | 975.6 | 0 |
| push_failed | 12 | 1 | 0.40 | 870.5 | 956.3 | 2.5 |

(The full table also has verdict_undelivered 16, plan_not_approved 11, rework_blocked_stale 6, rework_cap 3, blocked_by_dependency 3, repeated_noop_dispatch 3, merged_pr_unverified 1, transition_denied 1.)

- **What resolved them** is not recorded. The schema has no resolution-cause column (fields: reason, first_detected_at, last_checked_at, notified_at, resolved_at, snoozed_until, context, remediation_attempts, next_remediation_at, grace_boot_time, diagnose_nudge_count, diagnose_next_eligible_at). I inferred the cause by matching `resolved_at` to the nearest status event within 120s (INFERRED, from a Python join):
  - pr_ready_unmerged (50): 41 resolved with `pr_pending>done`, which is a merge.
  - orphaned_triage (54): 46 resolved with `idea>ready`.
  - abandoned_review (64): 21 `review>in_progress auto-reopened after failed verdict`, 26 `review>pr_pending`.
  - bounce_cap_exhausted (7): 4 `review>done` (human override or auto-verify), 2 `review>pr_pending`.
  - pr_needs_fix (79): 18 auto-reopened for PR fix, 15 `pr_pending>done`, 34 no event within 120s.
  - autonomous_stuck (96), respawn_blocked_active (80) and stale_work (55): 36, 30 and 34 respectively resolved with no matching event, so they self-cleared when the underlying session activity resumed or the item moved on some other way.
- Many reasons resolve fast (median under 1 hour for 8 of the 12 listed) but have long tails (p90 above 10 hours for 6 of the 12; max over 500 hours for 5 of the 12). A tail-focused view (p90, max, still open) is more useful than a mean.
- `likely_flaky` (12) and `bounce_cap_exhausted` (7) are the long-lived reasons (median 6.9h and 288h), so they indicate real manual-intervention cost.
- `multiple_reasons` (90) is a synthetic composite row (ADR-001 in backlog-bounce-escalation). Counting it alongside its constituent reasons double-counts. Context text carries the constituent list ("autonomous_stuck, respawn_blocked_active").

## 4. Is the dequeue/spawn-fail thrash still occurring? (VERIFIED)

**Yes, still occurring on current data, though 300x below its peak.**

- Totals: 119,340 `ready>in_progress` ("dequeued: WIP slot freed"), 119,213 `in_progress>ready` ("dequeue spawn failed" 119,193), 4,395 `queued>in_progress` and 4,362 `in_progress>queued`.
- Weekly thrash events (ISO week, distinct items, events): week 36: 2/39,417; 37: 5/34,597; 38: 4/18,664; 39: 2/91; 41: 2/123; 42: 6/120; 43: 7/237; 44: 4/283. The 175k mass sits in Aug-Sep. October onward is hundreds per week.
- Latest thrash event: 2026-11-05 07:18:22 UTC. On 2026-11-05 a single item, `0791f5b4` ("feat: session federation"), produced 278 events (139 pairs), one pair every ~1 minute from 02:38 to 07:18 UTC. Other items since 2026-10-01: `cba71e86` 122, `4b30ec3f` 102, `c3ac1f84` 91, `f08268d4` 88, `50dff607` 87.
- **Root cause (code):** `DequeueNextQueuedItems` in `server/services/backlog_service_triage.go:837-938` CAS-claims a candidate to `in_progress`, calls `spawnSessionAfterGates`, and on any spawn error rolls back to `ready` or `queued` with note "dequeue spawn failed" (lines 918-933). Nothing records the failure on the item. The next sweep (the lifecycle ticker, about 60s) selects it again as a candidate. There is no per-item attempt counter, backoff or cooldown (a grep for spawn backoff, dequeue cooldown and spawn-fail counters finds nothing in `server` or `session`). Each iteration writes two `backlog_status_events` rows. INFERRED for "no backoff" (absence of a grep hit); the loop's one-minute cadence is VERIFIED from timestamps.
- **Why the spawn fails (log, VERIFIED):** `~/.stapler-squad/workspaces/6eb0b580fa0331d5/logs/staplersquad-2026-11-05T04-04-59.163.log.gz` has 138 "spawn failed for dequeued item; rolling back" lines for `0791f5b4`. 108 are `internal: failed to create git worktree: CreateBacklogWorktree: setup: failed to create worktree from branch ...` and 30 are `already_exists: a work session (...) is already active for this item ... likely stalled: no output in 24m15s (stale threshold 15m0s)`. The 138 failures match the 139 event pairs closely.
- **Second loop, PR-fix reopen, VERIFIED data:** `pr_pending>in_progress` has 4,437 events, 4,325 of them for item `a90b99cb` between 2026-10-30 and 2026-11-04 (per day: 781, 1,853, 746, 2,508, 2,202, 562). Last event 2026-11-04 06:23 UTC. The matching `in_progress>pr_pending` rollback has an empty note in 4,372 of 4,381 events, so **the rollback events cannot be told from real transitions by note**. The shared failure is in `autoReopenForPRFix` (`backlog_service_triage.go:2121-2240`): transition to in_progress, spawn, and on spawnErr roll back to `pr_pending` with `nil` precondition, so no note. A comment at lines 2140-2149 shows the active-session variant was already fixed, but the other spawn-error path was not. The logs for 2026-11-02 show 545 "worktree creation failed for git-managed repo" lines for `a90b99cb` (`CreateBacklogWorktree: setup: failed to create worktree from branch backlog/gh-signal-item-a90b99cb`).
- **Common root cause: a persistent spawn failure (here, worktree creation) retried every sweep, with no per-item backoff or circuit breaker.** A single unspawnable item can generate tens of thousands of events. The visible stuck signal is weak: `a90b99cb` also logged 1,130 `[BacklogLifecycle] escalated` lines in one log file.
- Implication for this feature: (a) collapse must key on the loop shape (alternating A>B, B>A with the same item and an interval below some threshold), not only on note text, because the PR-fix rollback has no note; (b) these items should be flagged as anomalies; (c) fixing the root cause (a backoff on spawn failures) is a trivial addition to the existing dequeue and PR-fix paths, but it is out of scope per requirements.md. It deserves its own backlog item.

## 5. Is "agent believes done" captured? (answers Open Questions 2 and 3)

**Open Question 2: yes, in three structured places and one unstructured one.**

1. **`backlog_progress_notes`** (1,385 rows, 124 items): per-criterion `status` of `done` (1,256), `pr_created` (93), `in_progress` (30), `ready` (6), with free-text `note`, written through `report_progress`. For every non-PASS criterion verdict that had an earlier note on that criterion, the latest prior note said `done` in 755 of 789 cases (FAIL 169 of 184, PARTIAL 113 of 117, UNVERIFIABLE 473 of 488; the remaining 34 were `in_progress`). 256 of 1,045 non-PASS criterion verdicts had no prior note. VERIFIED (Python join on `item_id`, `criterion_index`, `datetime(created_at) <= verdict time`).
2. **`backlog_status_events` `in_progress>review`** (635 events): `triggered_by='agent'` with note `request_review from <agent's summary text>` for 400 of them (503 agent-triggered events overall). 183 are system-triggered with a null note and 37 have note "all work sessions ended". The "all work sessions ended" ones are items pushed to review without the agent claiming done, so they are a distinct class. The agent summary frequently contains explicit signals: 132 of the request_review notes contain words like "already", "no code", "deviation", "unverifiable", "evidence gap", "reviewer" or "disclosed". Example: "this round closes the reviewer's evidence gaps and resolves no code gaps."
3. **`item_sessions.verification_notes`**: populated for 226 of 501 work sessions (45%), none for the other roles. It holds the test commands and results the agent ran, which is the evidence side of the claim.
4. **`backlog_activity_notes`** (39 rows, 18 items): mostly diagnose and triage results ("Diagnosis result: NOTE_ONLY ... review-loop artifact"). Low volume, but the diagnose sessions already label some items as "review-loop artifact", which is a pre-built deadlock label.

- **Not useful:** `item_sessions.end_reason` is empty for all 501 work sessions, all 4 diagnose sessions and 603 of 608 review sessions. It carries values only for a few triage sessions (parse_error 19, pool_saturated 7, shutdown 4). Session end reason cannot show "agent believed done".
- **Open Question 3 (thrash still occurring):** yes, see section 4.
- **Deadlock candidate heuristic that existing data supports (no new capture):** an item with N or more consecutive non-PASS verdicts where the latest `report_progress` status for the failing criteria is `done` and the agent filed `request_review` each round. Counts: 41 items have 3 or more non-PASS verdicts and at least one disputed criterion; 50 items have a 3+ non-PASS run; 26 have a 5+ run. A refinement worth testing is to exclude infra verdicts (the 146 non-judgments above).
- **Gap (needs new capture only if the plan wants it):** no durable record ties each verdict to the specific request_review event that triggered it, so pairing is done by timestamp. There is also no record of the agent's explicit "I disagree with reviewer" intent; the keyword matches are a weak proxy.

## 6. Existing features to reuse

- **`backlog_stuck_states` and `ListStuckBacklogItems` RPC** (`proto/session/v1/backlog.proto:1470`): returns only open, un-snoozed rows. It has no history and no aggregation, so it needs a new query path, not a new store. Gaps: no resolution cause, the `(item_id, reason)` unique key discards repeat episodes, and `multiple_reasons` is a composite.
- **`get_backlog_stuck_report` MCP tool**: its schema (loaded this session) takes only `item_id` and reconstructs one item's stuck timeline, criteria status, session cost and notes, plus a live-CI cross-reference for `pr_pending` items. It is per-item, so it complements an aggregate tool rather than overlapping it. I could not find its registration: `grep -rn -i "stuck_report\|StuckReport"` over `*.go` and `*.proto` in this worktree found none, and `server/mcp/tools_backlog.go` registers no stuck-report tool. So it is live on the running server but UNVERIFIED in this branch's source. I did not call it (it needs a live item and the server connection was reported unreliable).
- **`stage_transitions`**: not reusable for dwell (see section 0). It could supply stage names and ordering if the Flow view needs a configured stage order.
- **`backlog_status_events`**: the source of truth. The existing `backlogstatusevent_item_id_created_at` index serves per-item timelines. A global time-window scan takes about 50-70ms.
- **`project_plans/backlog-stuck-item-visibility`**: built the durable stuck-state store (ADR-001), `ListStuckBacklogItems`, snooze and reset-remediation RPCs. Reuse its reasons, remediation counters and notification fields.
- **`project_plans/backlog-bounce-escalation`**: introduced the `multiple_reasons` synthetic reason (ADR-001) and `bounce_cap_exhausted` (7 rows live, long-lived), and deferred flaky-test review differentiation (ADR-002). The `likely_flaky` reason (12 rows) already encodes "two verdicts on the identical diff differed".
- **`project_plans/backlog-operator-feedback-loop`**: steering path (`UpdateSession` widened, ADR-001/002). Relevant for the PR-fix nudge outcome metric: steer-vs-spawn is visible as `respawn_blocked_active` (80 rows) rather than a status event.
- **`project_plans/backlog-session-thrashing`**: present in the repo and not read in this pass. It is the likely home of earlier thrash analysis. Check it before writing the thrash-collapse design.
- **Rework cap**: `effectiveReworkCap` and `rework_cap_override` on `backlog_items`. Only 3 `rework_cap` stuck rows exist, but the 24- and 17-bounce items show the cap is rarely the binding stop; human override is.

## 7. Edge cases and unstated needs

1. Mixed timezone strings; normalize with `datetime()` or Go parsing, and test on a fixture containing both offsets.
2. Non-monotonic status (done>archived, archived>done, done>ready): "reached done" and cohort assignment need explicit rules. Suggest "first merge" via `pr_pending>done`.
3. Noise collapse by shape, not note text: the PR-fix rollback note is empty in 4,372 of 4,381 events, and the dequeue notes are free-form strings in a 3-version-old code path.
4. Archived items hide stalls: archived status is the end state for 149+ items regardless of how long they ground. Use bounce and verdict counts, not final status, for deadlock candidates.
5. Exclude reviewer non-judgments (no-commit FAIL, self-check-failed UNVERIFIABLE, auto-verified PASS) from reviewer-strictness rates, or report both raw and adjusted. Otherwise the view repeats the misleading 35% FAIL and 21% UNVERIFIABLE headline.
6. `review_verdicts.diff_truncated` is always false; do not surface it as a metric. If truncated-diff rates matter, they must be derived from summary text or a new capture. This is a candidate capture gap.
7. Stuck-state history is lossy (one row per item and reason). Time-in-stuck for a before/after comparison is therefore only reliable for first episodes or open rows. If before/after on stuck rates is a goal, add an append-only episode log (a capture gap) or accept the limit and label it in the UI.
8. Resolution cause is unrecorded. "What resolved it" is inferable only by a ±120s join and fails for about 40% of autonomous_stuck, pr_needs_fix and stale_work rows (34-36 of 79-96 each).
9. Before/after comparisons are confounded: 2026-08-06..17 data has a different timezone format, and thrash volume changed 300x between September and October. Windows straddling that need collapsed counts.
10. The 29 never-PASS items include in-flight items, so "stalled" needs an age or activity threshold, not just verdict shape.
11. Items with multiple work sessions per review round and review sessions with no verdict (`verdict_undelivered` 16 rows, "review session exited without ever writing a verdict" in the bouncing context) create verdict-less rounds; round counting by `in_progress>review` events (635) and by verdicts (528) differ by 107 for this reason.
12. Privacy and size: `note` text for `request_review` contains agent-written summaries, up to several hundred characters. Truncate in list views and keep them out of MCP aggregate responses unless the caller asks for the item.
13. Freshness: surface "as of" using `max(datetime(created_at))`; the latest event in the DB was 2026-11-06 04:15 UTC at query time.

## Method notes

- All SQL ran read-only. Python used `sqlite3.connect('file:sessions.db?mode=ro', uri=True)`. Scripts were run inline with heredocs and are not saved.
- Representative queries:
  - Thrash by week: `select strftime('%Y-%W',created_at) w, count(distinct item_id), count(*) from backlog_status_events where note like 'dequeue%' group by 1 order by 1 desc`
  - Verdict-to-item join: `select s.backlog_item_item_sessions, v.overall_outcome, datetime(v.created_at), per_criterion, summary from review_verdicts v join item_sessions s on s.id=v.item_session_review_verdict`
  - Stuck durations: computed in Python from `datetime(first_detected_at)` and `datetime(resolved_at)`.
  - Agent-claim join: latest `backlog_progress_notes.status` for `(item_id, criterion_index)` with `datetime(created_at) <= datetime(verdict.created_at)`.
- Regex-based counts in section 2 (non-judgment and keyword patterns) are heuristics. They are exact for the stated patterns but may undercount semantically equivalent phrasings.
- `Open Question 4` (where Flow lives) and the rest of `Open Question 5` (retention) are out of this pass's data scope. For retention: the table has 258k rows, a 30-day scan costs about 50-70ms, so pruning is not forced by performance.
