import { keyframes, style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const spinKeyframes = keyframes({
  from: { transform: "rotate(0deg)" },
  to: { transform: "rotate(360deg)" },
});

export const container = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: vars.space["2"],
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const compactContainer = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `2px ${vars.space["2"]}`,
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const spinner = style({
  display: "inline-block",
  width: 14,
  height: 14,
  border: `2px solid ${vars.color.borderMuted}`,
  borderTopColor: vars.color.primary,
  borderRadius: vars.radii.full,
  animation: `${spinKeyframes} 0.8s linear infinite`,
  flexShrink: 0,
});

export const spinnerHidden = style({
  display: "inline-block",
  width: 14,
  height: 14,
  border: `2px solid ${vars.color.borderMuted}`,
  borderTopColor: vars.color.primary,
  borderRadius: vars.radii.full,
  animation: `${spinKeyframes} 0.8s linear infinite`,
  flexShrink: 0,
});

export const label = style({
  flex: 1,
  color: vars.color.textSecondary,
});

export const elapsed = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  marginLeft: "auto",
});

export const cancelButton = style({
  background: "none",
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: vars.radii.sm,
  cursor: "pointer",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  padding: `2px ${vars.space["2"]}`,
  flexShrink: 0,
  minWidth: 44,
  minHeight: 28,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  ":hover": {
    color: vars.color.textPrimary,
    borderColor: vars.color.borderStrong,
  },
});

export const cancelButtonCompact = style({
  background: "none",
  border: "none",
  borderRadius: vars.radii.full,
  cursor: "pointer",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  padding: `2px ${vars.space["2"]}`,
  flexShrink: 0,
  minWidth: 28,
  minHeight: 28,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  ":hover": {
    color: vars.color.textPrimary,
    borderColor: vars.color.borderStrong,
  },
});
