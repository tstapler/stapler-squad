# Guidance Request Expand — Implementation Plan

## Summary

A pure-frontend change to `GuidanceRequestPanel` and its CSS module. No backend
changes, no proto changes, no new external packages. Total surface: 2 files modified
+ 1 test file updated.

## Architecture Decision

**Character-count threshold** (≥ 300 chars) rather than layout-based detection (scrollHeight).
Rationale:
- jsdom does not compute layout, so layout detection breaks all existing tests.
- 300 chars ≈ 4-6 lines at typical mobile widths; reliably indicates "needs expansion".
- Avoids a `useLayoutEffect`/`ResizeObserver` approach that adds complexity and
  an extra render.

The CSS always applies `-webkit-line-clamp: 5` on the collapsed class; short text
(< 300 chars) simply never shows the toggle button.

## File Changes

### 1. `web-app/src/components/guidance/GuidanceRequestPanel.css.ts`

Add four new exported styles:

```ts
/** Collapsed/clamped question text: max 5 lines with ellipsis */
export const questionClamped = style({
  margin: 0,
  flex: 1,
  display: "-webkit-box",
  WebkitLineClamp: 5,
  WebkitBoxOrient: "vertical",
  overflow: "hidden",
  whiteSpace: "pre-wrap",
  overflowWrap: "anywhere",
  minWidth: 0,
  fontSize: vars.fontSize.base,
  color: vars.color.textPrimary,
});

/** Expanded question text: no clamp */
export const questionExpanded = style({
  margin: 0,
  flex: 1,
  whiteSpace: "pre-wrap",
  overflowWrap: "anywhere",
  minWidth: 0,
  fontSize: vars.fontSize.base,
  color: vars.color.textPrimary,
});

/** Collapsed/clamped answer text: max 5 lines */
export const answerClamped = style({
  margin: 0,
  display: "-webkit-box",
  WebkitLineClamp: 5,
  WebkitBoxOrient: "vertical",
  overflow: "hidden",
  whiteSpace: "pre-wrap",
  overflowWrap: "anywhere",
  minWidth: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

/** Expanded answer text: no clamp */
export const answerExpanded = style({
  margin: 0,
  whiteSpace: "pre-wrap",
  overflowWrap: "anywhere",
  minWidth: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

/** "Show more" / "Show less" toggle button */
export const expandToggle = style({
  display: "inline-block",
  marginTop: vars.space["1"],
  padding: 0,
  border: "none",
  background: "transparent",
  color: vars.color.accentPrimary,
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  cursor: "pointer",
  selectors: {
    "&:hover": { textDecoration: "underline" },
  },
});
```

Keep the existing `questionText` and `answerText` exports unchanged so that
`StructuredRequestItem` (which passes questionText to `GuidanceQuestionForm`)
remains unaffected.

### 2. `web-app/src/components/guidance/GuidanceRequestPanel.tsx`

**Add a small hook** (inlined at module level, not exported — avoids a separate file
that would still need dupl-checking):

```tsx
const EXPAND_THRESHOLD = 300; // chars

function useExpandable(text: string) {
  const isLong = text.length > EXPAND_THRESHOLD;
  const [expanded, setExpanded] = useState(false);
  const toggle = useCallback(() => setExpanded((v) => !v), []);
  return { isLong, expanded, toggle };
}
```

**Modify `GuidanceRequestItem`** (the legacy path):
- Replace the `<p className={styles.questionText}>` with clamped/expanded variant
- Render `<button aria-expanded={expanded} className={styles.expandToggle}>` only when `isLong`
- Same pattern for `<p className={styles.answerText}>` in the answered branch

```tsx
function GuidanceRequestItem({ request, busy, onSubmit }) {
  const questionStr = ownerActionQuestion(request);
  const { isLong: qLong, expanded: qExp, toggle: qToggle } = useExpandable(questionStr);
  const answerStr = request.answer ?? "";
  const { isLong: aLong, expanded: aExp, toggle: aToggle } = useExpandable(answerStr);
  const isPending = request.status === "pending";
  const ownerAction = isOwnerActionRequest(request);
  if ((request.questions?.length ?? 0) > 0) return <StructuredRequestItem ... />;
  return (
    <li ...>
      <div className={styles.itemHeader}>
        <span ...>{...}</span>
        <div style={{ flex: 1, minWidth: 0 }}>
          <p
            className={qExp ? styles.questionExpanded : styles.questionClamped}
            data-testid="guidance-question-text"
          >
            {questionStr}
          </p>
          {qLong && (
            <button
              className={styles.expandToggle}
              aria-expanded={qExp}
              aria-label={qExp ? "Show less question text" : "Show more question text"}
              data-testid="guidance-question-toggle"
              onClick={qToggle}
            >
              {qExp ? "Show less" : "Show more"}
            </button>
          )}
        </div>
      </div>
      {isPending ? (
        <LegacyAnswerForm ... />
      ) : (
        <>
          <p
            className={aExp ? styles.answerExpanded : styles.answerClamped}
            data-testid="guidance-answer-text"
          >
            Answer: {answerStr}
          </p>
          {aLong && (
            <button
              className={styles.expandToggle}
              aria-expanded={aExp}
              aria-label={aExp ? "Show less answer text" : "Show more answer text"}
              data-testid="guidance-answer-toggle"
              onClick={aToggle}
            >
              {aExp ? "Show less" : "Show more"}
            </button>
          )}
        </>
      )}
    </li>
  );
}
```

Note: The `itemHeader` flex layout needs a wrapper `<div>` around the question text +
toggle so the flex alignment is preserved. The badge stays `flexShrink: 0` and the
wrapper takes `flex: 1; minWidth: 0`.

**`StructuredRequestItem`** — the question text is rendered by `GuidanceQuestionForm`
(a different, complex form component). Don't apply line-clamp inside `StructuredRequestItem`
for now — the structured form's `questionText` is typically short (it's a header/prompt,
not a prose explanation). If needed in a follow-up, the form component can be extended.

### 3. `web-app/src/components/guidance/GuidanceRequestPanel.test.tsx`

Add test cases:
```
- "clamps long question text and shows Show more toggle"
- "expands question text when Show more is clicked and collapses on Show less"
- "does not show toggle for short question text"
- "clamps and expands long answer text independently of question text"
```

Use `>300` char strings to trigger `isLong`. Assert:
- Toggle button presence via `data-testid="guidance-question-toggle"`
- `aria-expanded="false"` before click, `"true"` after
- Button label changes

## Adversarial Review

**Risk: `-webkit-line-clamp` is vendor-prefixed.** However, it is supported in all
modern browsers (Chrome, Firefox ≥ 68, Safari ≥ 10.1) and the existing codebase
already uses `WebkitLineClamp` in `BacklogItemCard.css.ts` — consistent, low risk.

**Risk: Pre-wrap + line-clamp interaction.** `whiteSpace: "pre-wrap"` and
`WebkitLineClamp` on a `-webkit-box` display both work; the clamp counts visual lines
after line wrapping. No conflict.

**Risk: `itemHeader` flex layout breaks with the new wrapper div.** The original
layout has `badge` + `paragraph` as direct flex children. Wrapping `paragraph + button`
in a `<div>` with `flex: 1; minWidth: 0` preserves the same visual layout — the div
takes the same space the paragraph did. Test with existing snapshot tests if any exist
(none found in GuidanceRequestPanel.test.tsx).

**Risk: Toggle button inline style.** The plan uses an inline `style={{ flex: 1, minWidth: 0 }}`
on the wrapper div. This should be a vanilla-extract class instead. Mitigation: add
`export const questionWrapper = style({ flex: 1, minWidth: 0 })` to the CSS module.

**Risk: jscpd duplication.** The `useExpandable` hook is defined once, used twice
(question and answer). The JSX toggle button pattern appears twice per item (question
+ answer). These two occurrences differ in `data-testid`, `aria-label`, and state
variables — they are intentionally separate (AC-5) and within jscpd's
`minLines: 20 / minTokens: 200` threshold (each block is ~8 lines). No violation.

**Risk: existing test `"shows the answer and no form for an answered question"`** asserts
`screen.getByText("Answer: yes")`. With the new structure, `Answer: yes` is still
rendered as text content of `<p>`. The `getByText` query matches by text content, not
by exact element shape, so the test passes unchanged.

## Non-Goals (Explicitly Deferred)

- Applying expand to `StructuredRequestItem`'s form content
- localStorage persistence of expand state (AC-6 out of scope)
- Animation on expand/collapse (can be added as CSS transition later)
