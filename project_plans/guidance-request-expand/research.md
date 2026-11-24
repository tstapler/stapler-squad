# Guidance Request Expand — Research

## Current Implementation (verified by Read)

### GuidanceRequestPanel.tsx
`web-app/src/components/guidance/GuidanceRequestPanel.tsx`

Three rendering paths:
1. **Legacy request (yes/no or short-answer)** — `GuidanceRequestItem`:
   - `itemHeader` flex row: `[badge] [questionText paragraph]`
   - Below: `LegacyAnswerForm` (pending) or `answerText` paragraph (answered)
2. **Structured request** — `StructuredRequestItem`:
   - `itemHeader` flex row: `[badge] [questionText paragraph]` (answered only)
   - `GuidanceQuestionForm` renders the structured form/readonly view
3. Panel has no expand affordance; no line-clamp in CSS.

### GuidanceRequestPanel.css.ts
`questionText` style:
```ts
export const questionText = style({
  margin: 0, flex: 1, whiteSpace: "pre-wrap",
  overflowWrap: "anywhere", minWidth: 0,
  fontSize: vars.fontSize.base, color: vars.color.textPrimary,
});
```
`answerText` style:
```ts
export const answerText = style({
  margin: 0, whiteSpace: "pre-wrap", overflowWrap: "anywhere",
  minWidth: 0, fontSize: vars.fontSize.sm, color: vars.color.textSecondary,
});
```
Neither has `overflow: hidden` or `-webkit-line-clamp`. The visual truncation in the
screenshot is caused by the parent container's layout constraints at mobile viewport
width — the flex container forces the text to overflow without clipping its own
height, but the parent's scroll container makes it appear cut off.

### Collapsible.tsx (existing section-level component)
`web-app/src/components/ui/Collapsible.tsx` — Radix Accordion `type="multiple"`.
Removes content from DOM when collapsed. **Not appropriate here**: we need text clamp
(content stays in DOM but is visually truncated), not DOM removal. A section-level
collapse of the entire panel was considered and rejected — it hides answered state and
the answer text too, losing too much context.

### CSS Architecture
Vanilla-extract (`*.css.ts`) is the only approved CSS mechanism. No inline `style={{}}`.
The clamp pattern in vanilla-extract:
```ts
export const questionClamped = style({
  display: "-webkit-box",
  WebkitLineClamp: 5,
  WebkitBoxOrient: "vertical",
  overflow: "hidden",
  whiteSpace: "pre-wrap",
  overflowWrap: "anywhere",
});
export const questionExpanded = style({
  whiteSpace: "pre-wrap",
  overflowWrap: "anywhere",
});
```

### Existing Expand Patterns in This Codebase
- `BacklogItemCard.css.ts` line 80-82: `overflow: "hidden"; WebkitLineClamp: 2` — same
  pattern for the card description preview.
- `useSectionExpandState` (`web-app/src/lib/hooks/useSectionExpandState.ts`) — for
  section-level localStorage persistence; overkill for per-item ephemeral state.
- No existing "show more / show less" text-expansion component found; this is a new,
  lightweight pattern.

### Testing Infrastructure
`GuidanceRequestPanel.test.tsx` mocks `useListGuidanceRequestsQuery` and
`useAnswerGuidanceRequestMutation`. Tests verify DOM text presence. For the new
toggle, tests can:
- Render with long text
- Assert toggle button presence and `aria-expanded` attribute
- Click toggle and assert text container's `data-expanded` attribute (or query the
  button text change)

Note: jsdom does not compute `scrollHeight`/`clientHeight`; `useRef`-based overflow
detection (read layout) won't work in tests. Use a **character-count threshold** (e.g.,
> 300 chars) or a fixed **line-clamp CSS class always applied** with the button's
presence determined by content length. Character count is reliable in jsdom.

### jscpd / Duplication Gate
Current threshold: 0.14% (`web-app/.jscpd.json`). The show-more/show-less block is
~15 lines of JSX and CSS. If implemented as a shared `ExpandableText` component or a
`useTextExpand` hook, duplication is zero. If inlined separately in both `GuidanceRequestItem`
and `StructuredRequestItem`, that ~15-line block would be a jscpd finding. **Use a
shared helper.**

## Recommended Approach

1. Add two CSS class variants in `GuidanceRequestPanel.css.ts`:
   `questionClamped` (line-clamp ≤5) and `questionExpanded` (no clamp).
2. Add a `useExpandable(text: string, threshold = 300)` hook (or inline logic) inside
   `GuidanceRequestPanel.tsx` returning `{ isLong, expanded, toggle }`.
3. Apply the appropriate class and render the toggle button (plain `<button>` with
   `aria-expanded`) only when `isLong`.
4. Reuse the same logic for `answerText` expansion in answered legacy requests.
5. Write 3-4 new test cases in `GuidanceRequestPanel.test.tsx`.
