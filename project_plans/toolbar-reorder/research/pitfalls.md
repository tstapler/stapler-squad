# Pitfalls Research: Risk Areas and Test Constraints

## Source Files Analyzed
- `web-app/src/components/sessions/__tests__/TerminalOutput.upload.test.tsx`
- `web-app/src/components/sessions/__tests__/TerminalOutput.logstream.test.tsx`
- `web-app/src/components/sessions/__tests__/TerminalOutputBug.test.tsx`

---

## Test IDs and Aria Labels Used in Existing Tests

### `data-testid` attributes referenced in tests
| testid | Location in JSX | Risk if changed |
|---|---|---|
| `"toolbar-toggle"` | Toggle button (⋯/✕) | Must not change |
| `"toolbar-actions"` | `toolbarActions` div wrapper | Must not change |
| `"toolbar-secondary"` | `secondaryGroup` div | Must not change |
| `"toolbar-more-button"` | Mobile "More ▾" button | Must not change |
| `"toolbar-overflow-row"` | Mobile overflow row div | Must not change |
| `"mock-xterm"` | XtermTerminal mock | Not toolbar-related |

### Aria labels asserted in tests
| Aria label pattern | Test file | Line | Risk if changed |
|---|---|---|---|
| `/attach image/i` | `upload.test.tsx` | 116, 118, 201, 225, 291 | **HIGH** — Galaxy button must keep "Attach images from gallery" aria-label |
| `/uploading \d+ file/i` | `upload.test.tsx` | 319 | **HIGH** — uploading state aria-labels must match `/uploading/i` |
| `/enable remote log streaming/i` | `logstream.test.tsx` | 127, 149, 179 | **HIGH** — must not change Log Stream aria-label |
| `/disable remote log streaming/i` | `logstream.test.tsx` | 166, 197 | **HIGH** — must not change Log Stream aria-label |
| `/toggle toolbar/i` | Implicit via data-testid | N/A | Medium |

### CSS class assertions
| Class | Test | Line | Assertion |
|---|---|---|---|
| `"devOnly"` | `logstream.test.tsx` | 139 | `expect(btn.className).toContain("devOnly")` — Log Stream must keep `styles.devOnly` class |

### Inline style assertions
| Style | Test | Line | Assertion |
|---|---|---|---|
| `{ backgroundColor: "#2a4" }` | `logstream.test.tsx` | 199 | `expect(activeBtn).toHaveStyle({ backgroundColor: "#2a4" })` — Log Stream active inline style must remain |

### localStorage key assertions
| Key | Test | Lines |
|---|---|---|
| `"stapler-squad-toolbar-expanded"` | Both test files, `beforeEach` | Set to `"true"` so toolbar buttons render |
| `"stapler-squad-remote-debug"` | `logstream.test.tsx` | 152, 161, 219 — toggled and asserted |

---

## Critical Constraint: All Tests Set Toolbar Expanded

Both `upload.test.tsx` and `logstream.test.tsx` run this in `beforeEach`:
```ts
localStorage.setItem("stapler-squad-toolbar-expanded", "true");
```

This means: **any button inside the `toolbarExpanded` conditional section must remain inside that conditional**. Do not move buttons outside `toolbarExpanded` unless they were already outside it.

The tests render `<TerminalOutput ... isVisible={false} />` which means they test the collapsed-toolbar default; they force expand via localStorage. If the `devGroupOpen` state initialization reads localStorage, a new `beforeEach` line in new tests should set `"stapler-squad-dev-toolbar": "true"` for dev-button tests.

---

## Common Pitfalls When Reordering React Toolbar Components

### Pitfall 1: Key stability in mapped arrays
The `secondaryActions` array uses `key={action.key}` for the `.map()`. Reordering array elements is safe because keys are stable strings. However, if a new button is added with a duplicate key, React will warn and potentially render incorrectly.

**Mitigation**: When reordering `secondaryActions`, verify all keys remain unique (`'mouse'`, `'copy'`, `'bottom'`, `'resize'`, `'clear'`). If Resize is moved to the dev group and removed from the array, the remaining keys are still unique.

### Pitfall 2: The `mobileOverflowRow` mirrors `secondaryActions`
The secondary actions are rendered twice:
1. Inside `secondaryGroup` (desktop only — hidden on mobile via CSS)
2. Inside `mobileOverflowRow` (mobile only — visible when "More" is open)

Both use the same `secondaryActions.map(...)` so they stay in sync. If Resize is removed from `secondaryActions` (to move it to dev group), it will disappear from both desktop secondary group and mobile overflow row. This is intentional but must be tracked.

### Pitfall 3: The Record button has no aria-label
Current JSX for Record button (line 1265–1280 of TerminalOutput.tsx):
```tsx
<button
  className={`${styles.toolbarButton} ${styles.devOnly}`}
  onClick={...}
  title={isRecording ? "Stop recording" : "Start recording terminal output"}
  style={isRecording ? { backgroundColor: '#ff4444', color: 'white' } : {}}
>
  {isRecording ? '⏹️ Stop Rec' : '⏺️ Record'}
</button>
```

**No `aria-label` attribute!** This is an existing accessibility bug. Moving the button does not fix it, but a new test asserting `getByRole("button", { name: /record/i })` would fail without the fix.

### Pitfall 4: The Paste button's error state changes aria-label implicitly
The Paste button renders `{pasteError ? '⚠️ ${pasteError}' : '📎 Paste'}` as its content and has `aria-label="Paste from clipboard"`. The text content changes but aria-label does not — tests that query by role+name using aria-label are safe. Tests that use `textContent` assertions would break if the error message format changes.

### Pitfall 5: Camera button is pointer-type gated, not viewport-gated
The Camera button uses CSS `(pointer: fine)` media query via `styles.mobileOnlyUpload`. JSDOM does not evaluate this media query, so in tests the Camera button is always present in the DOM (just CSS-hidden). Tests using `getAllByRole("button")` will include it. The upload test correctly uses the hidden `input[type="file"]` element directly, avoiding this.

### Pitfall 6: Inline style overrides cannot be tested via vanilla-extract classes
The Log Stream and Debug active states use both:
- `styles.debugActive` (empty vanilla-extract class — no rules)
- Inline `style={{ backgroundColor: '#2a4', color: 'white', fontWeight: 'bold' }}`

The `logstream.test.tsx` asserts `toHaveStyle({ backgroundColor: "#2a4" })`. This is testing the inline style, not the vanilla-extract class. If the implementation moves to using only vanilla-extract for the active state (removing the inline style), this test will fail. Recommendation: keep the inline style for active state OR update both the implementation and the test together.

---

## Keyboard Navigation / Accessibility Concerns

### Tab order
HTML buttons receive tab focus in DOM order. Reordering toolbar buttons reorders the tab sequence. This is desirable — tabs should navigate Copy → Paste → Bottom → Clear before Debug/Log Stream.

### Focus management when dev group collapses
When the dev group closes (user clicks ⚙ Dev again), focus will move to `<body>` if the focused element disappears. The `⚙ Dev` toggle button should receive focus programmatically when the group closes:
```tsx
const devToggleRef = useRef<HTMLButtonElement>(null);
// After setDevGroupOpen(false):
devToggleRef.current?.focus();
```

### ARIA for expandable sections
The dev toggle button should use `aria-expanded={devGroupOpen}` and `aria-controls="dev-group-id"`, and the group container should have `id="dev-group-id"` and `role="group"`.

### Color contrast
The active state inline style `backgroundColor: '#2a4'` is a dark green. Ensure white text on this background meets WCAG AA (4.5:1 for normal text). `#2a4` = `rgb(34, 170, 68)` — white `#fff` on this gives approximately 3.5:1 which is below AA for normal text but above for large text (3:1). This is a pre-existing issue, not introduced by the reorder.

---

## Analytics Event Schema Recommendation

Based on the existing `track()` usage pattern in `TerminalOutput.tsx`:

```ts
// Existing pattern:
track({ name: "session_attach", category: "performance", durationMs: totalLoadTime, labels: { phase: "attach" }, sessionId });

// Recommended toolbar button click schema:
track({
  name: "toolbar_button_click",
  category: "user_action",
  sessionId,
  component: "TerminalOutput",
  labels: {
    button: "<button-key>",  // e.g. "copy", "paste", "clear", "bottom", "gallery", "files", "mouse", "resize", "debug", "logstream", "record", "streaming_mode"
    state: "<optional-state>"  // e.g. "on"/"off" for toggles, streaming mode value for select
  }
});
```

**Button key naming convention** (snake_case, matches key field in `secondaryActions` array where applicable):
| Button | `button` label value |
|---|---|
| Copy | `"copy"` |
| Paste | `"paste"` |
| Bottom | `"bottom"` |
| Clear | `"clear"` |
| Gallery | `"gallery"` |
| Files | `"files"` |
| Camera (mobile) | `"camera"` |
| Mouse mode toggle | `"mouse"` + `state: "on"/"off"` |
| Resize | `"resize"` |
| Debug toggle | `"debug"` + `state: "on"/"off"` |
| Log Stream toggle | `"logstream"` + `state: "on"/"off"` |
| Record toggle | `"record"` + `state: "on"/"off"` |
| Streaming mode select | `"streaming_mode"` + `state: <mode-value>` |
| Dev group toggle | `"dev_group"` + `state: "open"/"closed"` |

This schema supports downstream queries like:
- "What are the most-clicked toolbar buttons?" → `GROUP BY labels.button`
- "How often do users turn on debug mode?" → `WHERE name='toolbar_button_click' AND labels.button='debug'`
- "What streaming mode do users prefer?" → `WHERE labels.button='streaming_mode' GROUP BY labels.state`

---

## Test Update Requirements

When implementing the reorder, the following test changes are required or likely:

1. **No changes required** to `upload.test.tsx` — Gallery button stays in the toolbar, aria-label unchanged.

2. **No changes required** to `logstream.test.tsx` — if Log Stream button keeps `styles.devOnly` and its inline active style. If the Log Stream button is moved into a `devGroupOpen` conditional, tests must additionally set `localStorage.setItem("stapler-squad-dev-toolbar", "true")` in `beforeEach`.

3. **New tests to add**:
   - Dev group toggle renders/hides dev buttons
   - `track()` is called with correct event schema for each button click
   - `track()` is called with correct `state` for toggle buttons
   - Dev group state persists to localStorage

4. **`TerminalOutputBug.test.tsx`** — does not test any toolbar buttons (tests terminal sizing/scrollback logic). No changes needed.
