import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const emptyText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  padding: `${vars.space["2"]} 0`,
  fontStyle: "italic",
});

export const list = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const row = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.surfaceSubtle,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderSubtle}`,
});

export const rowHeader = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  flexWrap: "wrap",
});

export const statusIcon = style({
  fontSize: vars.fontSize.sm,
  lineHeight: 1,
});

export const statusLabel = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const timestamp = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const statusReady = style({
  color: vars.color.success,
});

export const statusError = style({
  color: vars.color.error,
});

export const statusGenerating = style({
  color: vars.color.warning,
});

export const pill = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `1px ${vars.space["1"]}`,
  background: vars.color.accentBg,
  color: vars.color.textSecondary,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
});

export const activeTask = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  margin: 0,
});

export const previewDetails = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const previewText = style({
  marginTop: vars.space["1"],
  fontFamily: "monospace",
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
});
