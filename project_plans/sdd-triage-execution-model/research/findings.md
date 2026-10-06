# Research findings

All numbers below were produced on 2026-10-05 against the live deployment's
`~/.stapler-squad/workspaces/d685c4b1a423cca3/sessions.db` (read-only `sqlite3`) and `ps`. Queries are
reproduced under each table so they can be re-run.

## AC1 — triage cost / duration by mode (VERIFIED)

Source: `item_sessions` (`session_role='triage'`) joined to `backlog_items` on `backlog_item_item_sessions`.
Mode = the item's **current** `backlog_items.pipeline_mode` (`sdd` vs everything else = "default"); an item
whose mode changed after triage is misattributed (the per-session `pipeline_mode_snapshot` holds the full mode
text, not a name, so it can't be grouped cheaply). Duration = `ended_at − created_at` (`started_at` is NULL for
872 of 873 triage rows). Cost = `estimated_cost_usd`, which is only written when the CLI's terminal `result`
line (`total_cost_usd`, `session/headless/caller.go:62`) arrives.

```sql
select b.pipeline_mode, count(*), sum(s.estimated_cost_usd>0) priced, round(sum(s.estimated_cost_usd),1) total
from item_sessions s join backlog_items b on b.id=s.backlog_item_item_sessions
where s.session_role='triage' group by 1;
```

| mode | triage calls | with a recorded cost | recorded total | avg / priced call | calls >$20 | $ in calls >$20 | priced p50 / p90 duration |
|---|---|---|---|---|---|---|---|
| default | 217 ended | 26 | $80.4 | $3.09 | 0 | $0 | 7.3 / 11.2 min |
| sdd | 656 ended | 39 | $1,058.7 | $27.15 | 19 | $904.4 (85%) | 62.5 / 126.5 min |

Overall triage: 873 calls, $1,139.1 recorded (all `item_sessions` roles: work $100.3, review $6.4).

Findings:
- **sdd is 93% of recorded triage spend (1,058.7 / 1,139.1 = 0.929)**, and 85% of
  sdd spend sits in 19 calls over $20; 7 calls exceeded $50. The top call was $101.16 (2026-09-23, 92 min). #882 is
  the head of a heavy tail, not a one-off. Default triage's max recorded call is $6.34.
- **Cost is under-recorded exactly where it matters.** 190 of 656 sdd calls ended `timeout` and 12 `pool_saturated`; all
  of those have `$0` recorded, because cost only arrives on the final `result` event and a cancelled call never emits
  it (`end_reason` breakdown: `select b.pipeline_mode='sdd', s.end_reason, count(*), sum(estimated_cost_usd) …`).
  The real sdd total is therefore **higher** than $1,058.7; it cannot be bounded from this DB. Any ceiling that
  fires on a cancelled call must record its own partial counters (turns/subagents) because the dollar figure is lost.
- Default-mode outlier: one row with a 38,885-minute duration is a stale tombstone, not a call; medians use priced rows
  only.

**Gap (documented, with plan):** per-call *turn* and *subagent* counts are not stored anywhere (no column; log has no
such fields). Instrumentation plan: expose `fanoutCounter`'s final turns/subagents through a `CallOptions` stats callback and log
them with `mode` and cost at every triage call end (follow-up; this item's counter already reports them on a ceiling abort).

## AC2 — per-tmux-session overhead vs 8 concurrent triage calls (VERIFIED, point-in-time)

Source: `ps -eo pid,rss,args` on the live host, 2026-10-05, 17 live tmux-backed sessions under server pid 3375059.

| component | RSS | note |
|---|---|---|
| `claude` process per session | 260–425 MB (n=11 of 17 panes; the rest are `agy` 128–376 MB, one `zsh`) | **same binary a headless `claude -p` runs**, so paid in both models |
| tmux server for all 17 sessions | 18.5 MB total ≈ **1.1 MB / session** | pid 3375059 |
| per-session `tmux attach-session` client (stapler-squad's PTY hook) | 3.7–4.6 MB | |
| stapler-squad server | ~2.0 GB | fixed, independent of triage model |

Marginal tmux cost ≈ 1.1 + ~4.2 ≈ **5 MB/session; 8 concurrent triage sessions ≈ 40 MB** versus **~2.1–3.4 GB for the
8 `claude` processes themselves** (8 × 260–425 MB), which headless pays too. Memory/PTY is therefore **not** the
dominant cost of option 1. The real costs are non-memory: session-row persistence, worktree + cleanup plumbing,
orphan reconciliation, and a transcript-driven monitor that does not bound fan-out (below). UNVERIFIED: behaviour under
memory pressure (host had 37.6 GB available when measured), and long-lived-session scrollback/storage growth.

## Code facts (VERIFIED by reading)

- `triageSem` cap 8: `server/services/backlog_service.go:567`; acquired `backlog_service_trigger_triage.go:378`.
- `triageCallBudget = 3h` wall clock; overridable per mode via `livenessEngine.LivenessFor` (`backlog_service_trigger_triage.go:408-413`).
- Headless first calls already run `--output-format stream-json --verbose` and scan **every line** in
  `scanFirstCallLines`/`handleFirstCallLine` (`session/headless/caller.go:~470-545`), with an existing precedent for
  stream-driven termination (`ErrOutputCapExceeded` → `terminateStream`). So observation + kill need no tmux.
- `CapacityMonitor` needs a `*session.Instance` + parsed transcript; its `IdleWaitTurnCeiling` handles idle
  re-poll loops (#885). #882-style fan-out is many *productive* turns, which it neither detects nor bounds.
- Subagent tool name in current transcripts is `Agent` (7,631 occurrences across `~/.claude/projects/*/*.jsonl`);
  legacy name `Task` (23). The counter must match both.
- An aborted call lands in `reconcileOrphanedTriageItems`' non-`shutdown` branch (`session/backlog_lifecycle_triage.go:260-305`):
  it MarkStuck's with backoff, **no** immediate respawn — so a ceiling abort cannot create a retry loop.
- Sibling item #884 (branch `backlog/stapler-squad-triage-cost-ceiling`, planning only) adds a *dollar* ceiling in the same
  scan loop (`CallOptions.MaxCostUSD`). This item adds the *turn/subagent* ceiling; the two share a hook point and an
  error-classification case.
