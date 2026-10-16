import { style } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

export const overlay = style({
  position: "fixed",
  inset: 0,
  backgroundColor: vars.color.overlayBackground,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  zIndex: zIndex.modal,
  padding: vars.space["4"],
});

export const dialog = style({
  backgroundColor: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.lg,
  boxShadow: vars.shadow.lg,
  padding: vars.space["6"],
  maxWidth: "420px",
  width: "calc(100% - 2rem)",
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
});

export const heading = style({
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  margin: 0,
});

export const body = style({
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.base,
  margin: 0,
  lineHeight: "1.5",
});

export const actions = style({
  display: "flex",
  gap: vars.space["3"],
  justifyContent: "flex-end",
  marginTop: vars.space["2"],
});

export const secondaryButton = style({
  backgroundColor: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  ":hover": {
    backgroundColor: vars.color.hoverBackground,
    borderColor: vars.color.borderHover,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

export const dangerButton = style({
  backgroundColor: vars.color.error,
  color: vars.color.textInverse,
  border: "none",
  borderRadius: vars.radii.md,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  ":hover": {
    backgroundColor: vars.color.errorDark,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.error}`,
    outlineOffset: "2px",
  },
});
