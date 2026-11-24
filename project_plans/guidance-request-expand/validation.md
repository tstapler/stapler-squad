# Guidance Request Expand — Validation

## Scope Validation

The plan addresses all 10 acceptance criteria from requirements.md:

| AC | Coverage | Notes |
|----|----------|-------|
| AC-0 | ✅ | `questionClamped` with `WebkitLineClamp: 5` applied by default when `isLong` |
| AC-1 | ✅ | "Show more" button rendered when `isLong && !expanded` |
| AC-2 | ✅ | "Show less" button rendered when `isLong && expanded` |
| AC-3 | ✅ | `useExpandable` returns `isLong = false` for text ≤ 300 chars; no button rendered |
| AC-4 | ✅ | Separate `useExpandable` call for `answerStr` with its own state |
| AC-5 | ✅ | Each `GuidanceRequestItem` instance has its own hook instances |
| AC-6 | ✅ | Plain `useState` — resets on page reload; no localStorage call |
| AC-7 | ✅ | `<button aria-expanded={...} aria-label="...">` — keyboard-focusable by default |
| AC-8 | ✅ | 4 new test cases added to `GuidanceRequestPanel.test.tsx` |
| AC-9 | ✅ | Single `useExpandable` hook; toggle JSX blocks are ~8 lines each, under jscpd 200-token threshold |

## Constraint Validation

| Constraint | Status |
|-----------|--------|
| Vanilla-extract only (no inline style for layout) | ⚠️ Plan draft uses inline style on wrapper div; fixed in plan addendum: use `styles.questionWrapper` class |
| No new external dependencies | ✅ Uses only React `useState`/`useCallback` and vanilla-extract |
| No routing/navigation changes | ✅ |
| jscpd threshold (0.14%) | ✅ Two ~8-line button blocks are below 20-line/200-token minimum |
| dupl (Go) | ✅ No Go changes |

## Risk Assessment

**Low risk overall.** This is a purely additive frontend change:
- No backend surface exposed
- No proto changes
- No new npm packages
- 2 files modified, 1 test file extended
- The existing pattern (`WebkitLineClamp` in `BacklogItemCard.css.ts`) proves browser
  compatibility within this project

**One medium risk**: The `itemHeader` flex restructuring (wrapping the `<p>` in a
`<div>`). The plan mitigates this by keeping the wrapper's CSS class identical in flex
behavior to the original `<p>`'s flex properties. No existing snapshot test covers
`GuidanceRequestItem`'s DOM structure, so regression is test-detectable only through
the new tests and manual review.

## Pre-Implementation Checklist

- [ ] Confirm jscpd `minLines: 20` / `minTokens: 200` values in `web-app/.jscpd.json`
      (assumed from CLAUDE.md reference — verify before implementing)
- [ ] Run `bun run lint:duplicates` after the change to verify gate still passes
- [ ] Check `StructuredRequestItem` — confirm its `questionText` paragraph is short
      enough to not need expansion in practice (confirmed by looking at the structured
      form: it shows `request.questionText` as a header, typically 1-2 sentences)

## Estimated Complexity

- **CSS changes**: 30 min (5 new style exports)
- **GuidanceRequestPanel.tsx**: 1-1.5h (hook + JSX restructuring + careful flex layout preservation)
- **Tests**: 1h (4 new cases, 300+ char fixture strings, aria assertions)
- **Manual verification**: 30 min (backlog item detail, mobile viewport, answered + pending states)

**Total**: ~3-4h of implementation work, all frontend.
