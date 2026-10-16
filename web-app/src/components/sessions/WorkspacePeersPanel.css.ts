import { style, styleVariants } from "@vanilla-extract/css";
import { vars } from "../../styles/theme.css";

export const panelContainer = style({
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: vars.space["3"],
  marginTop: vars.space["3"],
});

export const headingRow = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space["2"],
  marginBottom: vars.space["2"],
});

export const heading = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textPrimary,
});

export const dismissButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "24px",
  height: "24px",
  minWidth: "24px",
  padding: 0,
  border: "none",
  background: "transparent",
  color: vars.color.textMuted,
  cursor: "pointer",
  fontSize: vars.fontSize.xs,
  borderRadius: vars.radii.sm,
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
      background: vars.color.hoverBackground,
    },
  },
});

export const peerList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  listStyle: "none",
  margin: 0,
  padding: 0,
});

export const peerItem = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  padding: vars.space["2"],
  borderRadius: vars.radii.sm,
  background: vars.color.hoverBackground,
});

export const peerTitleRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const peerTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textPrimary,
});

export const peerMeta = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const peerGoal = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
});

export const lifecycleChipBase = style({
  display: "inline-flex",
  alignItems: "center",
  padding: "2px 8px",
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  whiteSpace: "nowrap",
});

export const lifecycleChipVariants = styleVariants({
  active: {
    background: vars.statusBadge.processingBg,
    color: vars.statusBadge.processingFg,
    border: `1px solid ${vars.statusBadge.processingBorder}`,
  },
  stuck: {
    background: vars.statusBadge.approvalBg,
    color: vars.statusBadge.approvalFg,
    border: `1px solid ${vars.statusBadge.approvalBorder}`,
  },
  gone: {
    background: vars.color.errorBg,
    color: vars.color.errorText,
    border: `1px solid ${vars.color.error}`,
  },
});
