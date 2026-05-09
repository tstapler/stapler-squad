import { style, keyframes, globalStyle } from "@vanilla-extract/css";
import { vars, lightTheme, darkTheme } from "@/styles/theme.css";

const spin = keyframes({
  to: { transform: "rotate(360deg)" },
});

export const container = style({
  display: "flex",
  flexDirection: "column",
  height: "100%",
  overflow: "hidden",
  background: vars.color.terminalBackground,
});

export const emptyState = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  color: vars.color.textMuted,
  gap: 12,
});

export const emptyIcon = style({
  fontSize: 48,
  opacity: 0.5,
});

export const loading = style({
  display: "flex",
  alignItems: "center",
  gap: 8,
  padding: 24,
  color: vars.color.textMuted,
});

export const error = style({
  padding: 24,
  color: vars.color.error,
});

export const spinner = style({
  display: "inline-block",
  width: 14,
  height: 14,
  border: `2px solid ${vars.color.borderColor}`,
  borderTopColor: vars.color.primary,
  borderRadius: "50%",
  animation: `${spin} 0.6s linear infinite`,
});

export const breadcrumb = style({
  display: "flex",
  alignItems: "center",
  flexWrap: "wrap",
  gap: 0,
  padding: "6px 12px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  fontSize: 12,
  fontFamily: vars.font.mono,
  background: vars.color.terminalTabsBg,
  overflow: "hidden",
});

export const breadcrumbSegment = style({
  color: vars.color.textMuted,
});

export const breadcrumbCurrent = style({
  color: vars.color.terminalForeground,
  fontWeight: 500,
});

export const breadcrumbSep = style({
  color: vars.color.textMuted,
  padding: "0 2px",
});

export const truncationWarning = style({
  padding: "6px 12px",
  background: "rgba(204, 167, 0, 0.1)",
  borderBottom: "1px solid rgba(204, 167, 0, 0.3)",
  color: "#cca700",
  fontSize: 12,
});

export const viewer = style({
  flex: 1,
  overflow: "auto",
  height: 0,
});

export const shikiOutput = style({
  minHeight: "100%",
});

globalStyle(`${shikiOutput} pre`, {
  margin: 0,
  padding: 16,
  minHeight: "100%",
  background: "transparent !important",
  fontSize: 13,
  lineHeight: 1.6,
  fontFamily: vars.font.mono,
});

globalStyle(`${shikiOutput} code`, {
  counterReset: "line",
});

globalStyle(`${shikiOutput} .line::before`, {
  counterIncrement: "line",
  content: "counter(line)",
  display: "inline-block",
  width: 40,
  textAlign: "right",
  paddingRight: 16,
  color: vars.color.textMuted,
  userSelect: "none",
});

// Shiki dual-theme activation: light theme
globalStyle(`.${lightTheme} .${shikiOutput} .shiki`, {
  backgroundColor: "var(--shiki-light-bg) !important" as "inherit",
  color: "var(--shiki-light) !important" as "inherit",
});
globalStyle(`.${lightTheme} .${shikiOutput} .shiki span`, {
  color: "var(--shiki-light) !important" as "inherit",
});

// Shiki dual-theme activation: dark theme
globalStyle(`.${darkTheme} .${shikiOutput} .shiki`, {
  backgroundColor: "var(--shiki-dark-bg) !important" as "inherit",
  color: "var(--shiki-dark) !important" as "inherit",
});
globalStyle(`.${darkTheme} .${shikiOutput} .shiki span`, {
  color: "var(--shiki-dark) !important" as "inherit",
});

export const plainPre = style({
  margin: 0,
  padding: 16,
  fontSize: 13,
  lineHeight: 1.6,
  fontFamily: vars.font.mono,
  color: vars.color.terminalForeground,
  whiteSpace: "pre",
  overflow: "auto",
});

export const codeMirrorEditor = style({
  height: "100%",
});

globalStyle(`${codeMirrorEditor} .cm-editor`, {
  height: "100%",
  fontSize: 13,
});

export const binaryPlaceholder = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  gap: 8,
  color: vars.color.textMuted,
});

export const binaryIcon = style({
  fontSize: 48,
  opacity: 0.5,
});

export const binaryTitle = style({
  fontSize: 15,
  color: vars.color.terminalForeground,
  margin: 0,
});

export const binaryMeta = style({
  fontSize: 12,
  margin: 0,
});

export const downloadButton = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space[1],
  padding: `${vars.space[1]} ${vars.space[2]}`,
  fontSize: vars.fontSize.sm,
  borderRadius: vars.radii.sm,
  textDecoration: "none",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  marginLeft: "auto",
  ":hover": {
    color: vars.color.textPrimary,
    backgroundColor: vars.color.hoverBackground,
  },
});

export const imageViewer = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  flex: 1,
  overflow: "auto",
  padding: vars.space[4],
});

export const imagePreview = style({
  maxWidth: "100%",
  maxHeight: "100%",
  objectFit: "contain",
});
