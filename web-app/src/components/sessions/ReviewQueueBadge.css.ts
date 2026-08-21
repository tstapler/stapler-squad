import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const pulse = keyframes({
  "0%, 100%": { opacity: 1 },
  "50%": { opacity: 0.7 },
});

export const badge = style({
  display: "flex",
  gap: "8px",
  alignItems: "center",
  flexWrap: "wrap",
});

export const priorityAbbr = style({
  display: "none",
  fontSize: "10px",
  fontWeight: 700,
  letterSpacing: "0.02em",
  "@media": {
    "(min-width: 768px)": {
      display: "inline",
    },
  },
});

export const badgeCompact = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  width: "24px",
  height: "24px",
  borderRadius: "50%",
  fontSize: "14px",
  cursor: "help",
  transition: "transform 0.2s ease",
  selectors: {
    "&:hover": {
      transform: "scale(1.2)",
    },
  },
});

const sharedBadge = style({
  padding: "4px 12px",
  borderRadius: "12px",
  fontSize: "12px",
  fontWeight: 600,
  textTransform: "uppercase",
  whiteSpace: "nowrap",
});

export const priority = style([
  sharedBadge,
  {
    display: "flex",
    alignItems: "center",
    gap: "4px",
  },
]);

export const reason = style([sharedBadge]);

export const priorityUrgent = style({
  background: vars.statusBadge.approvalBg,
  color: vars.statusBadge.approvalFg,
  border: `1px solid ${vars.statusBadge.approvalBorder}`,
});

export const priorityHigh = style({
  background: vars.statusBadge.uncommittedBg,
  color: vars.statusBadge.uncommittedFg,
  border: `1px solid ${vars.statusBadge.uncommittedBorder}`,
});

export const priorityMedium = style({
  background: vars.statusBadge.inputBg,
  color: vars.statusBadge.inputFg,
  border: `1px solid ${vars.statusBadge.inputBorder}`,
});

export const priorityLow = style({
  background: vars.statusBadge.idleBg,
  color: vars.statusBadge.idleFg,
  border: `1px solid ${vars.statusBadge.idleBorder}`,
});

export const priorityUnspecified = style({
  background: vars.statusBadge.idleBg,
  color: vars.statusBadge.idleFg,
  border: `1px solid ${vars.statusBadge.idleBorder}`,
});

export const reasonApproval = style({
  background: vars.statusBadge.uncommittedBg,
  color: vars.statusBadge.uncommittedFg,
  border: `1px solid ${vars.statusBadge.uncommittedBorder}`,
});

export const reasonInput = style({
  background: vars.statusBadge.inputBg,
  color: vars.statusBadge.inputFg,
  border: `1px solid ${vars.statusBadge.inputBorder}`,
});

export const reasonError = style({
  background: vars.statusBadge.approvalBg,
  color: vars.statusBadge.approvalFg,
  border: `1px solid ${vars.statusBadge.approvalBorder}`,
});

export const reasonIdle = style({
  background: vars.statusBadge.processingBg,
  color: vars.statusBadge.processingFg,
  border: `1px solid ${vars.statusBadge.processingBorder}`,
});

export const reasonComplete = style({
  background: vars.statusBadge.completeBg,
  color: vars.statusBadge.completeFg,
  border: `1px solid ${vars.statusBadge.completeBorder}`,
});

export const reasonUnspecified = style({
  background: vars.statusBadge.idleBg,
  color: vars.statusBadge.idleFg,
  border: `1px solid ${vars.statusBadge.idleBorder}`,
});
