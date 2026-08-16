import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const section = style({
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: vars.space["4"],
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const sectionHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space["3"],
  flexWrap: "wrap",
});

export const sectionTitle = style({
  fontSize: vars.fontSize.lg,
  fontWeight: 700,
  color: vars.color.textPrimary,
  margin: 0,
});

export const countRegion = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  fontWeight: 500,
});

// Horizontal scroll strip (matches LevelFilterChips.css.ts) rather than
// flexWrap — with 19+ category chips, flexWrap depends on every ancestor
// flex box correctly shrinking to viewport width, which broke down on
// mobile (chips got cut off at the screen edge instead of wrapping).
// overflowX:auto contains the row's width regardless of ancestor sizing.
export const filterRow = style({
  display: "flex",
  gap: vars.space["2"],
  flexWrap: "nowrap",
  overflowX: "auto",
  scrollbarWidth: "none",
  WebkitOverflowScrolling: "touch",
  selectors: {
    "&::-webkit-scrollbar": {
      display: "none",
    },
  },
});

export const chip = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  color: vars.color.textSecondary,
  transition: "background 0.12s, color 0.12s",
  whiteSpace: "nowrap",
  flexShrink: 0,
  ":hover": {
    background: vars.color.hoverBackground,
    color: vars.color.textPrimary,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.inputFocusBorder}`,
    outlineOffset: "1px",
  },
});

export const chipActive = style({
  // primary+textInverse fails WCAG AA (4.43:1 in the clean theme, needs
  // 4.5:1) — primaryActive+primaryText is the existing "pressed state" token
  // pair and passes with margin (6.29:1) without touching primary's much
  // broader usage across the app.
  background: vars.color.primaryActive,
  color: vars.color.primaryText,
  borderColor: vars.color.primaryActive,
  ":hover": {
    // primaryHover (#818cf8) is a light indigo — textInverse (dark) is the
    // correct pairing here, not primaryText (white), which would fail
    // contrast against this specific background.
    background: vars.color.primaryHover,
    color: vars.color.textInverse,
  },
});

export const groupHeading = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textSecondary,
  margin: `${vars.space["2"]} 0 0 0`,
  paddingBottom: vars.space["1"],
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
});

export const group = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const itemList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const empty = style({
  textAlign: "center",
  color: vars.color.textMuted,
  padding: `${vars.space["8"]} 0`,
  fontSize: vars.fontSize.base,
});

export const filteredEmpty = style({
  textAlign: "center",
  color: vars.color.textMuted,
  padding: `${vars.space["6"]} 0`,
  fontSize: vars.fontSize.sm,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  alignItems: "center",
});

export const loading = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  padding: `${vars.space["4"]} 0`,
});

export const spinner = style({
  display: "inline-block",
  width: "0.9em",
  height: "0.9em",
  border: `2px solid ${vars.color.borderColor}`,
  borderTopColor: vars.color.primary,
  borderRadius: "50%",
  animation: "spin 0.6s linear infinite",
  "@keyframes": {
    spin: {
      from: { transform: "rotate(0deg)" },
      to: { transform: "rotate(360deg)" },
    },
  },
} as Parameters<typeof import("@vanilla-extract/css").style>[0]);

export const errorBanner = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space["3"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  background: vars.color.warningBg,
  color: vars.color.warningText,
  border: `1px solid ${vars.color.warning}`,
  fontSize: vars.fontSize.sm,
  flexWrap: "wrap",
});

export const errorBannerFullBody = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space["3"],
  padding: vars.space["4"],
  borderRadius: vars.radii.sm,
  background: vars.color.errorBg,
  color: vars.color.errorText,
  border: `1px solid ${vars.color.error}`,
  fontSize: vars.fontSize.sm,
  flexWrap: "wrap",
});

export const retryBtn = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `1px solid currentColor`,
  background: "transparent",
  color: "inherit",
  fontWeight: 600,
  flexShrink: 0,
  ":focus-visible": {
    outline: `2px solid ${vars.color.inputFocusBorder}`,
    outlineOffset: "1px",
  },
});

// ── Bulk "Reset all parked" affordance (docs/tasks/backlog-stuck-item-auto-remediation.md
// Phase A minimal UI requirement) — an admin action, so it's styled to stand
// apart from the read-only filter chips rather than blending in with them.

export const resetParkedBtn = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `1px solid ${vars.color.warning}`,
  background: "transparent",
  color: vars.color.warningText,
  fontWeight: 600,
  flexShrink: 0,
  ":hover": {
    background: vars.color.warningBg,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.inputFocusBorder}`,
    outlineOffset: "1px",
  },
  ":disabled": {
    opacity: 0.6,
    cursor: "not-allowed",
  },
});

export const resetParkedMessage = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const resetParkedMessageError = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.errorText,
});

export const clearFilterBtn = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  color: vars.color.textSecondary,
  ":hover": {
    background: vars.color.hoverBackground,
    color: vars.color.textPrimary,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.inputFocusBorder}`,
    outlineOffset: "1px",
  },
});
