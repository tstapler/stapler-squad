# Plan: Terminal Bar Layout Fix

**Date:** 2026-10-07
**Scope:** `web-app/src/components/sessions/` (frontend only)

## Approach

Two targeted changes to `TerminalOutput.tsx` and `TerminalOutput.css.ts` following the
established mobile overflow row pattern already in the codebase.

### Change 1 — Icon-only Redraw button at ≤480 px

**File:** `web-app/src/components/sessions/TerminalOutput.css.ts`

Add a new exported class `toolbarButtonIconOnly` (or a modifier) that hides the text
label on small phones:

```ts
// Applied to the Redraw button's text span on narrow viewports
export const toolbarButtonLabel = style({
  "@media": {
    "screen and (max-width: 480px)": {
      display: "none",
    },
  },
});
```

**File:** `web-app/src/components/sessions/TerminalOutput.tsx`

Wrap the Redraw button text in a `<span>`:

```jsx
<button
  className={styles.toolbarButton}
  onClick={...}
  aria-label="Redraw terminal (fixes a blank screen)"
  title="Redraw terminal (fixes a blank screen)"
>
  ↔️<span className={styles.toolbarButtonLabel}> Redraw</span>
</button>
```

This preserves the icon-only 44×44 px touch target; the `aria-label` and `title`
remain unaffected, so screen readers and long-press tooltips still work.

**Breakpoint rationale:** 480 px catches portrait iPhones (375–390 px) and smaller
Android phones without affecting tablets or landscape phones (≥481 px typically).
Existing breakpoint at 768 px is too wide — tablets should keep the label.

### Change 2 — Gallery, Files, Camera in mobile overflow row

**File:** `web-app/src/components/sessions/TerminalOutput.tsx`

Move the three upload buttons into `mobileOverflowRow`. They currently live in
`toolbarActions` (expanded inline section). Add them to the overflow row:

```jsx
{mobileOverflowOpen && toolbarExpanded && (
  <div className={styles.mobileOverflowRow} data-testid="toolbar-overflow-row">
    {/* Existing secondary actions */}
    {secondaryActions.map(...)}
    {/* NEW: Upload buttons in overflow row so they're reachable on narrow screens */}
    <button className={styles.toolbarButton} onClick={handleGalleryButtonClick} ...>
      🖼️ Gallery
    </button>
    <button className={styles.toolbarButton} onClick={handleFilesButtonClick} ...>
      📁 Files
    </button>
    <button className={`${styles.toolbarButton} ${styles.mobileOnlyUpload}`} ...>
      📷
    </button>
  </div>
)}
```

The upload buttons can remain in `toolbarActions` (expanded section) for desktop where
`mobileOverflowRow` is hidden (`display: none` outside the mobile media query). No
desktop behavior changes.

**Alternative considered:** Removing Gallery/Files from `toolbarActions` entirely and
putting them only in the overflow row would hide them on desktop too, because
`mobileMoreButton` is `display: none` on desktop. The correct approach is to keep them
in BOTH places: in `toolbarActions` for desktop, and duplicated in `mobileOverflowRow`
for mobile. Because these buttons open `<input>` file pickers via refs
(`galleryInputRef`, `filesInputRef`), the same refs can be triggered from either
location — the hidden `<input>` elements remain in `toolbarActions` regardless.

### Change 3 — CSS tightening (low risk, optional)

**File:** `web-app/src/components/sessions/TerminalOutput.css.ts`

If visual testing shows residual dead space after changes 1 and 2, tighten the
`toolbarToggle` and `mobileKeyboardToggle` padding at ≤480 px from `0.5rem` to `0.3rem`
(while keeping `minWidth/minHeight: 44px`). This recovers ~8 px per button.

### Non-changes

- **`ScrollModeChip`**: No change. Its width reflects the current scroll mode label;
  restyling it is a separate concern. After Change 1, ample space exists for it.
- **Desktop styles**: No change. All modifications are inside `@media (max-width: Xpx)`
  blocks or the mobile overflow row (which is `display: none` on desktop).
- **`data-testid` attributes**: Preserved exactly — no test assertion changes required.
- **Accessibility**: The Redraw button icon-only change (Change 1) retains its
  `aria-label`; the icon is decorative (wrapped in a button with an accessible name).
  WCAG 1.4.1 (color not sole means of info) is unaffected because connectivity state is
  shown via both the colored dot and the status text.

## Risk Assessment

| Risk | Likelihood | Mitigation |
|---|---|---|
| Icon-only Redraw harder to discover on first use | Medium | `title` tooltip + same icon users already see; label returns at ≥481 px |
| Gallery/Files in overflow row increases tap count on mobile by 1 (open More first) | Low | Gallery/Files are not primary actions; a 1-tap cost is acceptable |
| jscpd threshold tripped by duplicated upload buttons in overflow row | Low | Only ~3 small `<button>` elements duplicated; well below 20-line/200-token `minLines`/`minTokens` gate |
| Breakpoint at 480 px conflicts with existing 768 px styles | Very low | Additive; both can coexist in `@media` cascade |

## Adversarial Review

**Adversarial concern 1:** "Making Gallery/Files available only through 'More ▾' on mobile
means users must first expand the toolbar AND then open the overflow row — two taps vs.
one today (if they find the overflow scroll). Is this actually better?"

*Response:* Today, Gallery/Files are effectively unreachable because the horizontal
overflow is not discoverable (no visible scroll indicator on mobile). Two taps to a
visible destination is better than zero taps to a hidden one. The `mobileMoreButton` has
a visible label "More ▾" and is inside the expanded section, so the full path is:
expand toolbar (⋯) → tap More ▾ → tap Gallery/Files. This matches what all mobile
toolbar patterns (iOS share sheet, Android bottom sheet) already train users for.

**Adversarial concern 2:** "The Redraw button is described as a 'critical recovery action
for a blank screen'. Making it icon-only might cause confusion: what does ↔️ mean?"

*Response:* The `title` attribute and `aria-label` remain. Long-press on any mobile
browser shows the title as a tooltip. The icon ↔️ (left-right arrows) is a reasonable
glyph for "resize/redraw". However, a stronger icon might be 🔄 (already used for
"Reconnect") or ↩️. The icon choice can be adjusted without changing the layout approach.
This is a minor UX refinement separate from the layout fix.

**Adversarial concern 3:** "Duplicate JSX for upload buttons (in toolbarActions AND
mobileOverflowRow) violates DRY and the jscpd gate."

*Response:* The three upload buttons together are ~15 lines — below jscpd's
`minLines: 20` threshold. If they grow beyond that, extract an `<UploadButtons>`
component shared between both locations.

## Task Breakdown

1. **Add `toolbarButtonLabel` CSS class** (`TerminalOutput.css.ts`) — hide text on ≤480 px
2. **Wrap Redraw button text in `<span className={styles.toolbarButtonLabel}>`** (`TerminalOutput.tsx`)
3. **Add Gallery, Files, Camera to `mobileOverflowRow`** (`TerminalOutput.tsx`)
4. **Visual smoke-test** — open browser at 375 px, expand toolbar, open More, verify Gallery/Files visible; verify Redraw is icon-only
5. **Run Jest tests** — `pnpm jest --testPathPatterns="TerminalOutput" --no-coverage`
6. **Update/add tests** if any test asserted the old Redraw button text label

## Estimated Effort

~3–4 hours: 1h CSS + 1h JSX changes + 1h testing + 0.5h visual review + 0.5h PR.
