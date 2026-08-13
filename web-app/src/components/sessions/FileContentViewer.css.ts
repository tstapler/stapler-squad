import { style, keyframes, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

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

export const emptyHint = style({
  fontSize: vars.fontSize.sm,
  opacity: 0.6,
  margin: 0,
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
  background: "none",
  border: "none",
  cursor: "pointer",
  padding: 0,
  font: "inherit",
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
  background: vars.color.warningBg,
  borderBottom: `1px solid ${vars.color.warning}`,
  color: vars.color.warning,
  fontSize: 12,
});

export const viewer = style({
  flex: 1,
  overflow: "auto",
  height: 0,
});

export const wrapToggleButton = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `2px ${vars.space[2]}`,
  fontSize: vars.fontSize.sm,
  borderRadius: vars.radii.sm,
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  cursor: "pointer",
  ":hover": {
    color: vars.color.textPrimary,
    backgroundColor: vars.color.hoverBackground,
  },
});

export const wrapToggleButtonActive = style({
  color: vars.color.primary,
  borderColor: vars.color.primary,
});

export const codeMirrorEditor = style({
  height: "100%",
});

globalStyle(`${codeMirrorEditor} .cm-editor`, {
  height: "100%",
  fontSize: 13,
});

globalStyle(`${codeMirrorEditor} .cm-changeGutter`, {
  width: 4,
});

const gutterMarker = style({
  width: 4,
  height: "100%",
});

export const gutterMarkerAdd = style([gutterMarker, { background: vars.color.gitAdded }]);
export const gutterMarkerDelete = style([gutterMarker, { background: vars.color.gitDeleted }]);
export const gutterMarkerModify = style([gutterMarker, { background: vars.color.gitModified }]);

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

export const pdfViewer = style({
  flex: 1,
  height: 0,
  overflow: "hidden",
});

export const pdfEmbed = style({
  width: "100%",
  height: "100%",
  border: "none",
  display: "block",
});

export const videoViewer = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  flex: 1,
  overflow: "auto",
  padding: vars.space[4],
  gap: vars.space[3],
});

export const videoPlayer = style({
  maxWidth: "100%",
  maxHeight: "calc(100% - 48px)",
});

export const videoMeta = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  textAlign: "center",
  margin: 0,
});

const shimmerMove = keyframes({
  "0%": { backgroundPosition: "200% 0" },
  "100%": { backgroundPosition: "-200% 0" },
});

export const shimmer = style({
  height: "100%",
  background: "linear-gradient(90deg, transparent 0%, rgba(255,255,255,0.04) 50%, transparent 100%)",
  backgroundSize: "200% 100%",
  animation: `${shimmerMove} 1.5s linear infinite`,
});
