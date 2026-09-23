import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme.css";

// Visually distinct from mobilePaneTabStrip.css.ts's container (borderTop +
// cardBackground) so the two tablists read as separate UI layers when both
// are visible: borderBottom (this strip sits above the pane strip) and the
// page background instead of the card background.
export const windowTabStrip = style({
  display: "flex",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  background: vars.color.background,
  overflowX: "auto",
  flexShrink: 0,
  height: "36px",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `0 ${vars.space["2"]}`,
  // Hide scrollbar but allow scrolling
  scrollbarWidth: "none",
  selectors: {
    "&::-webkit-scrollbar": {
      display: "none",
    },
  },
});

export const windowAddButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  height: "28px",
  width: "28px",
  padding: 0,
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  cursor: "pointer",
  fontSize: "16px",
  color: vars.color.textMuted,
  flexShrink: 0,
  marginLeft: "auto",
  transition: "background 100ms, color 100ms",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
});

export const windowTabWrapper = style({
  position: "relative",
  display: "flex",
  alignItems: "center",
  flexShrink: 0,
  maxWidth: "160px",
});

export const windowTabButton = recipe({
  base: {
    display: "flex",
    alignItems: "center",
    height: "28px",
    padding: `0 ${vars.space["3"]}`,
    background: "transparent",
    border: `1px solid ${vars.color.borderColor}`,
    borderRadius: vars.radii.md,
    cursor: "pointer",
    fontSize: vars.fontSize.xs,
    fontFamily: vars.font.mono,
    whiteSpace: "nowrap",
    overflow: "hidden",
    textOverflow: "ellipsis",
    maxWidth: "160px",
    flexShrink: 0,
    transition: "background 100ms, color 100ms, border-color 100ms",
  },
  variants: {
    active: {
      true: {
        background: vars.color.primary,
        color: vars.color.textInverse,
        borderColor: vars.color.primary,
        fontWeight: "bold",
        textDecoration: "underline",
      },
      false: {
        color: vars.color.textSecondary,
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

export const windowTabCloseButton = recipe({
  base: {
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    height: "16px",
    width: "16px",
    marginLeft: vars.space["1"],
    padding: 0,
    background: "transparent",
    border: "none",
    borderRadius: vars.radii.sm,
    cursor: "pointer",
    fontSize: "12px",
    lineHeight: "16px",
    color: "inherit",
    opacity: 0,
    transition: "opacity 100ms, background 100ms",
    selectors: {
      "&:hover": {
        background: vars.color.hoverBackground,
      },
    },
  },
  variants: {
    active: {
      true: { opacity: 1 },
      false: {},
    },
  },
  defaultVariants: { active: false },
});

export const windowTabInput = style({
  height: "22px",
  width: "100%",
  padding: `0 ${vars.space["1"]}`,
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
});
