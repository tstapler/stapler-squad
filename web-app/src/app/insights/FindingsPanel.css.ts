// +feature: insights-findings-panel
import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const panel = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[3],
});

export const list = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[2],
  listStyle: "none",
  margin: 0,
  padding: 0,
});

export const card = style({
  display: "flex",
  alignItems: "flex-start",
  justifyContent: "space-between",
  gap: vars.space[3],
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  padding: `${vars.space[3]} ${vars.space[4]}`,
});

export const cardBody = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[1],
  minWidth: 0,
});

export const cardHeader = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
});

export const cardMessage = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
});

export const cardSessionLabel = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
});

export const cardActions = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "flex-end",
  gap: vars.space[1],
  flexShrink: 0,
});

export const cardImpact = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textSecondary,
});

export const cardAction = style({
  flexShrink: 0,
  alignSelf: "center",
  background: "none",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space[1]} ${vars.space[3]}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  cursor: "pointer",
  whiteSpace: "nowrap",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
    "&:focus-visible": {
      outline: `2px solid ${vars.color.primary}`,
      outlineOffset: "2px",
    },
  },
});

// Reuses cardAction's bordered-button visual language, same rationale as
// retryButton below.
export const dismissButton = style([cardAction, { fontSize: vars.fontSize.xs }]);

export const errorBoxContent = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space[3],
});

// Reuses cardAction's bordered-button visual language (see design/ux.md's
// "same visual token, errorBox" requirement) for the error state's [Retry]
// action — a real <button>, not a styled div, so it's keyboard-operable.
export const retryButton = style([cardAction, {}]);

export const skeletonList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[2],
});

export const cleanState = style({
  padding: `${vars.space[6]} ${vars.space[4]}`,
  textAlign: "center",
  color: vars.color.success,
  background: vars.color.successBg,
  borderRadius: vars.radii.lg,
  fontSize: vars.fontSize.base,
});

export const unpricedState = style({
  padding: `${vars.space[6]} ${vars.space[4]}`,
  textAlign: "center",
  color: vars.color.textSecondary,
  background: vars.color.hoverBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  fontSize: vars.fontSize.base,
});
