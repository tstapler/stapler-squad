import { keyframes, style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
  padding: vars.space["4"],
});

export const header = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space["3"],
  flexWrap: "wrap",
});

export const titleGroup = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  flexWrap: "wrap",
});

export const title = style({
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.bold,
  color: vars.color.textPrimary,
});

export const statusText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
});

export const toolbar = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  flexWrap: "wrap",
});

const buttonBase = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderMuted}`,
  background: "transparent",
  color: vars.color.textSecondary,
  ":hover": {
    background: vars.color.hoverBackground,
    borderColor: vars.color.borderStrong,
    color: vars.color.textPrimary,
  },
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
});

export const copyButton = style([buttonBase, {}]);

export const primaryButton = style([
  buttonBase,
  {
    background: vars.color.primary,
    color: vars.color.primaryText,
    borderColor: vars.color.primary,
    ":hover": {
      background: vars.color.primaryHover,
      borderColor: vars.color.primaryHover,
      color: vars.color.primaryText,
    },
  },
]);

export const copyFailureText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.error,
});

// ---------------------------------------------------------------------------
// Decisions-at-a-glance card
// ---------------------------------------------------------------------------

export const glanceCard = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: vars.space["3"],
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderSubtle}`,
  background: vars.color.cardBackground,
});

export const glanceTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textSecondary,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
});

export const glancePills = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space["2"],
});

export const glancePill = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  background: vars.color.surfaceMuted,
  color: vars.color.textPrimary,
});

export const glanceEmptyText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  fontStyle: "italic",
});

// ---------------------------------------------------------------------------
// Error card (no stale document)
// ---------------------------------------------------------------------------

export const errorCard = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: vars.space["4"],
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.errorDark}`,
  background: vars.color.errorBg,
});

export const errorLead = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.errorText,
});

export const errorStageText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.errorText,
});

export const errorTimestamp = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const errorDetails = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
});

export const errorDetailsBody = style({
  marginTop: vars.space["1"],
  padding: vars.space["2"],
  borderRadius: vars.radii.sm,
  background: vars.color.surfaceMuted,
  fontFamily: vars.font.mono,
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
});

// ---------------------------------------------------------------------------
// Stale-document banner (ERROR with a prior READY document)
// ---------------------------------------------------------------------------

export const staleBanner = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: vars.space["3"],
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.warning}`,
  background: vars.color.warningBg,
});

export const staleBannerText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.warningText,
});

export const staleBannerActions = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

// ---------------------------------------------------------------------------
// Terminal empty state (neverResolved)
// ---------------------------------------------------------------------------

export const emptyState = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  padding: vars.space["6"],
  alignItems: "center",
  textAlign: "center",
  color: vars.color.textMuted,
});

export const emptyStateHeading = style({
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textSecondary,
});

// ---------------------------------------------------------------------------
// Skeleton (GENERATING) — see design/ux.md surface (b) for the exact spec:
// 5 heading bars + 12 content bars = 17 total `summary-skeleton-block`s.
// ---------------------------------------------------------------------------

const shimmerFrames = keyframes({
  "0%": { backgroundPosition: "200% 0" },
  "100%": { backgroundPosition: "-200% 0" },
});

export const skeletonSection = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

const skeletonBlockBase = style({
  borderRadius: vars.radii.sm,
  background: vars.color.borderSubtle,
});

// Shimmering variant — used unless the user's system requests reduced motion
// (UX-AC-14). The component itself decides which of skeletonBlock /
// skeletonBlockReducedMotion to apply per block, based on a JS-level
// `prefers-reduced-motion` read (so the choice is assertable via className
// in tests, not just inert at the CSS layer).
export const skeletonBlock = style([
  skeletonBlockBase,
  {
    background: `linear-gradient(90deg, ${vars.color.borderSubtle} 25%, ${vars.color.borderMuted} 50%, ${vars.color.borderSubtle} 75%)`,
    backgroundSize: "200% 100%",
    animation: `${shimmerFrames} 1.5s infinite`,
  },
]);

// Static variant — no animation at all, per UX-AC-14.
export const skeletonBlockReducedMotion = style([
  skeletonBlockBase,
  {
    background: vars.color.borderMuted,
  },
]);

export const skeletonHeading = style({
  width: "140px",
  height: "16px",
});

export const skeletonPill = style({
  width: "60px",
  height: "24px",
  borderRadius: vars.radii.full,
});

export const skeletonLine = style({
  width: "100%",
  height: "14px",
});

// "last line at 60% width" (What Was Done) / "link-line bar at 40% width" (Changes),
// per design/ux.md surface (b)'s exact shape table.
export const skeletonLineWidth60 = style({
  width: "60%",
  height: "14px",
});

export const skeletonLineWidth40 = style({
  width: "40%",
  height: "14px",
});
