import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "./theme.css";

/**
 * Cockpit root — CSS Grid shell.
 *
 * Column 1: drawer nav (width driven by --drawer-width CSS custom property,
 *           set inline on the element so the drawer's own width controls the grid).
 * Column 2: main content (fills remaining space).
 */
export const cockpitRoot = style({
  display: "grid",
  gridTemplateColumns: "auto 1fr",
  height: "var(--viewport-height, 100dvh)",
  overflow: "hidden",
  backgroundColor: vars.color.background,
  color: vars.color.textPrimary,
  "@media": {
    // BottomNav is position:fixed below 900px. Shrink cockpit so content never renders
    // underneath it. Breakpoint relationship:
    //   ≤768px  — pane header hidden (paneHeader.css.ts), BottomNav visible, cockpit shrunk
    //   769–900px — pane header visible, BottomNav visible, cockpit shrunk
    //   >900px   — pane header visible, BottomNav hidden, full 100dvh available
    "(max-width: 900px)": {
      height: "calc(100dvh - var(--bottom-nav-height, 72px))",
    },
  },
});

export const drawerColumn = style({
  // Width is controlled by the DrawerNav component itself (recipe variant).
  // This column just needs to not overflow.
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
});

export const mainContent = style({
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
  minWidth: 0, // prevent grid blowout
});
