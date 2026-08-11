import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const row = style({
  display: "flex",
  alignItems: "center",
  gap: "10px",
  padding: "8px 12px",
  cursor: "pointer",
  borderRadius: "6px",
  listStyle: "none",
  userSelect: "none",
  transition: "background 0.1s",
  background: "transparent",
});

export const rowHighlighted = style({
  background: vars.color.accentBg,
});

export const folderIcon = style({
  flexShrink: 0,
  color: vars.color.textMuted,
  display: "flex",
  alignItems: "center",
});

export const content = style({
  flex: 1,
  minWidth: 0,
  display: "flex",
  flexDirection: "column",
  gap: "2px",
});

export const pathLine = style({
  display: "flex",
  alignItems: "baseline",
  gap: "2px",
  overflow: "hidden",
  whiteSpace: "nowrap",
});

export const parentPath = style({
  fontSize: "13px",
  color: vars.color.textMuted,
  overflow: "hidden",
  textOverflow: "ellipsis",
  flexShrink: 1,
  minWidth: 0,
});

export const separator = style({
  fontSize: "13px",
  color: vars.color.textMuted,
  flexShrink: 0,
});

export const repoName = style({
  fontSize: "13px",
  fontWeight: 600,
  color: vars.color.textPrimary,
  overflow: "hidden",
  textOverflow: "ellipsis",
  flexShrink: 0,
});

export const sessionCount = style({
  fontSize: "11px",
  color: vars.color.textMuted,
});

export const relativeTime = style({
  fontSize: "11px",
  color: vars.color.textMuted,
  flexShrink: 0,
  marginLeft: "auto",
  whiteSpace: "nowrap",
});
