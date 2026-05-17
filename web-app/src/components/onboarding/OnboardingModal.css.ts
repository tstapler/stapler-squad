import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import { zIndex } from "@/styles/theme-contract.css";

const fadeIn = keyframes({
  from: { opacity: 0 },
  to: { opacity: 1 },
});

const scaleIn = keyframes({
  from: { opacity: 0, transform: "scale(0.97)" },
  to: { opacity: 1, transform: "scale(1)" },
});

export const overlay = style({
  position: "fixed",
  inset: 0,
  background: vars.color.overlayBackground,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  zIndex: zIndex.modal,
  animation: `${fadeIn} ${vars.transition.base} forwards`,
});

export const content = style({
  position: "fixed",
  top: "50%",
  left: "50%",
  transform: "translate(-50%, -50%)",
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.lg,
  padding: vars.space["6"],
  maxWidth: "520px",
  width: "90vw",
  maxHeight: "85vh",
  overflowY: "auto",
  zIndex: zIndex.modal,
  animation: `${scaleIn} ${vars.transition.base} forwards`,
});

export const header = style({
  display: "flex",
  alignItems: "flex-start",
  justifyContent: "space-between",
  marginBottom: vars.space["4"],
});

export const stepIndicatorRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["1"],
  marginBottom: vars.space["4"],
});

export const dot = style({
  width: "6px",
  height: "6px",
  borderRadius: vars.radii.full,
  background: vars.color.borderColor,
  transition: vars.transition.fast,
});

export const dotActive = style({
  background: vars.color.primary,
});

export const dotCompleted = style({
  background: vars.color.primary,
  opacity: 0.5,
});

export const skipButton = style({
  position: "absolute",
  top: vars.space["4"],
  right: vars.space["4"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  background: "none",
  border: "none",
  cursor: "pointer",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  transition: vars.transition.fast,
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
      background: vars.color.hoverBackground,
    },
  },
});

export const headline = style({
  fontSize: vars.fontSize.xl,
  fontWeight: vars.fontWeight.bold,
  color: vars.color.textPrimary,
  marginBottom: vars.space["2"],
  paddingRight: vars.space["8"],
});

export const body = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  lineHeight: "1.6",
  marginBottom: vars.space["4"],
});

export const asciiDiagram = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  padding: vars.space["3"],
  background: vars.color.cardBackground,
  borderRadius: vars.radii.sm,
  whiteSpace: "pre",
  marginBottom: vars.space["4"],
  lineHeight: "1.5",
  border: `1px solid ${vars.color.borderColor}`,
});

export const shortcutTable = style({
  width: "100%",
  borderCollapse: "collapse" as const,
  marginBottom: vars.space["4"],
});

export const shortcutRow = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  padding: `${vars.space["1"]} 0`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  selectors: {
    "&:last-child": {
      borderBottom: "none",
    },
  },
});

export const shortcutLabel = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const kbd = style({
  display: "inline-block",
  padding: `1px ${vars.space["1"]}`,
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
  color: vars.color.textPrimary,
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  whiteSpace: "nowrap",
});

export const checkboxRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  marginBottom: vars.space["4"],
  cursor: "pointer",
});

export const checkboxLabel = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  cursor: "pointer",
});

export const footer = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space["2"],
  marginTop: vars.space["4"],
});

export const footerRight = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const primaryButton = style({
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.primaryText,
  background: vars.color.primary,
  border: "none",
  borderRadius: vars.radii.md,
  cursor: "pointer",
  transition: vars.transition.fast,
  selectors: {
    "&:hover": {
      background: vars.color.primaryHover,
    },
    "&:active": {
      background: vars.color.primaryActive,
    },
  },
});

export const secondaryButton = style({
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textPrimary,
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  cursor: "pointer",
  transition: vars.transition.fast,
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderHover,
    },
  },
});

export const linkButton = style({
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.primary,
  background: "none",
  border: "none",
  cursor: "pointer",
  textDecoration: "underline",
  transition: vars.transition.fast,
  selectors: {
    "&:hover": {
      color: vars.color.primaryHover,
    },
  },
});
