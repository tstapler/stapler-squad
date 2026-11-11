import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const panel = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const metaRow = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space["3"],
  alignItems: "center",
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const links = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space["3"],
  fontSize: vars.fontSize.sm,
});

export const suggestionCard = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: vars.space["2"],
  padding: vars.space["2"],
  border: `1px solid ${vars.color.borderSubtle}`,
  borderRadius: vars.radii.md,
  minHeight: "44px",
});

export const suggestionText = style({
  flex: "1 1 12rem",
  minWidth: 0,
});

export const dismissButton = style({
  minHeight: "44px",
  minWidth: "44px",
  cursor: "pointer",
});
