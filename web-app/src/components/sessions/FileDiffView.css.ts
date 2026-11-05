import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const wrapper = style({
  display: "flex",
  flexDirection: "column",
  height: "100%",
  minHeight: 0,
  gap: "8px",
});

export const bar = style({
  display: "flex",
  alignItems: "center",
  gap: "12px",
  flexShrink: 0,
});

export const backButton = style({
  padding: "4px 10px",
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  color: vars.color.textPrimary,
  cursor: "pointer",
  selectors: { "&:hover": { background: vars.color.hoverBackground } },
});

export const path = style({
  flex: 1,
  minWidth: 0,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  fontFamily: "inherit",
});

export const diff = style({
  flex: 1,
  minHeight: 0,
});
