import { style, keyframes } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme-contract.css";

const fadeIn = keyframes({
  from: { opacity: 0 },
  to: { opacity: 1 },
});

const scaleIn = keyframes({
  from: { opacity: 0, transform: "scale(0.96) translateY(8px)" },
  to: { opacity: 1, transform: "scale(1) translateY(0)" },
});

export const backdrop = style({
  position: "fixed",
  inset: 0,
  background: "rgba(0,0,0,0.6)",
  zIndex: zIndex.modal - 1,
  animation: `${fadeIn} 120ms ease`,
});

export const modal = style({
  position: "fixed",
  top: "3vh",
  left: "3vw",
  right: "3vw",
  bottom: "3vh",
  zIndex: zIndex.modal,
  display: "flex",
  flexDirection: "column",
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.lg,
  boxShadow: vars.shadow.lg,
  overflow: "hidden",
  animation: `${scaleIn} 140ms ease`,
});

export const modalHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: `${vars.space[2]} ${vars.space[3]}`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  flexShrink: 0,
  gap: vars.space[2],
});

export const modalTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  flex: 1,
});

export const peekBadge = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  background: vars.color.surfaceSubtle,
  border: `1px solid ${vars.color.borderSubtle}`,
  borderRadius: vars.radii.sm,
  padding: `1px ${vars.space[2]}`,
  flexShrink: 0,
});

export const closeButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "28px",
  height: "28px",
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderSubtle}`,
  background: "transparent",
  color: vars.color.textMuted,
  cursor: "pointer",
  fontSize: vars.fontSize.base,
  flexShrink: 0,
  ":hover": {
    background: vars.color.hoverBackground,
    color: vars.color.textPrimary,
  },
});

export const modalBody = style({
  flex: 1,
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
});
