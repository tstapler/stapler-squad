import { style, keyframes, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const ring = keyframes({
  "0%, 100%": { transform: "rotate(0deg)" },
  "10%, 30%, 50%, 70%, 90%": { transform: "rotate(-10deg)" },
  "20%, 40%, 60%, 80%": { transform: "rotate(10deg)" },
});

export const toast = style({
  position: "fixed",
  bottom: "24px",
  right: "24px",
  width: "380px",
  maxHeight: "calc(var(--viewport-height, 100dvh) - 48px)",
  background: vars.color.modalBackground,
  border: `2px solid var(--priority-color, ${vars.color.primary})`,
  borderRadius: "12px",
  boxShadow: "0 8px 32px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.1)",
  zIndex: 10000,
  overflow: "hidden",
  transform: "translateX(450px)",
  opacity: 0,
  transition: "all 0.3s cubic-bezier(0.4, 0, 0.2, 1)",
  "@media": {
    "(prefers-color-scheme: dark)": {
      boxShadow: "0 8px 32px rgba(0, 0, 0, 0.6), 0 0 0 1px rgba(255, 255, 255, 0.05)",
    },
    "screen and (max-width: 768px)": {
      left: "16px",
      right: "16px",
      width: "auto",
      bottom: "16px",
    },
  },
  selectors: {
    "&:nth-child(2)": {
      bottom: "calc(24px + 120px)",
      opacity: 0.9,
      transform: "scale(0.95) translateX(450px)",
    },
    "&:nth-child(3)": {
      bottom: "calc(24px + 240px)",
      opacity: 0.8,
      transform: "scale(0.9) translateX(450px)",
    },
    "&:nth-child(n + 4)": {
      display: "none",
    },
  },
});

export const visible = style({
  transform: "translateX(0)",
  opacity: 1,
  selectors: {
    [`${toast}&:nth-child(2)`]: {
      transform: "scale(0.95) translateX(0)",
    },
    [`${toast}&:nth-child(3)`]: {
      transform: "scale(0.9) translateX(0)",
    },
  },
});

export const exiting = style({
  transform: "translateX(450px)",
  opacity: 0,
});

export const minimized = style({
  width: "260px",
  maxHeight: "48px",
  overflow: "hidden",
  cursor: "pointer",
  borderRadius: "24px",
  bottom: "16px",
});

export const toastApproval = style({
  width: "480px",
  "@media": {
    "screen and (max-width: 768px)": {
      left: "16px",
      right: "16px",
      width: "auto",
      bottom: "16px",
    },
  },
});

export const header = style({
  display: "flex",
  alignItems: "center",
  gap: "12px",
  padding: "16px 16px 12px 16px",
  background: `linear-gradient(to bottom, var(--priority-color, ${vars.color.primary}), transparent)`,
  backgroundSize: "100% 4px",
  backgroundRepeat: "no-repeat",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  selectors: {
    [`${minimized} &`]: {
      padding: "10px 12px",
      borderBottom: "none",
      background: "none",
    },
  },
});

export const icon = style({
  fontSize: "24px",
  lineHeight: 1,
  flexShrink: 0,
  animation: `${ring} 0.5s ease-in-out`,
  selectors: {
    [`${minimized} &`]: {
      fontSize: "16px",
      animation: "none",
    },
  },
});

export const titleWrapper = style({
  flex: 1,
  display: "flex",
  flexDirection: "column",
  gap: "4px",
  minWidth: 0,
});

export const titleRow = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
});

globalStyle(`${titleRow} strong`, {
  fontSize: "15px",
  fontWeight: 600,
  color: vars.color.textPrimary,
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
  flex: 1,
  minWidth: 0,
});

globalStyle(`${minimized} ${titleRow} strong`, { fontSize: "13px" });

export const typeLabel = style({
  fontSize: "10px",
  fontWeight: 600,
  textTransform: "uppercase",
  letterSpacing: "0.5px",
  padding: "2px 6px",
  borderRadius: "4px",
  background: `var(--priority-color, ${vars.color.primary})`,
  color: "white",
  whiteSpace: "nowrap",
  flexShrink: 0,
  selectors: {
    [`${minimized} &`]: {
      display: "none",
    },
  },
});

export const subtitleRow = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  fontSize: "12px",
  color: vars.color.textMuted,
  selectors: {
    [`${minimized} &`]: {
      display: "none",
    },
  },
});

export const sourceApp = style({
  fontWeight: 500,
  color: vars.color.textSecondary,
});

export const timestamp = style({
  fontSize: "12px",
  color: vars.color.textMuted,
});

export const closeButton = style({
  background: "none",
  border: "none",
  fontSize: "28px",
  lineHeight: 1,
  color: vars.color.textMuted,
  cursor: "pointer",
  padding: 0,
  width: "28px",
  height: "28px",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  borderRadius: "4px",
  transition: "all 0.15s ease",
  flexShrink: 0,
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
    [`${minimized} &`]: {
      fontSize: "18px",
      width: "20px",
      height: "20px",
    },
  },
});

export const body = style({
  padding: "12px 16px",
  overflowY: "auto",
  maxHeight: "300px",
  selectors: {
    [`${minimized} &`]: {
      display: "none",
    },
  },
});

export const message = style({
  margin: 0,
  fontSize: "14px",
  lineHeight: 1.5,
  color: vars.color.textPrimary,
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
});

export const workingDir = style({
  margin: "8px 0 0 0",
  fontSize: "12px",
  color: vars.color.textMuted,
  display: "flex",
  alignItems: "center",
  gap: "4px",
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
});

export const actions = style({
  display: "flex",
  gap: "8px",
  padding: "12px 16px 16px 16px",
  borderTop: `1px solid ${vars.color.borderColor}`,
  selectors: {
    [`${minimized} &`]: {
      display: "none",
    },
  },
});

const baseActionButton = style({
  flex: 1,
  padding: "10px 16px",
  border: "none",
  borderRadius: "6px",
  fontSize: "14px",
  fontWeight: 500,
  cursor: "pointer",
  transition: "all 0.15s ease",
});

export const viewButton = style([baseActionButton, {
  background: `var(--priority-color, ${vars.color.primary})`,
  color: "white",
  selectors: {
    "&:hover": {
      filter: "brightness(1.1)",
      transform: "translateY(-1px)",
      boxShadow: "0 4px 8px rgba(0, 0, 0, 0.2)",
    },
  },
}]);

export const dismissButton = style([baseActionButton, {
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderColor}`,
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.textMuted,
    },
  },
}]);

export const focusButton = style([baseActionButton, {
  background: "transparent",
  color: vars.color.primary,
  border: `1px solid ${vars.color.primary}`,
  flex: "0 0 auto",
  selectors: {
    "&:hover": {
      background: vars.color.primary,
      color: "white",
    },
  },
}]);

export const approveButton = style([baseActionButton, {
  background: "#22c55e",
  color: "white",
  selectors: {
    "&:hover": {
      background: "#16a34a",
      transform: "translateY(-1px)",
      boxShadow: "0 4px 8px rgba(0, 0, 0, 0.2)",
    },
  },
}]);

export const denyButton = style([baseActionButton, {
  background: "#ef4444",
  color: "white",
  selectors: {
    "&:hover": {
      background: "#dc2626",
      transform: "translateY(-1px)",
      boxShadow: "0 4px 8px rgba(0, 0, 0, 0.2)",
    },
  },
}]);

export const minimizeHint = style({
  display: "none",
});
