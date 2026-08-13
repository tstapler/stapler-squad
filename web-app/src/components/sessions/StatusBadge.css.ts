import { style, styleVariants } from "@vanilla-extract/css";
import { vars } from "../../styles/theme.css";

export const badge = style({
  display: "inline-flex",
  alignItems: "center",
  gap: "4px",
  padding: "3px 10px",
  borderRadius: "12px",
  fontSize: "0.75rem",
  fontWeight: 600,
  whiteSpace: "nowrap",
});

export const reasonVariants = styleVariants({
  approval: {
    background: vars.statusBadge.approvalBg,
    color: vars.statusBadge.approvalFg,
    border: `1px solid ${vars.statusBadge.approvalBorder}`,
  },
  input: {
    background: vars.statusBadge.inputBg,
    color: vars.statusBadge.inputFg,
    border: `1px solid ${vars.statusBadge.inputBorder}`,
  },
  error: {
    background: vars.statusBadge.approvalBg,
    color: vars.statusBadge.approvalFg,
    border: `1px solid ${vars.statusBadge.approvalBorder}`,
  },
  testsFailing: {
    background: vars.statusBadge.approvalBg,
    color: vars.statusBadge.approvalFg,
    border: `1px solid ${vars.statusBadge.approvalBorder}`,
  },
  idle: {
    background: vars.statusBadge.idleBg,
    color: vars.statusBadge.idleFg,
    border: `1px solid ${vars.statusBadge.idleBorder}`,
  },
  complete: {
    background: vars.statusBadge.completeBg,
    color: vars.statusBadge.completeFg,
    border: `1px solid ${vars.statusBadge.completeBorder}`,
  },
  uncommitted: {
    background: vars.statusBadge.uncommittedBg,
    color: vars.statusBadge.uncommittedFg,
    border: `1px solid ${vars.statusBadge.uncommittedBorder}`,
  },
  stale: {
    background: vars.statusBadge.idleBg,
    color: vars.statusBadge.staleFg,
    border: `1px solid ${vars.statusBadge.idleBorder}`,
  },
  processing: {
    background: vars.statusBadge.processingBg,
    color: vars.statusBadge.processingFg,
    border: `1px solid ${vars.statusBadge.processingBorder}`,
  },
  active: {
    background: vars.statusBadge.inputBg,
    color: vars.statusBadge.inputFg,
    border: `1px solid ${vars.statusBadge.inputBorder}`,
  },
  unknown: {
    background: vars.color.cardBackground,
    color: vars.color.textSecondary,
    border: `1px solid ${vars.color.borderColor}`,
  },
});

export const icon = style({
  fontSize: "0.8125rem",
  lineHeight: 1,
});
