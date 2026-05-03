import { style, keyframes, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

const fadeIn = keyframes({
  from: { opacity: 0 },
  to: { opacity: 1 },
});

// Story 6.1: Theme-specific opening animation — scale + fade for cyberpunk feel
const scanlineReveal = keyframes({
  from: { opacity: 0, transform: "translateY(-16px) scaleY(0.95)", filter: "brightness(2)" },
  "60%": { filter: "brightness(1.2)" },
  to: { opacity: 1, transform: "translateY(0) scaleY(1)", filter: "brightness(1)" },
});

const spin = keyframes({
  from: { transform: "rotate(0deg)" },
  to: { transform: "rotate(360deg)" },
});

export const overlay = style({
  position: "fixed",
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  background: vars.color.overlayBackground,
  display: "flex",
  alignItems: "flex-start",
  justifyContent: "center",
  paddingTop: "10vh",
  zIndex: 1000,
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      animationName: fadeIn,
      animationDuration: "0.15s",
      animationTimingFunction: "ease-out",
    },
  },
});

export const modal = style({
  background: vars.color.cardBackground,
  borderRadius: 12,
  width: "100%",
  maxWidth: 600,
  // Story 6.1: Theme-aware glow border on omnibar
  boxShadow: `0 25px 50px -12px rgba(0, 0, 0, 0.5), 0 0 0 1px ${vars.color.glowSecondary}`,
  overflow: "hidden",
  position: "relative",
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      animationName: scanlineReveal,
      animationDuration: "0.22s",
      animationTimingFunction: "ease-out",
    },
  },
});

export const inputContainer = style({
  display: "flex",
  alignItems: "center",
  padding: 16,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  gap: 12,
});

export const typeIndicator = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: 32,
  height: 32,
  fontSize: 18,
  flexShrink: 0,
});

export const input = style({
  flex: 1,
  background: "transparent",
  border: "none",
  outline: "none",
  color: vars.color.textPrimary,
  fontSize: 16,
  fontFamily: "inherit",
  selectors: {
    "&::placeholder": {
      color: vars.color.textMuted,
    },
  },
  "@media": {
    "(prefers-color-scheme: light)": {
      color: "#111",
    },
  },
});

export const detectionInfo = style({
  padding: "12px 16px",
  background: vars.color.hoverBackground,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  "@media": {
    "(prefers-color-scheme: light)": {
      background: "#f5f5f5",
      borderBottomColor: "#e5e5e5",
    },
  },
});

export const detectionBadge = style({
  display: "inline-flex",
  alignItems: "center",
  gap: 6,
  padding: "4px 10px",
  background: vars.color.accentBg,
  color: vars.color.primary,
  borderRadius: 4,
  fontSize: 13,
  fontWeight: 500,
});

export const unknown = style({
  background: vars.color.warningBg,
  color: vars.color.warning,
});

export const body = style({
  padding: 16,
  display: "flex",
  flexDirection: "column",
  gap: 16,
});

export const field = style({
  display: "flex",
  flexDirection: "column",
  gap: 6,
});

export const label = style({
  fontSize: 13,
  fontWeight: 500,
  color: vars.color.textSecondary,
});

export const fieldInput = style({
  background: vars.color.hoverBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 6,
  padding: "10px 12px",
  color: vars.color.textPrimary,
  fontSize: 14,
  outline: "none",
  transition: "border-color 0.15s",
  selectors: {
    "&:focus": {
      borderColor: vars.color.primary,
    },
    "&::placeholder": {
      color: vars.color.textMuted,
    },
  },
  "@media": {
    "(prefers-color-scheme: light)": {
      background: "#f5f5f5",
      borderColor: "#e5e5e5",
      color: "#111",
    },
  },
});

export const hint = style({
  fontSize: 12,
  color: vars.color.textMuted,
});

export const select = style({
  background: vars.color.hoverBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 6,
  padding: "10px 12px",
  color: vars.color.textPrimary,
  fontSize: 14,
  outline: "none",
  cursor: "pointer",
  selectors: {
    "&:focus": {
      borderColor: vars.color.primary,
    },
  },
  "@media": {
    "(prefers-color-scheme: light)": {
      background: "#f5f5f5",
      borderColor: "#e5e5e5",
      color: "#111",
    },
  },
});

export const checkbox = style({
  display: "flex",
  alignItems: "center",
  gap: 8,
  cursor: "pointer",
  fontSize: 14,
  color: vars.color.textPrimary,
});

globalStyle(`${checkbox} input`, {
  width: 16,
  height: 16,
  cursor: "pointer",
});

export const collapsible = style({
  borderTop: `1px solid ${vars.color.borderColor}`,
  marginTop: 8,
  paddingTop: 16,
});

export const collapsibleHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  cursor: "pointer",
  userSelect: "none",
  marginBottom: 12,
});

export const collapsibleTitle = style({
  fontSize: 13,
  fontWeight: 500,
  color: vars.color.textSecondary,
});

export const collapsibleIcon = style({
  color: vars.color.textMuted,
  transition: "transform 0.2s",
});

export const expanded = style({
  transform: "rotate(180deg)",
});

export const collapsibleContent = style({
  display: "flex",
  flexDirection: "column",
  gap: 12,
});

export const footer = style({
  display: "flex",
  justifyContent: "flex-end",
  gap: 8,
  padding: 16,
  borderTop: `1px solid ${vars.color.borderColor}`,
  background: vars.color.hoverBackground,
  "@media": {
    "(prefers-color-scheme: light)": {
      background: "#f5f5f5",
      borderTopColor: "#e5e5e5",
    },
  },
});

export const button = style({
  padding: "8px 16px",
  borderRadius: 6,
  fontSize: 14,
  fontWeight: 500,
  cursor: "pointer",
  transition: "all 0.15s",
  border: "none",
});

export const buttonSecondary = style({
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
});

export const buttonPrimary = style({
  background: vars.color.primary,
  color: "white",
  selectors: {
    "&:hover": {
      background: vars.color.accentHover,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const error = style({
  color: vars.color.error,
  fontSize: 13,
  padding: "8px 12px",
  background: vars.color.errorBg,
  borderRadius: 6,
  margin: "0 16px",
});

export const shortcuts = style({
  display: "flex",
  gap: 16,
  padding: "8px 16px",
  background: vars.color.hoverBackground,
  fontSize: 12,
  color: vars.color.textMuted,
  "@media": {
    "(prefers-color-scheme: light)": {
      background: "#f5f5f5",
    },
  },
});

export const shortcut = style({
  display: "flex",
  alignItems: "center",
  gap: 4,
});

export const shortcutKey = style({
  background: vars.color.cardBackground,
  padding: "2px 6px",
  borderRadius: 4,
  fontFamily: "monospace",
  "@media": {
    "(prefers-color-scheme: light)": {
      background: "#e5e5e5",
    },
  },
});

export const completionError = style({
  padding: "4px 16px",
  fontSize: 12,
  color: vars.color.error,
});

export const pathIndicator = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: 20,
  height: 20,
  fontSize: 14,
  flexShrink: 0,
});

export const pathIndicatorValid = style({
  color: "#22c55e",
});

export const pathIndicatorInvalid = style({
  color: "#ef4444",
});

export const pathIndicatorLoading = style({
  color: vars.color.textMuted,
  animation: `${spin} 1s linear infinite`,
  display: "inline-block",
});
