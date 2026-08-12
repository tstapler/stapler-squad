import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const chip = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  lineHeight: 1.5,
  flexShrink: 0,
  whiteSpace: "nowrap",
});

export const chipPrReady = style([
  chip,
  {
    background: vars.color.successBg,
    color: vars.color.successText,
    border: `1px solid ${vars.color.success}`,
  },
]);

export const chipAbandonedReview = style([
  chip,
  {
    background: vars.color.warningBg,
    color: vars.color.warningText,
    border: `1px solid ${vars.color.warning}`,
  },
]);

export const chipReworkCap = style([
  chip,
  {
    background: vars.color.errorBg,
    color: vars.color.errorText,
    border: `1px solid ${vars.color.error}`,
  },
]);

export const chipStaleWork = style([
  chip,
  {
    background: vars.color.warningBg,
    color: vars.color.warningText,
    border: `1px solid ${vars.color.warning}`,
  },
]);

export const chipBouncing = style([
  chip,
  {
    background: vars.color.errorBg,
    color: vars.color.errorText,
    border: `1px solid ${vars.color.error}`,
  },
]);

export const chipPushFailed = style([
  chip,
  {
    background: vars.color.errorBg,
    color: vars.color.errorText,
    border: `1px solid ${vars.color.error}`,
  },
]);

export const chipOrphanedTriage = style([
  chip,
  {
    background: vars.color.warningBg,
    color: vars.color.warningText,
    border: `1px solid ${vars.color.warning}`,
  },
]);

export const chipAutonomousStuck = style([
  chip,
  {
    background: vars.color.warningBg,
    color: vars.color.warningText,
    border: `1px solid ${vars.color.warning}`,
  },
]);

export const chipSpawnFailed = style([
  chip,
  {
    background: vars.color.errorBg,
    color: vars.color.errorText,
    border: `1px solid ${vars.color.error}`,
  },
]);

export const chipPlanNotApproved = style([
  chip,
  {
    background: vars.color.warningBg,
    color: vars.color.warningText,
    border: `1px solid ${vars.color.warning}`,
  },
]);

export const chipPrPendingNoPR = style([
  chip,
  {
    background: vars.color.errorBg,
    color: vars.color.errorText,
    border: `1px solid ${vars.color.error}`,
  },
]);

export const chipReworkBlockedStale = style([
  chip,
  {
    background: vars.color.errorBg,
    color: vars.color.errorText,
    border: `1px solid ${vars.color.error}`,
  },
]);

export const chipPrNeedsFix = style([
  chip,
  {
    background: vars.color.warningBg,
    color: vars.color.warningText,
    border: `1px solid ${vars.color.warning}`,
  },
]);

export const chipRespawnBlockedActive = style([
  chip,
  {
    background: vars.color.warningBg,
    color: vars.color.warningText,
    border: `1px solid ${vars.color.warning}`,
  },
]);

// Escalation chip (multiple_reasons / bounce_cap_exhausted) — deliberately its
// own independent style(), not a `chipXxx` variant reused from an existing
// reason, and using the `critical` token trio (unused by every other chip in
// this file) so it reads as visually distinct rather than "another warning/
// error chip" — see research/ux.md's "never repurpose existing chip colors
// for severity" constraint (plan.md Story 2.1.2).
export const chipEscalated = style([
  chip,
  {
    background: vars.color.criticalBg,
    color: vars.color.criticalText,
    border: `2px solid ${vars.color.critical}`,
  },
]);

export const chipUnknown = style([
  chip,
  {
    background: vars.color.surfaceMuted,
    color: vars.color.textMuted,
    border: `1px solid ${vars.color.borderMuted}`,
  },
]);
