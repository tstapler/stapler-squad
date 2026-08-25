import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import { zIndex } from "@/styles/theme-contract.css";

// ── Animated modal chrome ───────────────────────────────────────────────────
// Fade/slide-up overlay + panel used by the session dialogs (ConfirmKillDialog,
// ResumeSessionModal). Hardcodes pixel values to match those components'
// existing (non-token-based) sizing.

const fadeIn = keyframes({
  from: { opacity: 0 },
  to: { opacity: 1 },
});

const slideUp = keyframes({
  from: { transform: "translateY(20px)", opacity: 0 },
  to: { transform: "translateY(0)", opacity: 1 },
});

export const animatedOverlay = style({
  position: "fixed",
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  background: vars.color.overlayBackground,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  zIndex: zIndex.modal,
  animation: `${fadeIn} 0.2s ease`,
});

export const animatedModal = style({
  background: vars.color.modalBackground,
  borderRadius: "12px",
  padding: 0,
  maxWidth: "520px",
  width: "90%",
  maxHeight: "80vh",
  display: "flex",
  flexDirection: "column",
  boxShadow: "0 20px 60px rgba(0, 0, 0, 0.3)",
  animation: `${slideUp} 0.3s ease`,
});

export const animatedHeader = style({
  padding: "24px 24px 16px 24px",
  borderBottom: `1px solid ${vars.color.modalBorder}`,
});

export const animatedTitle = style({
  margin: "0 0 4px 0",
  fontSize: "20px",
  fontWeight: 600,
  color: vars.color.textPrimary,
});

/** Base subtitle style — compose with `style([animatedSubtitleBase, {...}])` for per-site tweaks. */
export const animatedSubtitleBase = style({
  margin: 0,
  fontSize: "14px",
  color: vars.color.textSecondary,
});

// ── Static confirm-dialog action row ────────────────────────────────────────
// Token-based (non-animated) footer buttons shared by the backlog/settings
// confirm dialogs (VaguenessPromptModal, BackwardSyncConfirmDialog).

export const confirmActions = style({
  display: "flex",
  gap: vars.space["3"],
  justifyContent: "flex-end",
  marginTop: vars.space["2"],
});

export const confirmPrimaryButton = style({
  backgroundColor: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.md,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  transition: vars.transition.fast,
  ":hover": {
    backgroundColor: vars.color.primaryHover,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

export const confirmSecondaryButton = style({
  backgroundColor: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  transition: vars.transition.fast,
  ":hover": {
    backgroundColor: vars.color.hoverBackground,
    borderColor: vars.color.borderHover,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});
