import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// Sticky inside the detail column: not dismissible, never scrolled away (RO-5).
export const banner = style({
  flex: "0 0 auto",
  position: "sticky",
  top: 0,
  zIndex: 1,
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  padding: `${vars.space[2]} ${vars.space[3]}`,
  background: vars.color.surfaceMuted,
  color: vars.color.textPrimary,
  borderBottom: `1px solid ${vars.color.borderMuted}`,
});

export const icon = style({
  fontSize: vars.fontSize.base,
  flex: "0 0 auto",
});

export const text = style({
  display: "flex",
  flexDirection: "column",
  minWidth: 0,
});

export const primary = style({
  fontWeight: vars.fontWeight.semibold,
  fontSize: vars.fontSize.sm,
});

export const secondary = style({
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.sm,
});
