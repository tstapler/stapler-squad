// +feature: shell-tabs
import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const overlay = style({
  position: "fixed",
  inset: 0,
  backgroundColor: vars.color.overlayBackground,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  zIndex: 1000,
});

export const dialog = style({
  backgroundColor: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.lg,
  padding: vars.space[6],
  width: "420px",
  maxWidth: "calc(100vw - 32px)",
  display: "flex",
  flexDirection: "column",
  gap: vars.space[4],
});

export const title = style({
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  margin: 0,
});

export const fieldGroup = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[1],
});

export const label = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
});

export const input = style({
  backgroundColor: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space[2]} ${vars.space[3]}`,
  fontSize: vars.fontSize.base,
  outline: "none",
  width: "100%",
  boxSizing: "border-box",
  selectors: {
    "&::placeholder": {
      color: vars.color.placeholderColor,
    },
    "&:focus": {
      borderColor: vars.color.inputFocusBorder,
    },
  },
});

export const actions = style({
  display: "flex",
  justifyContent: "flex-end",
  gap: vars.space[2],
  marginTop: vars.space[2],
});

export const cancelButton = style({
  backgroundColor: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space[2]} ${vars.space[4]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const submitButton = style({
  backgroundColor: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.md,
  padding: `${vars.space[2]} ${vars.space[4]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.primaryHover,
    },
    "&:disabled": {
      opacity: "0.5",
      cursor: "not-allowed",
    },
  },
});

export const errorText = style({
  color: vars.color.errorText,
  fontSize: vars.fontSize.sm,
});

export const suggestions = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space[1],
  marginTop: vars.space[1],
});

export const suggestionChip = style({
  backgroundColor: vars.color.hoverBackground,
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.full,
  padding: `${vars.space[1]} ${vars.space[3]}`,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.primary,
      color: vars.color.primaryText,
    },
  },
});
