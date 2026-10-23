# Validation: Backlog Process Output Formatting

**Date:** 2026-10-21  
**Phase:** SDD-4 Validate

---

## Validation Method

Adversarial review of `implementation/plan.md` against `requirements.md` and `research.md`.

---

## AC Coverage Check

| AC | Covered by plan? | Notes |
|----|-----------------|-------|
| AC-1 Triage results on item detail | ✓ Phase 4 + 7 | `TriageResultPanel` embedded in `BacklogItemDetailPanel` |
| AC-2 Review verdict on item detail | ✓ Phase 5 + 7 | `ReviewVerdictPanel` via `useReviewVerdict` hook |
| AC-3 Diagnose result as formatted card | ✓ Phase 6 | `DiagnoseResultCard` renders in Activity stream |
| AC-4 Graceful degradation (skeleton/hidden) | ✓ Phases 4, 5, 6 | States documented per component |
| AC-5 "View raw JSON" toggle | ✓ Phases 4, 5 | `<details>` disclosure with `JSON.stringify` |
| AC-6 Triage panel feature-flagged off by default | ✓ Phase 4 | `backlog-triage-results-panel` flag guard |
| AC-7 Review panel feature-flagged off by default | ✓ Phase 5 | `backlog-review-results-panel` flag guard |
| AC-8 Jest unit tests for new components | ✓ Phase 8 | All states listed per component |
| AC-9 No existing tests broken | ✓ Phase 8 | Minimal Go changes; proto only if field missing |
| AC-10 Mobile usability (≥ 320 px) | ✓ Rollout step 3 | Verify at 320 px before flag flip |

**Result:** All 10 ACs covered.

---

## Risk Validation

| Risk from research.md | Plan mitigation | Verdict |
|-----------------------|----------------|---------|
| `triage_result` not in proto | Phase 1.1 — verify first, add field if missing | Covered |
| `GetReviewVerdict` not accessible from web-app | Phase 1.2 — verify RPC registration, add hook if missing | Covered |
| Diagnose prefix parsing fragile | Phase 6 — fall back to plain text on non-match | Covered |
| jscpd duplication on two card CSS files | Adversarial review finding — shared `ResultPanel.css.ts` | Covered |
| Mobile overflow | Rollout step 3 — test at 320 px before flag flip | Covered |

---

## Gaps and Issues Found

### Issue 1: `BacklogItemDetailPanel` exact path not confirmed
**Severity:** Low  
**Description:** The plan assumes `web-app/src/components/backlog/BacklogItemDetailPanel.tsx` but this was inferred; the actual component name/path needs verification on first day of implementation.  
**Resolution:** Phase 7 says "Locate `BacklogItemDetail` component" — implementor should `Glob` for `**/backlog/**Detail*.tsx` before starting Phase 7.

### Issue 2: `useFeatureFlag` hook API not verified
**Severity:** Low  
**Description:** The plan uses `useFeatureFlag("flag-name")` pattern, which was observed in other components but the exact import path and boolean return type were not verified.  
**Resolution:** Check `web-app/src/lib/hooks/useFeatureFlag.ts` or equivalent before Phase 4.

### Issue 3: Feature flag registration for new flags
**Severity:** Medium  
**Description:** New feature flag names (`backlog-triage-results-panel`, `backlog-review-results-panel`) need to be registered in the feature flag registry (likely a Go constant + proto enum).  
**Resolution:** Add to Phase 1: register new flag names in the Go feature flag registry. Check `config/feature_flags.go` or equivalent.

### Issue 4: `DiagnoseResultCard` not feature-flagged
**Severity:** Low  
**Description:** AC-3 doesn't require a feature flag for diagnose results, and the plan doesn't add one. This is intentional — diagnose notes are low-stakes. But if the Activity list rendering is complex, it may be worth adding a guard.  
**Resolution:** Acceptable as-is; document the omission explicitly in the implementation PR.

### Issue 5: `useReviewVerdict` polling approach
**Severity:** Low  
**Description:** Polling at 5 s while `in_review` works but the existing codebase uses event-driven updates (EventBus → WebSocket). The plan should note that a follow-up can switch to event-driven once the WebSocket stream includes verdict-available events.  
**Resolution:** Add note to plan: "MVP uses polling; follow-up: subscribe to review-verdict event from WebSocket stream."

---

## Final Verdict

**Plan is valid for implementation.** Three minor issues require first-day verification (component path, flag hook API, flag registration). No blocking gaps. The adversarial review from the plan's Phase 9 already caught the CSS duplication risk and the route-meta gap. 

**Estimated implementation cost:**
- Backend (proto/flag registration): Low — 1 wave, mostly verification
- Frontend schemas + hooks: Low — 1 wave, straightforward zod + fetch
- Frontend components (3): Medium — 1 wave, 3 parallel component tasks
- Integration + tests: Low-Medium — 1 wave
- Total waves: ~4 sequential, with parallelism within each wave

**Recommended start:** Phase 1 (backend verification) → parallel Phase 2+3 → parallel Phase 4+5+6 → Phase 7+8+9.
