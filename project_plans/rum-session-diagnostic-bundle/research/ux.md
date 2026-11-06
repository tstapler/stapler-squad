# UX Research: RUM Session Diagnostic Bundle

## 1. Export Control Placement

The session detail view has two distinct UI surfaces for actions:

**Desktop: `SessionActionsOverflow` (MoreHorizontal ⋯ kebab menu)**
- Used in session list rows and the backlog detail panel.
- Renders as a fixed-position dropdown menu with collapsible accordion sections (Workflow, Organize, Modes, More), each a `<MenuSection>` with `role="menu"` and `aria-label`.
- File: `web-app/src/components/sessions/SessionActionsOverflow.tsx`

**Session detail header: ActionBar + "⋯" More Actions button → Modal action sheet**
- `SessionDetailView.tsx` renders a header `ActionBar` with individual buttons and a single "⋯" `moreActionsButton` (`aria-label="Session actions"`, `data-testid="more-actions-button"`).
- Clicking "⋯" opens a Radix Dialog modal (`actionSheetOpen` state) containing a vertical list of `actionSheetItem` buttons (min-height 52px, touch-friendly). Current items: Pause/Resume, Rename, Edit Tags, Create Checkpoint, Switch Workspace, Restart, Spawn new shell, Delete.
- File: `web-app/src/components/sessions/SessionDetailView.tsx`

**Recommendation:** Add "Export Diagnostics" as a new item in the Modal action sheet (`SessionDetailView`), inserted before the destructive `<hr>` divider. For the list-view kebab (`SessionActionsOverflow`), add it to the "Workflow" accordion section alongside "Create PR" and "Checkpoint". This mirrors where Chrome DevTools and Sentry place export actions — near workflow operations, not destructive ones. The action sheet's `actionSheetItem` style (52px min-height, 16px font) is already touch-friendly for mobile.

---

## 2. Loading / Progress Patterns for Async Operations

**Found patterns (in priority order for this feature):**

### a. Button label swap (existing export pattern)
- `useExportRules` (`web-app/src/lib/hooks/useExportRules.ts`): boolean `loading` state; button text changes to `"Exporting…"` while in flight.
- `ExportButton.tsx`: `exporting` flag changes icon; button is disabled during export.
- Appropriate for: near-instant operations (< 1s).

### b. TriageLoadingIndicator (inline progress, 5–180s range)
- `web-app/src/components/backlog/TriageLoadingIndicator.tsx`: CSS spinner + text label + elapsed seconds counter + cancel button.
- Uses `role="status"` and `aria-live="polite"` for screen-reader announcement.
- Label copy changes at 60s threshold.
- Has both full and compact pill layouts.
- Appropriate for: operations up to 3 min with a cancel path.

### c. Toast-based feedback (post-completion only)
- `showActionToast(message, "success"|"error", key)` via `useNotifications()`.
- Lightweight, deduped by key, bypasses history panel.

**Recommendation for the 5-second bundle assembly:**
Use the button-label-swap pattern while assembling (disable button, change label to `"Assembling bundle…"`), display an inline spinner next to the label. Then:
- On success (download): restore button, show `showActionToast("Bundle downloaded.", "success", "bundle-export-<sessionId>")`.
- On success (server-save): restore button, show `showActionToast("Bundle saved to server.", "success", ...)` and reveal the "Create backlog item" secondary action inline.
- On error: show `showActionToast("Bundle assembly failed: <error>", "error", ...)` and an inline `role="alert"` banner.

A full progress bar is unnecessary for a capped 5-second operation — it implies predictable throughput that doesn't apply here.

---

## 3. Comparable UX Flows in Developer Tools

**Chrome DevTools HAR export:**
- Right-click on Network panel → "Save all as HAR with content".
- Single-step action, no delivery-mode selection; file downloads immediately.
- Lesson: keep the primary path (download) to a single click; secondary path (server-save) can be secondary in a dropdown.

**Sentry session replay export:**
- Toolbar button "Export" opens a small popover with format choices.
- Feedback: button shows spinner while preparing, then triggers download.
- Lesson: a split-button or dropdown with "Download" as the default action and "Save on server" as a secondary option matches this established mental model.

**Recommended interaction flow for this feature:**
1. User clicks "Export Diagnostics" in the action sheet (or kebab menu).
2. A small inline modal or popover opens with two radio options: "Download .zip" (default) and "Save on server".
3. "Export" button triggers assembly; button becomes "Assembling bundle…" with spinner.
4. On complete: for download, browser saves file and toast confirms. For server-save, toast shows file path and a "Create backlog item →" button appears inline.

---

## 4. WCAG / Accessibility Patterns for Download Buttons

**`ExportButton.tsx` (the closest existing analogue):**
```tsx
<button
  aria-haspopup="menu"
  aria-expanded={showDropdown}
  aria-label={`Export ${logs.length} logs`}
  disabled={disabled || logs.length === 0 || exporting}
>
  {exporting ? '...' : '↓'} Export
</button>
```
The dropdown uses `role="menu"` and each option uses `role="menuitem"`.

**`SessionActionsOverflow` patterns:**
- Overflow trigger: `aria-label="More session actions"`, `aria-expanded`, `aria-haspopup="menu"`, `aria-controls="overflow-menu-<id>"`.
- Focus trap via `useFocusTrap` hook; Escape closes.

**`TriageLoadingIndicator` in-progress pattern:**
```tsx
<div role="status" aria-live="polite" aria-label={`Triage in progress, ${ariaElapsed} seconds elapsed`}>
```

**Recommendation for the export control:**
- The trigger button: `aria-haspopup="dialog"`, `aria-label="Export session diagnostics"`.
- During assembly: wrap status in `role="status"` with `aria-live="polite"`.
- Error state: `role="alert"` (assertive) on the error banner, per `ApprovalRulesPanel` pattern.
- Modal dialog: use the existing `Modal`/`ModalContent` (Radix `Dialog.Root`) which provides `aria-modal="true"` and manages focus automatically.

---

## 5. Error State UX

**If bundle assembly fails mid-way:**

Pattern 1 — Action toast (preferred for transient errors):
```ts
showActionToast("Bundle assembly failed: server error", "error", `bundle-export-${sessionId}`);
```

Pattern 2 — Inline error banner (preferred for persistent errors):
```tsx
{exportError && (
  <div className={generateErrorBanner} role="alert" data-testid="export-error-banner">
    <span>Export failed: {exportError.message}</span>
    // dismiss button with aria-label="Dismiss error"
  </div>
)}
```

**Recommendation:** Show both: an action toast for immediate feedback, plus an inline error note in the modal with a "Retry" button. Distinguish user-actionable errors (e.g., "Server storage full — try Download instead") from transient errors (e.g., "Request timed out — retry").

---

## 6. Progress Feedback for a 5-Second Operation

5 seconds is long enough that "button went silent" feels broken, but short enough that a full progress bar adds noise without value.

**Recommended UX:**
- **Phase 1 (0–2s):** Button label → `"Assembling bundle…"`, inline CSS spinner appears. No elapsed counter yet.
- **Phase 2 (2–5s):** If still running, optionally show "This may take up to 5s" as a sub-label.
- **Phase 3 (> 5s / timeout):** Show `"Taking longer than expected…"` and expose a Cancel button.

---

## 7. Job-to-Be-Done Lens

| Layer | Job | Design implication |
|---|---|---|
| **Functional** | Capture all relevant session context for a bug report in one action | Bundle must be complete and automatic; user should not need to think about what to include |
| **Emotional** | Confidence nothing was missed; relief there is now a tracked path forward | Show bundle contents summary in confirmation step; one-click "Create backlog item" reduces fear of losing context |
| **Social** | Hand diagnostic context to a teammate or future-self (the backlog) | File name should be human-readable (`session-title-YYYYMMDD-HHMMSS.zip`); backlog item pre-population removes friction in the hand-off |

The "Create backlog item from bundle" action is high-emotional-value: it closes the loop between "I found a bug" and "this is now tracked work." Present it prominently (not buried in a menu) after a server-save.

---

## 8. Mobile Considerations

**Mobile download behavior differences:**
- iOS Safari does not download `.zip` files directly; it opens a share sheet (`navigator.share()` or a "Save to Files" prompt). The `<a download>` pattern used in `ExportButton.tsx` and `useExportRules.ts` is unreliable on iOS Safari for binary files.
- Android Chrome handles `<a download>` correctly.
- The existing codebase has no mobile-specific download workaround.

**How other features handle this:**
- `ApprovalRulesPanel` hides the Export YAML button on mobile (`headerButtonsHiddenOnMobile`, `max-width: 640px`) and provides a FAB for the primary action.
- `SessionDetailView` action sheet items use 52px min-height and 16px font size — inherently mobile-optimized.

**Recommendations for this feature:**
- **Primary mobile path should be "Save on server"** (no download complications). Present this as the default option on small screens.
- **For the "Download" path on iOS**: use `navigator.share({ files: [...] })` (Web Share API level 2) with a graceful fallback to the `<a download>` pattern. Feature-detect with `navigator.canShare?.({ files: [...] })`.
- The export control lives in the `SessionDetailView` action sheet, which is already shown on mobile. No separate mobile entry point is needed.

---

## Key Sources

- `web-app/src/components/sessions/SessionDetailView.tsx` — header ActionBar, "⋯" more-actions button, action sheet modal
- `web-app/src/components/sessions/SessionActionsOverflow.tsx` — kebab menu, accordion sections, focus trap, ARIA patterns
- `web-app/src/components/logs/ExportButton.tsx` — download Blob pattern, `aria-haspopup`, `aria-label`, `aria-expanded`
- `web-app/src/lib/hooks/useExportRules.ts` — RPC → Blob → `<a download>` pattern
- `web-app/src/components/backlog/TriageLoadingIndicator.tsx` — `role="status"`, `aria-live`, elapsed label, cancel button
- `web-app/src/lib/contexts/notificationContexts.ts` — `showActionToast(message, "success"|"error", key)` API
- `web-app/src/components/sessions/ApprovalRulesPanel.tsx` — export button, mobile FAB, error banner, `role="alert"`
