import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const root = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  position: "relative",
});

export const button = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  transition: "background 0.15s ease, border-color 0.15s ease, color 0.15s ease",

  ":hover": {
    background: vars.color.hoverBackground,
    borderColor: vars.color.borderMuted,
    color: vars.color.textPrimary,
  },

  ":disabled": {
    opacity: 0.4,
    cursor: "not-allowed",
    pointerEvents: "none",
  },
});

export const inlineForm = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: vars.space["3"],
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  boxShadow: "0 4px 12px rgba(0, 0, 0, 0.12)",
  minWidth: "260px",
  zIndex: 10,
});

export const formLabel = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textPrimary,
  marginBottom: vars.space["1"],
});

export const textInput = style({
  width: "100%",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  color: vars.color.inputText,
  fontSize: vars.fontSize.sm,
  outline: "none",
  boxSizing: "border-box",

  ":focus": {
    borderColor: vars.color.inputFocusBorder,
    boxShadow: `0 0 0 2px ${vars.color.accentBg}`,
  },
});

export const formActions = style({
  display: "flex",
  gap: vars.space["2"],
  justifyContent: "flex-end",
});

export const submitButton = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  cursor: "pointer",
  transition: "background 0.15s ease",

  ":hover": {
    background: vars.color.primaryHover,
  },

  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
    pointerEvents: "none",
  },
});

export const cancelButton = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  transition: "background 0.15s ease",

  ":hover": {
    background: vars.color.hoverBackground,
  },
});

export const errorText = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.error,
  marginTop: vars.space["1"],
});
