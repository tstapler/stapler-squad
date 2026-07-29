# Research: Technology stack for Omnibar modal scroll fix

## Stack

- **CSS-in-JS**: vanilla-extract (`@vanilla-extract/css`), pinned at `1.20.1` per
  `web-app/package.json` (`"@vanilla-extract/css": "^1.20.1"`). ADR-009
  (`.claude/rules/css-architecture.md`) mandates vanilla-extract for all styling.
- **API used**: plain `style()` calls returning class-name strings — no `recipe()`,
  no `sprinkles`, no `createTheme` involved in the modal files touched by this fix.
  `Omnibar.css.ts` already imports `style`, `keyframes`, `globalStyle` from
  `@vanilla-extract/css` and `vars` from `@/styles/theme-contract.css` (line 1-2).
- **No new dependencies needed** — confirmed. The fix only adds/changes plain
  object properties (`maxHeight`, `display`, `flexDirection`, `overflowY`, `flex`,
  `minHeight`) inside the existing `style({...})` calls for `.modal` and `.body`.
  No new imports, no new packages.

## Sibling modal pattern (proven, to be reused verbatim)

### `web-app/src/components/ui/Modal.css.ts` (lines 25-36) — generic modal
```ts
export const modal = style({
  background: vars.color.background,
  borderRadius: "12px",
  boxShadow: "0 20px 40px rgba(0, 0, 0, 0.3)",
  maxWidth: "600px",
  width: "90%",
  maxHeight: "80vh",
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
  animation: `${slideIn} 0.15s ease-out`,
});
```

### `web-app/src/components/sessions/ResumeSessionModal.css.ts` (lines 28-39, 59-63)
```ts
export const modal = style({
  background: vars.color.modalBackground,
  borderRadius: "12px",
  padding: 0,
  maxWidth: "520px",
  width: "90%",
  maxHeight: "80vh",
  display: "flex",
  flexDirection: "column",
  boxShadow: "0 20px 60px rgba(0, 0, 0, 0.3)",
  animation: `${slideUp} 0.3s ease`,
});

export const body = style({
  padding: "24px",
  overflowY: "auto",
  flex: 1,
});
```
Note: this file's `body` block does **not** include an explicit `minHeight: 0` —
`flex: 1` alone happens to work here because there's no sibling flex item pushing
overflow (no long unbroken content forcing min-content sizing conflicts observed in
this file). Requirements.md's FR3 explicitly asks for `minHeight: 0` on `.body` in
Omnibar, which is the more defensive/correct form (prevents the classic flexbox
"min-height: auto" bug where a flex child refuses to shrink below its content size,
defeating `overflow-y: auto`). Safe and recommended to include even though this one
sibling omits it.

### `web-app/src/components/sessions/WorkspaceSwitchModal.css.ts` (lines 25-36)
```ts
export const modal = style({
  background: vars.color.background,
  borderRadius: "12px",
  boxShadow: "0 20px 40px rgba(0, 0, 0, 0.3)",
  maxWidth: "600px",
  width: "90%",
  maxHeight: "80vh",
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
  animation: `${slideIn} 0.15s ease-out`,
});
```
This file has no single `.body` export (it's a list-heavy modal with several
scrollable sections) — its scroll regions (lines 76-77, 126, 160, 216) each set
`flex: 1` + `overflowY: "auto"` on the relevant inner flex child, same core
mechanism. Confirms the pattern is `maxHeight` + `display:flex/flexDirection:column`
on the modal shell, `flex:1` + `overflowY:auto` (+ `minHeight:0` where needed) on
the scrolling child — consistently applied across all 3 existing modals.

## Current state of `Omnibar.css.ts` (the file to change)

`web-app/src/components/sessions/Omnibar.css.ts`:
- `.modal` (lines 42-58): has `overflow: "hidden"` (the bug — clips instead of
  scrolling), no `maxHeight`, no `display: flex`. Already has a nested `"@media":
  { "(prefers-reduced-motion: no-preference)": {...} }` block (lines 51-56) for the
  `scanlineReveal` keyframe animation.
- `.overlay` (lines 21-40) also has an existing nested `@media` block
  (`prefers-reduced-motion`) for `fadeIn` — untouched by this fix per FR5.
- `.body` (lines 116-121): already `display: "flex"`, `flexDirection: "column"`,
  `gap: 16`, `padding: 16` — just needs `overflowY: "auto"`, `flex: 1`,
  `minHeight: 0` added alongside the existing properties.

## vanilla-extract version gotchas

- v1.20.1 fully supports plain CSS properties (`maxHeight`, `display`,
  `flexDirection`, `overflowY`, `flex`, `minHeight`) as flat keys in `style({...})`
  — these are standard `csstype`-typed properties, nothing exotic.
- Nested `"@media"` blocks inside `style()` are a stable, long-supported
  vanilla-extract feature (not new in 1.20.1) and coexist fine with unrelated
  top-level properties in the same `style()` call — `Modal.css.ts` and
  `ResumeSessionModal.css.ts` don't currently combine `@media` + `display:flex` in
  the same block, but there is no documented or structural conflict: `@media` in
  vanilla-extract compiles to a plain nested CSS `@media` rule wrapping only the
  properties declared inside it, and does not interact with sibling top-level
  properties like `display`/`maxHeight`/`flexDirection` at all. Adding
  `maxHeight: "80vh"`, `display: "flex"`, `flexDirection: "column"` as new
  top-level keys in `.modal` alongside the existing `"@media": {...}` key
  (animation properties only) is safe — confirmed by inspecting vanilla-extract's
  `style()` type signature usage elsewhere in the codebase (no other file shows
  breakage from this combination) and by the fact `Modal.css.ts`'s `.modal` block
  itself mixes `animation` (a top-level property, itself media-gateable elsewhere)
  with `display: flex` without issue.

## Conclusion

No new dependencies required. The exact vanilla-extract `style()` API used by
`ResumeSessionModal.css.ts` / `WorkspaceSwitchModal.css.ts` / `Modal.css.ts` is
already in use in `Omnibar.css.ts` (same imports, same file, same `vars` token
system). The fix is a pure property-addition inside the two existing `style({...})`
calls for `.modal` and `.body` — no restructuring of the `@media` blocks, no risk
of collision with the reduced-motion animation blocks.
