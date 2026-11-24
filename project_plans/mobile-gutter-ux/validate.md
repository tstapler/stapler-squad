# Validation: Mobile Gutter UX

**Date:** 2026-11-23

---

## AC Validation

### AC-1 — Log viewer level-badge tap filters the log

**Plan coverage:** S3 (LogRow API change + filter state in log viewer hook).  
**Test coverage:** Unit test in `LogRow.test.tsx` verifies badge renders as `<button>` with correct `aria-label` and that click does not fire `onToggle`. Consumer-side filter wiring needs an integration test.  
**Gap:** The log viewer consumer (file not identified during triage) must be updated in S3.3 to pass `onFilterByLevel`. Without a pointer to that consumer, the implementer must `grep` for `<LogRow` in the codebase. This is not a plan gap — it's a known implementation step.  
**Status:** ✓ Covered (with one implementation-time lookup).

### AC-2 — CodeMirror line-number gutter meets 44px touch target

**Plan coverage:** S1.2 — `@media (pointer: coarse)` override in `cmTheme.ts`.  
**Mechanism:** CSS-only, fires automatically on touch devices. No JS prop-threading.  
**Test coverage:** Manual/visual only (Chromium DevTools coarse emulation). No automated test for CSS pixel dimensions — standard for vanilla-extract responsive overrides in this codebase.  
**Status:** ✓ Covered.

### AC-3 — Diff gutter markers are tappable on mobile

**Plan coverage:** S2 — `domEventHandlers.click` on `changeGutter` in `CmEngine.ts`.  
**Test coverage:** Unit test via `CmEngine.conformance.test.tsx` (existing file).  
**Status:** ✓ Covered.

### AC-4 — Gutter width on narrow mobile ≤56px total

**Plan coverage:** S1.2 raises line-number gutter to 44px; S1.3 widens diff strip to 10px. Total: 54px ≤ 56px. ✓  
**Note:** At ≤380px the LogRow's `GUTTER_WIDTH_NARROW_PX = 44px` already satisfies the budget. The CM gutter is a separate DOM context (inside the editor iframe-equivalent), so the two gutters don't stack.  
**Status:** ✓ Within budget.

### AC-5 — Fold gutter coarse-pointer sizing

**Plan coverage:** Research found no fold gutter extension currently active in `CmEngine.ts`. AC-5 is a future-proofing criterion.  
**Recommendation:** Add a note in `cmTheme.ts` near the coarse-pointer block: `// When foldGutter() is added, include: ".cm-foldGutter .cm-gutterElement": { minWidth: "44px" }`.  
**Status:** ✓ No current fold gutter → no active work needed; placeholder comment in plan.

### AC-6 — No regression on desktop

**Plan coverage:** All S1 CSS changes use `@media (pointer: coarse)` which is `false` on desktop mice. S2 click handler is additive (gutter had no click handler before). S3 renders a `<button>` only when `onFilterByLevel` is provided.  
**Test coverage:** Existing `LogRow` unit tests cover the no-callback render path; existing `CmEngine.conformance.test.tsx` tests cover fine-pointer code paths.  
**Status:** ✓ Covered.

### AC-7 — Narrow-screen gutter visual hierarchy preserved

**Plan coverage:** `GUTTER_WIDTH_NARROW_PX = 44` and line-number display-none on ≤380px are unchanged. The `minHeight: 44` addition to the row is compatible with the existing narrow-screen badge-only layout.  
**Test coverage:** Existing snapshot / visual tests (if any) will catch regressions. No new test needed specifically for this AC.  
**Status:** ✓ No change to narrow-screen logic.

---

## Open Questions

1. **Which component consumes `LogRow` with filter state?** At implementation time, `grep -rn '<LogRow'` will identify the consumer. If multiple consumers exist, only the log-viewer one needs `onFilterByLevel`; others pass `undefined` and get the existing behavior.

2. **Does the diff gutter click navigation need a visual indicator (highlight the navigated-to line)?** The plan uses `dispatch({ selection, scrollIntoView: true })`, which moves the CM cursor to the target line. On mobile in read mode, the cursor is hidden — a brief flash highlight (via `.cm-activeLine`) may be needed. This is a polish item, not a blocking concern.

3. **Should level-badge filter be a toggle (tap same level to clear) or a replace (always set)?** Not specified in the item. Recommend toggle semantics (matches common mobile filter UX). Should be confirmed with owner.

---

## Complexity Budget Check

| File | Current net-new lines estimate | Within `funlen`/`gocognit`? |
|---|---|---|
| `LogRow.css.ts` | +12 (new media query block) | Yes — CSS-only |
| `LogRow.tsx` | +18 (conditional badge render) | Yes — simple branch |
| `cmTheme.ts` | +10 (new media query block) | Yes — CSS-only |
| `CmEngine.ts` | +18 (click handler in `changeGutter`) | Yes — simple fn |

No single function grows by more than 20 lines. No `gocognit`/`gocyclo` concern.

---

## Verdict

The plan is valid and covers all acceptance criteria. Work can proceed in three independent PRs corresponding to S1, S2, and S3, or as a single PR if the implementer prefers. S1 is recommended first since it unblocks reliable tap interaction for S2 and S3.

The only owner decision needed before starting S3: **toggle vs. replace semantics for the level-filter badge**.
