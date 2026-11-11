# Requirements: Diagnose Session Rich View & Workflow Updates

## Background

A diagnose session (`session_role = "diagnose"`) is a hidden, one-shot agent session spawned by the system when a backlog item is stuck. The diagnostic agent reads a `DiagnosticBundle` (item details, session logs, git state, activity history) and outputs one of five outcome buckets: `BUG_FILED`, `NOTE_ONLY`, `NUDGED`, `REMEDIATION_RESET`, `NO_ACTION`.

Currently diagnose sessions have three significant gaps:

1. **No rich view in the item detail page.** `classifySessionKind` falls through to `"work"` (no `"diagnose"` case), so diagnose sessions render as plain terminal links instead of structured diagnostic panels — analogous to how triage and review sessions get `TriageReviewPanel` / `ReviewVerdictPanel`.

2. **No workflow information updates.** `submit_diagnosis_result` only appends a free-text `BacklogActivityNote`. The diagnose agent cannot update the item's status, notes, pipeline configuration, or any workflow field — even when the diagnosis clearly warrants it (e.g., moving an item back to `ready` after resolving a stall, or flagging a dependency gap).

3. **No structured mechanism for harness-gap reporting.** The agent can call `create_backlog_item` to file an internal bug, but there's no structured `report_harness_gap` tool or GitHub issue creation surface — the existing `create_backlog_item` tool path lacks: (a) auto-categorization as a "harness" bug, (b) cross-linking to the diagnosed item, (c) any structured evidence attachment.

## Item Description (verbatim)

> Diagnose sessions still do not have a rich view or update the workflow information for the backlog item. I'd also like them to recommend and take actions or file bugs if they sense a gap in the harness.

## Goals

1. Surface diagnose session results as a first-class structured panel in the backlog item detail view — matching the richness of `TriageReviewPanel` (triage) and `ReviewVerdictPanel` (review).
2. Enable the diagnose agent to update the backlog item's workflow state: post structured notes, suggest or apply status transitions, and update item configuration fields.
3. Give the diagnose agent a structured tool for reporting harness gaps as trackable bugs, with cross-linking to the item under diagnosis.

## Non-Goals

- Giving the diagnose session unrestricted access to `TransitionBacklogItemStatus` or `UpdateBacklogItem` (preserves the safety of the diagnostic tool surface).
- Filing GitHub issues directly (too broad; keep it as a future extension hook).
- Rewriting the existing `DiagnoseResultCard` from scratch (it can be enhanced rather than replaced).

## Acceptance Criteria

1. When a backlog item has at least one diagnose `ItemSession`, the Sessions section of the item detail page shows a collapsible diagnose panel (new `DiagnoseSessionPanel` component or an extended `SessionDiagnosticPanel` branch) displaying: outcome badge, summary text, session timing, session cost (if available), and a "View transcript" link to the hidden session's terminal.

2. The `classifySessionKind` function in `sessionKind.ts` returns a dedicated `"diagnose"` kind (or routes into `"headless_diagnostic"`) for sessions with `role === "diagnose"`, so they no longer fall through to `"work"`.

3. `submit_diagnosis_result` stores the structured outcome (outcome enum + summary) in the `ItemSession` row (a new `diagnose_result` field or an equivalent proto extension) in addition to posting the activity note, so the UI can read it without parsing the activity note text.

4. The diagnose MCP tool surface includes a `suggest_workflow_action` tool that lets the agent post a structured operator recommendation to the item (e.g., "transition to ready", "set skip_review_gate=true") — visible as a dismissible card in the item detail view — without directly executing the transition.

5. The diagnose MCP tool surface includes a `report_harness_gap` tool that creates a new backlog item pre-categorized as a harness bug, automatically tagged with `harness:diagnose`, linked to the originating item via `blocked_by`, and carrying the structured evidence bundle (session UUID, item ID, outcome, summary) in its description.

6. The `DiagnoseResultCard` in the Activity Log is enhanced to include: a session link (using the `author_session_uuid` already present in `BacklogActivityNote`), the outcome badge (already present), and any structured workflow recommendations posted for that session.

7. All new MCP tools respect the `denyIfDiagnoseCaller` gate pattern — they are only callable by sessions with `session_role === "diagnose"`.

8. All new UI components are covered by unit tests (Jest) and the new `report_harness_gap` and `suggest_workflow_action` tools are covered by Go unit tests verifying their persistence semantics and gate enforcement.

## Related Work

- `project_plans/backlog-process-output-formatting/` — overlapping plan covering AC-3 (diagnose results formatted card). This project supersedes/subsumes that plan's AC-3 and extends it significantly.
- `server/mcp/tools_diagnose.go` — existing MCP tools: `submit_diagnosis_result`, `diagnose_nudge_session`, `diagnose_reset_remediation`
- `server/mcp/diagnose_role_gate.go` — `denyIfDiagnoseCaller` gate
- `web-app/src/lib/backlog/sessionKind.ts` — `classifySessionKind` (the classification bug)
- `web-app/src/components/backlog/detail/SessionDiagnosticPanel.tsx` — the panel that needs a new diagnose branch
- `proto/session/v1/diagnose.proto` — DiagnosticService RPCs
- `proto/session/v1/backlog.proto` — BacklogItem, ItemSession data model
