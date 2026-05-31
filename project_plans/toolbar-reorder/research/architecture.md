# Architecture Research: Dev Tool Grouping Options

## Context

The current toolbar has 4 dev-only buttons/controls (Debug, Log Stream, Record, Raw streaming mode select) that need to be visually separated or hidden to reduce desktop button count from 13 to ≤8. The component already uses vanilla-extract for CSS, has a `toolbarExpanded` boolean state, and `devOnly` CSS class.

---

## Option A: Inline "⚙ Dev" Toggle (Collapsible Section)

### How it works
Add a `devGroupOpen` boolean state (persisted to localStorage, like `toolbarExpanded`). A single "⚙ Dev" button appears in the toolbar. Clicking it toggles a sub-section that expands inline within `toolbarActions`, showing the 4 dev controls.

```tsx
const [devGroupOpen, setDevGroupOpen] = useState(
  () => localStorage.getItem("stapler-squad-dev-toolbar") === "true"
);

// In JSX:
<button
  className={styles.toolbarButton}
  onClick={() => setDevGroupOpen(v => !v)}
  aria-label={devGroupOpen ? "Collapse dev tools" : "Expand dev tools"}
  aria-expanded={devGroupOpen}
  data-testid="toolbar-dev-toggle"
>
  ⚙ Dev{devGroupOpen ? ' ▴' : ' ▾'}
</button>
{devGroupOpen && (
  <div className={styles.devGroup} data-testid="toolbar-dev-group">
    {/* Debug, Log Stream, Record, Raw select */}
  </div>
)}
```

### CSS needed (vanilla-extract, minimal)
```ts
export const devGroup = style({
  display: "flex",
  gap: "0.5rem",
  paddingLeft: "0.5rem",
  borderLeft: `1px solid ${vars.color.borderColor}`,
});
```

### Pros
- Meets ≤8 primary button target (removes 4 buttons from main view, replaces with 1 toggle = net -3)
- No z-index / stacking context issues — everything stays in document flow
- Consistent with the existing `toolbarExpanded` pattern already in the component
- `devOnly` class can remain on inner buttons (still hides on mobile)
- localStorage persistence means power users don't have to re-expand on every visit
- `data-testid="toolbar-dev-group"` makes it easily testable

### Cons
- Two-click access to dev tools (click Dev toggle → click button)
- The collapsed "⚙ Dev" button takes one primary slot (net total: primary bar becomes ~9 → need to also demote Resize or Mouse to stay ≤8)
- The `toolbarActions` container uses `flexWrap: "wrap"` — expanded dev group will wrap correctly but may look wide on small desktops

### Complexity
**Low-medium.** One new `useState`, one CSS style export, restructure of the 4 dev buttons into a conditional section. ~30 lines of JSX change.

---

## Option B: Right-Aligned Group with Visual Separator

### How it works
Move the 4 dev buttons to a right-aligned flex group within `toolbarActions` using `marginLeft: auto` or `flexGrow: 1` on a spacer between primary and dev groups.

```tsx
<div className={styles.toolbarActions}>
  {/* Primary buttons: Copy, Paste, Bottom, Clear, Gallery, Files, Mouse, Resize */}
  <div className={styles.primaryGroup}>...</div>
  {/* Dev group: pushed to the right by spacer */}
  <div className={styles.devGroupRight}>
    {/* Debug, Log Stream, Record, Raw select */}
  </div>
</div>
```

### CSS needed
```ts
export const primaryGroup = style({ display: "flex", gap: "0.5rem" });
export const devGroupRight = style({
  display: "flex",
  gap: "0.5rem",
  marginLeft: "auto",
  paddingLeft: "0.75rem",
  borderLeft: `1px solid ${vars.color.borderColor}`,
  "@media": {
    "screen and (max-width: 768px)": { display: "none" },
  },
});
```

### Pros
- All buttons always visible, no toggle interaction needed
- Clean visual separation signals low priority
- Zero new state management
- Well-understood pattern (browser DevTools, VS Code toolbar)

### Cons
- Does NOT meet ≤8 target — still 12 buttons visible on desktop (13 - 1 for the select being a select)
- `marginLeft: auto` only works if `toolbarActions` has `display: flex` without `flexWrap: wrap`. Current CSS uses `flexWrap: "wrap"` which breaks `marginLeft: auto`. Would need to remove wrap or use a nested flex container.
- The existing `toolbarActions` `flexWrap: "wrap"` would need to change, which could affect mobile layout

### Complexity
**Low.** Two new CSS exports, restructure JSX into two divs. ~20 lines of change. But fails the acceptance criterion of ≤8 buttons.

---

## Option C: Overflow/Kebab Menu ("...")

### How it works
A "⋮" button opens a dropdown menu listing dev tool actions. The dropdown floats above the toolbar.

### CSS needed
A floating absolute-positioned panel. Per the CSS architecture rules:
> `position: fixed` or `position: absolute` modals/sheets without `createPortal` — `fixed` positioning silently breaks when any ancestor has a CSS transform, filter, or will-change. Always use `createPortal(..., document.body)` for overlays.

Would require:
1. `createPortal` call to render the menu to `document.body`
2. Click-outside handler (`useEffect` with `document.addEventListener`)
3. Position calculation with `getBoundingClientRect()` to place the portal relative to the button
4. New CSS styles for the dropdown panel

### Pros
- Cleanest primary bar — 1 button replaces 4
- Familiar UX pattern (VS Code's "More Actions ▾")
- Excellent discoverability via clear "⋮" affordance

### Cons
- **Highest complexity** — requires `createPortal`, position calculation, click-outside handler
- CSS architecture rule violation risk if `createPortal` is forgotten
- Portal needs z-index management (though named z-index slots exist in `theme-contract.css.ts`)
- Testing is harder — portalled content renders outside the component tree
- ~80–100 lines of new code

### Complexity
**High.** Not recommended given low net value over Option A.

---

## Option D: Dev Mode Activation Gate (localStorage-based Visibility)

### How it works
Dev buttons only render when `debugMode === true` (already a state variable in the component, seeded from `localStorage.getItem('debug-terminal')`). The buttons disappear entirely when debug mode is off.

```tsx
{debugMode && (
  <>
    {/* Log Stream, Record, Raw select */}
  </>
)}
```

### Pros
- Zero buttons for end users — absolutely clean toolbar
- Zero new state, zero new CSS — uses existing `debugMode`
- Simplest possible implementation

### Cons
- **Discoverability is broken** — the Debug button that activates debug mode is itself hidden behind debug mode
- This creates a chicken-and-egg: to show Debug, you'd need to open DevTools and set `localStorage['debug-terminal'] = 'true'` manually
- Record and Log Stream are useful during active debugging sessions — hiding them until debug mode is on means the user must toggle Debug, then find and toggle Log Stream
- A partial workaround (keep Debug visible, hide the rest behind it) makes Debug a two-level toggle which is confusing UX

### Complexity
**Very Low.** But the discoverability problem makes this option inadequate for the acceptance criteria.

---

## Recommendation: Option A (Inline Dev Toggle)

**Recommended approach: Option A**, with the following refinements:

1. **Rename** the toggle button to "⚙" (icon only with aria-label "Developer tools") to save horizontal space. Add a tooltip: "Developer tools: Debug, Log Stream, Record, Raw".

2. **Default state**: `devGroupOpen = false` for new installs (clean toolbar), `true` only if localStorage has the key (power users keep their state).

3. **Combine with reordering**: Move the Dev toggle button to the far right of the primary bar (after Resize), making the visual hierarchy: primary actions → upload → controls → dev.

4. **Keep `devOnly` class** on the 4 dev buttons inside the group. This keeps them hidden on mobile even when the group is expanded on desktop (mobile already uses the `mobileOverflowRow` pattern for secondary actions).

5. **New button count after this change**:
   - Before: 13 buttons (Debug + Log Stream + Record + Raw select + Paste + Gallery + Files + Mouse + Copy + Bottom + Resize + Clear + Camera-mobile)
   - After: Copy + Paste + Bottom + Clear + Gallery + Files + Mouse + Resize + ⚙Dev = **9 visible** + Camera (mobile-only, hidden on desktop)
   - If Resize is demoted into the dev group: **8 visible** = meets acceptance criterion exactly

**Exact recommended primary bar (8 buttons)**:
```
[Copy] [Paste] [Bottom] [Clear] [Gallery] [Files] [Mouse] [⚙ Dev▾]
```

**Dev group (expanded inline)**:
```
[Debug] [Log Stream] [Record] [Raw▾] [Resize]
```

Resize is moved to the dev group because it's near-redundant (auto-fit handles 95% of cases) and its manual use case (recovering from a stuck resize) is a developer/power-user action.

### Implementation steps for Option A
1. Add `devGroupOpen` state (localStorage persisted, key `"stapler-squad-dev-toolbar"`)
2. Add `devGroup` style to `TerminalOutput.css.ts`
3. Restructure `toolbarActions` JSX:
   - New order: Paste → Gallery → Files → secondary actions (Copy, Bottom, Resize→moved-to-dev, Clear, Mouse→reordered) → Dev toggle → conditional dev group
4. Move Debug, Log Stream, Record, Raw select, and Resize into conditional dev group
5. Update `secondaryActions` array: remove Resize, reorder as `[copy, bottom, clear, mouse]`
6. Add `track()` calls to all button handlers (see analytics schema in `stack.md`)
