# Research: Backlog Process Output Formatting

**Date:** 2026-10-21  
**Phase:** SDD-2 Research

---

## 1. Current Data Storage

### Triage results
- **Storage:** `item_session.triage_result` column (raw JSON string, nullable).
- **Type:** `HeadlessTriageResult` in `session/backlog_triage.go`:
  ```go
  type HeadlessTriageResult struct {
      Title              string             `json:"title"`
      Summary            string             `json:"summary"`
      Priority           int                `json:"priority,omitempty"`
      ItemCategory       string             `json:"item_category,omitempty"`
      Suggestions        []TriageSuggestion `json:"suggestions"`
      Tasks              []TriageTask       `json:"tasks,omitempty"`
      AcceptanceCriteria []AcCriterion      `json:"acceptance_criteria,omitempty"`
      Iteration          int                `json:"iteration,omitempty"`
      Feedback           string             `json:"feedback,omitempty"`
      InputHash          string             `json:"input_hash,omitempty"`
  }
  ```
- **Written by:** `UpdateItemSessionTriageResult` (both headless `TriggerTriage` and interactive `submitTriageResult` MCP paths).
- **Proto exposure:** `GetBacklogItem` RPC returns an `ItemSession` message that includes `triage_result` as a raw JSON string field (confirmed via `proto/session/v1/backlog.proto`).

### Review verdicts
- **Storage:** `review_verdict` table, keyed by `item_session_id`.
- **Type:** `ReviewVerdictData{OverallOutcome string, PerCriterion []byte, Summary string, DiffHash string}`.
- **Written by:** `storage.SaveReviewVerdict` called from `submitReviewVerdict` MCP handler.
- **Proto exposure:** `GetReviewVerdict` RPC exists in `server/services/backlog_service_review.go`; returns `ReviewVerdictResponse`. Whether `PerCriterion` (a JSON `[]byte`) is exposed as typed proto fields or as raw bytes needs verification.

### Diagnose results
- **Storage:** `activity_note` table. Content is `"Diagnosis result: <OUTCOME>\n\n<summary>"` (text prefix, no structured field).
- **Outcome enum:** `BUG_FILED`, `NOTE_ONLY`, `NUDGED`, `NO_ACTION` (string constants in `server/mcp/tools_diagnose.go`).
- **No proto exposure** of structured diagnose results — currently rendered as unformatted text in the Activity section.

---

## 2. Existing UI Patterns to Reuse

### SessionSummaryPanel (closest precedent)
- `web-app/src/components/sessions/SessionSummaryPanel.tsx`
- Uses `ReactMarkdown` + `remarkGfm` for markdown string rendering.
- Feature-flagged with `// +feature: session-summary-tab`.
- Loading: animated 17-block skeleton.
- Error: graceful fallback with retry button.
- Tab: added to `SessionDetailView.tsx` tab strip, disabled while session is running.
- **Pattern to adopt:** the skeleton loading, feature-flag guard, and ReactMarkdown rendering approach.

### NotePanel
- `web-app/src/components/sessions/NotePanel.tsx`
- Simpler markdown render + edit toggle. Relevant for the "raw JSON" disclosure toggle pattern.

### BacklogItemDetailPanel (needs investigation)
- Key question: does a `BacklogItemDetailPanel` or equivalent already exist in `web-app/src/components/`?
- From project_plans/backlog-item-detail-ux/ requirements it exists — need exact path to understand existing card layout.
- Pattern for embedding new sections: look at how AC criteria are already rendered (they are shown as a list on the item detail).

### Feature-flag guard
- `web-app/src/app/FeatureGuard.tsx` — wraps route subtrees.
- Individual panel-level flags use `useFeatureFlag("flag-name")` hook pattern (as used in other components).

### Vanilla-extract CSS
- All new `.css.ts` files use `style()` / `recipe()` from `@vanilla-extract/css`.
- No Tailwind, no CSS modules, no inline styles.

---

## 3. Proto/RPC Gaps

To display triage results in the UI, the frontend needs typed fields. Current gaps:

1. **Triage result fields** — `item_session.triage_result` is a raw JSON string. The `GetBacklogItem` response likely surfaces it as a `string`; the frontend must `JSON.parse()` it. This is viable but fragile. A better approach: expose typed proto fields (`TaskEntry`, `SuggestionEntry`) in the `ItemSession` proto message, populated server-side when `triage_result` is parsed. However, this is a larger proto change. For MVP: parse on the client, validate schema with zod.
2. **Review verdict per-criterion** — `PerCriterion` is stored as `[]byte` (JSON). Same tradeoff: parse client-side or add typed proto fields.
3. **Diagnose result** — No structured storage at all. Two options: (a) parse the `"Diagnosis result: <OUTCOME>\n\n<summary>"` prefix client-side (fragile), or (b) add a `type` column to `activity_note` and a new enum-typed `outcome` field. For MVP: client-side prefix parsing.

---

## 4. Relevant Existing Components and Hooks

| Path | Relevance |
|------|-----------|
| `web-app/src/lib/hooks/useSessionSummary.ts` | Pattern for polling/fetching stored result |
| `web-app/src/lib/hooks/useBacklogItem.ts` (if it exists) | Would provide the triage_result field |
| `web-app/src/components/backlog/` | Container for new result panels |
| `session/backlog_triage.go` | `HeadlessTriageResult`, `TriageTask`, `TriageSuggestion` types |
| `session/backlog_review.go` | `CriterionVerdict`, `headlessVerdictJSON` types |
| `server/services/backlog_service_review.go` | `GetReviewVerdict` RPC handler |
| `proto/session/v1/backlog.proto` | Proto definitions to extend |

---

## 5. Feature Comparison: Session Tab vs. Backlog Item Detail

Two possible presentation surfaces:

### Option A: New tab in Session Detail View
- Pros: operator can open the session and immediately see structured results alongside terminal/diff.
- Cons: only accessible while/after the session exists; sessions may be archived/deleted before the operator reviews results; triage + review happen in different sessions (two tabs would be needed).

### Option B: Formatted sections on Backlog Item Detail View
- Pros: single place to see all pipeline results for an item regardless of session lifecycle; operators navigate to the item, not the session, to check status.
- Cons: requires adding new sections to the item detail view (already has AC, description, status, notes).
- **Verdict: preferred** — the backlog item is the durable artifact; sessions are ephemeral.

### Recommended: Both, with emphasis on Option B
- Primary: formatted result sections on the Backlog Item detail view.
- Secondary (follow-up): a "Results" tab in Session Detail for sessions that are pipeline sessions (triage/review/diagnose role).

---

## 6. Related Existing Work

- `project_plans/session-completion-summary/` — already shipped; provides the `SessionSummaryPanel` precedent.
- `project_plans/backlog-item-detail-ux/` — covers the existing item detail view layout; new sections must integrate with it.
- `project_plans/backlog-status-visibility/` — covers showing richer status metadata on items; aligned goal.
- `project_plans/backlog-agent-communication/` — covers agent-to-agent handoff context; separate concern.

---

## 7. Risk Assessment

| Risk | Likelihood | Mitigation |
|------|-----------|-----------|
| `item_session.triage_result` not surfaced via `GetBacklogItem` proto | Low — field exists in proto | Verify proto definition before implementing |
| `GetReviewVerdict` RPC not yet accessible from web-app | Medium — RPC exists but frontend hook may be absent | Check for existing `useReviewVerdict` hook; add if missing |
| Diagnose prefix parsing breaks on multi-line summaries | Low — prefix is always `"Diagnosis result: <OUTCOME>\n\n"` | Unit-test the parser |
| jscpd duplication gate trips on new structured card components | Medium — card layouts share structure | Vary component composition to avoid literal copy-paste; run `bun run lint:duplicates` in CI |
| Mobile layout overflow for wide task lists | Medium | Test at 320 px; use word-break and truncation |
