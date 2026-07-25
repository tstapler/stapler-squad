# Requirements: Triage Autonomous Migration

## Target User

**Operator** — a developer using Stapler Squad's backlog feature to manage AI-driven implementation tasks. The operator adds backlog items, triggers triage, reviews triage output, approves plan artifacts, and spawns work sessions. They interact with the backlog panel in the web UI and monitor session progress.

## Problem Statement

Two interconnected bugs were identified in the backlog/triage subsystem:

1. **Hidden session leak**: `WatchSessions` sends all sessions in the initial snapshot without filtering `inst.Hidden`. Triage and review sessions (created with `hidden: true`) appear in the main session list for every client. `ListSessions` correctly filters hidden sessions but `WatchSessions` does not.

2. **Missing autonomous orchestration in triage sessions**: `TriggerTriage` and `TriggerReReview` create sessions with `oneShot: true` (Claude runs `-p --output-format json`). Research confirmed that `claude -p` mode runs without an MCP server connection — meaning `submit_triage_result` **can never be called** from a oneShot triage session. Triage was silently broken. Fix: switch to non-oneShot + AutonomousDriver orchestration.

3. **No goal/status tracking for autonomous sessions**: `AutonomousDriver` sessions have zero structured goal/status reporting. The `set_session_goal` MCP tool and `session_goals` DB table exist but nothing populates them for autonomous sessions. Operators have no visibility into what a running triage or work session is doing. **This is explicitly deferred to a follow-on.** See "Deferred" section below.

A fourth question surfaced during ideation and was answered by research: `OneShot` mode and `AutonomousDriver` are **complementary, not duplicative**. OneShot = structured JSON output with no MCP access. AutonomousDriver = multi-turn interactive sessions with full MCP access. Triage requires MCP (`submit_triage_result`), so it must use AutonomousDriver. No migration of non-backlog oneShot usages is planned.

## Research Questions

1. **Architecture audit**: Map every use of `OneShot: true` and `AutonomousDriver` across the codebase. Identify semantics, lifecycle, and overlap.
2. **Duplication analysis**: Is `OneShot` (Claude `-p` flag, single LLM turn, exits) truly duplicative with `AutonomousDriver` (external orchestrator, multi-turn injection)? What does each do that the other cannot?
3. **Migration feasibility**: Can triage/review sessions switch from `oneShot: true` to `AutonomousDriver` without behavioral regression? What is the impact on `isOneShot()` tag-based no-retry logic?
4. **Completion signal design**: `AutonomousDriver` completion is currently detected by the orchestrator LLM reading the terminal tail and saying `DONE:`. For triage, `submit_triage_result` is the canonical completion signal. How should the two be wired together?

## Acceptance Criteria

### AC-1: WatchSessions hidden filter
- `WatchSessions` initial snapshot MUST NOT send sessions where `inst.Hidden == true` (unless the request sets `IncludeHidden`).
- Real-time events MUST NOT forward `SessionCreated` / `SessionUpdated` events for hidden sessions to clients that did not request hidden sessions.
- Existing behavior for non-hidden sessions is unchanged.

### AC-2: Triage sessions use AutonomousDriver
- `TriggerTriage` creates sessions without `OneShot: true` when a headless pool is available.
- An `AutonomousDriver` is started on each new triage session with the triage prompt as the goal.
- When `headlessPool` is nil, `TriggerTriage` falls back to `oneShot: true` (graceful degradation — mirrors `SpawnSessionFromItem` pattern).
- The same change applies to `TriggerReReview`.

### AC-3: Completion signal strategy
- **Triage sessions** (`submit_triage_result`): Completion is detected solely by the orchestrator LLM reading the terminal tail and emitting `DONE:`. No MCP-triggered `Stop()` call is used for triage because `AutonomousDriver.Stop()` unconditionally fires `fireCompletion(Stuck=true)` — calling it from the MCP tool would flag a successful triage as stuck. LLM detection is sufficient: after the agent calls `submit_triage_result`, the session becomes idle, the orchestrator sees the successful call in the tail, and emits `DONE:`.
- **Re-review sessions** (`submit_review_verdict`): A belt-and-suspenders explicit stop IS added. Because the role-aware completion callback (AC-5) skips all status transitions for `SessionRoleReview`, the spurious `Stuck=true` signal from `Stop()` is harmless.
- The orchestrator LLM MUST also detect completion from the terminal tail for both session types (existing AutonomousDriver behavior unchanged).

### AC-4: Architecture clarity (research output, not code)
- The research phase MUST produce a clear decision: either (a) `OneShot` and `AutonomousDriver` serve distinct purposes and both should be retained, OR (b) `OneShot` is subsumed by `AutonomousDriver` and a migration path is defined.
- If (b): include a migration plan for non-backlog `OneShot` usages (e.g. `CreateSession` with `oneShot` from the frontend).

### AC-5: Stuck-triage notification
- When `onAutonomousDriverComplete` fires with `Stuck=true` for a session with `SessionRoleTriage`, a `NotificationEvent` MUST be published to the event bus so the operator is informed.
- Notification payload: title = "Triage stuck", body = the backlog item's title, actionable = true. Badge severity = warning.
- When `Stuck=true` for `SessionRoleTriage`, the item status MUST remain unchanged (existing Epic 4 behaviour). The notification is the only signal; no automatic status change.
- Re-trigger affordance (UI) is explicitly out of scope for this change but is documented as a follow-on. See "Deferred" section.

## Success Metrics

These three signals confirm the fix is working after ship. All are observable without dedicated telemetry.

| # | Signal | How to verify |
|---|--------|---------------|
| SM-1 | Hidden sessions do not appear in the main session list | Start a triage job; confirm `WatchSessions` stream (via browser DevTools or `grpc-dump`) never emits the triage session. Existing `ListSessions` hidden filter is the control. |
| SM-2 | `submit_triage_result` is called successfully at least once | Check `~/.stapler-squad/logs/stapler-squad.log` for `submit_triage_result` log line after a real triage run. (Previously impossible from oneShot mode — confirming it appears verifies MCP access is restored.) |
| SM-3 | Backlog item advances from `idea` to `ready` after triage | Trigger triage on an `idea` item; confirm item status reaches `ready` in the backlog panel after the autonomous session completes. |

## Scope Boundaries

**In scope:**
- `server/services/session_service.go` — `WatchSessions`, `CreateDirectorySession`
- `server/services/backlog_service.go` — `TriggerTriage`, `TriggerReReview`
- `server/mcp/tools_backlog.go` — `submit_triage_result` completion hook
- `session/autonomous_driver.go` — completion callback wiring
- Research document on OneShot vs AutonomousDriver architecture

**Out of scope:**
- Migrating non-backlog `OneShot` usages (frontend `CreateSession` with `oneShot: true`) — that is a follow-on if research recommends it.
- Changes to `ReviewQueuePoller` hidden-session filtering (already correct via `shouldSkipSession`).
- UI changes (re-trigger affordance for stuck triage, triage-in-progress visual state, triage artifact viewer) — see "Deferred" section.
- Goal/status tracking for autonomous sessions — see "Deferred" section.

## Constraints

- All changes must pass `make quick-check` (build + test + lint).
- No breaking changes to the `CreateDirectorySession` interface signature.
- `isOneShot()` tag-based no-retry logic in `session_driver.go` must remain correct regardless of whether the `OneShot` field is set.
- Graceful degradation: everything must work when `headlessPool` is nil.

## Deferred

The following are explicitly out of scope for this change. They are named here so they can be tracked as follow-on tickets rather than silently forgotten.

### D-1: Goal and status tracking for autonomous sessions
`AutonomousDriver` sessions currently have zero structured goal/status reporting. The `set_session_goal` MCP tool and `session_goals` DB table exist but nothing calls them. Operators have no visibility into what a running triage or work session is doing while it runs.

**Why deferred**: Implementing goal/status tracking requires changes to `AutonomousDriver`'s system prompt, the `AppendSystemPrompt` field, and potentially a new turn-level MCP callback. This is a self-contained improvement that can ship independently. Adding it here would widen scope and delay the hidden-filter + triage-MCP fixes.

**Follow-on work**: A future ticket should: (a) set session goal on `TriggerTriage`/`TriggerReReview` via `set_session_goal`, (b) instruct the agent in the system prompt to call `update_session_task` at key milestones, (c) surface the current task in the backlog item panel.

### D-2: Re-trigger affordance for stuck triage (UI)
When triage is stuck (AC-5 emits a notification), there is no UI button to re-trigger triage from the backlog item detail panel. The operator must navigate to the item and re-trigger manually through the API or CLI.

**Why deferred**: UI-only change; does not affect correctness of the fix. The backend notification (AC-5) is the minimum to make stuck triage observable. The re-trigger button can be added in a UI-focused follow-on sprint.

### D-3: "Triage in progress" visual state on backlog items
While triage is running, the backlog item shows no in-progress indicator. All states (queued / running / stuck / done) look identical until the item status changes.

**Why deferred**: Requires a new backlog item state and a polling or push update from the triage session to the item. Out of scope for the correctness fix.
