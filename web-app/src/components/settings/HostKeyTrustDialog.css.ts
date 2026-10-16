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
  maxWidth: "480px",
  width: "calc(100% - 2rem)",
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
  "@media": {
    "(max-width: 600px)": {
      // Bottom-sheet layout on mobile per ux.md Surface 3's mobile mock.
      position: "fixed",
      left: 0,
      right: 0,
      bottom: 0,
      maxWidth: "100%",
      width: "100%",
      borderBottomLeftRadius: 0,
      borderBottomRightRadius: 0,
    },
  },
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

export const detailList = style({
  display: "grid",
  gridTemplateColumns: "auto 1fr",
  columnGap: vars.space["3"],
  rowGap: vars.space["2"],
  margin: 0,
  padding: vars.space["3"],
  background: vars.color.surfaceSubtle,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
});

export const detailTerm = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textMuted,
  margin: 0,
});

export const detailValue = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  margin: 0,
  wordBreak: "break-all",
});

export const fingerprint = style({
  fontFamily: vars.font.mono,
});

export const actions = style({
  display: "flex",
  gap: vars.space["3"],
  justifyContent: "flex-end",
  marginTop: vars.space["2"],
  "@media": {
    "(max-width: 600px)": {
      flexDirection: "column-reverse",
    },
  },
});

export const primaryButton = style({
  backgroundColor: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
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
