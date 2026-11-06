# Attention inbox: baselines (B1, U1)

Reference for the measured starting values behind the attention-inbox success metrics
(`project_plans/attention-inbox-consolidation/requirements.md`, Success Metrics). Produced by
`scripts/soak/baseline_report.go`; measured 2026-10-24 on the operator's live workspace
(`~/.stapler-squad/workspaces/d685c4b1a423cca3`), all sources opened read-only.

## Run it

```bash
go run ./scripts/soak --data-dir ~/.stapler-squad/workspaces/<hash> \
  --transcripts-root ~/.claude/projects --sample-n 100 \
  --harvest-out /path/outside/repo/harvest.csv      # harvest text to label
go run ./scripts/soak --data-dir ... --labels harvest-labelled.csv   # after labelling
```

Flags: `--window-days` (default 30), `--miss-minutes` (default 30). Harvested transcript text is
written only to `--harvest-out` (never commit it); the report prints counts only.

## B1: needs-operator episodes that were missed

**Method.** (a) Every notification-history record of type `APPROVAL_NEEDED`, `INPUT_REQUIRED`,
`CONFIRMATION_NEEDED`, `ERROR` or `FAILURE` (not `auto_remediating`) is a detected needs-operator
episode, timed at its latest occurrence. It is "missed" if the session shows no operator touch in
the following 30 minutes: `last_viewed`, `last_user_response`, `last_acknowledged` (session store)
or the record's `read_at`. Grouped by program (first word of the session `program`, basename).
(b) The last assistant message of >= 100 recent Claude transcripts is harvested; the owner labels
each "question?" and "operator touched within 30 min?". B1 = missed / all needs-operator episodes.

**Sources.** `notifications.json` (`server/notifications/store.go`: JSON file, `MaxNotificationAge`
7 days keyed on last occurrence, 500-record cap), `sessions.db` `sessions` table, rotated logs
under `<data-dir>/logs`.

**Result (a), command above, 2026-10-24:**

| Program | Episodes | Missed | Rate |
|---|---|---|---|
| claude | 58 | 47 | 81.0% |
| unknown (session deleted) | 23 | 16 | 69.6% |
| ALL | 81 | 63 | 77.8% |

**Actual covered window: 2026-10-18 to 2026-10-24 (5.8 days), not 30.** The store prunes anything
not seen in 7 days, so a 30-day detected baseline cannot be computed from it. Rotated logs begin
2026-10-22T00:24 (100 gz files kept), about 2.5 days. Every session in the store is Claude
(610 `claude` paths, 10 bare `claude`), so per-program splits for aider/agy/pi are empty.

**Result (b):** pending owner labelling. N harvested = 100 transcripts (command above); N
labelled = 0. No estimate is reported and the combined B1 is not computed until labels exist.

**Reading the 77.8%.** Treat it as an upper bound, not a clean rate: touch columns store only the
latest value, so a session viewed 31+ minutes later counts as missed, and the operator may have
acted via another channel. It is also dominated by background/automation sessions (for example
the repeating "Session went stale" records). Owner confirms the targets once labelling is done.

**Limitations.**
- Notification history only holds what was detected: (a) understates misses; (b) is a sample, not a census.
- History collapses repeats into one record (first/last occurrence), so an episode is one record.
- 30 minutes is an arbitrary cutoff.
- Retained history covers 5.8 days, not 30 (above).
- A view without input counts only through `last_viewed`/`read_at`, where recorded.
- Latest-value-only touch columns overstate misses (above).

**Target rule.** B1 >= 5%: missed share falls to at most half of B1 (or 5%, whichever is larger).
B1 < 5%: missed share does not exceed B1 and prose-question recall on the labelled corpus >= 0.80
(`SelectTarget`, tested by `TestBaselineReport_Should_SwitchToNoIncreasePlusRecallTarget_When_B1Below5Percent`).

## U1: review-surface opens per day

**Sink trace.** `usePageView` (`web-app/src/lib/analytics/usePageView.ts`) calls `track({name:
"page_view", category: "navigation", page: pathname})` through `AnalyticsContext`; events reach
`POST /api/analytics` (`server/handlers/analytics_handler.go`) and are persisted by
`server/analytics/sqlite_provider.go` into the `analytics_events` table of `analytics.db`
(ent entity `AnalyticsEvent`, `session/ent/schema/analytics_event.go`; retention in
`server/analytics/retention.go`). Persisted from 2026-09-12 onward in this workspace.

**Result (trailing 14 complete days, 2026-10-10 to 2026-10-23, local dates):**
- `page = '/review-queue'` exactly (plan's literal): 72 events, U1 = 5.14 per day.
- `page IN ('/review-queue','/review-queue/')`: 976 events, 69.71 per day.

The trailing-slash form dominates until 2026-10-20 (e.g. 130 on 2026-10-16) and is 0 from
2026-10-22, matching the switch to the Bun build that stopped emitting trailing slashes (INFERRED;
not verified against the build change). The two forms must not be mixed without that caveat.
**Use 5.14/day (no trailing slash, the current path form) as U1; the 14-day mean is not stable.**
The events are not filtered to human use: automation driving the live UI also emits `page_view`
(no client label is stored), so U1 is an upper bound on operator opens.
