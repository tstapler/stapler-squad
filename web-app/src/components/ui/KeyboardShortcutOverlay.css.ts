import { style } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

export const backdrop = style({
  position: "fixed",
  inset: 0,
  background: vars.color.overlayBackground,
  zIndex: zIndex.modal,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  padding: vars.space[4],
  "@media": {
    /* Mobile: no padding so dialog fills the screen */
    "(max-width: 768px)": {
      padding: 0,
      alignItems: "stretch",
    },
  },
});

export const dialog = style({
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.lg,
  width: "100%",
  maxWidth: "640px",
  maxHeight: "80vh",
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
  boxShadow: vars.shadow.lg,
  "@media": {
    /* Mobile: full screen, scrollable, no border-radius clipping */
    "(max-width: 768px)": {
      maxWidth: "100%",
      maxHeight: "100%",
      height: "100%",
      borderRadius: 0,
      overflowY: "auto",
    },
  },
});

export const dialogHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: `${vars.space[4]} ${vars.space[6]}`,
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const dialogTitle = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  margin: 0,
});

export const closeButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "44px",  /* WCAG 2.1 AA touch target minimum; was 32px */
  height: "44px",
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  color: vars.color.textMuted,
  cursor: "pointer",
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.base,
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
    "&:focus-visible": {
      outline: `2px solid ${vars.color.primary}`,
      outlineOffset: "2px",
    },
  },
});

export const searchInput = style({
  width: "100%",
  padding: `${vars.space[2]} ${vars.space[4]}`,
  background: vars.color.inputBackground,
  border: "none",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  color: vars.color.inputText,
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.base,
  outline: "none",
  selectors: {
    "&::placeholder": {
      color: vars.color.placeholderColor,
    },
    "&:focus": {
      borderBottomColor: vars.color.inputFocusBorder,
    },
  },
});

export const scrollArea = style({
  overflowY: "auto",
  flex: 1,
  padding: `${vars.space[2]} 0`,
});

export const contextSection = style({
  padding: `${vars.space[2]} ${vars.space[6]}`,
});

export const contextHeading = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.08em",
  margin: `${vars.space[2]} 0 ${vars.space[1]} 0`,
});

export const shortcutRow = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: `${vars.space[1]} ${vars.space[2]}`,
  borderRadius: vars.radii.sm,
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
  },
});

export const shortcutLabel = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const emptyMessage = style({
  padding: vars.space[6],
  textAlign: "center",
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
});
