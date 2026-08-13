import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const pulse = keyframes({
  "0%, 100%": { opacity: 1 },
  "50%": { opacity: 0.4 },
});

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  overflow: "hidden",
});

export const toolbar = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.surfaceMuted,
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const toolbarTitle = style({
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textSecondary,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  fontFamily: vars.font.mono,
  flex: 1,
});

export const liveIndicator = style({
  width: "6px",
  height: "6px",
  borderRadius: "50%",
  background: vars.color.primary,
  flexShrink: 0,
  animationName: pulse,
  animationDuration: "1.5s",
  animationTimingFunction: "ease-in-out",
  animationIterationCount: "infinite",
});

export const viewToggle = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderMuted}`,
  background: "transparent",
  color: vars.color.textMuted,
  cursor: "pointer",
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
  ":hover": {
    background: vars.color.hoverBackground,
    color: vars.color.textPrimary,
  },
});

export const viewToggleActive = style({
  background: vars.color.accentBg,
  color: vars.color.primary,
  borderColor: vars.color.primary,
});

export const openLink = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderMuted}`,
  background: "transparent",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  textDecoration: "none",
  ":hover": {
    background: vars.color.hoverBackground,
    color: vars.color.textPrimary,
  },
});

export const outputArea = style({
  height: "240px",
  overflowY: "auto",
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.cardBackground,
});

// Terminal view
export const terminalOutput = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textPrimary,
  whiteSpace: "pre-wrap",
  wordBreak: "break-all",
  lineHeight: "1.5",
  margin: 0,
});

// Conversation view
export const messageList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const message = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  maxWidth: "100%",
});

export const messageRole = style({
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  fontFamily: vars.font.mono,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
});

export const messageRoleUser = style({
  color: vars.color.primary,
});

export const messageRoleAssistant = style({
  color: vars.color.textSecondary,
});

export const messageContent = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textPrimary,
  lineHeight: "1.5",
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
  background: vars.color.surfaceMuted,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  fontFamily: vars.font.mono,
});

export const emptyState = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  fontStyle: "italic",
});

export const errorState = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  gap: vars.space["2"],
  height: "100%",
  color: vars.color.error,
  fontSize: vars.fontSize.xs,
  textAlign: "center",
  padding: vars.space["3"],
});

export const errorRetryButton = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  background: "transparent",
  border: `1px solid ${vars.color.error}`,
  borderRadius: vars.radii.sm,
  color: vars.color.error,
  fontSize: vars.fontSize.xs,
  cursor: "pointer",
  selectors: {
    "&:hover": { background: vars.color.hoverBackground },
  },
});

export const inputRow = style({
  display: "flex",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderTop: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  alignItems: "center",
});

export const quickActions = style({
  display: "flex",
  gap: vars.space["1"],
  flexShrink: 0,
});

export const quickActionButton = style({
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderMuted}`,
  background: vars.color.surfaceMuted,
  color: vars.color.textSecondary,
  cursor: "pointer",
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
  flexShrink: 0,
  ":hover": {
    background: vars.color.hoverBackground,
    color: vars.color.textPrimary,
    borderColor: vars.color.borderStrong,
  },
});

export const textInput = style({
  flex: 1,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
  outline: "none",
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
  },
});

export const sendButton = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  border: "none",
  background: vars.color.primary,
  color: vars.color.primaryText,
  cursor: "pointer",
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  flexShrink: 0,
  ":hover": { background: vars.color.primaryHover },
  ":disabled": { opacity: 0.4, cursor: "not-allowed" },
});
