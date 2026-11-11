# Validation: Diagnose Session Rich View & Workflow Updates

## Adversarial Review

### Challenge 1: Is the proto approach overkill — can we just fix the classification bug and read the activity note?

**Argument for minimal approach (Option A):** `DiagnoseResultCard` already parses the `"Diagnosis result: "` prefix from the activity note and renders it. `BacklogActivityNote` already carries `author_session_uuid`. To get a "rich view" in the Sessions section, we could:
1. Fix `classifySessionKind` (10-line change)
2. Add a diagnose branch to `SessionDiagnosticPanel` that looks up the matching activity note by `author_session_uuid` and renders the existing `DiagnoseResultCard`

No proto changes needed. Much smaller scope.

**Counter-argument (why the full approach is still right):**
- The text-parsing coupling (`fmt.Sprintf("Diagnosis result: %s\n\n%s", ...)` → parsed in `DiagnoseResultCard`) is a maintenance trap. Any format tweak (e.g., adding a third section separator) silently breaks all past diagnose result rendering.
- Workflow suggestions (`suggest_workflow_action`) REQUIRE a new storage field regardless — there's nowhere to put structured operator recommendations in the current data model.
- `report_harness_gap` is a tool call, not a UI change — it's always needed once we commit to "recommending and filing bugs."
- Verdict: **the full approach is proportionate given that two of the three main features (workflow actions, harness gaps) require backend changes anyway.**

### Challenge 2: `suggest_workflow_action` could be a footgun — the agent posts recommendations nobody reads

**Risk:** Diagnose sessions run automatically, hidden, and rarely. If `WorkflowSuggestion` records pile up without the operator noticing, they become noise.

**Mitigation already in plan:**
- `WorkflowSuggestionsSection` is surfaced prominently in item detail (between status and ACs)
- Each suggestion has a "Dismiss" button — zero-friction cleanup
- Diagnose sessions are `OneShot=true` and infrequent — suggestion volume is low

**Additional mitigation (add to plan):** Auto-expire `WorkflowSuggestion` records after 7 days if not dismissed. The ent schema can handle this with a `expires_at` field + a background sweep. Add this to Task 1.4.

### Challenge 3: `report_harness_gap` vs. `create_backlog_item` — why add a second tool?

**Risk:** The agent already has `create_backlog_item`. Adding `report_harness_gap` adds complexity without fundamentally new capability.

**Decision:** `report_harness_gap` is primarily a prompt-ergonomics tool: it sets defaults (tags, cross-link, evidence format) that the agent would have to construct manually with `create_backlog_item`. Without it, harness bugs filed by diagnose sessions are indistinguishable from normal backlog items. The structured defaults justify the new tool. This is a narrow, low-risk tool call.

**Validation:** Confirmed that `create_backlog_item` does NOT auto-link to the originating item or apply any tags — the new tool's added value is real, not theoretical.

### Challenge 4: Proto field numbering — will adding `diagnose_result` to `ItemSession` conflict?

**Risk:** Proto field numbers must not conflict with existing fields. We need to verify the next available field number in `ItemSession` before assigning.

**Verification needed during implementation:** `grep -n "= [0-9]" proto/session/v1/backlog.proto | grep -A5 "message ItemSession"` to find the highest existing field number. The plan says "field 15 — verify" — this must be confirmed before merging.

**Also check:** `WorkflowSuggestion` and `repeated workflow_suggestions` on `BacklogItem` — verify no field number conflict with existing fields (item is a large message; field numbers above ~45 are likely safe, but must be confirmed).

### Challenge 5: Hidden session link — does `/?session=<uuid>` actually work for hidden sessions?

**Risk:** Hidden sessions are excluded from the default session list. Does navigating to `/?session=<uuid>` correctly surface a hidden session's terminal?

**Verified (from research):** `StuckItemDetail.tsx` already renders a "View diagnostic session →" link to `routes.sessionDetail(dispatchedSessionUuid)` immediately after dispatching a diagnose session. This link works — the route renders the session terminal regardless of the `Hidden` flag. The `DiagnoseSessionPanel` can safely use the same route.

### Challenge 6: Old diagnose sessions (no `diagnose_result` field) — UI fallback

**Risk:** Existing diagnose sessions in production have no `diagnose_result` on their `ItemSession` row. After deployment, their `DiagnoseSessionPanel` would render an empty outcome badge.

**Mitigation in plan:** The panel checks `if (session.diagnoseResult?.outcome)` — if empty, it falls back to parsing the matching activity note. This fallback path keeps old sessions looking correct. After 30 days (once all active items have been rediagnosed), the fallback can be dropped.

---

## Validation Checklist

- [ ] Proto field numbers verified (ItemSession + BacklogItem) — no conflicts
- [ ] `make proto-gen` && `make build` passes after proto change
- [ ] `classifySessionKind` unit test added for `role === "diagnose"` → `"diagnose"` kind
- [ ] `DiagnoseSessionPanel` renders correctly with structured `diagnose_result` and falls back gracefully for old sessions with empty field
- [ ] `suggest_workflow_action` gate enforcement tested: non-diagnose session gets rejected
- [ ] `report_harness_gap` creates item with correct `tags`, `blocked_by`, and description format
- [ ] `WorkflowSuggestionsSection` dismiss button fires `DismissWorkflowSuggestion` RPC and removes the card
- [ ] Feature flag `diagnose-session-panel` gates both `DiagnoseSessionPanel` and `WorkflowSuggestionsSection`
- [ ] `backlog-process-output-formatting` AC-3 cross-reference added to that plan

## Open Questions

1. **`WorkflowSuggestion` auto-expiry** — 7-day TTL seems right for items in `in_progress`; should it be longer for items in `review` or `queued` where diagnose sessions run less often? Recommend: start with 14 days, operator-configurable.

2. **`report_harness_gap` scope** — should it be available only to diagnose sessions (via `denyIfDiagnoseCaller` inversion), or to all session roles? Filing harness bugs from review sessions could also be useful. Recommend: open to all sessions to maximize coverage, since there's no harm in a work/review session filing a harness bug.

3. **`suggest_workflow_action` execution** — should the UI offer a one-click "Apply" button that actually executes the transition (e.g., calls `TransitionBacklogItemStatus` on behalf of the operator)? Recommend: not in v1 — just show the recommendation; the operator applies it manually. Add a "Quick Apply" button in v2 after UX validation.
