import { style, keyframes, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const spin = keyframes({
  to: { transform: "rotate(360deg)" },
});

export const container = style({
  width: "100%",
  height: "100%",
  overflow: "hidden",
  background: vars.color.terminalBackground,
  fontFamily: vars.font.mono,
  fontSize: 13,
});

export const loading = style({
  display: "flex",
  alignItems: "center",
  gap: 8,
  padding: 16,
  color: vars.color.textMuted,
});

export const error = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "flex-start",
  gap: 8,
  padding: 16,
  color: vars.color.error,
});

export const retryButton = style({
  padding: "4px 10px",
  fontSize: 12,
  background: vars.color.terminalBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 4,
  color: vars.color.terminalForeground,
  cursor: "pointer",
  selectors: {
    "&:hover": {
      background: vars.color.terminalHoverBg,
    },
  },
});

export const empty = style({
  padding: 16,
  color: vars.color.textMuted,
  fontStyle: "italic",
});

export const node = style({
  display: "flex",
  alignItems: "center",
  height: 28,
  cursor: "pointer",
  userSelect: "none",
  borderRadius: 3,
  selectors: {
    "&:hover": {
      background: vars.color.terminalHoverBg,
    },
  },
});

export const selected = style({
  background: "var(--selection-bg, rgba(40, 100, 255, 0.25)) !important",
});

export const keyboardFocused = style({
  outline: `2px solid ${vars.color.primary}`,
  outlineOffset: "-2px",
});

export const nodeInner = style({
  display: "flex",
  alignItems: "center",
  gap: 5,
  width: "100%",
  paddingRight: 8,
  overflow: "hidden",
});

export const icon = style({
  flexShrink: 0,
  width: 16,
  textAlign: "center",
  fontSize: 11,
  color: vars.color.textMuted,
});

export const name = style({
  flex: 1,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  color: vars.color.terminalForeground,
});

export const ignored = style({});

globalStyle(`${ignored} .${name}`, {
  opacity: 0.45,
  fontStyle: "italic",
});

export const symlinkBadge = style({
  flexShrink: 0,
  fontSize: 10,
  color: vars.color.textMuted,
  background: vars.color.terminalBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 3,
  padding: "0 4px",
});

export const statusBadge = style({
  flexShrink: 0,
  fontSize: 11,
  fontWeight: 600,
  marginLeft: "auto",
  padding: "0 3px",
});

export const spinner = style({
  display: "inline-block",
  width: 10,
  height: 10,
  border: `2px solid ${vars.color.borderColor}`,
  borderTopColor: vars.color.primary,
  borderRadius: "50%",
  animation: `${spin} 0.6s linear infinite`,
  flexShrink: 0,
});

export const inlineError = style({
  flexShrink: 0,
  color: vars.color.error,
  fontSize: 12,
});

export const searchContainer = style({
  display: "flex",
  alignItems: "center",
  gap: 6,
  padding: "6px 8px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const searchInput = style({
  flex: 1,
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 4,
  color: vars.color.terminalForeground,
  fontSize: 12,
  padding: "3px 8px",
  outline: "none",
  selectors: {
    "&:focus": {
      borderColor: vars.color.primary,
    },
    "&::placeholder": {
      color: vars.color.textMuted,
    },
  },
});

export const toolbar = style({
  display: "flex",
  alignItems: "center",
  gap: 4,
  padding: "4px 8px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const toolbarButton = style({
  padding: "3px 8px",
  fontSize: 11,
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 3,
  color: vars.color.textMuted,
  cursor: "pointer",
  selectors: {
    "&:hover": {
      background: vars.color.terminalHoverBg,
      color: vars.color.terminalForeground,
    },
  },
});

export const toolbarLabel = style({
  display: "flex",
  alignItems: "center",
  gap: 4,
  fontSize: 11,
  color: vars.color.textMuted,
  cursor: "pointer",
});

globalStyle(`${toolbarLabel} input[type='checkbox']`, { cursor: "pointer" });

export const treeWrapper = style({
  flex: 1,
  overflow: "hidden",
});

export const mark = style({
  background: "rgba(255, 200, 0, 0.25)",
  borderRadius: 2,
});

export const searchEmpty = style({
  padding: 16,
  color: vars.color.textMuted,
  fontStyle: "italic",
});

export const searchTruncated = style({
  padding: "4px 8px",
  fontSize: 11,
  color: vars.color.textMuted,
  background: vars.color.terminalTabsBg,
  borderBottom: `1px solid ${vars.color.borderColor}`,
});
