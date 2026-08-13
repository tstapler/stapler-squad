import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme.css";

export const mobileTabStrip = style({
  display: "flex",
  borderTop: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  overflowX: "auto",
  flexShrink: 0,
  height: "40px",
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

export const mobileAddPaneButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  height: "32px",
  width: "32px",
  padding: 0,
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  cursor: "pointer",
  fontSize: "18px",
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

export const mobileTabButton = recipe({
  base: {
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    height: "32px",
    padding: `0 ${vars.space["3"]}`,
    background: "transparent",
    border: `1px solid ${vars.color.borderColor}`,
    borderRadius: vars.radii.md,
    cursor: "pointer",
    fontSize: vars.fontSize.xs,
    fontFamily: vars.font.mono,
    whiteSpace: "nowrap",
    flexShrink: 0,
    transition: "background 100ms, color 100ms, border-color 100ms",
  },
  variants: {
    active: {
      true: {
        background: vars.color.primary,
        color: vars.color.textInverse,
        borderColor: vars.color.primary,
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
