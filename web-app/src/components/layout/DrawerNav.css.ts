import { style, keyframes } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars, zIndex } from "@/styles/theme.css";

const DRAWER_WIDTH_OPEN = "240px";
const DRAWER_WIDTH_CLOSED = "56px";

export const drawer = recipe({
  base: {
    position: "relative",
    height: "100dvh",
    display: "flex",
    flexDirection: "column",
    background: vars.color.cardBackground,
    borderRight: `1px solid ${vars.color.borderColor}`,
    overflow: "hidden",
    zIndex: zIndex.raised,
    transition: "width 200ms ease",
    "@media": {
      "(prefers-reduced-motion: reduce)": {
        transition: "none",
      },
      /* On mobile BottomNav handles navigation — hide sidebar entirely */
      "(max-width: 768px)": {
        display: "none",
      },
    },
  },
  variants: {
    open: {
      true: { width: DRAWER_WIDTH_OPEN },
      false: { width: DRAWER_WIDTH_CLOSED },
    },
  },
  defaultVariants: { open: true },
});

export const navList = style({
  listStyle: "none",
  margin: 0,
  padding: `${vars.space[2]} 0`,
  flex: 1,
  overflowY: "auto",
  overflowX: "hidden",
});

export const navItem = recipe({
  base: {
    display: "flex",
    alignItems: "center",
    gap: vars.space[3],
    padding: `${vars.space[2]} ${vars.space[3]}`,
    color: vars.color.textSecondary,
    textDecoration: "none",
    borderLeft: "3px solid transparent",
    transition: "background 120ms ease, border-left-color 120ms ease, color 120ms ease",
    cursor: "pointer",
    whiteSpace: "nowrap",
    userSelect: "none",
    minHeight: "44px", /* WCAG 2.1 AA touch target minimum */
    "@media": {
      "(prefers-reduced-motion: reduce)": {
        transition: "none",
      },
    },
    selectors: {
      "&:hover": {
        background: vars.color.hoverBackground,
        color: vars.color.textPrimary,
      },
    },
  },
  variants: {
    active: {
      true: {
        borderLeftColor: vars.color.primary,
        color: vars.color.textPrimary,
        background: vars.color.accentBg,
      },
      false: {},
    },
  },
  defaultVariants: { active: false },
});

export const navIcon = style({
  flexShrink: 0,
  width: "20px",
  height: "20px",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  fontSize: vars.fontSize.lg,
});

export const navLabel = recipe({
  base: {
    fontFamily: vars.font.mono,
    fontSize: vars.fontSize.sm,
    fontWeight: vars.fontWeight.medium,
    overflow: "hidden",
    transition: "opacity 150ms ease, max-width 200ms ease",
    "@media": {
      "(prefers-reduced-motion: reduce)": {
        transition: "none",
      },
    },
  },
  variants: {
    visible: {
      true: { opacity: 1, maxWidth: "200px" },
      false: { opacity: 0, maxWidth: "0px" },
    },
  },
  defaultVariants: { visible: true },
});

export const badge = style({
  marginLeft: "auto",
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "18px",
  height: "18px",
  padding: "0 5px",
  borderRadius: vars.radii.full,
  background: vars.color.primary,
  color: vars.color.primaryText,
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.bold,
  flexShrink: 0,
});

export const toggleButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  margin: vars.space[2],
  padding: vars.space[2],
  minHeight: "44px", /* WCAG 2.1 AA touch target minimum */
  minWidth: "44px",
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  color: vars.color.textMuted,
  cursor: "pointer",
  transition: "background 120ms ease, color 120ms ease",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
    "&:focus-visible": {
      outline: `2px solid ${vars.color.primary}`,
      outlineOffset: "2px",
    },
  },
});

export const drawerDivider = style({
  height: "1px",
  background: vars.color.borderColor,
  margin: `${vars.space[1]} 0`,
});
