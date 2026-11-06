import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const banner = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space[2],
  padding: `${vars.space[2]} 16px`,
  fontSize: "0.8125rem",
  color: vars.color.textPrimary,
  background: vars.color.surfaceMuted,
});

export const bannerButton = style({
  minHeight: "44px",
  padding: "0 12px",
  cursor: "pointer",
});

export const asOfLine = style({
  margin: 0,
  padding: `0 16px ${vars.space[1]}`,
  fontSize: "0.75rem",
  color: vars.color.textSecondary,
});
