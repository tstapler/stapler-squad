import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// Covers the window panel without affecting layout; clicks pass through except on the failed item.
export const placeholderRoot = style({
  position: "absolute",
  inset: 0,
  display: "flex",
  gap: 8,
  padding: 8,
  pointerEvents: "none",
});

export const placeholderItem = style({
  flex: 1,
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  gap: 8,
  minHeight: 0,
  background: vars.color.background,
  color: vars.color.textSecondary,
  fontSize: 13,
});
