import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const button = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  border: `1px solid ${vars.color.primary}`,
  background: vars.color.primary,
  color: vars.color.primaryText,
  ":hover": {
    background: vars.color.primaryHover,
    borderColor: vars.color.primaryHover,
  },
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
});

export const errorContainer = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const errorText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.errorText,
});

export const errorDetails = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const errorRawText = style({
  marginTop: vars.space["1"],
  fontFamily: "monospace",
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
});
