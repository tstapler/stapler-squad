import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const fadeIn = keyframes({
  from: { opacity: 0 },
  to: { opacity: 1 },
});

const slideUp = keyframes({
  from: { opacity: 0, transform: "translate(-50%, calc(-50% + 8px))" },
  to: { opacity: 1, transform: "translate(-50%, -50%)" },
});

export const overlay = style({
  position: "fixed",
  inset: 0,
  background: vars.color.overlayBackground,
  animation: `${fadeIn} 0.15s ease`,
  zIndex: 1100,
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
  width: "90vw",
  maxWidth: "560px",
  maxHeight: "85vh",
  overflowY: "auto",
  animation: `${slideUp} 0.15s ease`,
  zIndex: 1101,
  // Safe area for mobile
  paddingBottom: `max(${vars.space["6"]}, env(safe-area-inset-bottom))`,
});

export const title = style({
  fontSize: vars.fontSize.lg,
  fontWeight: "600",
  color: vars.color.textPrimary,
  marginBottom: vars.space["2"],
});

export const description = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  marginBottom: vars.space["4"],
});

export const footer = style({
  display: "flex",
  justifyContent: "flex-end",
  gap: vars.space["2"],
  marginTop: vars.space["6"],
  paddingTop: vars.space["4"],
  borderTop: `1px solid ${vars.color.borderColor}`,
});

export const closeButton = style({
  position: "absolute",
  top: vars.space["4"],
  right: vars.space["4"],
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "32px",
  height: "32px",
  borderRadius: vars.radii.md,
  border: "none",
  background: "transparent",
  color: vars.color.textMuted,
  cursor: "pointer",
  fontSize: "18px",
  selectors: {
    "&:hover": { background: vars.color.hoverBackground, color: vars.color.textPrimary },
    "&:focus-visible": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "2px" },
  },
});
