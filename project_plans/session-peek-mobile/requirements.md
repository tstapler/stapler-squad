# Requirements: Session Peek Modal on Mobile Picker

## Item
Backlog item b4014bc3-96f2-42bf-b351-d72eb4a5ba5b

## Problem Statement
When using the mobile bottom-sheet "Open session in…" picker, there is no option to open a session in a temporary modal overlay. On desktop, the picker action bar already has "Open in Modal (M)" (keyboard shortcut M), which renders a `SessionPeekModal` over the existing layout without modifying the pane tree. This option is absent from the mobile/narrow-screen bottom sheet, forcing mobile users to either split into a new full-screen pane or replace an existing one — both of which persist layout changes the user may not want.

## Screenshot Evidence
The attached screenshot (`1793198167038-1009338430-10573.png`) shows the mobile bottom-sheet with:
- "New full-screen pane"
- "A stapler-squad-perf"
- "B stapler-squad"
- "C arp-projection-cache-and-sharding"
- "D arp-read-mvp"
- "E kibitzer"
- "F nkp-deck-spike"
- "Cancel"

No temporary/modal option is present.

## Scope
Add a "Temporary modal" (or "Open in overlay") option to the mobile bottom-sheet picker. Tapping it should open the session in the existing `SessionPeekModal` component (which already renders as a fixed full-coverage overlay with Escape-to-close) and close the picker sheet without modifying the pane tree.

## Out of Scope
- Any changes to `SessionPeekModal` itself (it already works correctly)
- Changes to the desktop picker action bar (already has the modal option)
- New backend/API changes (purely front-end)

## Acceptance Criteria
1. The mobile bottom-sheet includes a "Temporary modal" button positioned between "New full-screen pane" and the existing-panes list.
2. Tapping "Temporary modal" opens the `SessionPeekModal` for the picked session and dismisses the bottom-sheet picker.
3. The `SessionPeekModal` opened via mobile picker is identical in behavior to the one opened via desktop M shortcut (Escape closes it, backdrop tap closes it, full `SessionDetail` is rendered, "Peek" badge is shown).
4. The button has a `data-testid="mobile-picker-peek-modal"` attribute.
5. The mobile picker test file (`PaneTilingContainer.mobilePicker.test.tsx`) has a test covering the new button: it asserts the button is present, clicking it closes the sheet, and the peek modal is rendered.
6. Existing mobile picker tests continue to pass.
