# Research: pitfalls for the Omnibar modal maxHeight/flex/scroll fix

Scope reminder (from requirements.md): CSS-only, `.modal` + `.body` exports in
`web-app/src/components/sessions/Omnibar.css.ts` only.

## 1. `overflow: "hidden"` on `.modal` + `display:flex` + `maxHeight` + `overflowY:auto` child — validated, not a problem

This exact combination (`overflow:"hidden"` on the flex container that also has
`maxHeight` + `display:"flex"` + `flexDirection:"column"`, paired with
`overflowY:"auto"` + `flex:1` on an inner body child) is **already shipped and
working** in `web-app/src/components/sessions/WorkspaceSwitchModal.css.ts`
(lines 25-36):

```ts
export const modal = style({
  ...
  maxHeight: "80vh",
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
  ...
});
```

No double-scrollbar and no clipped inner scrollbar results, because:
- `overflow:hidden` on `.modal` only clips content that exceeds `.modal`'s own
  box (used here purely for border-radius clipping, per the item's stated intent).
- `.body`'s own scrollbar renders inside `.body`'s box, which never exceeds
  `.modal`'s box once `.body` is a `flex:1; minHeight:0` flex child bounded by
  `.modal`'s `maxHeight` — so there's nothing left for `.modal`'s
  `overflow:hidden` to clip.

`ResumeSessionModal.css.ts` (lines 28-63) is a second, simpler proof of the
pattern (no `overflow:hidden` on modal there, but same `maxHeight` + flex-column
+ `overflowY:auto`/`flex:1` body split) — confirms `.body` scrolling doesn't
depend on the parent's own `overflow` value at all.

**Conclusion: FR1-FR3 as specified will not create a double-scrollbar or
clipped-scrollbar bug.** This class of interaction is a non-issue here.

## 2. `.body` is NOT the only thing between the fixed header and fixed footer — real siblings to check

`OmnibarCreationPanel.tsx` returns a **Fragment** (`<>...</>`), not a wrapping
div. Its top-level children — `pathDisplay` (conditional, pre-selected repo
path), `createRepoNotice` (conditional, "create repo" opt-in), and the
`existing_worktree`-path-error notice (conditional) — are all rendered
**before** `<div className={body}>` (OmnibarCreationPanel.tsx lines 272-344),
and the `error` div + `footer` div come **after** `.body` closes (lines
708-724). Because it's a Fragment, all of these land as **direct children of
`.modal`** in the real DOM, exactly like `.body` and `.footer` do — sibling to
`.body`, not descendants of it.

Full direct-child order of `.modal` in the DOM (confirmed by reading
`Omnibar.tsx` lines 871-1170 together with `OmnibarCreationPanel.tsx`
lines 272-724):

1. `inputContainer` (Omnibar.tsx)
2. `OmnibarResultList` (discovery mode only)
3. `PathCompletionDropdown` (creation mode only)
4. `completionErrorClass` div (conditional)
5. `detectionInfo` div (conditional)
6. **[OmnibarCreationPanel Fragment children, spliced in]:**
   - `styles.pathDisplay` (conditional)
   - `styles.createRepoNotice` (conditional)
   - `styles.createRepoNotice` for existing-worktree error (conditional)
   - `body` div ← the one FR3 targets (`flex:1; overflowY:auto; minHeight:0`)
   - `errorClass` div (conditional)
   - `footer` div
7. Path-confirmation dialog (`position:"absolute", inset:0` inline style,
   conditional — Omnibar.tsx lines 1036-1123)
8. `shortcuts` div (always rendered, Omnibar.tsx line 1126)

**Implication:** giving only `.body` `flex:1`/`overflowY:auto` is still
correct per NFR1 (scope is CSS-only, `.modal`+`.body`) and matches
`ResumeSessionModal`'s pattern, because every other sibling (including
`pathDisplay`/`createRepoNotice`/`error`/`shortcuts`) keeps its default
`flex-grow:0; flex-shrink:1` behavior as a flex item and is NOT squeezed to
zero height — only `.body` grows/shrinks. FR4 ("footer always reachable")
holds because `footer` and `shortcuts` sit after `.body` as normal
non-growing flex items, so they're always rendered at their natural height,
below whatever height `.body` occupies within the capped `.modal`.

**Residual edge case worth flagging (not a blocker, but worth a manual check
on short viewports):** `pathDisplay`, `createRepoNotice`, and the
existing-worktree-error notice sit **outside** the scrollable `.body` region
(they're siblings before it, not inside it). If a user hits a very short
viewport AND has one of these notices visible AND has Advanced Options open,
the sum of the fixed (non-scrolling) chrome — `inputContainer` +
`detectionInfo` + `pathDisplay`/`createRepoNotice` + `error` + `footer` +
`shortcuts` — could itself approach `80vh`, leaving `.body` little/no room to
scroll into. This is an inherent consequence of scoping the fix to `.body`
only (per NFR1) rather than wrapping all of these in the scroll region, and
mirrors exactly how `ResumeSessionModal` behaves (its `header` sits outside
its scrollable `.body` too) — so it's consistent with the reused pattern, not
a regression this fix introduces. No action needed beyond awareness.

## 3. `position:"absolute", inset:0` path-confirmation overlay inside `.modal` — validated, not a problem

`Omnibar.tsx` lines 1036-1123 render a confirmation dialog as a direct child
of `.modal` using inline `style={{ position: "absolute", inset: 0, ... }}`.
Absolutely positioned elements are removed from normal flow and take no part
in flex layout — turning `.modal` into `display:flex` does not affect this
overlay's sizing or positioning. It already depends on `.modal` having
`position: "relative"` (Omnibar.css.ts line 50, already set, unchanged by this
fix). No interaction risk.

## 4. Advanced Options expand (`collapsibleContent`/`advancedSection` maxHeight animation) — validated, not a problem

The "Advanced Options" collapsible (`OmnibarCreationPanel.tsx` lines 654-706,
using `styles.advancedSection`/`styles.advancedSectionOpen` from
`OmnibarCreationPanel.css.ts` lines 93-102, `maxHeight: 0 → "600px"`
transition) is nested **inside** `.body` (opens line 344, closes line 706) —
it is the *last* element inside `.body`, before `.body`'s own closing tag.
Expanding it grows content at the bottom of the already-scrollable `.body`
region. Because it's appended at the end and `.body` already has
`overflowY:auto`, the browser does not reset/jump `scrollTop` when content is
appended below the current scroll position — no scroll-jump jank expected.
(General verified web-platform behavior: appending content below the fold in
a scroll container preserves `scrollTop`; only content inserted *above* the
current view would shift it, which doesn't happen here.)

## 5. Existing tests — Jest/RTL: no dimension assertions found; Playwright: one low-risk visual-regression screenshot

- Searched all Omnibar-adjacent test files (`Omnibar.discovery.test.tsx`,
  `Omnibar.pathcompletion.test.tsx`, `OmnibarCreationPanel.attach.test.tsx`,
  `OmnibarResultList.test.tsx`, `useModeReducer.test.ts`, `detector.test.ts`,
  `dispatch.test.ts`) for `maxHeight`, `overflow`, `getBoundingClientRect`,
  `scrollTop`, `scrollHeight`, `clientHeight` — **no matches**. None of these
  tests assert on modal/body dimensions, so this fix cannot break a Jest/RTL
  assertion (jsdom doesn't lay out CSS anyway, so these tests were never going
  to catch this class of bug either).
- `tests/e2e/visual-regression.spec.ts` has one relevant spec: `"omnibar open"`
  (line 33), which does a full-page `toHaveScreenshot("omnibar-open.png", {
  maxDiffPixelRatio: 0.01 })` across 4 theme projects, right after pressing
  `Meta+k` on a fresh page load with empty input. Risk is **low**:
  - The default/just-opened state has no `pathDisplay`/`createRepoNotice`
    content (those are conditional on a typed path), Advanced Options is
    collapsed, and the modal's natural content height is well under `80vh` on
    the standard Playwright viewport — so `maxHeight:80vh` shouldn't visually
    change anything in this exact snapshot state.
  - The only theoretically-observable shift: turning `.modal` into
    `display:flex` changes its formatting context so direct-child **sibling
    margin collapsing** (a block-layout-only behavior) stops applying between
    `.modal`'s direct children. In practice none of `.modal`'s currently-visible
    direct children in this screenshot's state (`inputContainer`, `footer`,
    `shortcuts`) have vertical margins — only the *conditional*
    `createRepoNotice` (`margin: space[2] ... 0`, not rendered in this
    screenshot's state) does — so no visible diff is expected. Still worth a
    `--update-snapshots` re-run for this spec if CI flags it, since it's a
    pixel-diff test and this is exactly the kind of subtle layout-mode change
    that occasionally trips visual regression thresholds even when nothing
    "looks" different to the eye.
  - No other e2e spec (`one-off-session.spec.ts`,
    `session-create-directory.spec.ts`, `session-create-new-project.spec.ts`)
    asserts on Omnibar dimensions/scroll — they test functional flows only.

## Summary of what to actually watch for during implementation

- Implement exactly per FR1-FR3 (`.modal`: `maxHeight:"80vh"`,
  `display:"flex"`, `flexDirection:"column"`; `.body`: `overflowY:"auto"`,
  `flex:1`, `minHeight:0"`) — this is safe and matches two already-proven
  sibling modals.
- No code changes needed outside `Omnibar.css.ts` — `OmnibarCreationPanel.tsx`
  already correctly nests all form fields + the Advanced Options expansion
  inside `.body`, so FR3 alone contains the growth.
- After implementing, manually re-run
  `visual-regression.spec.ts -g "omnibar open"` (or the relevant project) to
  confirm no snapshot diff; if one appears, it's expected/benign per point 5
  and just needs `--update-snapshots`.
- No Jest/RTL test changes required.
