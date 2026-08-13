import { style, globalStyle } from "@vanilla-extract/css";
import { vars } from "../../styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: "6px",
  padding: "10px 12px",
  background: vars.color.hoverBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  fontSize: "13px",
});

export const row = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
});

export const label = style({
  color: vars.color.textMuted,
  fontSize: "12px",
  minWidth: "52px",
  flexShrink: 0,
});

export const branch = style({
  fontFamily: vars.font.mono,
  color: vars.color.primary,
  fontWeight: "500",
});

export const clean = style({
  color: vars.color.success,
  fontWeight: "500",
});

export const dirty = style({
  color: vars.color.warning,
  fontWeight: "500",
});

export const changes = style({
  display: "flex",
  flexWrap: "wrap",
  gap: "6px",
  paddingLeft: "60px",
});

export const stat = style({
  fontSize: "11px",
  padding: "2px 6px",
  borderRadius: "4px",
  background: vars.color.borderColor,
  color: vars.color.textSecondary,
  fontFamily: vars.font.mono,
});

export const remote = style({
  fontSize: "12px",
  color: vars.color.textSecondary,
  fontFamily: vars.font.mono,
});

export const vcsTypeIcon = style({
  fontSize: "14px",
});
