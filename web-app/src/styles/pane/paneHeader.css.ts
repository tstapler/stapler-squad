import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme.css";

export const paneHeader = style({
  minHeight: "32px",
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  background: vars.color.cardBackground,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  flexShrink: 0,
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  userSelect: "none",
  cursor: "default",
  "@media": {
    // Hide pane header at ≤768px — SessionDetail's own tab bar (and MobilePaneTabStrip when
    // multiple panes exist) covers pane navigation at that width. At 769–900px the pane header
    // IS visible; cockpitRoot still subtracts the BottomNav height at that range (≤900px) so
    // both can coexist. Above 900px the BottomNav disappears and the full 100dvh is available.
    "(max-width: 768px)": {
      display: "none",
    },
  },
});

export const paneTitle = style({
  flex: 1,
  minWidth: 0,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.xs,
});

// Groups all action buttons so they wrap together as a unit when the pane is too narrow
// to show both the session name and the buttons on one line.
export const paneHeaderActions = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  flexShrink: 0,
});

export const paneHeaderButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "20px",
  height: "20px",
  padding: 0,
  background: "transparent",
  border: "none",
  borderRadius: vars.radii.sm,
  color: vars.color.textMuted,
  cursor: "pointer",
  fontSize: vars.fontSize.xs,
  flexShrink: 0,
  transition: "background 100ms, color 100ms",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
});

export const paneCloseButton = style([
  paneHeaderButton,
  {
    selectors: {
      "&:hover": {
        background: vars.color.errorBg,
        color: vars.color.error,
      },
    },
  },
]);

export const paneTabButton = recipe({
  base: {
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    height: "20px",
    padding: `0 ${vars.space["1"]}`,
    background: "transparent",
    border: "none",
    borderRadius: vars.radii.sm,
    cursor: "pointer",
    fontSize: vars.fontSize.xs,
    fontFamily: vars.font.mono,
    transition: "background 100ms, color 100ms",
  },
  variants: {
    active: {
      true: {
        background: vars.color.primary,
        color: vars.color.textInverse,
      },
      false: {
        color: vars.color.textMuted,
        selectors: {
          "&:hover": {
            background: vars.color.hoverBackground,
            color: vars.color.textPrimary,
          },
        },
      },
    },
  },
  defaultVariants: { active: false },
});
