import { keyframes, style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const spinKeyframes = keyframes({
  from: { transform: "rotate(0deg)" },
  to: { transform: "rotate(360deg)" },
});

export const buttonSpinner = style({
  display: "inline-block",
  width: 12,
  height: 12,
  border: `2px solid currentColor`,
  borderTopColor: "transparent",
  borderRadius: vars.radii.full,
  animation: `${spinKeyframes} 0.7s linear infinite`,
  opacity: 0.7,
  flexShrink: 0,
});

export const card = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: vars.space["3"],
  cursor: "pointer",
  transition: "border-color 0.15s ease, box-shadow 0.15s ease",
  ":hover": {
    borderColor: vars.color.borderHover,
    boxShadow: vars.shadow.sm,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

// Epic 6.1 (backlog-event-driven-updates): brief background-tint pulse when
// a genuine live event updates this card's item (ux.md §1 — "Linear/Jira-
// style", ~250ms, fading). Composed with `card` at the call site the same
// way `actionButtonDone` overrides `actionButton` above — a later-declared
// single-class rule, so its `backgroundColor` wins over `card`'s.
const flashKeyframes = keyframes({
  "0%": { backgroundColor: vars.color.accentHover },
  "100%": { backgroundColor: vars.color.cardBackground },
});

export const justChanged = style({
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      animationName: flashKeyframes,
      animationDuration: "250ms",
      animationTimingFunction: "ease-out",
      animationFillMode: "forwards",
    },
    // Reduced motion: no animation/transition — just an instant, flat tint.
    // The class itself is still removed by BacklogItemCard's timeout, so
    // the background still "sets then clears," just without a keyframe.
    "(prefers-reduced-motion: reduce)": {
      backgroundColor: vars.color.accentHover,
    },
  },
});

export const cardHeader = style({
  display: "flex",
  alignItems: "flex-start",
  gap: vars.space["2"],
  justifyContent: "space-between",
});

export const title = style({
  fontWeight: vars.fontWeight.semibold,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  lineHeight: "1.4",
  overflow: "hidden",
  display: "-webkit-box",
  WebkitLineClamp: 2,
  WebkitBoxOrient: "vertical",
  flex: 1,
});

export const priorityBadge = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  borderRadius: vars.radii.sm,
  padding: `0 ${vars.space["1"]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.bold,
  fontFamily: vars.font.mono,
  flexShrink: 0,
  minWidth: "24px",
  height: "20px",
  background: vars.color.accentBg,
  color: vars.color.primary,
  border: `1px solid ${vars.color.borderMuted}`,
});

export const statusLabel = style({
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textMuted,
  flexShrink: 0,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
});

// Provenance badge (Epic 4.1, backlog-github-two-way-sync): "imported from
// GitHub" indicator + link. Neutral surface/text/border tokens, not a
// GitHub-brand color — ux.md flags brand colors here as unaudited for
// contrast in both themes.
export const provenanceBadge = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  borderRadius: vars.radii.sm,
  padding: `0 ${vars.space["1"]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  height: "20px",
  background: vars.color.surfaceMuted,
  color: vars.color.textMuted,
  border: `1px solid ${vars.color.borderMuted}`,
  textDecoration: "none",
  flexShrink: 0,
  ":hover": {
    color: vars.color.textSecondary,
    borderColor: vars.color.borderHover,
  },
});

export const cardFooter = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  flexWrap: "wrap",
  gap: vars.space["2"],
});

export const acSummary = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  fontFamily: vars.font.mono,
});

export const actionButton = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  borderRadius: vars.radii.sm,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderMuted}`,
  background: vars.color.accentBg,
  color: vars.color.primary,
  transition: "background 0.1s ease, border-color 0.1s ease",
  ":hover": {
    background: vars.color.accentHover,
    borderColor: vars.color.primary,
  },
  ":disabled": {
    opacity: 0.4,
    cursor: "not-allowed",
  },
});

export const actionButtonDone = style({
  background: vars.statusBadge.completeBg,
  color: vars.statusBadge.completeFg,
  borderColor: vars.statusBadge.completeBorder,
  cursor: "default",
  ":hover": {
    background: vars.statusBadge.completeBg,
    borderColor: vars.statusBadge.completeBorder,
  },
});
