import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
});

export const sectionTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  marginBottom: vars.space["1"],
});

const cardBase = style({
  borderRadius: vars.radii.md,
  padding: vars.space["3"],
  borderLeft: "4px solid",
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const cardNoPlan = style([
  cardBase,
  {
    borderLeftColor: vars.color.textMuted,
    background: vars.color.cardBackground,
  },
]);

export const cardPendingReview = style([
  cardBase,
  // "pending-blue" — reuses the accent tint (Primary-hued info blue), which
  // is distinct from GateVerdictBox's grey PENDING and from the warning/error
  // hues used elsewhere on this page.
  {
    borderLeftColor: vars.color.primary,
    background: vars.color.accentBg,
  },
]);

export const cardApproved = style([
  cardBase,
  {
    borderLeftColor: vars.color.success,
    background: vars.color.successBg,
  },
]);

export const cardChangesRequested = style([
  cardBase,
  // P9: deliberately NOT vars.color.warning/warningBg — that pair is already
  // MergeabilityPill's "changes requested" PR-review pill color
  // (web-app/src/components/shared/vcs-widget/MergeabilityPill.css.ts). Both
  // can render on the same item detail page (a plan-review verdict and a PR
  // mergeability pill), so this state uses the "critical" tier instead — a
  // distinct hue from both warning (MergeabilityPill) and error (GateVerdictBox
  // FAIL) — paired with "Revisions requested" copy (not "Changes requested")
  // so the two states never read as duplicates.
  {
    borderLeftColor: vars.color.critical,
    background: vars.color.criticalBg,
  },
]);

export const cardSkipped = style([
  cardBase,
  // Distinct border style (dashed) from cardNoPlan's solid grey border — an
  // intentionally-bypassed plan review is semantically distinct from "no
  // plan yet", even though both use the same muted color family.
  {
    borderLeftStyle: "dashed",
    borderLeftColor: vars.color.borderStrong,
    background: vars.color.surfaceMuted,
  },
]);

export const cardHeader = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const iconNoPlan = style({ color: vars.color.textMuted });
// accentText (not primary) — primary fails WCAG AA (4.09:1) against this
// card's accentBg background; accentText is tuned per-theme to guarantee
// >=4.5:1. See theme.css.ts's accentText notes and InlineNotice.css.ts's
// icon style for the same fix applied to an identical pairing.
export const iconPendingReview = style({ color: vars.color.accentText });
export const iconApproved = style({ color: vars.color.success, fontWeight: vars.fontWeight.bold });
export const iconChangesRequested = style({ color: vars.color.criticalText });
export const iconSkipped = style({ color: vars.color.textMuted });

export const label = style({
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.bold,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  fontFamily: vars.font.mono,
});

export const labelNoPlan = style({ color: vars.color.textMuted });
// See iconPendingReview above — same WCAG AA contrast fix.
export const labelPendingReview = style({ color: vars.color.accentText });
export const labelApproved = style({ color: vars.color.success });
export const labelChangesRequested = style({ color: vars.color.criticalText });
export const labelSkipped = style({ color: vars.color.textMuted });

export const reasonText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  whiteSpace: "pre-wrap",
});

export const actions = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  flexWrap: "wrap",
});

const buttonBase = style({
  display: "inline-flex",
  alignItems: "center",
  minHeight: "44px",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontWeight: vars.fontWeight.medium,
});

export const secondaryButton = style([
  buttonBase,
  {
    background: "none",
    border: `1px solid ${vars.color.borderMuted}`,
    color: vars.color.textSecondary,
    ":hover": {
      borderColor: vars.color.borderStrong,
      color: vars.color.textPrimary,
    },
    ":disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
]);

export const primaryButton = style([
  buttonBase,
  {
    background: vars.color.primary,
    color: vars.color.primaryText,
    border: "none",
    ":hover": {
      background: vars.color.primaryHover,
    },
    ":disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
]);

export const form = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const formLabel = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
});

export const formTextarea = style({
  width: "100%",
  minHeight: "72px",
  padding: vars.space["2"],
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.sans,
  resize: "vertical",
  ":focus": {
    outline: "none",
    borderColor: vars.color.inputFocusBorder,
  },
});

export const formActions = style({
  display: "flex",
  gap: vars.space["2"],
});
