# Requirements: Workflow Session Affordances

## Problem Statement

Sessions created by workflows are currently indistinguishable from manually-created sessions. Users have no way to know:
- Which workflow fired a session or what instructions it was given
- That stale/failed runs from previous workflow firings are accumulating
- How to clean them up in bulk or prevent future accumulation

The root cause surfaced during a debug session where 12+ orphaned "Knowledge Maintenance" sessions had to be hunted down and deleted manually via the DB because the UI provided no path to find or remove them.

## Goals

1. Make workflow sessions visually identifiable in the session list
2. Expose workflow metadata (name, description, command, schedule) in the session detail panel
3. Provide automatic and manual cleanup paths for stale workflow sessions

## Users

Primary: Power users running recurring cron workflows (Claude Code agents, knowledge maintenance bots, etc.) who need to audit what ran and clean up what didn't finish.

## Functional Requirements

### FR-1: Session List — Visual Identity
- **FR-1a**: Workflow sessions display a workflow badge/icon on the session card
- **FR-1b**: The workflow name (e.g. "Knowledge Maintenance") appears as a label/tag on the card
- **FR-1c**: "Workflow" is available as a grouping strategy (alongside existing: Category, Tag, Branch, Path, Program, Status, Session Type, None)
- **FR-1d**: The session list supports filtering by workflow ID (show only sessions from a specific workflow)

### FR-2: Session List — Default Visibility
- **FR-2a**: Workflow sessions are hidden from the default session list view
- **FR-2b**: A toggle or filter option exists to show workflow sessions ("Show workflow sessions")
- **FR-2c**: When grouped by Workflow, workflow sessions are always visible within their group

### FR-3: Session Detail Panel — Workflow Metadata
- **FR-3a**: Show the workflow name and description that created this session
- **FR-3b**: Show the initial command/prompt that was injected (the `initial_prompt` or `command` field)
- **FR-3c**: Show a link/button to open the workflow's configuration/settings page
- **FR-3d**: Show the timestamp when this session was fired and the workflow's cron schedule

### FR-4: Cleanup — Auto-Archive
- **FR-4a**: Workflow sessions are auto-archived after a configurable delay once they reach a completed/idle terminal state (default: 24 hours)
- **FR-4b**: The delay is configurable per-workflow in the workflow definition (e.g. `archive_after_hours: 24`)
- **FR-4c**: Auto-archive does not apply to the most recent N sessions for a workflow (default: keep 1) so the user can always see the last run

### FR-5: Cleanup — Per-Workflow Retention Settings
- **FR-5a**: Each workflow definition has an optional `keep_sessions` field (integer, default: 1): keep only the N most recent sessions, auto-archive the rest
- **FR-5b**: Each workflow definition has an optional `archive_after_hours` field (integer, default: 24): archive completed sessions after this many hours

### FR-6: Cleanup — Manual Bulk Action
- **FR-6a**: The workflow detail/settings page has a "Delete all sessions" or "Archive all sessions" action
- **FR-6b**: The action shows a count of sessions that will be affected before confirming
- **FR-6c**: A "Delete failed runs" variant that only targets sessions that never received their initial prompt (timed out / EIO)

## Non-Goals

- Workflow sessions do NOT get special runtime behavior (no read-only mode, no different interaction model)
- No re-run button on session cards (out of scope for this iteration)
- No changes to how workflow sessions are created or how prompts are injected

## Data Model Notes

Relevant fields already in the `sessions` table:
- `workflow_id` (text, nullable) — foreign key to the workflow that created this session
- `initial_prompt` (text, nullable) — the command/prompt injected at session start
- `archived_at` (datetime, nullable) — set to archive a session

The `WorkflowProto` message has: `id`, `slug`, `name`, `description`, `command`, `cron`, `input_template`, `target_directory`, `agent_type`, `model`, `session_type`.

New fields needed on `WorkflowProto` / workflow DB schema:
- `keep_sessions` (int32, default 1)
- `archive_after_hours` (int32, default 24)

## Acceptance Criteria

1. A user can look at the session list and immediately identify which sessions came from a workflow
2. Workflow sessions are hidden by default but easily discoverable via filter or grouping
3. A user can open a workflow session and read exactly what command was injected and which workflow scheduled it
4. After a workflow session completes, it is automatically archived within 24 hours (or the configured delay)
5. A user can delete/archive all sessions for a workflow in one action from the workflow page
6. Zero regressions in existing session list grouping, filtering, and display behavior
