import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme.css";

export const pageRoot = style({
  maxWidth: "960px",
  margin: "0 auto",
  padding: "2rem 1.5rem",
});

export const pageTitle = style({
  color: vars.color.textPrimary,
  fontSize: "1.5rem",
  fontWeight: 700,
  marginBottom: "1.5rem",
});

export const tabList = style({
  display: "flex",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  gap: 0,
  marginBottom: "2rem",
});

export const tab = recipe({
  base: {
    padding: "0.625rem 1rem",
    fontSize: vars.fontSize.sm,
    fontWeight: vars.fontWeight.medium,
    color: vars.color.textMuted,
    background: "none",
    border: "none",
    borderBottom: "2px solid transparent",
    cursor: "pointer",
    marginBottom: "-1px",
    transition: vars.transition.fast,
    selectors: {
      "&:hover": {
        color: vars.color.textPrimary,
      },
      '&[data-state="active"]': {
        color: vars.color.primary,
        borderBottomColor: vars.color.primary,
      },
    },
  },
  variants: {
    selected: {
      true: {
        color: vars.color.primary,
        borderBottomColor: vars.color.primary,
      },
      false: {},
    },
  },
  defaultVariants: {
    selected: false,
  },
});

export const tabPanel = style({
  padding: "0",
  overflowY: "auto",
});

export const sectionGroup = style({
  display: "flex",
  flexDirection: "column",
  gap: "2rem",
});

export const section = style({
  borderBottom: `1px solid ${vars.color.borderColor}`,
  paddingBottom: "2rem",
  selectors: {
    "&:last-child": {
      borderBottom: "none",
      paddingBottom: 0,
    },
  },
});

export const helpSection = style({
  paddingTop: "1rem",
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
});

export const helpSectionTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textSecondary,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  marginBottom: "0.5rem",
});

export const helpRow = style({
  display: "flex",
  alignItems: "center",
  gap: "1rem",
  flexWrap: "wrap",
});

export const helpButton = style({
  padding: "0.5rem 1rem",
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textPrimary,
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  cursor: "pointer",
  transition: vars.transition.fast,
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderHover,
    },
  },
});

export const helpLink = style({
  padding: "0.5rem 1rem",
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.primary,
  textDecoration: "none",
  selectors: {
    "&:hover": {
      textDecoration: "underline",
    },
  },
});

// Keyboard shortcuts tab styles
export const shortcutsTable = style({
  display: "flex",
  flexDirection: "column",
  gap: "2rem",
});

export const shortcutsContextSection = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.5rem",
});

export const shortcutsContextHeading = style({
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  paddingBottom: "0.5rem",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  marginBottom: "0.25rem",
});

export const shortcutRow = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  padding: "0.375rem 0",
});

export const shortcutLabel = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});
