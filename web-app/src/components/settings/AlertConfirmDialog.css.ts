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
  maxWidth: "520px",
  width: "100%",
  maxHeight: "100%",
  overflowY: "auto",
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
  lineHeight: "1.5",
});

// Desktop: right-aligned, Cancel then confirm. Phone: stacked full width with
// Cancel on top and a 16px gap so a thumb mis-tap cannot land on confirm (UX-41).
export const actions = style({
  display: "flex",
  gap: vars.space["3"],
  justifyContent: "flex-end",
  marginTop: vars.space["2"],
  "@media": {
    "(max-width: 600px)": {
      flexDirection: "column",
      alignItems: "stretch",
      gap: vars.space["4"],
    },
  },
});

export const cancelButton = style({
  backgroundColor: "transparent",
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  transition: vars.transition.fast,
  minHeight: "44px",
  ":hover": {
    backgroundColor: vars.color.hoverBackground,
    borderColor: vars.color.borderHover,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

export const confirmButton = style({
  backgroundColor: vars.color.primary,
  color: vars.color.primaryText,
  border: "1px solid transparent",
  borderRadius: vars.radii.md,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  transition: vars.transition.fast,
  minHeight: "44px",
  ":hover": {
    backgroundColor: vars.color.primaryHover,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
  selectors: {
    '&[aria-disabled="true"]': {
      opacity: 0.55,
      cursor: "not-allowed",
    },
  },
});
