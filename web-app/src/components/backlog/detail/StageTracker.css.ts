import { style, styleVariants } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

// Positioning context for the archived ribbon overlay (Task 2.1.1a).
export const container = style({
  position: "relative",
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
});

export const track = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: vars.space["1"],
  margin: 0,
  padding: 0,
  listStyle: "none",
});

const nodeBase = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  whiteSpace: "nowrap",
});

// Per-node state, matching GoalPanel.css.ts's statusChipVariants pattern:
// pending (not yet reached) / active (item.status's current stage) / done
// (already passed). Never color-only — the node's label text is always
// present alongside the color (see StageTracker.tsx).
export const nodeVariants = styleVariants({
  pending: [
    nodeBase,
    {
      background: vars.statusBadge.idleBg,
      color: vars.statusBadge.idleFg,
      border: `1px solid ${vars.statusBadge.idleBorder}`,
    },
  ],
  active: [
    nodeBase,
    {
      background: vars.statusBadge.processingBg,
      color: vars.statusBadge.processingFg,
      border: `1px solid ${vars.statusBadge.processingBorder}`,
      fontWeight: vars.fontWeight.semibold,
    },
  ],
  done: [
    nodeBase,
    {
      background: vars.statusBadge.completeBg,
      color: vars.statusBadge.completeFg,
      border: `1px solid ${vars.statusBadge.completeBorder}`,
    },
  ],
});

// Modifier badge (e.g. "Queued", "PR pending") — visually distinct from a
// stage node so it never reads as a 6th stage, per ux.md's "keep the tracker
// and blocker chip visually distinct" guidance applied here.
export const modifierBadge = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `2px ${vars.space["1"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  background: vars.statusBadge.uncommittedBg,
  color: vars.statusBadge.uncommittedFg,
  border: `1px solid ${vars.statusBadge.uncommittedBorder}`,
  whiteSpace: "nowrap",
});

// Archived ribbon overlay — deliberately distinct from any node's shape
// (a banner across the whole tracker, not a pill) so it can never be
// mistaken for a 6th stage node.
export const archivedRibbon = style({
  position: "absolute",
  inset: 0,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  background: vars.color.surfaceMuted,
  color: vars.color.textDisabled,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  letterSpacing: "0.05em",
  textTransform: "uppercase",
  border: `1px dashed ${vars.color.borderMuted}`,
  zIndex: zIndex.raised,
});

// The tracker itself dims to a neutral state underneath the ribbon rather
// than guessing which stage the item was archived from.
export const trackDimmed = style({
  opacity: 0.35,
});
