# Requirements: Diagnose and instrument autonomous-mode session stalls

Backlog item `4a709ebc-51de-4e6a-8f1d-150cde3925bc`. Requirements derived directly from the
item description and acceptance criteria (no ideation interview — no user present).

## Problem

Autonomous-mode backlog sessions stop before finishing their work. Today every driver
exit that is not an explicit DONE is reported as one undifferentiated "stuck" outcome
(`AutonomousDriverOutcome{Stuck:true, Reason:<free text>}`), the instance only records
`AutonomousOutcome = "done" | "stuck"`, and the recovery path
(`onAutonomousDriverComplete` -> `AutoRespawnAutonomousWork`) ends the item session and
spawns a fresh one without first looking at what the session was doing.

## Acceptance criteria (verbatim from the item)

1. The root cause of autonomous-mode sessions stopping before completion is identified and
   documented, with each stop path cited to the code that produces it.
2. Each time an autonomous session stops, the stop reason is recorded and exposed in an
   observable form (logs, status, or a UI field) that distinguishes at least
   idle/awaiting-input, error, and completed-or-exited states.
3. Before a session is restarted, its prior state (last activity and output) is inspected
   and captured, and the restart path can choose a nudge over a full respawn when the
   session is only waiting.
4. Tests cover the stop-reason classification and the nudge-versus-respawn decision for
   each state.

## Derived requirements

- R1 (AC1): A diagnosis document enumerates every code path by which an autonomous driver
  stops, marks which are real stalls vs. mislabelled, and cites `file:line` for each.
- R2 (AC2): Driver exits carry a typed stop reason (not free text) covering at least:
  done, max-turns, startup-timeout, LLM/orchestrator error, submit failure, rate-limit
  exhausted, cancelled (external Stop), and panic. The reason is logged structurally,
  persisted on the stuck row's detail, and surfaced on the instance (status field).
- R3 (AC2): The *session's* observed state at stop time is classified into
  awaiting-input / idle / error / exited-or-completed, independent of why the driver stopped.
- R4 (AC3): Before any respawn, the pane's last activity and output tail are captured and
  recorded (log + stuck-row detail / notification body).
- R5 (AC3): A pure decision function maps (stop reason, session state, nudge allowance)
  to {nudge, respawn, none}. A session that is only waiting for input is nudged instead
  of respawned, bounded by the existing nudge cap so it cannot loop.
- R6 (AC4): Table-driven unit tests cover classification and the decision for every state.

## Non-goals / constraints

- No new proto fields unless needed for exposure; prefer extending existing strings
  (`autonomous_outcome` is already on the wire) to avoid regenerating clients.
- Must not weaken existing safeguards: archival checks (`noarchivedrevival`), rework cap,
  remediation backoff, nudge cap (`EvaluateDiagnoseNudgeAllowance`).
- An externally-requested stop (`Stop()` after request_review) must not be recorded as a
  stall or trigger a respawn.
