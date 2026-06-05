import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const overlay = style({
  position: "fixed",
  inset: 0,
  background: vars.color.overlayBackground,
  zIndex: 50,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  padding: vars.space["4"],
});

export const dialog = style({
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.lg,
  padding: vars.space["6"],
  width: "100%",
  maxWidth: "760px",
  maxHeight: "80vh",
  overflowY: "auto",
  boxShadow: vars.shadow.lg,
});

export const dialogTitle = style({
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  marginBottom: vars.space["1"],
});

export const dialogSubtitle = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  marginBottom: vars.space["4"],
});

export const grid = style({
  display: "grid",
  gridTemplateColumns: "repeat(3, 1fr)",
  gap: vars.space["3"],
  "@media": {
    "(max-width: 800px)": { gridTemplateColumns: "repeat(2, 1fr)" },
    "(max-width: 480px)": { gridTemplateColumns: "1fr" },
  },
});

export const card = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  padding: vars.space["3"],
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: vars.color.cardBackground,
  cursor: "pointer",
  textAlign: "left",
  transition: "border-color 0.15s, box-shadow 0.15s",
  ":hover": {
    borderColor: vars.color.primary,
    boxShadow: vars.shadow.sm,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

export const cardIcon = style({
  fontSize: "1.5rem",
  lineHeight: 1,
});

export const cardTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const cardDesc = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  lineHeight: 1.4,
});

export const cardDecision = style({
  display: "inline-block",
  marginTop: "auto",
  paddingTop: vars.space["1"],
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
});

export const footer = style({
  marginTop: vars.space["4"],
  textAlign: "center",
});

export const scratchLink = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  background: "none",
  border: "none",
  cursor: "pointer",
  textDecoration: "underline",
  ":hover": { color: vars.color.textPrimary },
});
