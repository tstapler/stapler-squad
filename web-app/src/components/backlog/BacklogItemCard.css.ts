import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

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

export const cardFooter = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
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
