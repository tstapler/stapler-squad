import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  marginTop: vars.space["2"],
  padding: vars.space["3"],
  border: `1px solid ${vars.color.warning}`,
  borderRadius: vars.radii.md,
  background: vars.color.warningBg,
});

export const heading = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.warningText,
});

export const entry = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const entryText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  wordBreak: "break-word",
});

export const entryActions = style({
  display: "flex",
  gap: vars.space["2"],
});

export const actionButton = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  background: "none",
  border: `1px solid ${vars.color.borderMuted}`,
  color: vars.color.textSecondary,
});
