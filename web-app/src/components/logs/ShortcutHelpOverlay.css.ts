import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import { zIndex } from "@/styles/theme-contract.css";

export const overlay = style({
  position: "fixed",
  inset: 0,
  zIndex: zIndex.modal,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  backgroundColor: vars.color.overlayBackground,
  backdropFilter: "blur(2px)",
});

export const panel = style({
  backgroundColor: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.lg,
  padding: vars.space["6"],
  minWidth: 280,
  maxWidth: 420,
  width: "90vw",
  boxShadow: vars.shadow.lg,
  position: "relative",
});

export const title = style({
  margin: 0,
  marginBottom: vars.space["4"],
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const table = style({
  width: "100%",
  borderCollapse: "collapse",
});

export const row = style({
  selectors: {
    "&:not(:last-child)": {
      borderBottom: `1px solid ${vars.color.borderSubtle}`,
    },
  },
});

export const keyCell = style({
  padding: `${vars.space["1"]} ${vars.space["3"]} ${vars.space["1"]} 0`,
  verticalAlign: "middle",
  width: "35%",
});

export const descCell = style({
  padding: `${vars.space["1"]} 0`,
  verticalAlign: "middle",
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.sm,
});

export const kbd = style({
  display: "inline-block",
  padding: "1px 6px",
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: vars.radii.sm,
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textPrimary,
  whiteSpace: "nowrap",
});

export const closeButton = style({
  position: "absolute",
  top: vars.space["3"],
  right: vars.space["3"],
  background: "transparent",
  border: "none",
  cursor: "pointer",
  fontSize: vars.fontSize.xl,
  color: vars.color.textMuted,
  lineHeight: 1,
  padding: vars.space["1"],
  borderRadius: vars.radii.sm,
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
});
