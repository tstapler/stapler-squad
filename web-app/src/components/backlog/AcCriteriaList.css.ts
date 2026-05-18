import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const list = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  margin: 0,
  padding: 0,
  listStyle: "none",
});

export const item = style({
  display: "flex",
  alignItems: "flex-start",
  gap: vars.space["2"],
  fontSize: vars.fontSize.sm,
  lineHeight: "1.5",
  padding: `${vars.space["1"]} 0`,
});

export const checkbox = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  width: "18px",
  height: "18px",
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderMuted}`,
  flexShrink: 0,
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  marginTop: "1px",
});

export const checkboxPending = style({
  background: vars.color.surfaceMuted,
  color: vars.color.textDisabled,
  borderColor: vars.color.borderMuted,
});

export const checkboxInProgress = style({
  background: vars.statusBadge.uncommittedBg,
  color: vars.statusBadge.uncommittedFg,
  borderColor: vars.statusBadge.uncommittedBorder,
});

export const checkboxDone = style({
  background: vars.statusBadge.completeBg,
  color: vars.statusBadge.completeFg,
  borderColor: vars.statusBadge.completeBorder,
});

export const criterionIndex = style({
  color: vars.color.textMuted,
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  flexShrink: 0,
  minWidth: "20px",
  textAlign: "right",
});

export const criterionText = style({
  color: vars.color.textPrimary,
  flex: 1,
});

export const criterionTextDone = style({
  color: vars.color.textMuted,
  textDecoration: "line-through",
});

export const statusLabel = style({
  fontSize: vars.fontSize.xs,
  borderRadius: vars.radii.sm,
  padding: `0 ${vars.space["1"]}`,
  flexShrink: 0,
  fontWeight: vars.fontWeight.medium,
  lineHeight: "18px",
});

export const statusLabelPending = style({
  color: vars.color.textMuted,
  background: vars.color.surfaceMuted,
});

export const statusLabelInProgress = style({
  color: vars.statusBadge.uncommittedFg,
  background: vars.statusBadge.uncommittedBg,
});

export const statusLabelDone = style({
  color: vars.statusBadge.completeFg,
  background: vars.statusBadge.completeBg,
});

export const empty = style({
  color: vars.color.textMuted,
  fontStyle: "italic",
  fontSize: vars.fontSize.sm,
  padding: `${vars.space["2"]} 0`,
});
