# ADR-029: sdd-mode triage stays on `claude -p`, gains a stream-observed turn/subagent ceiling

Status: Accepted — 2026-10-05
Item: 6f262fa7 (research: should "sdd" pipeline-mode triage move off `claude -p`?). Evidence: `project_plans/sdd-triage-execution-model/research/findings.md`.

## Context

sdd-mode triage chains full SDD skills inside one headless call. Recorded data: sdd is 93% of recorded triage spend
($1,058.7 of $1,139.1); 19 of 39 priced sdd calls cost >$20 and hold 85% of sdd spend; the worst was $101.16. Default
triage's worst recorded call was $6.34. 190 of 656 ended sdd calls were `timeout` with **$0 recorded** (cost arrives
only on the final `result` event), so true spend is higher. The only backstops are a 3 h wall clock and an idle timeout.

## Decision

Hybrid, cheapest-first:

1. **Option 4 (new): observe the headless stream, don't move to tmux.** The headless pool already scans every
   `stream-json` line. Add a per-call `fanoutCounter` that counts assistant turns and subagent (`Agent`/`Task`)
   launches, and abort via the existing `terminateStream` path when a configured ceiling is crossed
   (`headless.ErrFanoutCeilingExceeded`, classified `fanout_ceiling`).
2. **Option 2 (soft layer):** the sdd triage prompt gets a stated phase/fan-out budget so the model wraps up before the
   hard ceiling. Advisory only; the ceiling is the enforcement. (The mode row is seeded create-if-absent, so existing
   deployments keep their stored prompt until it is edited; the ceiling applies to them regardless.)
3. **Option 3 stays with #884** (dollar ceiling in the same scan loop). Not implemented here; this ADR defines the
   hand-off (see Interactions).
4. **Default triage is unchanged:** still `-p`, no ceiling by default (AC4: no material waste — max $6.34, p90 11.2 min).
5. **Option 1 (tmux for sdd) is rejected for now.**

Ceilings are `config.Config` fields (`HeadlessTriageMaxTurns`, `HeadlessTriageMaxSubagents`), applied to sdd-mode
triage only by default; `0` disables. Defaults are deliberately generous (below the #882 shape of 1,094 turns / 262
subagent completions, above a normal call) and are to be re-tuned from the new log fields.

## Rationale

- Failure mode is fan-out of *productive-looking* turns. `CapacityMonitor` (idle-wait `/compact`, #885) neither detects
  nor bounds that, so tmux would add cost without adding the needed control. Kill-on-threshold is already possible
  from a headless call via ctx/`terminateStream`; only *compaction* needs tmux, and no data shows compaction would help.
- Memory is not the argument against tmux: marginal ≈ 5 MB/session, ≈ 40 MB for 8, vs 2.1–3.4 GB for the 8 `claude`
  processes both models pay (findings, AC2). The argument is non-memory: session persistence, worktree/cleanup and
  orphan-reconcile complexity, and no control gained.

## Rejected alternatives

- **Option 1, tmux-backed sdd triage:** see above; revisit only if data shows a need for mid-flight compaction.
- **`claude --max-turns`:** blunt, counts only main-agent turns, gives no subagent count and no distinct error/logging.
- **Migrating all triage to tmux:** out of scope per the item; default triage shows no material waste.
- **Cost-only backstop (option 3 alone):** cost is invisible on cancelled calls and unknowable mid-stream without
  pricing; counts are available immediately and independent of the pricing table. Kept as a complementary layer.

## Interactions (AC6)

- **`triageSem` (cap 8):** unchanged. The slot is held for the call's life and released by `defer`; an early abort
  returns the slot *sooner*, so the ceiling only increases effective throughput. No extra semaphore.
- **`triageCallBudget` (3 h):** unchanged and still the absolute wall-clock bound, derived from the liveness engine per
  mode. The fan-out ceiling is an additional, earlier, orthogonal bound; whichever trips first wins and is distinguishable
  by `end_reason` (`fanout_ceiling` vs `timeout`).
- **Liveness / orphan reconciliation:** an abort ends the `item_sessions` row with `end_reason=fanout_ceiling` through
  the normal error path. `reconcileOrphanedTriageItems` only auto-respawns `end_reason=="shutdown"`; any other reason is
  MarkStuck'd with backoff (`session/backlog_lifecycle_triage.go:260-305`), so the abort surfaces to the operator and
  **cannot loop into repeat spend**. No reconciler change required.
- **#884 (dollar ceiling):** same hook (`handleFirstCallLine`) and same classifier. Both are independent accumulators
  with their own error values; whichever merges second adds its `case` beside this one. Expect a trivial textual merge.

## Consequences

+ Bounds the #882 shape without new infrastructure; aborts record turn/subagent counters (completed-call counters are a follow-up).
− A hard abort discards partial `project_plans/` output in the triage worktree, which is cleaned up on failure; the
  prompt-level budget exists to make a graceful finish the common case.
− Thresholds are initial guesses until the new log fields accumulate data (follow-up: re-tune after ≥2 weeks).
