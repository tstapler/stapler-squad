# Implementation Plan: Diagnose Session Rich View & Workflow Updates

## Chosen Approach: Option B (Full — Proto field + new panel + new tools)

Rationale: The fragile text-parsing coupling in Option A is ongoing maintenance debt; structured proto fields enable clean UI and future extension. See `research.md` for the tradeoff analysis.

---

## Phase 1 — Proto & Backend Foundation

### Task 1.1 — Add `DiagnoseResult` proto message and `ItemSession.diagnose_result` field

**File:** `proto/session/v1/backlog.proto`

Add:
```protobuf
message DiagnoseResult {
  string outcome = 1;      // BUG_FILED | NOTE_ONLY | NUDGED | REMEDIATION_RESET | NO_ACTION
  string summary = 2;      // human-readable findings summary
  string filed_item_id = 3;  // ID of backlog item filed (outcome == BUG_FILED only)
  google.protobuf.Timestamp diagnosed_at = 4;
}
```

Add to `ItemSession`:
```protobuf
DiagnoseResult diagnose_result = 15;  // next available field number; verify against current schema
```

Run `make proto-gen` to regenerate. Verify `make build` passes.

**Backwards compat:** Old diagnose sessions have empty `diagnose_result.outcome`; the UI falls back to parsing the activity note when it's empty.

### Task 1.2 — Update `submit_diagnosis_result` to populate `ItemSession.diagnose_result`

**File:** `server/mcp/tools_diagnose.go` (the `submit_diagnosis_result` handler)

After writing the activity note, also call `storage.UpdateItemSessionDiagnoseResult(ctx, itemID, callerUUID, DiagnoseResult{...})` (new storage method — see 1.3).

Keep the activity note write for backwards compatibility. Add `filed_item_id` population when outcome is `BUG_FILED` and the agent passes the new item's ID.

### Task 1.3 — Add storage method `UpdateItemSessionDiagnoseResult`

**Files:** `session/storage.go` (or whichever file holds `AppendActivityNote`), `session/ent/schema/` if a schema change is needed.

The `ItemSession` ent schema (`session/ent/schema/`) needs a new JSON blob column for `diagnose_result` (protobuf-serialized or as a JSON struct) OR the storage layer can serialize the `DiagnoseResult` proto into the existing `metadata` column if one exists.

Check the ent schema before deciding. Keep the write atomic with the activity note append (wrap in a transaction or use ent's upsert).

### Task 1.4 — Add `suggest_workflow_action` MCP tool

**File:** `server/mcp/tools_diagnose.go`

New tool, only callable with `session_role === "diagnose"` (enforced by `denyIfDiagnoseCaller` pattern or a symmetric `allowOnlyDiagnoseCaller` check):

```go
// suggest_workflow_action
// Parameters: item_id string, recommendation string, action_type string
//   (action_type enum: "transition_status", "set_config", "operator_note")
// Effect: writes a BacklogWorkflowSuggestion record for the item, visible in the UI
```

New proto message in `proto/session/v1/backlog.proto`:
```protobuf
message WorkflowSuggestion {
  string suggestion_id = 1;
  string action_type = 2;    // transition_status | set_config | operator_note
  string recommendation = 3; // human-readable, e.g. "Move item back to ready — stall resolved"
  string session_uuid = 4;   // the diagnose session that filed this
  google.protobuf.Timestamp created_at = 5;
  bool dismissed = 6;
}
```

Add `repeated WorkflowSuggestion workflow_suggestions` to `BacklogItem`.

New RPC `DismissWorkflowSuggestion(suggestion_id)` in `proto/session/v1/backlog_service.proto` (or the existing backlog service) for the operator to dismiss from the UI.

**Gate:** Only diagnose sessions can call `suggest_workflow_action`. Operators can dismiss via the new RPC (no role restriction needed).

### Task 1.5 — Add `report_harness_gap` MCP tool

**File:** `server/mcp/tools_diagnose.go`

New tool (available to diagnose sessions; also reasonable to open to all sessions — decision: leave open to all since the gate isn't strictly necessary):

```go
// report_harness_gap
// Parameters: title string, evidence string, severity string (low|medium|high)
// Effect: creates a new BacklogItem with:
//   - tags: ["harness:bug", "auto-diagnose"]
//   - blocked_by: [<originating item id>]  (diagnose session knows its item ID)
//   - description: formatted with the evidence bundle (session UUID, item ID, summary)
//   - pipeline_mode: "harness-bug" if that mode exists, else default
// Returns: new_item_id string
```

The implementation wraps `CreateBacklogItem` (the existing storage call) with the extra metadata. The session role check (`denyIfDiagnoseCaller`) is NOT needed here since filing bugs is safe from any context.

Add `report_harness_gap` to `diagnosticSessionAllowedTools` in `session_service_diagnose.go` (it also should be available to triage/review sessions eventually, but scope it to diagnose for now).

---

## Phase 2 — UI: Session Classification Fix

### Task 2.1 — Fix `classifySessionKind` to handle `role === "diagnose"`

**File:** `web-app/src/lib/backlog/sessionKind.ts`

Add before the final fallback:
```typescript
if (session.role === "diagnose") return "diagnose";
```

Export `"diagnose"` as a new `SessionKind` variant (or reuse `"headless_diagnostic"` and add a sub-discriminator — see Task 2.2 for which is cleaner).

**Decision:** Use a new `"diagnose"` kind string rather than reusing `"headless_diagnostic"` — the components are structurally distinct enough that merging them adds conditional complexity that obscures intent.

Update `SessionsSection.tsx` to handle the new `"diagnose"` kind in the same `isSynthetic = true` branch as `"headless_diagnostic"` (renders as a collapsible row, not a plain `<a>` link).

### Task 2.2 — Build `DiagnoseSessionPanel` component

**File:** `web-app/src/components/backlog/detail/DiagnoseSessionPanel.tsx` (new)

Props: `session: ItemSession` (the diagnose `ItemSession` row from the item's `item_sessions` list)

Renders:
1. **Outcome badge** — from `session.diagnoseResult.outcome` (or parsed from activity note if empty, for old sessions)
2. **Summary text** — from `session.diagnoseResult.summary`
3. **Session metadata row** — start time, end time, cost (if `cost_usd > 0`)
4. **"View transcript →" link** — routes to `/?session=<session.sessionUuid>` (the hidden session terminal)
5. **Filed bug link** — if `diagnoseResult.outcome === "BUG_FILED"` and `filed_item_id` is set, render a link to that item
6. **Workflow suggestion** — if any `workflowSuggestions` on the parent item are linked to this session's UUID, render them with a "Dismiss" button

Wire `DiagnoseSessionPanel` into `SessionDiagnosticPanel.tsx`:
```typescript
if (session.diagnoseResult || session.role === "diagnose") {
  return <DiagnoseSessionPanel session={session} />;
}
```

### Task 2.3 — Enhance `DiagnoseResultCard` in ActivityLogSection

**File:** `web-app/src/components/backlog/DiagnoseResultCard.tsx`

Add: a "View session →" link using `note.authorSessionUuid` (already in `BacklogActivityNote`). Keep the existing outcome badge and summary. Remove the text-parsing if `ItemSession.diagnose_result` is populated (use the structured field instead via a new prop).

### Task 2.4 — Display `WorkflowSuggestion` cards in item detail

**File:** `web-app/src/components/backlog/detail/BacklogItemDetail.tsx` (or a new sub-section)

When `item.workflowSuggestions` has non-dismissed entries, render a `WorkflowSuggestionsSection` between the status row and the ACs section. Each suggestion shows:
- Action type badge (transition / config / note)
- Recommendation text
- "Dismiss" button → calls `DismissWorkflowSuggestion` RPC

---

## Phase 3 — Backlog Item Detail Feature Flag

### Task 3.1 — Feature flag `diagnose-session-panel`

**File:** `server/services/feature_flags.go` (or wherever flags are registered)

Add flag `diagnose-session-panel` (default off). Gate `DiagnoseSessionPanel` and `WorkflowSuggestionsSection` behind it. `FeatureGuard` wrapping is unnecessary since these are component-level, not page-level — use the `useFeatureFlag("diagnose-session-panel")` hook instead.

---

## Phase 4 — Tests

### Task 4.1 — Go unit tests for new MCP tools

**Files:** `server/mcp/tools_diagnose_test.go` (or new `_test.go` file)

- `TestSubmitDiagnosisResult_PopulatesItemSessionField` — verify `ItemSession.diagnose_result` is set after `submit_diagnosis_result`
- `TestSuggestWorkflowAction_CreatesSuggestion` — verify record written
- `TestSuggestWorkflowAction_DiagnoseRoleGate` — verify non-diagnose sessions are blocked (if gated)
- `TestReportHarnessGap_CreatesItemWithMetadata` — verify item created with correct tags + blocked_by

### Task 4.2 — Jest unit tests for new UI components

**Files:** `web-app/src/components/backlog/detail/DiagnoseSessionPanel.test.tsx` (new)

- `classifySessionKind_diagnose_role` — verify `"diagnose"` kind returned
- `DiagnoseSessionPanel_renders_outcome_badge` — snapshot + badge color
- `DiagnoseSessionPanel_renders_session_link` — verify `/?session=<uuid>` link
- `DiagnoseSessionPanel_fallback_to_activity_note` — verify graceful fallback when `diagnose_result` is empty (old sessions)
- `WorkflowSuggestionsSection_dismiss_button` — verify dismiss call fires

---

## Coordination Notes

- **`backlog-process-output-formatting` AC-3** — that plan's AC-3 ("diagnose results formatted card in Activity section") is superseded by this project. Update that plan's AC-3 to `status: "done_here"` and add a cross-reference.
- **Proto gen gate** — `make build` auto-runs `ent-gen` and `proto-gen` as dependencies; never commit generated files directly (see `CLAUDE.md` ent schema note).
- **`diagnosticSessionAllowedTools` string** — this is built as a comma-joined string in `session_service_diagnose.go`; add `report_harness_gap,suggest_workflow_action` there after the tools exist.

---

## Effort Estimates

| Phase | Tasks | Estimate |
|---|---|---|
| Phase 1: Proto + backend | 1.1–1.5 | 6h backend |
| Phase 2: UI | 2.1–2.4 | 5h frontend |
| Phase 3: Feature flag | 3.1 | 0.5h backend |
| Phase 4: Tests | 4.1–4.2 | 4h test |
| Total | | ~15.5h agent cost |
