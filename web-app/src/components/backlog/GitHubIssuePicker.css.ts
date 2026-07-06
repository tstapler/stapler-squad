import { style, styleVariants } from "@vanilla-extract/css";
import { vars } from "../../styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[3],
});

export const phaseHeader = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
});

export const backButton = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[1],
  padding: `${vars.space[1]} ${vars.space[2]}`,
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
});

export const repoChip = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space[1],
  padding: `${vars.space[1]} ${vars.space[2]}`,
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: 500,
  color: vars.color.textPrimary,
});

export const searchInput = style({
  width: "100%",
  padding: `${vars.space[2]} ${vars.space[3]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  outline: "none",
  boxSizing: "border-box",
  selectors: {
    "&:focus": {
      borderColor: vars.color.inputFocusBorder,
    },
  },
});

export const listContainer = style({
  display: "flex",
  flexDirection: "column",
  maxHeight: "240px",
  overflowY: "auto",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: vars.color.cardBackground,
});

export const listItem = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  padding: `${vars.space[2]} ${vars.space[3]}`,
  cursor: "pointer",
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  selectors: {
    "&:last-child": {
      borderBottom: "none",
    },
    "&:hover": {
      background: vars.color.hoverBackground,
    },
    "&[aria-selected=true]": {
      background: vars.color.hoverBackground,
    },
  },
});

export const listItemName = style({
  fontWeight: 500,
  flex: "1 1 auto",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const listItemMeta = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  flex: "0 0 auto",
  maxWidth: "50%",
});

export const issueNumber = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  fontVariantNumeric: "tabular-nums",
  flex: "0 0 auto",
});

export const labelBadge = style({
  display: "inline-block",
  padding: `1px ${vars.space[1]}`,
  borderRadius: vars.radii.sm,
  fontSize: "10px",
  fontWeight: 500,
  background: vars.color.hoverBackground,
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  maxWidth: "80px",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const filterBar = style({
  display: "flex",
  gap: vars.space[2],
  alignItems: "center",
});

export const stateToggle = style({
  display: "flex",
  gap: vars.space[1],
  padding: vars.space[1],
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
});

export const stateButton = styleVariants({
  inactive: {
    padding: `${vars.space[1]} ${vars.space[2]}`,
    background: "transparent",
    color: vars.color.textSecondary,
    border: "none",
    borderRadius: vars.radii.sm,
    fontSize: vars.fontSize.xs,
    cursor: "pointer",
  },
  active: {
    padding: `${vars.space[1]} ${vars.space[2]}`,
    background: vars.color.primary,
    color: vars.color.primaryText,
    border: "none",
    borderRadius: vars.radii.sm,
    fontSize: vars.fontSize.xs,
    cursor: "pointer",
    fontWeight: 600,
  },
});

export const emptyState = style({
  padding: vars.space[4],
  textAlign: "center",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});

export const authErrorBox = style({
  padding: vars.space[3],
  background: "var(--error-bg)",
  color: "var(--error-text)",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  border: "1px solid var(--error)",
});

export const localBadge = style({
  fontSize: "10px",
  padding: `1px ${vars.space[1]}`,
  background: vars.color.hoverBackground,
  color: vars.color.textMuted,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
  flex: "0 0 auto",
});

export const loadingText = style({
  padding: vars.space[3],
  textAlign: "center",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  fontStyle: "italic",
});

export const listItemSelected = style({
  background: `${vars.color.hoverBackground} !important`,
});

export const historyDivider = style({
  height: "1px",
  background: vars.color.borderColor,
  margin: `${vars.space[1]} 0`,
  flexShrink: 0,
});

export const historyIcon = style({
  fontSize: "11px",
  flex: "0 0 auto",
  opacity: 0.5,
  lineHeight: 1,
});

export const matchHighlight = style({
  fontWeight: 700,
  background: "transparent",
  color: vars.color.primary,
  fontStyle: "normal",
});

export const prTypeBadge = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  fontSize: "9px",
  fontWeight: 700,
  letterSpacing: "0.02em",
  padding: `1px ${vars.space[1]}`,
  borderRadius: vars.radii.sm,
  background: "var(--primary)",
  color: "var(--primary-text)",
  flex: "0 0 auto",
  minWidth: "20px",
  textTransform: "uppercase",
});

export const issueTypeBadge = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  fontSize: "11px",
  fontWeight: 700,
  padding: `1px ${vars.space[1]}`,
  borderRadius: vars.radii.sm,
  background: vars.color.hoverBackground,
  color: vars.color.textMuted,
  border: `1px solid ${vars.color.borderColor}`,
  flex: "0 0 auto",
  minWidth: "20px",
});

export const relativeDate = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  flex: "0 0 auto",
  marginLeft: "auto",
  fontVariantNumeric: "tabular-nums",
});
