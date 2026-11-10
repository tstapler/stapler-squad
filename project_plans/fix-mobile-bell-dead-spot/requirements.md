# Requirements: Fix Mobile Bell Dead Spot

**Project:** fix-mobile-bell-dead-spot  
**Item:** Clicking on the mobile UI in line with the notification bell has a dead spot where I can't select things, this gets in the way when picking things from our list

## Problem Statement

On the mobile/narrower-desktop UI, the floating notification bell (`TrayHandle`) is a `position: fixed` element (44 px wide, ≥88 px tall) that sits on the right edge of the viewport at high z-index. Because it isn't accounted for in the layout of scrollable list content (session list, backlog list, etc.), any list item that visually extends to the right edge of the viewport is **unclickable** in the band that overlaps the bell — the fixed handle intercepts all pointer events before they reach the content underneath.

## Scope

- Affects the desktop edge TrayHandle (always visible when `isInnerScreen` is true)  
- May also affect the optional mobile floating bell (when `floatingBellEnabled` is true on a phone)  
- Primary repro: session list and backlog list at mid-width / narrow desktop viewports (900–1099 px) where list items extend to the full container width  

## Acceptance Criteria

1. List items (session cards, backlog rows) are fully clickable across their entire width — no dead zone at the right edge where the TrayHandle overlaps.
2. When the notification tray is open and the handle shifts to `right: min(400px, 40vw)`, the dead zone also moves / disappears correctly.
3. The fix does not change the visible size or shape of list cards on wide desktop (≥1100 px) where the list sits in a fixed-width left column and the handle never overlaps it.
4. The fix applies to all affected list views: session list, backlog list (and any other full-width list in the scrollable body area).
5. A unit or snapshot test confirms the right-side spacing is applied at the target viewport widths.

## Out of Scope

- Redesigning the TrayHandle position or making it disappear  
- Changing any server-side Go code  
- Changing the optional phone floating bell behavior (separate opt-in feature)
