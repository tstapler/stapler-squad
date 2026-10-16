# UX Design: RUM Session Diagnostic Bundle

**Feature:** Exportable per-session diagnostic bundle  
**Design method:** Heuristic evaluation + task-flow analysis  
**Surfaces:** 4 interactive, 2 non-interactive  
**Acceptance criteria:** 22

---

## Surface Inventory

| # | Surface | Type | File(s) |
|---|---------|------|---------|
| S1 | "Export Diagnostics" item — action sheet in `SessionDetailView` | Interactive | `SessionDetailView.tsx` |
| S2 | "Export Diagnostics" item — `SessionActionsOverflow` kebab "Workflow" section | Interactive | `SessionActionsOverflow.tsx` |
| S3 | `ExportBundleModal` — delivery-mode selection, consent, progress, success/error | Interactive | `ExportBundleModal.tsx` (new) |
| S4 | "Create backlog item →" inline button in `ExportBundleModal` after server-save | Interactive | `ExportBundleModal.tsx` (new) |
| S5 | Toast notification output (success / error) | Non-interactive | `notificationContexts.ts` |
| S6 | Server-saved file path format (headless / log output) | Non-interactive | `ExportSessionDiagnosticBundle` RPC response |

---

## S1 — Action Sheet Entry Point (`SessionDetailView`)

### Context

The `SessionDetailView` header has a "⋯" button (`aria-label="Session actions"`, `data-testid="more-actions-button"`) that opens a Radix Dialog action sheet. Current items: Pause/Resume, Rename, Edit Tags, Create Checkpoint, Switch Workspace, Restart, Spawn new shell — then a `<hr>` before Delete. "Export Diagnostics" inserts before the `<hr>`.

### ASCII Wireframe

```
┌───────────────────────────────────────┐
│  Session Actions                   ✕  │
│                                       │
│  ┌─────────────────────────────────┐  │
│  │  ▶  Resume                      │  │
│  ├─────────────────────────────────┤  │
│  │  ✏  Rename                      │  │
│  ├─────────────────────────────────┤  │
│  │  🏷  Edit Tags                   │  │
│  ├─────────────────────────────────┤  │
│  │  📷  Create Checkpoint           │  │
│  ├─────────────────────────────────┤  │
│  │  🔄  Switch Workspace            │  │
│  ├─────────────────────────────────┤  │
│  │  ↻   Restart                    │  │
│  ├─────────────────────────────────┤  │
│  │  ⊞   Spawn new shell            │  │
│  ├─────────────────────────────────┤  │
│  │  ↓   Export Diagnostics         │  ← NEW (before divider)
│  ├─────────────────────────────────┤  │
│  │  ─────────────────────────────  │  │  (destructive divider)
│  ├─────────────────────────────────┤  │
│  │  🗑  Delete                      │  │
│  └─────────────────────────────────┘  │
└───────────────────────────────────────┘

Each item: min-height 52px, font-size 16px (touch target ≥ 44px, WCAG 2.5.5)
```

### Interaction Flow

1. User taps/clicks "⋯" in the session detail header.
2. Action sheet opens (Radix Dialog); focus moves to first item (Pause/Resume).
3. User navigates to "Export Diagnostics" via Tab, arrow keys, or tap.
4. User activates the item (Enter / Space / tap).
5. Action sheet closes (`actionSheetOpen = false`).
6. `ExportBundleModal` (S3) opens immediately.

### Error and Edge Cases

| Condition | Behavior |
|-----------|----------|
| Session is in a terminal/deleted state | Item is still visible; the modal (S3) will show a warning if the session has no scrollback or analytics data, but export is still permitted |
| Session detail view is loading | "⋯" button is disabled while loading; "Export Diagnostics" is not reachable |

---

## S2 — Kebab Menu Entry Point (`SessionActionsOverflow`)

### Context

`SessionActionsOverflow` renders a fixed-position dropdown with accordion sections: Workflow, Organize, Modes, More. "Create PR" and "Checkpoint" live in the Workflow section. "Export Diagnostics" inserts after "Checkpoint" in that same section.

### ASCII Wireframe

```
┌──────────────────────────────────┐
│  ▾ Workflow                      │  (accordion, expanded by default)
│  ┌────────────────────────────┐  │
│  │   Create PR                │  │
│  ├────────────────────────────┤  │
│  │   Create Checkpoint        │  │
│  ├────────────────────────────┤  │
│  │   Export Diagnostics       │  ← NEW
│  └────────────────────────────┘  │
│                                  │
│  ▾ Organize                      │  (collapsed)
│  ▾ Modes                         │  (collapsed)
│  ▾ More                          │  (collapsed)
└──────────────────────────────────┘

role="menu", aria-label="More session actions"
Each item: role="menuitem"
```

### Interaction Flow

1. User clicks/activates "⋯" kebab icon on a session list row or backlog detail panel.
2. Dropdown opens; Workflow section is expanded by default.
3. User activates "Export Diagnostics" (click, Enter, or Space).
4. Dropdown closes.
5. `ExportBundleModal` (S3) opens.

### Error and Edge Cases

| Condition | Behavior |
|-----------|----------|
| Keyboard nav: user opens kebab with Enter | Focus traps inside the menu; Escape closes without action |
| Item is in the "More" accordion | Not applicable — "Export Diagnostics" is in "Workflow", which is default-open |

---

## S3 — `ExportBundleModal`

This is the primary interactive surface. It handles delivery-mode selection, consent acknowledgment, progress feedback, and success/error states.

### Default State Wireframe

```
┌─────────────────────────────────────────┐
│  Export Session Diagnostics          ✕  │
│  ─────────────────────────────────────  │
│                                         │
│  Delivery                               │
│  ┌─────────────────────────────────┐    │
│  │ ◉  Download .zip               │    │  (default on desktop)
│  │    Bundle saved directly to    │    │
│  │    your Downloads folder       │    │
│  ├─────────────────────────────────┤    │
│  │ ○  Save on server              │    │  (default on mobile, window.innerWidth < 640)
│  │    Writes to                   │    │
│  │    ~/.stapler-squad/diagnostics/ │   │
│  └─────────────────────────────────┘    │
│                                         │
│  ┌─────────────────────────────────┐    │
│  │ ☐  I understand this bundle    │    │  (consent checkbox)
│  │    includes session content,   │    │
│  │    scrollback text, and        │    │
│  │    analytics events for this   │    │
│  │    session.                    │    │
│  └─────────────────────────────────┘    │
│                                         │
│  ┌──────────────┐  ┌──────────────┐     │
│  │    Cancel    │  │    Export    │     │  Export disabled until consent checked
│  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────┘

ARIA:
- Dialog: role="dialog", aria-modal="true", aria-labelledby="export-modal-title"
- Delivery options: role="radiogroup", aria-label="Delivery mode"
- Each option: role="radio"
- Consent: <label> wrapping <input type="checkbox">
- Export button: aria-disabled="true" until consent checked
```

### In-Progress State (0–2 s)

```
┌─────────────────────────────────────────┐
│  Export Session Diagnostics          ✕  │
│  ─────────────────────────────────────  │
│                                         │
│  [delivery options — disabled/greyed]   │
│  [consent checkbox — disabled/greyed]   │
│                                         │
│  ┌──────────────┐  ┌─────────────────┐  │
│  │    Cancel    │  │  ⟳ Assembling…  │  │  button label swapped, spinner shown
│  └──────────────┘  └─────────────────┘  │
│                                         │
│  ┌──────────────────────────────────┐   │
│  │  role="status" aria-live="polite"│   │
│  │  Assembling bundle…              │   │
│  └──────────────────────────────────┘   │
└─────────────────────────────────────────┘
```

### In-Progress State (2–5 s)

```
│  ⟳ Assembling…   (button)                │
│                                           │
│  ┌──────────────────────────────────────┐ │
│  │  Assembling bundle… this may take    │ │
│  │  up to 5 seconds.                    │ │
│  └──────────────────────────────────────┘ │
```

### In-Progress State (> 5 s / timeout)

```
│  ⟳ Still assembling…  (button)            │
│  ┌──────────────┐                         │
│  │    Cancel    │   ← enabled             │
│  └──────────────┘                         │
│                                           │
│  ┌──────────────────────────────────────┐ │
│  │  Taking longer than expected…        │ │
│  │  [Cancel]                            │ │
│  └──────────────────────────────────────┘ │
```

### Success State — Download

Modal closes automatically. An action toast fires:

```
showActionToast("Bundle downloaded.", "success", `bundle-export-${sessionId}`)
```

### Success State — Save on Server

```
┌─────────────────────────────────────────┐
│  Export Session Diagnostics          ✕  │
│  ─────────────────────────────────────  │
│                                         │
│  ✓  Bundle saved                        │
│     ~/.stapler-squad/diagnostics/        │
│     my-session-20261016-143022.zip      │
│                                         │
│  ┌──────────────────────────────────┐   │
│  │  Create backlog item →           │   │  (S4; prominent, not secondary)
│  └──────────────────────────────────┘   │
│                                         │
│  ┌──────────────┐                       │
│  │    Close     │                       │
│  └──────────────┘                       │
└─────────────────────────────────────────┘

ARIA: role="status" aria-live="polite" on the success paragraph
```

### Error State

```
┌─────────────────────────────────────────┐
│  Export Session Diagnostics          ✕  │
│  ─────────────────────────────────────  │
│                                         │
│  ┌────────────────────────────────────┐ │
│  │  role="alert"                      │ │
│  │  Export failed: <error message>    │ │
│  │  [Retry]  [Try Download instead]   │ │  "Try Download" only for server-save errors
│  └────────────────────────────────────┘ │
│                                         │
│  [delivery options re-enabled]          │
│  [consent checkbox re-enabled]          │
│                                         │
│  ┌──────────────┐  ┌──────────────┐     │
│  │    Cancel    │  │    Export    │     │
│  └──────────────┘  └──────────────┘     │
└─────────────────────────────────────────┘

Action toast also fires:
showActionToast("Bundle assembly failed: <error>", "error", `bundle-export-${sessionId}`)
```

### Error State — OTel Disabled (AC11)

```
│  ┌────────────────────────────────────┐  │
│  │  Note: OpenTelemetry is disabled.  │  │
│  │  The bundle will not include trace │  │
│  │  data but is otherwise complete.   │  │
│  └────────────────────────────────────┘  │
```

This is a non-blocking informational notice (not `role="alert"`), shown after the consent section if the backend reports OTel is disabled. Export proceeds normally.

### Interaction Flow

1. Modal opens (focus moves to first focusable element: "Download .zip" radio).
2. Mobile context (`window.innerWidth < 640`): "Save on server" radio pre-selected.
3. User selects delivery mode via click or keyboard.
4. User reads consent notice and checks checkbox.
5. Export button becomes enabled.
6. User clicks Export (or presses Enter on focused button).
7. UI enters in-progress state (button label swaps, inputs disabled, status region announces).
8. On success:
   - **Download**: modal closes, toast fires.
   - **Server-save**: modal shows success state with file path and S4 button.
9. On error: error banner appears (`role="alert"`), inputs re-enabled, Retry available.

### Keyboard Navigation

| Key | Action |
|-----|--------|
| Tab / Shift+Tab | Cycles through: radio group → consent checkbox → Cancel → Export |
| Arrow keys (inside radio group) | Moves between "Download .zip" and "Save on server" |
| Space | Toggles consent checkbox; activates focused button |
| Enter | Activates focused button |
| Escape | Closes modal (cancels if in-progress: Cancel action fires) |

---

## S4 — "Create backlog item →" Inline Button

Appears inside `ExportBundleModal` after a successful server-save. High emotional value: closes the loop between capturing a bug and tracking it.

### Wireframe

```
┌──────────────────────────────────────────────────┐
│  ✓  Bundle saved                                  │
│     ~/.stapler-squad/diagnostics/                  │
│     my-session-20261016-143022.zip                │
│                                                    │
│  ┌────────────────────────────────────────────┐   │
│  │  Create backlog item →                     │   │
│  └────────────────────────────────────────────┘   │
│  Pre-populated with: session title, bundle path,  │
│  timestamp, and a summary.                        │
│                                                    │
│  [Close]                                          │
└──────────────────────────────────────────────────┘

Button: full-width, primary style (not secondary or ghost)
aria-label="Create backlog item from this diagnostic bundle"
data-testid="create-backlog-item-from-bundle"
```

### Interaction Flow

1. User sees the success state with file path.
2. User clicks "Create backlog item →".
3. Button shows an inline spinner; label becomes "Creating…".
4. On success: modal closes; user is navigated to the new backlog item detail view (or a toast fires with a link if navigation is not desirable from the current context).
5. On error: inline `role="alert"` error message appears; "Retry" and "Close" are available.

### Pre-population Contract

The backlog item created must include:

| Field | Value |
|-------|-------|
| Title | `Diagnostic bundle: <session title> (<YYYYMMDD>)` |
| Description | Link to saved file path, session UUID, session status at time of export, timestamp |
| Labels / tags | `diagnostic-bundle` |

---

## S5 — Toast Notification Output (Non-Interactive)

These are fire-and-forget success/error toasts from `showActionToast`.

**Representative output samples:**

```
// Success — download
showActionToast("Bundle downloaded.", "success", `bundle-export-${sessionId}`)

// Success — server-save
showActionToast("Bundle saved: ~/.stapler-squad/diagnostics/my-session-20261016-143022.zip", "success", `bundle-export-${sessionId}`)

// Error
showActionToast("Bundle assembly failed: request timed out", "error", `bundle-export-${sessionId}`)
```

**Acceptance criteria (non-interactive surface):**
- Toast key is stable per session (`bundle-export-<sessionId>`) so rapid retries deduplicate.
- File path in server-save toast is the exact path returned by the RPC, not a hardcoded template.
- Error message includes the underlying reason (not just "failed").
- Toasts are announced to screen readers via the existing `aria-live` region that powers the toast deck.

---

## S6 — Server-Saved File Path Format (Non-Interactive / Headless)

**Representative output:**

```
~/.stapler-squad/diagnostics/<session-uuid>-<YYYYMMDD-HHMMSS>.zip
# e.g.
~/.stapler-squad/diagnostics/a1b2c3d4-e5f6-7890-abcd-ef1234567890-20261016-143022.zip
```

**Acceptance criteria (non-interactive surface):**
- Path is human-readable: session UUID and timestamp are both present and separated by `-`.
- Directory `~/.stapler-squad/diagnostics/` is created by the RPC handler if absent (no user action needed).
- The RPC response body returns the absolute path (tilde-expanded), not the template string.
- File name does not contain characters that are illegal on macOS, Linux, or Windows NTFS (`/ \ : * ? " < > |`).

---

## Acceptance Criteria (UX)

### AC1 — Entry points trigger export from both surfaces

- AC1.1: User can open `ExportBundleModal` in ≤ 2 clicks/taps from the session detail header ("⋯" → "Export Diagnostics").
- AC1.2: User can open `ExportBundleModal` in ≤ 3 clicks/taps from a session list row ("⋯" → Workflow accordion → "Export Diagnostics").
- AC1.3: "Export Diagnostics" appears before the destructive `<hr>` divider in the action sheet.
- AC1.4: "Export Diagnostics" appears in the "Workflow" accordion section of `SessionActionsOverflow`, after "Create Checkpoint".

### AC3 — Download triggers a browser file download

- AC3.1: Selecting "Download .zip" and clicking Export initiates a browser file download within 5 seconds.
- AC3.2: The downloaded file has a `.zip` extension and a name matching the pattern `<session-title>-<YYYYMMDD-HHMMSS>.zip`.
- AC3.3: On iOS where `<a download>` is unreliable, the feature uses `navigator.share({ files: [...] })` when `navigator.canShare?.({ files: [blob] })` returns `true`; falls back to `<a download>` otherwise.
- AC3.4: Modal closes automatically on a successful download; a success toast fires.

### AC4 — Save on server shows file path confirmation

- AC4.1: Selecting "Save on server" and clicking Export saves the bundle and shows the absolute file path in the modal success state.
- AC4.2: The file path shown in the modal matches the path in the action toast.
- AC4.3: On mobile (`window.innerWidth < 640`), "Save on server" is pre-selected as the default delivery mode.

### AC5 — "Create backlog item →" is available after server-save

- AC5.1: The "Create backlog item →" button is visible and focused after a successful server-save (it is the first focusable element in the success state).
- AC5.2: Activating the button creates a `BacklogItem` pre-populated with the session title, bundle file path, and timestamp.
- AC5.3: The button is not shown after a download-only export.
- AC5.4: If backlog item creation fails, an inline `role="alert"` error appears with a Retry option; the modal does not close.

### AC9 — Bundle assembly gives feedback within 5 seconds

- AC9.1: Within 200 ms of clicking Export, the button label changes to "Assembling bundle…" and an inline CSS spinner appears.
- AC9.2: At 2 s elapsed, a sub-label "This may take up to 5 seconds." appears (does not replace the spinner; additive).
- AC9.3: At 5 s elapsed without completion, the label changes to "Taking longer than expected…" and a Cancel button is enabled.
- AC9.4: During assembly, the `role="status"` / `aria-live="polite"` region announces "Assembling bundle…" to screen readers.
- AC9.5: Clicking Cancel during assembly closes the modal and does not leave a partial bundle on disk.

### AC11 — Graceful degradation when OTel is disabled

- AC11.1: When OTel is disabled, an informational (non-blocking) notice appears in the modal before the user clicks Export: "OpenTelemetry is disabled. The bundle will not include trace data but is otherwise complete."
- AC11.2: The Export button is not disabled by the OTel notice; export proceeds normally.
- AC11.3: The downloaded/saved bundle is valid (parseable JSON manifest, required fields present) even when OTel data is absent.
- AC11.4: The notice is not shown when OTel is enabled.

### Accessibility

- AC-A1: The entire `ExportBundleModal` is keyboard-navigable: Tab order follows the visual reading order (radio group → consent checkbox → Cancel → Export).
- AC-A2: Escape closes the modal from any state (including in-progress; triggers Cancel).
- AC-A3: The "Export Diagnostics" action sheet item has an accessible label; screen readers announce "Export Diagnostics, button".
- AC-A4: The Export button is not merely visually disabled when consent is unchecked — it carries `aria-disabled="true"` so screen readers surface the state.
- AC-A5: Color contrast for all text and interactive elements in the modal meets WCAG AA (≥ 4.5:1 for normal text, ≥ 3:1 for large text and UI components), in both light and dark themes.
- AC-A6: The error banner uses `role="alert"` (assertive live region) so screen readers announce it immediately without waiting for the user to focus it.
- AC-A7: The "Create backlog item →" button has an explicit `aria-label="Create backlog item from this diagnostic bundle"` (the "→" glyph is decorative and must not be read aloud).

---

## Open Questions for Implementation

1. **Focus management after download success**: when the modal auto-closes, where should focus return? Recommended: return to the triggering "⋯" button in the session detail header (`SessionDetailView`).
2. **Backlog item navigation after creation**: should activating "Create backlog item →" navigate the user to the new item, or stay in the modal and show a link? Either is acceptable; pick the pattern used by "Create Checkpoint → View checkpoint" in the existing codebase.
3. **Consent text review**: the consent notice copy above is a draft. Final copy should be reviewed by the product owner to confirm scope accuracy before the feature ships.
