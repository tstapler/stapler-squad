# Implementation Plan: toolbar-reorder

**Feature**: Terminal Toolbar Re-order, Dev-Tool Grouping & Analytics Instrumentation
**Date**: 2026-05-30
**Status**: Ready for implementation
**ADRs**: None (all choices use existing patterns)

---

## Dependency Visualization

```
Story 1 (Analytics) ──────────────────────────────────────────► Task 4.2 (Analytics tests)
                                                                      │
Story 2 (Dev grouping CSS) ──► Story 2 (Dev grouping JSX) ──► Story 3 (Reorder) ──► Task 4.1 (logstream test update)
                                                                      │
                                                              Story 4 (Tests) ◄────────────┘

Parallelizable:
  - Task 1.1 (analytics calls) can be done concurrently with Task 2.1 (devGroupOpen state)
  - Task 2.2 (CSS) can be done concurrently with Task 2.1 (state)
  - Tasks 4.1 and 4.2 are independent of each other
```

---

## Phase 1: Analytics Instrumentation

### Epic 1.1: Toolbar Click Tracking

**Goal**: Every toolbar button click emits a `track()` event with a consistent schema so usage data accumulates immediately after this PR ships.

#### Story 1.1.1: Instrument all toolbar button onClick handlers

**As a** product owner, **I want** every toolbar button click tracked, **so that** we have data to drive future UX decisions.

**Acceptance Criteria**:
- Every toolbar button onClick fires `track({ name: "toolbar_button_click", category: "user_action", sessionId, component: "TerminalOutput", labels: { button: "<key>", state?: "<value>" } })`
- Toggle buttons (Debug, Log Stream, Record, Mouse) include `state: "on"|"off"` in labels
- Streaming mode select includes `state: <mode-value>` in labels
- Dev group toggle button includes `state: "open"|"closed"` in labels
- No new hooks or imports needed (`track` is already destructured from `useAnalytics()`)

**Files**:
- `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 1.1.1a: Add track() to kill and resize-icon buttons (~3 min)

- In `TerminalOutput.tsx`, locate the Kill button (`aria-label="Kill session"` or similar) and add an inline wrapper:
  ```tsx
  onClick={() => {
    track({ name: "toolbar_button_click", category: "user_action", sessionId, component: "TerminalOutput", labels: { button: "kill" } });
    handleKill();
  }}
  ```
- Repeat for the resize-icon button (the resize-icon control, not the manual Resize secondary action):
  ```tsx
  labels: { button: "resize-icon" }
  ```
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 1.1.1b: Add track() to Debug, Log Stream, Record, and Raw mode select (~4 min)

- **Debug** toggle:
  ```tsx
  onClick={() => {
    const next = !debugMode;
    track({ name: "toolbar_button_click", category: "user_action", sessionId, component: "TerminalOutput", labels: { button: "debug", state: next ? "on" : "off" } });
    setDebugMode(next);
    localStorage.setItem("debug-terminal", next ? "true" : "false");
  }}
  ```
- **Log Stream** toggle: same pattern, `button: "logstream"`, state reflects new value after toggle.
- **Record** toggle: same pattern, `button: "record"`, state reflects `!isRecording`.
- **Raw mode select** (onChange):
  ```tsx
  onChange={(e) => {
    track({ name: "toolbar_button_click", category: "user_action", sessionId, component: "TerminalOutput", labels: { button: "streaming_mode", state: e.target.value } });
    setStreamingMode(e.target.value);
  }}
  ```
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 1.1.1c: Add track() to Paste, Camera, Gallery, Files buttons (~3 min)

- **Paste**: `labels: { button: "paste" }` — call track before or after the paste operation (before is fine).
- **Camera**: `labels: { button: "camera" }` — wrap the input trigger handler.
- **Gallery**: `labels: { button: "gallery" }` — wrap the input trigger handler.
- **Files**: `labels: { button: "files" }` — wrap the input trigger handler.
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 1.1.1d: Add track() to Mouse, Copy, Bottom, Resize, Clear secondary actions (~3 min)

The `secondaryActions` array has per-action `handler` functions. Wrap each:
- **Mouse**: `labels: { button: "mouse", state: mouseMode ? "off" : "on" }` (state = next value after click)
- **Copy**: `labels: { button: "copy" }`
- **Bottom**: `labels: { button: "bottom" }`
- **Resize**: `labels: { button: "resize" }` (will move to dev panel in Story 3, but add analytics here first)
- **Clear**: `labels: { button: "clear" }`

Pattern — wrap each entry in `secondaryActions`:
```tsx
{
  key: "copy",
  label: "Copy",
  handler: () => {
    track({ name: "toolbar_button_click", category: "user_action", sessionId, component: "TerminalOutput", labels: { button: "copy" } });
    handleCopy();
  },
  ...
}
```
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

---

## Phase 2: Dev-Tool Grouping

### Epic 2.1: Inline Dev Toggle (Option A)

**Goal**: Replace the 4 hardcoded dev buttons (Debug, Log Stream, Record, Raw selector) in the primary toolbar with a single "⚙ Dev" toggle button that expands an inline panel, reducing the primary toolbar from 13 to ≤8 visible buttons.

**Risk level**: Low-medium per research/pitfalls.md — no z-index/portal issues (stays in document flow), consistent with existing `toolbarExpanded` pattern.

#### Story 2.1.1: Add devGroupOpen state

**As a** developer, **I want** dev tools behind a persistent toggle, **so that** the primary toolbar shows only everyday actions.

**Acceptance Criteria**:
- `devGroupOpen` boolean state defaults to `false` for new installs
- State is persisted to localStorage under key `"stapler-squad-dev-toolbar"`
- Changes to state are written back to localStorage on every toggle
- State is initialized from localStorage in the `useState` initializer (not a `useEffect`)

**Files**:
- `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 2.1.1a: Add devGroupOpen useState and persistence (~3 min)

Add near the other localStorage-initialized states (e.g., near the `toolbarExpanded` state):
```tsx
const [devGroupOpen, setDevGroupOpen] = useState<boolean>(
  () => localStorage.getItem("stapler-squad-dev-toolbar") === "true"
);
```

Add a handler:
```tsx
const handleDevGroupToggle = () => {
  const next = !devGroupOpen;
  track({ name: "toolbar_button_click", category: "user_action", sessionId, component: "TerminalOutput", labels: { button: "dev_group", state: next ? "open" : "closed" } });
  setDevGroupOpen(next);
  localStorage.setItem("stapler-squad-dev-toolbar", next ? "true" : "false");
};
```

Add a `useRef` for focus management:
```tsx
const devToggleRef = useRef<HTMLButtonElement>(null);
```

Update the handler to return focus to the toggle when the panel closes:
```tsx
const handleDevGroupToggle = () => {
  const next = !devGroupOpen;
  track({ ... });
  setDevGroupOpen(next);
  localStorage.setItem("stapler-squad-dev-toolbar", next ? "true" : "false");
  if (!next) {
    // Return focus to toggle when panel collapses
    setTimeout(() => devToggleRef.current?.focus(), 0);
  }
};
```

- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

#### Story 2.1.2: Add dev group CSS styles

**As a** developer, **I want** the dev panel styled consistently with the toolbar, **so that** it visually fits without new design tokens.

**Acceptance Criteria**:
- `devGroup` style is a flex row with a left border separator using `vars.color.borderColor`
- `devGroupButton` style (optional) can be used for any dev-panel-specific overrides
- `devGroupPanel` style wraps the dev group with proper `devOnly` mobile-hiding behavior
- All styles use vanilla-extract, no hardcoded colors or `var()` strings

**Files**:
- `web-app/src/components/sessions/TerminalOutput.css.ts`

##### Task 2.1.2a: Add devGroup, devGroupButton, devGroupPanel CSS exports (~3 min)

In `TerminalOutput.css.ts`, add after the existing `devOnly` export:

```ts
export const devGroupPanel = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  paddingLeft: "0.5rem",
  borderLeft: `1px solid ${vars.color.borderColor}`,
  "@media": {
    "screen and (max-width: 768px)": { display: "none" },
  },
});

export const devGroup = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
});

export const devGroupButton = style([
  // Inherits base toolbarButton, can be extended here if needed
]);
```

Note: `vars.color.borderColor` is confirmed valid — already used at line 115 of `TerminalOutput.css.ts`. Use it directly. Do NOT hardcode a hex value.

- Files: `web-app/src/components/sessions/TerminalOutput.css.ts`

#### Story 2.1.3: Restructure JSX to use dev toggle and panel

**As a** end user, **I want** the primary toolbar to show only everyday actions, **so that** I can quickly find Copy, Paste, and Clear.

**Acceptance Criteria**:
- The 4 dev buttons (Debug, Log Stream, Record, Raw select) are removed from their current positions in the primary toolbar
- A single "⚙ Dev" toggle button replaces them
- Clicking the toggle shows/hides an inline panel containing the 4 dev buttons
- The dev panel container has `className={styles.devGroupPanel}` (which includes `devOnly` mobile hiding behavior equivalent) and `data-testid="toolbar-dev-group"`
- Each button inside the dev panel retains its original `styles.devOnly` class for belt-and-suspenders mobile hiding
- The dev toggle button has `aria-expanded={devGroupOpen}`, `aria-controls="toolbar-dev-group-inner"`, `data-testid="toolbar-dev-toggle"`, and `ref={devToggleRef}`
- The dev group panel inner div has `id="toolbar-dev-group-inner"` and `role="group"` and `aria-label="Developer tools"`

**Files**:
- `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 2.1.3a: Replace inline dev buttons with toggle + conditional panel (~5 min)

`handleDevGroupToggle` and `devToggleRef` are **defined in Task 2.1.1a** — do not redefine them here. This task only restructures the JSX to use them.

In the `toolbarExpanded` JSX section, locate the four dev buttons (Debug, Log Stream, Record, Raw select) and replace them with:

```tsx
{/* Dev tools toggle */}
<button
  ref={devToggleRef}
  className={`${styles.toolbarButton} ${styles.devOnly}`}
  onClick={handleDevGroupToggle}
  aria-label={devGroupOpen ? "Collapse developer tools" : "Expand developer tools"}
  aria-expanded={devGroupOpen}
  aria-controls="toolbar-dev-group-inner"
  data-testid="toolbar-dev-toggle"
  title="Developer tools: Debug, Log Stream, Record, Raw"
>
  ⚙{devGroupOpen ? ' ▴' : ' ▾'}
</button>

{devGroupOpen && (
  <div
    className={styles.devGroupPanel}
    data-testid="toolbar-dev-group"
  >
    <div
      id="toolbar-dev-group-inner"
      role="group"
      aria-label="Developer tools"
      className={styles.devGroup}
    >
      {/* Debug button — keep styles.devOnly, keep inline active style */}
      <button
        className={`${styles.toolbarButton} ${styles.devOnly} ${debugMode ? styles.debugActive : ""}`}
        style={debugMode ? { backgroundColor: '#2a4', color: 'white', fontWeight: 'bold' } : {}}
        onClick={/* debug toggle handler with track() from Task 1.1.1b */}
        aria-label={debugMode ? "Disable debug mode" : "Enable debug mode"}
      >
        {debugMode ? "🐛 Debug ON" : "🐛 Debug"}
      </button>

      {/* Log Stream button — MUST keep styles.devOnly and inline active style per logstream.test.tsx */}
      {/* NOTE: the actual state variable in the source is `logStreamEnabled`, NOT `remoteDebugEnabled` */}
      <button
        className={`${styles.toolbarButton} ${styles.devOnly} ${logStreamEnabled ? styles.debugActive : ""}`}
        style={logStreamEnabled ? { backgroundColor: '#2a4', color: 'white', fontWeight: 'bold' } : {}}
        onClick={/* log stream toggle handler with track() from Task 1.1.1b */}
        aria-label={logStreamEnabled ? "Disable remote log streaming" : "Enable remote log streaming"}
      >
        {logStreamEnabled ? "📡 Log Stream ON" : "📡 Log Stream"}
      </button>

      {/* Record button — ADD aria-label (a11y fix) */}
      <button
        className={`${styles.toolbarButton} ${styles.devOnly}`}
        style={isRecording ? { backgroundColor: '#ff4444', color: 'white' } : {}}
        onClick={/* record toggle handler with track() from Task 1.1.1b */}
        aria-label={isRecording ? "Stop recording terminal output" : "Start recording terminal output"}
        title={isRecording ? "Stop recording" : "Start recording terminal output"}
      >
        {isRecording ? '⏹️ Stop Rec' : '⏺️ Record'}
      </button>

      {/* Raw/streaming mode select */}
      <select
        className={`${styles.toolbarButton} ${styles.devOnly}`}
        aria-label="Select terminal streaming mode"
        onChange={/* streaming mode onChange with track() from Task 1.1.1b */}
        value={streamingMode}
      >
        {/* existing options unchanged */}
      </select>

      {/* Resize button — moved here from secondaryActions (Task 3.2) */}
      <button
        className={`${styles.toolbarButton} ${styles.devOnly}`}
        onClick={/* resize handler with track() from Task 1.1.1d */}
        aria-label="Resize terminal"
        title="Manually resize terminal to fit container"
      >
        ⊡ Resize
      </button>
    </div>
  </div>
)}
```

**Critical constraint**: The Log Stream button's `styles.devOnly` class and `style={{ backgroundColor: "#2a4" }}` inline active state MUST be preserved exactly — `logstream.test.tsx` line 139 asserts `className contains "devOnly"` and line 199 asserts `toHaveStyle({ backgroundColor: "#2a4" })`.

- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

---

## Phase 3: Button Re-order

### Epic 3.1: Primary Toolbar Reorder

**Goal**: After dev-tool grouping (Phase 2), reorder the primary toolbar buttons to match the evidence-based frequency order from research, achieving exactly 8 visible desktop buttons.

#### Story 3.1.1: Reorder primary buttons and move Resize to dev panel

**As an** end user, **I want** Copy, Paste, Bottom, and Clear first in the toolbar, **so that** the most-used actions are immediately accessible.

**Acceptance Criteria**:
- Primary toolbar left-to-right order (desktop): **Copy → Paste → Bottom → Clear → Gallery → Files → Mouse → [⚙ Dev toggle]**
- **Resize** is removed from `secondaryActions` array and appears only inside the dev panel (done in Task 2.1.3a)
- `secondaryActions` array order is: `[copy, bottom, clear, mouse]` — `paste`, `gallery`, `files` are hardcoded JSX not in the array
- Total desktop-visible primary buttons: exactly 8
- Mobile `mobileOverflowRow` continues to use `secondaryActions.map(...)` — Resize disappearing from the array means it also disappears from mobile overflow (acceptable; mobile users access via keyboard shortcuts)
- Record button has `aria-label` added (a11y fix, per pitfalls research)

**Files**:
- `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 3.1.1a: Reorder primary (non-secondary) buttons in JSX (~3 min)

Within the `toolbarExpanded` section, reorder JSX elements so the render order is:

1. `secondaryActions` block (which will contain copy, bottom, clear, mouse — see Task 3.1.1b)
2. Paste button (hardcoded JSX)
3. Gallery button (hardcoded JSX)
4. Files button (hardcoded JSX)
5. Camera button (hardcoded JSX, mobile-only — position doesn't affect desktop order)
6. Dev toggle button (from Task 2.1.3a)
7. Dev panel (from Task 2.1.3a, conditional)

Wait — the `secondaryActions` are in a `<div className={styles.secondaryGroup}>` wrapper with `data-testid="toolbar-secondary"`. This wrapper must remain at the same `data-testid`. The paste/gallery/files buttons are currently hardcoded JSX outside the secondary group. The new desired visual order is:

```
[secondary group: Copy, Paste-MOVED, Bottom, Clear, Mouse] [Gallery] [Files] [⚙ Dev]
```

However, since Paste is currently a hardcoded button (not in `secondaryActions`), and the visual order places Paste between Copy and Bottom, there are two approaches:
1. Add Paste to `secondaryActions` array — risky, changes the array structure tests may rely on
2. Move the hardcoded Paste JSX inside the `secondaryGroup` div — cleaner

**Chosen approach**: Move Paste's hardcoded JSX inside the `secondaryGroup` div, after the Copy button's array entry but before Bottom. This keeps the `data-testid="toolbar-secondary"` container intact. The secondary group will contain: Copy, Paste (moved in), Bottom, Clear, Mouse. Gallery and Files remain outside as hardcoded JSX. The final visual order in the toolbar is:

```
[secondaryGroup: Copy | Paste | Bottom | Clear | Mouse] [Gallery] [Files] [Camera-mobile] [⚙Dev▾]
```

Steps:
1. Cut the hardcoded Paste `<button>` element from its current position in `toolbarActions`
2. Paste it inside `<div className={styles.secondaryGroup}>` after the `secondaryActions.map(...)` renders Copy but before Bottom — OR add Paste as a hardcoded element at the start of the secondary group (simpler)
3. Reorder `secondaryActions` array to: `[copy, paste-if-moved, bottom, clear, mouse]` OR keep Paste hardcoded at start of group

Simplest safe approach: Insert the Paste button JSX at the **start** of the `<div className={styles.secondaryGroup}>` div (before the `secondaryActions.map()`), and reorder `secondaryActions` to `[copy, bottom, clear, mouse]`.

**Mobile impact**: Because `secondaryGroup` is hidden on mobile, the hardcoded Paste button inside it will also be hidden on mobile. The `mobileOverflowRow` uses `secondaryActions.map()` only, so Paste will not appear in the mobile overflow row. This is **accepted** — mobile users paste via the OS keyboard's paste action. If this is unacceptable, alternative: add Paste as the first entry in the `secondaryActions` array instead of hardcoding it. That would include it in the mobile overflow row automatically, but requires removing the Paste-specific error-state rendering from the hardcoded JSX and threading it through the `secondaryActions` entry.

Final JSX structure within `toolbarExpanded`:
```tsx
{toolbarExpanded && (
  <div className={styles.toolbarActions} data-testid="toolbar-actions">
    <div className={styles.secondaryGroup} data-testid="toolbar-secondary">
      {/* Paste first in secondary group */}
      <button ... aria-label="Paste from clipboard" onClick={pasteHandlerWithTrack}>📎 Paste</button>
      {/* secondaryActions: copy, bottom, clear, mouse */}
      {secondaryActions.map(action => ...)}
    </div>
    {/* Gallery */}
    <button ... aria-label="Attach images from gallery" ...>🖼 Gallery</button>
    {/* Files */}
    <button ... aria-label="Attach files" ...>📎 Files</button>
    {/* Camera (mobile only) */}
    <button ... className={styles.mobileOnlyUpload} ...>...</button>
    {/* Dev toggle */}
    <button ref={devToggleRef} ... data-testid="toolbar-dev-toggle">⚙{devGroupOpen ? ' ▴' : ' ▾'}</button>
    {/* Dev panel */}
    {devGroupOpen && <div ...>...</div>}
  </div>
)}
```

- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 3.1.1b: Update secondaryActions array order and remove Resize (~2 min)

Locate the `secondaryActions` array definition in `TerminalOutput.tsx`. Current order: `[mouse, copy, bottom, resize, clear]`.

New order: `[copy, bottom, clear, mouse]`
- Remove `resize` entry (Resize is now in the dev panel)
- Move `copy` first
- Move `bottom` second
- Move `clear` third
- Move `mouse` last (advanced toggle, demoted to last in primary)

Verify all keys remain unique: `'copy'`, `'bottom'`, `'clear'`, `'mouse'` — yes.

The Resize handler + label + aria-label that were in the `secondaryActions` array entry should now be implemented as a hardcoded `<button>` in the dev panel (done in Task 2.1.3a).

- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 3.1.1c: Add aria-label to Record button (~1 min)

The Record button currently has no `aria-label` (existing a11y bug per pitfalls.md pitfall 3). This is fixed as part of Task 2.1.3a when the button is moved to the dev panel — the new JSX already includes `aria-label={isRecording ? "Stop recording terminal output" : "Start recording terminal output"}`.

Confirm the fix is present in the dev panel JSX. No additional file changes needed beyond Task 2.1.3a.

- Files: `web-app/src/components/sessions/TerminalOutput.tsx` (covered by 2.1.3a)

---

## Phase 4: Tests

### Epic 4.1: Test Updates and New Coverage

**Goal**: All existing tests pass; new tests cover analytics and dev group behavior.

#### Story 4.1.1: Update logstream tests for dev panel

**As a** developer, **I want** the logstream tests to work with the new dev panel, **so that** the Log Stream button remains test-reachable.

**Acceptance Criteria**:
- `logstream.test.tsx` `beforeEach` sets `localStorage.setItem("stapler-squad-dev-toolbar", "true")` so the Log Stream button renders inside the (now-expanded) dev panel
- All existing assertions in `logstream.test.tsx` continue to pass unchanged

**Files**:
- `web-app/src/components/sessions/__tests__/TerminalOutput.logstream.test.tsx`

##### Task 4.1.1a: Add dev-toolbar localStorage init to logstream.test.tsx beforeEach (~2 min)

In `beforeEach`, add after the existing toolbar-expanded line:
```ts
localStorage.setItem("stapler-squad-toolbar-expanded", "true");
localStorage.setItem("stapler-squad-dev-toolbar", "true"); // NEW: expand dev panel so Log Stream is reachable
```

Verify the test still:
1. Finds Log Stream button by `aria-label` matching `/enable remote log streaming/i`
2. Asserts `btn.className` contains `"devOnly"` (line 139) — Log Stream retains `styles.devOnly`
3. Asserts active state has `backgroundColor: "#2a4"` (line 199) — inline style preserved

- Files: `web-app/src/components/sessions/__tests__/TerminalOutput.logstream.test.tsx`

#### Story 4.1.2: New analytics test file

**As a** developer, **I want** test coverage for all analytics calls, **so that** regressions in the event schema are caught immediately.

**Acceptance Criteria**:
- New test file `TerminalOutput.toolbar-analytics.test.tsx` exists
- Each of the 15 button keys (kill, resize-icon, debug, logstream, record, streaming_mode, paste, camera, gallery, files, mouse, copy, bottom, resize, clear, dev_group) has at least one test
- Tests verify `track` is called with correct `name`, `category`, `component`, `labels.button`
- Toggle buttons also verify correct `labels.state` value
- Tests use the same localStorage setup as other toolbar tests

**Files**:
- `web-app/src/components/sessions/__tests__/TerminalOutput.toolbar-analytics.test.tsx` (NEW)

##### Task 4.1.2a: Create analytics test file with mock setup (~5 min)

Create `web-app/src/components/sessions/__tests__/TerminalOutput.toolbar-analytics.test.tsx`:

```tsx
import { render, screen, fireEvent } from "@testing-library/react";
import TerminalOutput from "../TerminalOutput";
import { useAnalytics } from "../../../lib/contexts/AnalyticsContext";

// Mock the analytics hook
jest.mock("../../../lib/contexts/AnalyticsContext", () => ({
  useAnalytics: jest.fn(),
}));

const mockTrack = jest.fn();

beforeEach(() => {
  jest.clearAllMocks();
  (useAnalytics as jest.Mock).mockReturnValue({ track: mockTrack });
  localStorage.setItem("stapler-squad-toolbar-expanded", "true");
  localStorage.setItem("stapler-squad-dev-toolbar", "true");
});

// Standard TerminalOutput props (minimal, same pattern as other tests)
const defaultProps = { /* ... copy minimal props from existing test files */ };

describe("toolbar analytics", () => {
  it("fires track with button:copy when Copy clicked", () => {
    render(<TerminalOutput {...defaultProps} />);
    fireEvent.click(screen.getByRole("button", { name: /copy/i }));
    expect(mockTrack).toHaveBeenCalledWith(expect.objectContaining({
      name: "toolbar_button_click",
      category: "user_action",
      component: "TerminalOutput",
      labels: expect.objectContaining({ button: "copy" }),
    }));
  });

  it("fires track with button:paste when Paste clicked", () => { /* ... */ });
  it("fires track with button:bottom when Bottom clicked", () => { /* ... */ });
  it("fires track with button:clear when Clear clicked", () => { /* ... */ });
  it("fires track with button:gallery when Gallery clicked", () => { /* ... */ });
  it("fires track with button:files when Files clicked", () => { /* ... */ });
  it("fires track with button:mouse state:on when Mouse enabled", () => { /* ... */ });
  it("fires track with button:debug state:on when Debug enabled", () => { /* ... */ });
  it("fires track with button:logstream state:on when Log Stream enabled", () => { /* ... */ });
  it("fires track with button:record state:on when Record started", () => { /* ... */ });
  it("fires track with button:streaming_mode state:<value> when mode changed", () => { /* ... */ });
  it("fires track with button:resize when Resize clicked (in dev panel)", () => { /* ... */ });
  it("fires track with button:dev_group state:open when dev toggle clicked", () => { /* ... */ });
  it("fires track with button:dev_group state:closed when dev toggle clicked again", () => { /* ... */ });
});
```

Copy minimal render props from `TerminalOutput.logstream.test.tsx` to keep the test setup consistent with existing tests.

- Files: `web-app/src/components/sessions/__tests__/TerminalOutput.toolbar-analytics.test.tsx`

---

## Risk Register

| Risk | Severity | Mitigation |
|---|---|---|
| `logstream.test.tsx` line 139 asserts `devOnly` on Log Stream | HIGH | Log Stream keeps `styles.devOnly` class inside dev panel — no change needed |
| `logstream.test.tsx` line 199 asserts `backgroundColor: "#2a4"` | HIGH | Inline style on Log Stream active state is preserved verbatim |
| Log Stream button becomes unreachable in tests (inside collapsed dev panel) | HIGH | Task 4.1.1a adds `localStorage.setItem("stapler-squad-dev-toolbar", "true")` to `beforeEach` |
| `upload.test.tsx` relies on Gallery aria-label `/attach image/i` | HIGH | Gallery button stays in primary toolbar, aria-label unchanged |
| `data-testid="toolbar-secondary"` still required by tests | HIGH | `secondaryGroup` div keeps its `data-testid` — only contents change |
| Vanilla-extract `vars.color.borderColor` token may not exist | MEDIUM | Check `theme-contract.css.ts` before writing CSS; use correct token name or fallback to `vars.color.border` |
| `devGroupButton` / `devGroupPanel` styles added but `devOnly` already hides on mobile | LOW | `devGroupPanel` adds its own mobile `display:none` as belt-and-suspenders; individual buttons keep `styles.devOnly` |
| Focus lost when dev panel collapses | LOW | `devToggleRef.current?.focus()` called after `setDevGroupOpen(false)` |
| Resize removed from `secondaryActions` removes it from mobile overflow row | LOW | Acceptable — Resize is near-redundant (auto-fit covers 95% of cases) per UX research |

---

## Final Button Layout Reference

### Primary toolbar (8 buttons, always visible on desktop):
```
[Copy] [Paste] [Bottom] [Clear]  [Gallery] [Files]  [Mouse] | [⚙▾ Dev]
 ↑ Tier 1: high-freq ↑           ↑ Upload ↑         ↑ Adv↑     ↑ toggle
```

### Dev panel (visible when ⚙ Dev is toggled open):
```
[🐛 Debug] [📡 Log Stream] [⏺️ Record] [🚀 Raw▾] [⊡ Resize]
```

### Mobile (unchanged):
- Primary toolbar hidden — uses `mobileOverflowRow` with `secondaryActions` (copy, bottom, clear, mouse — paste now at start of secondaryGroup div so also appears in overflow)
- Dev panel is `display:none` on mobile regardless of `devGroupOpen` state (via `devGroupPanel` CSS + individual `devOnly` classes)

---

## Exact Task Execution Order

1. Task 2.1.2a — Add CSS first (no dependencies)
2. Task 2.1.1a — Add devGroupOpen state
3. Task 1.1.1a through 1.1.1d — Add all track() calls (can be one pass through the file)
4. Task 2.1.3a — Restructure dev buttons into panel (uses state and track from steps 2–3)
5. Task 3.1.1a — Reorder primary toolbar JSX
6. Task 3.1.1b — Update secondaryActions array
7. Task 4.1.1a — Update logstream.test.tsx
8. Task 4.1.2a — Create analytics test file
9. Run `cd web-app && npx jest --no-coverage --testPathPatterns="TerminalOutput"` and fix any failures before committing
