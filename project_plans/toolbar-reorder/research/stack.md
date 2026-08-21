# Stack Research: Terminal Toolbar Implementation

## Source Files

- Component: `web-app/src/components/sessions/TerminalOutput.tsx`
- Styles: `web-app/src/components/sessions/TerminalOutput.css.ts`
- Analytics context: `web-app/src/lib/contexts/AnalyticsContext.tsx`
- Analytics types: `web-app/src/lib/analytics/types.ts`

---

## Current Toolbar Structure (from source, lines 1139–1388)

### Always-visible layer (outside `toolbarExpanded` guard)
1. **Toolbar toggle** (`data-testid="toolbar-toggle"`, `aria-label="Toggle toolbar"`) — ⋯ / ✕ toggle
2. **Keyboard toggle** (`aria-label="Show/Hide mobile keyboard"`, `styles.mobileKeyboardToggle`) — ⌨️
3. **Reconnect** — conditional on `showReconnectButton`, rendered outside the expanded section

### Inside `toolbarExpanded` div (`data-testid="toolbar-actions"`)

**Dev-only buttons** (CSS class `styles.devOnly` — hidden on mobile ≤768px):
1. **Debug** — `aria-label="Enable/Disable debug mode"`, `styles.devOnly`, `styles.debugActive` when on, inline `backgroundColor: '#2a4'`
2. **Log Stream** — `aria-label="Enable/Disable remote log streaming"`, `styles.devOnly`, `styles.debugActive` when on, inline `backgroundColor: '#2a4'`
3. **Record** — `aria-label` absent (no aria-label on this button!), `styles.devOnly`, inline `backgroundColor: '#ff4444'` when active

**Dev-only select**:
4. **Raw/streaming mode select** — `aria-label="Select terminal streaming mode"`, `styles.devOnly`, options: 🚀 Raw / 📦 Raw+LZMA / 🔄 State Sync / 🔬 Hybrid

**Upload buttons**:
5. **Paste** — `aria-label="Paste from clipboard"`, no dev class
6. **Camera** (mobile only) — `styles.mobileOnlyUpload`, `aria-label="Take photo with camera"` / uploading variant
7. **Gallery** — `aria-label="Attach images from gallery"`, always visible
8. **Files** — `aria-label="Attach files"`, always visible

**Secondary actions** (in `styles.secondaryGroup`, hidden on mobile via CSS; duplicated in overflow row):
9. **Mouse** — key `mouse`, `styles.mouseModeActive` when active
10. **Copy** — key `copy`
11. **Bottom** — key `bottom`
12. **Resize** — key `resize`
13. **Clear** — key `clear`

**Mobile-only**:
14. **More ▾** — `data-testid="toolbar-more-button"`, `styles.mobileMoreButton`

**Total visible desktop buttons: 13** (Debug + Log Stream + Record + Raw select + Paste + Gallery + Files + Mouse + Copy + Bottom + Resize + Clear + More/keyboard controls)

---

## Analytics Implementation

### How `track()` is consumed

```ts
const { track } = useAnalytics();
```

Called at lines 301 and 305 (performance events only, no user_action events currently):

```ts
track({ name: "session_attach", category: "performance", durationMs: totalLoadTime, labels: { phase: "attach" }, sessionId });
track({ name: "stream_terminal_first_byte", category: "performance", durationMs: connectionDuration, sessionId });
```

### AnalyticsEvent schema (from `types.ts`)

```ts
interface AnalyticsEvent {
  name: string;
  category: "user_action" | "performance" | "navigation" | "rpc";
  durationMs?: number;
  sessionId?: string;
  page?: string;
  component?: string;
  labels?: Record<string, string>;
}
```

### Pattern for toolbar button clicks

For button analytics, the correct pattern is:
```ts
track({
  name: "toolbar_button_click",
  category: "user_action",
  sessionId,
  component: "TerminalOutput",
  labels: { button: "copy" }
});
```

`sessionId` is already in scope as a component prop. `track` is already destructured at the top of the component. Zero new hooks required.

---

## CSS Classes for the Toolbar

All defined in `TerminalOutput.css.ts` using vanilla-extract:

| Export name | Purpose |
|---|---|
| `toolbar` | Outer flex row (status left, actions right) |
| `actions` | Right-side flex container |
| `toolbarActions` | Inner flex for collapsible section, wraps on desktop |
| `toolbarButton` | Base button style (hover/active transitions) |
| `toolbarToggle` | Toggle button style (always visible) |
| `devOnly` | Hides on mobile ≤768px — already applied to Debug, Log Stream, Record, Raw select |
| `debugActive` | Visual active-state (currently an empty style — no actual CSS rules) |
| `secondaryGroup` | Groups secondary actions, hidden on mobile |
| `mobileOnlyUpload` | Shows only on coarse-pointer (touch) devices |
| `mobileKeyboardToggle` | Keyboard toggle, always inline |
| `mobileMoreButton` | "More ▾" trigger — hidden on desktop, visible on mobile |
| `mobileMoreActive` | Active state for More button |
| `mobileOverflowRow` | Below-toolbar overflow row on mobile |
| `mouseModeActive` | Active state for mouse mode button |
| `mobileKeyActive` | Active state for sticky CTRL/ALT modifier keys |

### Key CSS facts
- `devOnly` already exists and works — it is the right mechanism for "hide on mobile, show on desktop"
- `debugActive` is an **empty style** (`export const debugActive = style({})`) — it has no visual effect beyond what the inline styles provide. This is an existing inconsistency.
- `secondaryGroup` hides the Mouse/Copy/Bottom/Resize/Clear group on mobile; these reappear in `mobileOverflowRow`

---

## Risk Assessment

### LOW RISK (just JSX reordering)

1. **Reordering buttons within `toolbarActions`** — pure JSX order change, no CSS impact, no test ID impact
2. **Moving `secondaryActions` array items** — reordering `[mouse, copy, bottom, resize, clear]` in the array affects render order but no tests assert order
3. **Adding `track()` calls** — the function is already destructured; adding `() => { track({...}); action.handler(); }` wrappers is mechanical

### MEDIUM RISK (small CSS additions needed)

4. **Adding a visual separator between primary and dev groups** — needs a new `devSeparator` CSS export in `.css.ts` (a `::before` pseudo-element or `marginLeft: auto` trick). Low CSS complexity but requires a new vanilla-extract style.
5. **Moving dev buttons behind a toggle** — needs a new `devGroupExpanded` boolean state and a `devGroup` container style. The existing `toolbarActions` pattern is directly reusable.

### HIGH RISK (new patterns required)

6. **Overflow/kebab menu** — requires `position: relative` parent, `position: absolute` dropdown — per CSS architecture rules, this needs `createPortal` to avoid stacking context breaks. This is the highest complexity option.
7. **Removing `debugActive` empty class** — would break `TerminalOutput.logstream.test.tsx` line 139 which asserts `expect(btn.className).toContain("devOnly")` — similar assertion exists for `debugActive` class presence in tests.

---

## Key Test Constraints

- `TerminalOutput.logstream.test.tsx` line 139: asserts `btn.className` contains `"devOnly"` — the Log Stream button must keep this class
- `TerminalOutput.logstream.test.tsx` line 199: asserts `activeBtn` has `style={{ backgroundColor: "#2a4" }}` — inline styles on active Log Stream button must remain
- `TerminalOutput.upload.test.tsx`: uses `getByRole("button", { name: /attach image/i })` — the Gallery button aria-label must remain unchanged
- `TerminalOutput.upload.test.tsx` line 319: uses `getAllByRole("button").filter(b => b.getAttribute("aria-label")?.match(/uploading/i))` — uploading aria-labels must remain
- All tests set `localStorage.setItem("stapler-squad-toolbar-expanded", "true")` in `beforeEach` — the toolbar expansion mechanism must remain
