import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const wrapper = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

// The one-line icon+copy header shown above the reused GateVerdictBox card
// for the four DiagnoseOutcomeKind cases, and standalone for Pending/Stalled.
export const outcomeHeader = style({
  margin: 0,
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
});

export const pendingBanner = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textSecondary,
});

// Adapts stuck-item-autonomous-stuck-copy's plain, non-alarming treatment --
// an informational settle with a recovery lever, never role="alert".
export const stalledBanner = style({
  margin: 0,
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
});

// Standing "nudging disabled" notice (Surface 10) -- informational, never
// styled as a warning or error.
export const flagOffBanner = style({
  margin: 0,
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: vars.space["2"],
  borderRadius: vars.radii.md,
  background: vars.color.surfaceMuted,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const dispatchFailedBox = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: vars.space["3"],
  borderRadius: vars.radii.md,
  borderLeft: `4px solid ${vars.color.error}`,
  background: vars.color.errorBg,
});

export const dispatchFailedText = style({
  margin: 0,
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  fontSize: vars.fontSize.sm,
  color: vars.color.errorText,
});

export const retryButton = style({
  alignSelf: "flex-start",
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.error}`,
  background: vars.color.background,
  color: vars.color.errorText,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  cursor: "pointer",
  ":disabled": {
    cursor: "not-allowed",
    opacity: 0.6,
  },
});

export const link = style({
  color: vars.color.primary,
  textDecoration: "none",
  ":hover": {
    textDecoration: "underline",
  },
});
