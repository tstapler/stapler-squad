import { style } from "@vanilla-extract/css";
import { vars } from "../../styles/theme-contract.css";

export const container = style({
  height: "100%",
  overflowY: "auto",
  padding: vars.space["4"],
  display: "flex",
  flexDirection: "column",
  gap: vars.space["6"],
});

export const emptyState = style({
  padding: vars.space["6"],
  color: vars.color.textSecondary,
  textAlign: "center",
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  gap: vars.space["2"],
});

export const section = style({});

export const sectionTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textSecondary,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  marginBottom: vars.space["2"],
});

export const list = style({
  listStyle: "none",
  padding: 0,
  margin: 0,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const link = style({
  color: vars.color.primary,
  textDecoration: "none",
  fontSize: vars.fontSize.sm,
  selectors: {
    "&:hover": { textDecoration: "underline" },
  },
});

export const sha = style({
  fontFamily: "monospace",
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  marginRight: vars.space["2"],
});

export const shaFull = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  fontFamily: "monospace",
});

export const urlToggleButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  color: vars.color.primary,
  fontSize: vars.fontSize.sm,
  padding: `${vars.space["1"]} 0`,
  selectors: {
    "&:hover": { textDecoration: "underline" },
  },
});

export const lists = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["6"],
});

export const header = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space["2"],
});

export const statusLine = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const hint = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const actions = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space["2"],
});

// 44px minimum touch target (mobile).
export const actionButton = style({
  minHeight: "44px",
  minWidth: "44px",
  padding: `0 ${vars.space["4"]}`,
  cursor: "pointer",
  color: vars.color.primary,
  background: "none",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  fontSize: vars.fontSize.sm,
  selectors: {
    "&:hover": { textDecoration: "underline" },
  },
});

export const alert = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space["2"],
  padding: vars.space["3"],
  background: vars.color.errorBg,
  color: vars.color.errorText,
  borderRadius: "6px",
  overflowWrap: "anywhere",
});
