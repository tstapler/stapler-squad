import { style, styleVariants } from "@vanilla-extract/css";
import { vars } from "../../styles/theme.css";

export const panelContainer = style({
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: vars.space["3"],
  marginTop: vars.space["3"],
});

export const summary = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  cursor: "pointer",
  userSelect: "none",
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  listStyle: "none",
  selectors: {
    "&::-webkit-details-marker": {
      display: "none",
    },
  },
});

export const summaryLabel = style({
  color: vars.color.textPrimary,
});

export const statusChipBase = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `2px 8px`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  whiteSpace: "nowrap",
});

export const statusChipVariants = styleVariants({
  idle: {
    background: vars.statusBadge.idleBg,
    color: vars.statusBadge.idleFg,
    border: `1px solid ${vars.statusBadge.idleBorder}`,
  },
  working: {
    background: vars.statusBadge.processingBg,
    color: vars.statusBadge.processingFg,
    border: `1px solid ${vars.statusBadge.processingBorder}`,
  },
  blocked: {
    background: vars.statusBadge.approvalBg,
    color: vars.statusBadge.approvalFg,
    border: `1px solid ${vars.statusBadge.approvalBorder}`,
  },
  done: {
    background: vars.statusBadge.completeBg,
    color: vars.statusBadge.completeFg,
    border: `1px solid ${vars.statusBadge.completeBorder}`,
  },
});

export const body = style({
  marginTop: vars.space["2"],
});

export const goalText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  lineHeight: "1.5",
});

export const goalTextClamped = style({
  display: "-webkit-box",
  WebkitLineClamp: "2",
  WebkitBoxOrient: "vertical",
  overflow: "hidden",
});

export const expandButton = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.primary,
  background: "none",
  border: "none",
  cursor: "pointer",
  padding: 0,
  marginLeft: vars.space["1"],
  selectors: {
    "&:hover": {
      textDecoration: "underline",
    },
  },
});

export const taskFraction = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  marginTop: vars.space["1"],
});

export const taskList = style({
  listStyle: "none",
  margin: 0,
  padding: 0,
  marginTop: vars.space["2"],
});

export const taskItem = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `2px 0`,
  fontSize: vars.fontSize.sm,
});

export const taskTitle = style({
  color: vars.color.textPrimary,
  flex: 1,
});

export const taskChildren = style({
  paddingLeft: vars.space["4"],
  marginTop: vars.space["1"],
});

export const taskStatusChipBase = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `1px 6px`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  whiteSpace: "nowrap",
});

export const taskStatusVariants = styleVariants({
  pending: {
    background: vars.statusBadge.idleBg,
    color: vars.statusBadge.idleFg,
  },
  in_progress: {
    background: vars.statusBadge.processingBg,
    color: vars.statusBadge.processingFg,
  },
  done: {
    background: vars.statusBadge.completeBg,
    color: vars.statusBadge.completeFg,
  },
  blocked: {
    background: vars.statusBadge.approvalBg,
    color: vars.statusBadge.approvalFg,
  },
});
