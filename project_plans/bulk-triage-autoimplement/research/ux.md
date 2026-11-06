# UX Research: Bulk Triage + Bulk Auto-Implement

## 1. Existing Delete Confirmation Dialog Pattern

Source: `web-app/src/components/backlog/BacklogBulkActions.tsx` (lines 79–107)

The existing bulk delete confirmation uses:
- A full-screen `overlay` div (`onClick` → cancel) rendered inline after the toolbar `<div>` (NOT via `createPortal`)
- A `dialog` div with `role="dialog"`, `aria-modal="true"`, `aria-labelledby`, `onClick` stopPropagation, and `onKeyDown` (Escape → cancel)
- `autoFocus` on the Cancel button (so keyboard users immediately land on the safe action)
- No `useFocusTrap` — Tab can escape the dialog. This is a known gap in the existing delete dialog; the new auto-implement dialog MUST fix this.

CSS classes live in `BacklogBulkActions.css.ts`: `overlay` (fixed, rgba(0,0,0,0.5), centered flex), `dialog` (card bg, 8px radius, 1.5rem padding, max 420px wide), `dialogActions` (flex, flex-end, 0.75rem gap, 1rem top margin).

## 2. Proper Focus Trap Pattern (Project Standard)

All other dialogs in the codebase use the `useFocusTrap` hook from `@/lib/hooks/useFocusTrap`:

```tsx
const dialogRef = useRef<HTMLDivElement>(null);
const triggerRef = useRef<HTMLButtonElement>(null);  // the button that opened the dialog
useFocusTrap(dialogRef, isOpen, triggerRef);
```

- `useFocusTrap` moves focus to the first focusable child on activation, traps Tab/Shift+Tab within the container, and restores focus to `triggerRef` on deactivation.
- Escape is handled separately via `useEffect(() => { document.addEventListener("keydown", handleKeyDown); ... }, [isOpen])` — `useFocusTrap` does NOT handle Escape itself (confirmed: `JulesDispatchDialog.tsx` line 114, `BacklogFileBrowserModal.tsx`).
- Precedents: `JulesDispatchDialog.tsx`, `BacklogFileBrowserModal.tsx`, `DeleteSessionConfirmModal.tsx` (parent provides the ref).

The new auto-implement dialog must use this pattern. The existing bulk-delete dialog does NOT — it is a pre-existing gap but out of scope to fix here.

## 3. Available CSS Classes and Theme Variables

**From `BulkActions.css.ts`** (already imported in BacklogBulkActions):
- `actionButton` — primary CTA style (blue, hover lift, disabled 40% opacity)
- `danger` — error color override for destructive actions
- `clearButton` / `selectAllButton` — secondary/ghost style
- `feedback` — status text below toolbar

**From `BacklogBulkActions.css.ts`** (local):
- `overlay`, `dialog`, `dialogActions` — reusable for the new confirmation dialog

**Theme vars** for the autonomous-execution warning block:
- `vars.color.warning` — amber tone (used in log badges)
- `vars.color.warningBg` — background for warning callouts
- `vars.color.warningText` — text on warning backgrounds

A new `warningBlock` style should be added to `BacklogBulkActions.css.ts` using these three vars for the "sessions will run autonomously" callout inside the auto-implement dialog.

## 4. Ineligibility Communication Strategy

### Auto-implement confirmation dialog (required)
Show counts directly in the dialog body before the user confirms:
```
Start autonomous sessions for 5 items?
3 items are ineligible (wrong status) and will be skipped.
⚠ Sessions will run autonomously without manual steps.
```
This front-loads the skip count where the user can act on it (deselect and re-select) before committing.

### Triage button (no dialog)
A tooltip is the cleanest UX, but the codebase has no shared Tooltip component (tooltips appear only in chart libraries). The pragmatic option: use the HTML `title` attribute for a basic browser tooltip (`title="3 of 8 items are ineligible and will be skipped"`), or include the skip count in the 4s post-action feedback string (`"Triage triggered for 5 items (3 skipped — ineligible)"`). The feedback string is simpler, already has a display slot (`role="status"` div), and requires no new component. A `title` attribute can be added in addition at negligible cost.

## 5. Auto-Implement Dialog: Full Spec

```tsx
// State
const [confirmingAutoImplement, setConfirmingAutoImplement] = useState(false);
const dialogRef = useRef<HTMLDivElement>(null);
const triggerRef = useRef<HTMLButtonElement>(null); // ref on the "Auto-implement selected" button

useFocusTrap(dialogRef, confirmingAutoImplement, triggerRef);

// Escape handler
useEffect(() => {
  if (!confirmingAutoImplement) return;
  const handle = (e: KeyboardEvent) => { if (e.key === "Escape") setConfirmingAutoImplement(false); };
  document.addEventListener("keydown", handle);
  return () => document.removeEventListener("keydown", handle);
}, [confirmingAutoImplement]);
```

Dialog content:
- Title: `"Start autonomous sessions for {eligibleCount} {noun}?"`
- If `skipCount > 0`: secondary text: `"{skipCount} {skipNoun} ineligible and will be skipped."`
- Warning block (amber, `warningBg`/`warningText`): `"Sessions will run autonomously without manual steps."`
- Cancel button (`clearButton`, `autoFocus`) — safe default
- Confirm button (`actionButton`) — `"Start {eligibleCount} sessions"`

Keyboard: Tab cycles between Cancel and Confirm (handled by `useFocusTrap`). Escape cancels (handled by `useEffect` above). Enter on focused Confirm button fires it naturally (no special handling needed — button default).

Focus returns to the "Auto-implement selected" button when dialog closes (handled by `useFocusTrap` cleanup + `triggerRef`).

## 6. Triage Button: No Confirmation, Immediate Feedback

Pattern mirrors the existing Archive action (line 59–67):
- Directly calls `onTriageSelected()` on click, no confirmation dialog
- Sets a feedback string for ~4s via the existing `feedback` prop mechanism
- Feedback text: `"Triage started for {n} {noun}."` or `"Triage started for {n} {noun} ({skipCount} skipped — ineligible)."` if skipCount > 0
- Button disabled when `busy=true` (same guard as Archive/Delete)
- Optional: `title` attribute with skip info while items are selected

## 7. Component Props Change

Add to `BacklogBulkActionsProps`:
```ts
onTriageSelected: () => void;
onAutoImplementSelected: () => void;
/** Counts of eligible vs. ineligible items for the confirmation dialog and tooltip. */
triageEligibleCount?: number;
autoImplementEligibleCount?: number;
triageSkipCount?: number;
autoImplementSkipCount?: number;
```

The parent (BacklogBoard or equivalent) computes eligibility — it already knows item statuses — and passes counts down. This keeps BacklogBulkActions a pure presentational component.
