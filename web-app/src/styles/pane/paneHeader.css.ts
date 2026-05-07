import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme.css";

export const paneHeader = style({
  height: "32px",
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `0 ${vars.space["2"]}`,
  background: vars.color.cardBackground,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  flexShrink: 0,
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  userSelect: "none",
  cursor: "default",
  "@media": {
    // Hide pane header below 768px — SessionDetail's own tab bar (and MobilePaneTabStrip when
    // multiple panes exist) covers navigation. The 769–900px range has no pane header either,
    // but the BottomNav only appears below 900px (layout.css.ts cockpitRoot), so a device in
    // that range gets a full-height cockpit minus any bottom-nav padding. This is intentional:
    // tablets in portrait orientation have enough vertical room and the pane header would waste
    // space that the terminal benefits from.
    "(max-width: 768px)": {
      display: "none",
    },
  },
});

export const paneTitle = style({
  flex: 1,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.xs,
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
