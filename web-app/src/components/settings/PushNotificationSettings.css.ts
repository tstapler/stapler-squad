import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.75rem",
});

export const title = style({
  fontSize: vars.fontSize.lg,
  fontWeight: 600,
  color: vars.color.textPrimary,
  margin: "0 0 0.25rem",
});

export const description = style({
  fontSize: vars.fontSize.base,
  color: vars.color.textSecondary,
  margin: 0,
});

export const statusBadge = style({
  display: "inline-block",
  padding: `${vars.space["1"]} 0.625rem`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
});

export const statusBlocked = style({
  background: vars.color.errorBg,
  color: vars.color.errorText,
});

export const statusUnsupported = style({
  background: vars.color.hoverBackground,
  color: vars.color.textMuted,
});

export const instructions = style({
  fontSize: vars.fontSize.base,
  color: vars.color.textSecondary,
  margin: "0.25rem 0 0",
});

export const toggleRow = style({
  display: "flex",
  alignItems: "center",
  gap: "0.75rem",
});

export const toggleLabel = style({
  fontSize: vars.fontSize.base,
  color: vars.color.textPrimary,
  cursor: "pointer",
  userSelect: "none",
});

export const errorText = style({
  fontSize: vars.fontSize.base,
  color: vars.color.error,
  margin: 0,
});
