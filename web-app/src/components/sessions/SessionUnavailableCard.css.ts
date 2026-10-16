import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const card = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "flex-start",
  gap: vars.space[2],
  maxWidth: "32rem",
  margin: `${vars.space[6]} auto`,
  padding: vars.space[4],
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: vars.radii.md,
});

export const heading = style({
  margin: 0,
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.semibold,
});

export const detail = style({
  margin: 0,
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.base,
});

export const quote = style({
  margin: 0,
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.base,
  overflowWrap: "anywhere",
});

export const actions = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space[2],
  marginTop: vars.space[2],
});

// >= 44px touch target (UX RO-3).
export const button = style({
  minHeight: "44px",
  minWidth: "44px",
  padding: `${vars.space[2]} ${vars.space[4]}`,
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
});

export const secondaryButton = style([
  button,
  {
    background: vars.color.surfaceMuted,
    color: vars.color.textPrimary,
    border: `1px solid ${vars.color.borderMuted}`,
  },
]);
