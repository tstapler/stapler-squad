# UX Research: PR Panel Actions

## Existing card structure (PRCard.tsx)

The card is a vertical flex column:
1. `prHeader` — PR title link + chip row (CI/draft/approvals/conflict chips)
2. `prMeta` — repo, branch, optional host/worktree spans
3. Optional `changesRequestedNote` paragraph
4. `sessionList` — linked sessions with Open Session buttons
5. `NudgeButton` (when sessions present)
6. `prActions` — the current action zone: "Open PR on GitHub" (when changes requested) + "No session / + Session" (when no sessions)

The card's `prActions` row already collapses to `flexDirection: column; alignItems: stretch` at 480px via the `NARROW` media query in `PRCard.css.ts`.

All interactive controls use `MIN_TOUCH_TARGET` (44px) from `prTouchTokens.ts` — WCAG 2.5.5 compliance is already baked into the chip pattern.

## Where to place the action buttons

Put the new Merge / Close / Mark Ready / Convert to Draft buttons **inside the existing `prActions` div**, appended after the current "+ Session" button. Do not introduce a new wrapper row; the existing flex-wrap + narrow-breakpoint column-stretch already handles mobile reflow correctly.

Rationale: the `prActions` div is the card's designated mutation zone. Adding buttons there keeps the visual hierarchy consistent (info above, actions below) and avoids a second flex row that would fragment touch targets on narrow screens. The "+ Session" button already sits here and its style (`createSessionButton`) is a low-prominence ghost button — action buttons that can actually destroy state (Close, merge) should be visually heavier.

For the merge button specifically, use a **split button** pattern: a primary "Merge" button (using `vars.color.primary`/`vars.color.primaryText`) joined by a small chevron/caret button that opens a popover selecting merge method (merge commit, squash, rebase). This keeps the most-common action one tap but exposes method choice without permanently taking up horizontal space. On mobile the split button stacks naturally because `prActions` switches to `flexDirection: column`.

There is no existing split-button primitive in the codebase (the grep for "SplitButton/ButtonGroup" returned test files only, not a component). A new `SplitButton.tsx` + `SplitButton.css.ts` in `web-app/src/components/ui/` would be the right home for it; or a self-contained inline implementation directly in the card is acceptable for a first pass given the narrow scope.

## Merge-method selector on mobile

On mobile the chevron should open a **bottom-sheet-style popover** using `position: fixed` + `createPortal(..., document.body)` (required by the css-architecture rule — `position: fixed` breaks when any ancestor has `transform`/`filter`). The sheet appears anchored to the bottom of the viewport, full-width, with three tappable rows (Merge commit / Squash / Rebase) each at `minHeight: 44px`. Each row should emit `aria-checked` (it is a single-select group, `role="radiogroup"` with `role="radio"` children) so screen readers announce the selection.

On desktop the same popover can appear as a small dropdown anchored below the caret button (absolute-positioned within a `position: relative` wrapper). The css-architecture rule says "always `createPortal` for overlays that must escape the component tree", so use a portal here too and position it via `getBoundingClientRect` + `useLayoutEffect` to avoid stacking context issues.

The selected merge method should be persisted to `localStorage` under a key like `ssq.pr.mergeMethod` so the user's preference survives navigation. Default: `merge`.

## Extended info (body, labels, stats): collapsed by default

Collapse the description preview, labels, and +N/-N/N files stats behind a disclosure button ("Show details" / "Hide details") that lives between `prMeta` and `changesRequestedNote`. Default: closed.

Rationale:
- The current mobile screenshot shows 3–4 PR cards visible at once without scrolling. Showing extended info by default would push lower cards off-screen, requiring scroll to see whether any PR needs attention.
- The user's primary job-to-be-done is status triage, not reading body text. The chips already surface the high-signal state (CI, conflicts, approvals). Extended info is a secondary look before acting.
- Use `aria-expanded` + `aria-controls` on the disclosure button (same pattern as `FailingChecks` in the card today — that component is the existing precedent).

The description preview should be a `<p>` capped at 300 characters with a "…" suffix, rendered in `vars.fontSize.xs`. Labels render as small chips using the existing `chipNeutral`/`chipBase` style. Stat badges (`+N` in green, `-N` in red, `N files` in neutral) render as `vars.fontSize.xs` spans with semantic color tokens (`vars.color.successText`/`vars.color.errorText`) — no new chip variant needed.

## Confirmation flow for destructive actions

### Close PR (destructive)

Use the **existing modal pattern** from `DeleteSessionConfirmModal.tsx` / `BoardCompleteConfirmDialog.tsx`:
- `createPortal(..., document.body)` overlay
- `role="dialog" aria-modal="true" aria-labelledby={headingId}`
- `useFocusTrap` (already in `web-app/src/lib/hooks/useFocusTrap.ts`, used by `CreatePullRequestModal` and `ConfirmKillDialog`)
- Escape key dismisses, backdrop click dismisses (guard against `isSubmitting`)
- Danger button styled with `vars.color.error` / `vars.color.textInverse` (same as `killButton` in `ConfirmKillDialog.css.ts`)
- Auto-focus the Cancel button (not the destructive button) — matches `DeleteSessionConfirmModal`'s `autoFocus` on Cancel

The dialog text: "Close PR #N: {title}?" / "This cannot be undone." / Cancel + Close PR buttons.

### Merge PR (irreversible but not destructive)

Merge does not require a confirmation dialog — GitHub's own UI merges on a single click. However, the button must be disabled while the RPC is in flight and must surface the result clearly (see success feedback below).

### Convert to Draft / Mark Ready

No confirmation needed — these are easily reversible. Toggle inline with an optimistic update.

## ARIA patterns for action buttons

| Button | role | state | notes |
|---|---|---|---|
| Merge (primary) | `button` | `aria-disabled` when CI failing or merge conflict | disable if PR has known blockers |
| Merge method caret | `button` | `aria-expanded` + `aria-haspopup="listbox"` | opens method picker |
| Method picker | `listbox` or `radiogroup` | `aria-checked` per option | portal-rendered |
| Close PR | `button` | standard | triggers dialog |
| Mark Ready / Convert to Draft | `button` | — | label changes based on `pr.isDraft` |
| Show/hide details disclosure | `button` | `aria-expanded` + `aria-controls` | same as FailingChecks today |

All buttons inherit `min-height: 44px` via `MIN_TOUCH_TARGET`.

## Success feedback (critical anxiety: "did it actually happen?")

The existing codebase has a `ToastStack` + `NotificationContext` / `useNotificationCommands` for transient in-app toasts — this is the right channel for post-action feedback. The `useNotificationCommands` hook (from `notificationContexts.ts`) exposes an `add` / `notify` path that fires a toast.

Pattern to follow after a successful merge/close:

1. While the RPC is in-flight: button label changes to "Merging…" / "Closing…" and `disabled={true}`.
2. On RPC success: fire a toast ("PR #N merged" / "PR #N closed") and refetch PR data so the card either disappears (closed/merged) or reflects the new state. Use `role="alert"` on the toast or the `AnnouncerProvider`'s announce path (already wired in `NotificationContext.tsx`) for screen-reader announcement.
3. On RPC failure: show an inline `role="alert"` error message inside the card (same `vars.color.errorBg`/`vars.color.errorText` pattern as `ConfirmKillDialog.css.ts`'s `errorState`). Do not use a toast for errors — inline feedback keeps the context visible.

## CSS / vanilla-extract notes

- New styles go in `PRCard.css.ts` for card-level additions; if the split-button or method popover is extracted to `ui/`, they get their own `.css.ts`.
- All tokens via `vars.color.*` — no hardcoded hex.
- Any new `zIndex` slot (for the method popover) must be added to `zIndex` in `theme-contract.css.ts` first — the architecture doc's explicit rule.
- The merge-method popover is a portal, so it needs a named `zIndex` slot (e.g. `zIndex.dropdown`) that sits between `zIndex.base` and `zIndex.modal`.
- The mobile breakpoint constant `NARROW = "screen and (max-width: 480px)"` is defined in `PRCard.css.ts` — reuse it for any new media queries inside that file.
