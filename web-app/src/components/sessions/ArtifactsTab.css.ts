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
