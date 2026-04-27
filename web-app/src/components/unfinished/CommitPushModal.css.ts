import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const overlay = style({
  position: "fixed",
  inset: 0,
  background: vars.color.overlayBackground,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  zIndex: 1000,
});

export const modal = style({
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.lg,
  padding: vars.space["6"],
  width: "min(480px, 90vw)",
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
});

export const title = style({
  fontSize: vars.fontSize.lg,
  fontWeight: 600,
  color: vars.color.textPrimary,
  margin: 0,
});

export const label = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  marginBottom: vars.space["1"],
  display: "block",
});

export const textarea = style({
  width: "100%",
  minHeight: "80px",
  padding: vars.space["2"],
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  color: vars.color.inputText,
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.sm,
  resize: "vertical",
  boxSizing: "border-box",
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
    outline: "none",
  },
  "::placeholder": {
    color: vars.color.placeholderColor,
  },
});

export const buttonRow = style({
  display: "flex",
  gap: vars.space["2"],
  justifyContent: "flex-end",
});

export const btnCancel = style({
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  color: vars.color.textSecondary,
  ":hover": {
    background: vars.color.hoverBackground,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.inputFocusBorder}`,
    outlineOffset: "1px",
  },
});

export const btnSubmit = style({
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  cursor: "pointer",
  border: "none",
  background: vars.color.primary,
  color: vars.color.textInverse,
  transition: "background 0.12s",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.primaryHover,
    },
  },
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.inputFocusBorder}`,
    outlineOffset: "1px",
  },
});

export const errorMsg = style({
  color: vars.color.errorText,
  fontSize: vars.fontSize.sm,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.errorBg,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.error}`,
});
