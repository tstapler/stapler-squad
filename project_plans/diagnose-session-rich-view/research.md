# Research: Diagnose Session Rich View & Workflow Updates

## Current State — Verified

### Session Classification (The Root Bug)

**File:** `web-app/src/lib/backlog/sessionKind.ts`

`classifySessionKind` checks five conditions in order:
1. `role === "triage"` OR `sessionId.startsWith("headless-")` → `"headless_diagnostic"` (renders `SessionDiagnosticPanel` → `TriageReviewPanel`)
2. `sessionId.startsWith("review-blocked-")` or `"diff-error-"` → `"blocked_guardrail"` (renders `SessionDiagnosticPanel` → `GateVerdictBox`)
3. `sessionId.startsWith("manual-review-")` → `"manual_review_marker"` (renders a marker notice)
4. `role === "review"` → `"review"` (renders a real session link + `ReviewVerdictPanel`)
5. fallback → `"work"` ← diagnose sessions land here

A diagnose session has `role === "diagnose"` and a plain UUID (not "headless-" prefixed — `SpawnDiagnosticSession` → `CreateDirectorySession` creates a real instance). It falls through to `"work"` and renders as a `<a href="/?session=<uuid}">` anchor. Because diagnose sessions are `Hidden=true`, navigating to that link opens a terminal view that is correct but not surfaced in the normal session list — the user has to already know the UUID. No panel, no structured view.

### SessionDiagnosticPanel Branch Gap

**File:** `web-app/src/components/backlog/detail/SessionDiagnosticPanel.tsx`

The panel handles three cases:
- `session.triageResult` populated → `TriageReviewPanel` (triage output)
- `session.reviewVerdict` populated → `GateVerdictBox` (review output)
- fallback → `BlockedNotice kind="missing_diagnostic_data"`

Diagnose `ItemSession` rows have neither `triageResult` nor `reviewVerdict` set. Even if `classifySessionKind` is fixed to route diagnose sessions here, they would hit the `missing_diagnostic_data` fallback with no meaningful output.

### submit_diagnosis_result — Storage Semantics

**File:** `server/mcp/tools_diagnose.go` (line 213)

```go
note := fmt.Sprintf("Diagnosis result: %s\n\n%s", outcome, summary)
dh.storage.AppendActivityNote(ctx, itemID, callerUUID, "diagnose", note)
```

The structured outcome (outcome enum + summary) is stored *only* as a free-text `BacklogActivityNote`. The `ItemSession` row for the diagnose session is not updated — no `diagnose_result` or equivalent field exists in the `ItemSession` proto.

**Consequence for the UI:** The activity note is queryable by `author_session_uuid` (which = the diagnose session's UUID), so the UI could reconstruct the per-session result by text-parsing the note. `DiagnoseResultCard` does exactly this — it looks for the `"Diagnosis result: "` prefix. This is a fragile coupling that breaks if the note format ever changes.

### DiagnoseResultCard — Current Rendering

**File:** `web-app/src/components/backlog/DiagnoseResultCard.tsx`

What it does:
- Parses the `"Diagnosis result: {OUTCOME}\n\n{summary}"` prefix
- Renders an `🩺`-headed card with an outcome badge (red = `BUG_FILED`, neutral gray = `NO_ACTION`, yellow = everything else)
- Shows the summary paragraph as plain text

What it doesn't show:
- No link to the diagnostic session terminal
- No session cost or timing metadata
- No structured breakdown of what was found/done (e.g., which bug was filed, which nudge was sent)
- No display of any workflow recommendations

### Backlog Item Workflow — What the Diagnose Agent Can't Update

The diagnose session's MCP tool surface (`diagnosticSessionAllowedTools` in `session_service_diagnose.go` lines 86–92):
```
Read, Grep, Glob, Bash,
create_backlog_item, post_backlog_update, get_backlog_item,
submit_diagnosis_result, diagnose_nudge_session, diagnose_reset_remediation
```

Blocked by `denyIfDiagnoseCaller` (in `server/mcp/diagnose_role_gate.go`):
- `transition_backlog_item_status`
- `update_backlog_item`
- `update_session`, `steer_session`, `write_to_session`, `resume_session` (non-diagnostic direct access)

**Result:** A diagnose session that correctly identifies "this item should be moved back to `ready` after the stall" can only post a free-text note suggesting that — it cannot do it, and there's no structured mechanism for the operator to see and act on that recommendation in one click.

### Harness Gap Filing — Current Path

The diagnose prompt instructs the agent: "If you find a genuine defect in the harness itself, use `create_backlog_item`."  
`create_backlog_item` creates a fresh item with arbitrary title/description. There is no structured link back to the diagnosed item (no `blocked_by` auto-wiring, no `harness:diagnose` tag, no evidence attachment). The result is an orphan bug item that an operator may not connect to the diagnose that found it.

### Related Project Plan

**`project_plans/backlog-process-output-formatting/`** (dated 2026-10-21, all ACs pending):
- AC-3 specifically: "Diagnose results rendered as a formatted card in the Activity section." — This is covered by the existing (but minimal) `DiagnoseResultCard`.
- That plan doesn't address: (a) Sessions section panel, (b) workflow updates, (c) harness gap tool.

**Decision:** This project supersedes `backlog-process-output-formatting`'s AC-3 and extends the scope significantly. The other ACs in that plan (triage results panel, review verdict panel) remain in scope there and are NOT duplicated here.

## Proto Data Model — Key Fields

**`BacklogItem` (`proto/session/v1/backlog.proto`):**
- `item_sessions` (repeated `ItemSession`) — includes `session_role`, `session_uuid`, `triage_result`, `review_verdict`, `ended_at`, `end_reason`, cost fields
- `activity_notes` (repeated `BacklogActivityNote`) — includes `author_session_uuid` (matches diagnose session UUID), `message`, `created_at`

**`ItemSession` (`proto/session/v1/backlog.proto`):**
- Currently has `TriageResult triage_result` and `ReviewVerdict review_verdict` — no `DiagnoseResult` equivalent
- Has `session_uuid`, `session_role`, `started_at`, `ended_at`, `end_reason`, `cost_usd`

**Gap:** No `DiagnoseResult` message in the proto, no `diagnose_result` field on `ItemSession`. The proto needs a new message type and field.

## UI Pattern Comparison

| Session type | classifySessionKind kind | Panel component | Data source |
|---|---|---|---|
| Triage | `headless_diagnostic` | `TriageReviewPanel` | `ItemSession.triage_result` |
| Re-review | `headless_diagnostic` or `blocked_guardrail` | `GateVerdictBox` | `ItemSession.review_verdict` |
| Review (real) | `review` | `ReviewVerdictPanel` (feature-flagged) | `ItemSession.review_verdict` |
| **Diagnose** | ~~`"work"` (bug)~~ → should be `"diagnose"` | `DiagnoseSessionPanel` (new) | `ItemSession.diagnose_result` (new) |

## Implementation Approach Options

### Option A: Minimal — Fix classification + reuse activity note
- Fix `classifySessionKind` to route `role === "diagnose"` to `"headless_diagnostic"`
- Add a diagnose branch to `SessionDiagnosticPanel` that looks up the matching activity note by `author_session_uuid` and renders an enhanced `DiagnoseResultCard` there
- No proto changes needed
- **Risk:** Fragile text-parsing coupling persists; can't add structured recommendations without changing the note format

### Option B: Full — Proto field + new panel + new tools (Recommended)
- Add `DiagnoseResult` proto message + `ItemSession.diagnose_result` field
- Update `submit_diagnosis_result` to populate `ItemSession.diagnose_result` (and still write the activity note for backwards compat)
- Fix `classifySessionKind` to add `"diagnose"` kind
- Build `DiagnoseSessionPanel` component (extends `SessionDiagnosticPanel` routing)
- Add `suggest_workflow_action` MCP tool (stores structured recommendation in a new `BacklogWorkflowSuggestion` proto message embedded in the item)
- Add `report_harness_gap` MCP tool (creates item with auto-tagging + cross-link)
- **Risk:** Proto migration, more surface to test; structured approach is worth it

**Recommendation: Option B.** The fragile text-parsing coupling in Option A creates ongoing maintenance debt every time the note format changes, and doesn't support the structured workflow-recommendations AC.

## Risk Analysis

1. **Proto migration** — adding `DiagnoseResult` to `ItemSession` is additive (no breaking change to existing sessions). Old diagnose sessions will have empty `diagnose_result`; the UI can fall back to parsing the activity note when `diagnose_result` is absent.

2. **`suggest_workflow_action` abuse** — the diagnose agent could spam workflow recommendations. Mitigated by: (a) diagnose sessions are one-shot (`OneShot=true`), (b) the action doesn't execute anything — it only stores a recommendation the operator must dismiss or act on.

3. **`report_harness_gap` and `create_backlog_item` duplication** — the agent already has `create_backlog_item`. The new tool must call through to the same storage path with extra metadata. Coordinate with the `denyIfDiagnoseCaller` gate to keep `report_harness_gap` exclusive to diagnose sessions if desired, or make it broadly available (simpler).

4. **`backlog-process-output-formatting` overlap** — this project supersedes AC-3 of that plan. Update that plan's AC-3 as "done here" or close it to avoid duplicate implementation work.
