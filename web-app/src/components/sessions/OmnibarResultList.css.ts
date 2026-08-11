import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const list = style({
  listStyle: "none",
  margin: 0,
  padding: "4px 0",
  width: "100%",
  maxHeight: "320px",
  overflowY: "auto",
});

export const sectionHeader = style({
  padding: "4px 12px",
  fontSize: "11px",
  fontWeight: 600,
  letterSpacing: "0.08em",
  textTransform: "uppercase",
  color: vars.color.textMuted,
  userSelect: "none",
  listStyle: "none",
});

export const separator = style({
  height: "1px",
  background: vars.color.borderColor,
  margin: "4px 0",
  listStyle: "none",
});

export const createNewItem = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  padding: "8px 12px",
  cursor: "pointer",
  listStyle: "none",
  color: vars.color.textMuted,
  fontStyle: "italic",
  fontSize: "13px",
  transition: "background 0.1s ease",
  borderRadius: "6px",
});

export const createNewHighlighted = style({
  background: vars.color.hoverBackground,
});

export const createNewIcon = style({
  fontWeight: 700,
  fontStyle: "normal",
  color: vars.color.textSecondary,
  fontSize: "14px",
});
