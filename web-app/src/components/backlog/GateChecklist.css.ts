import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const sectionTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
});

export const list = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  listStyle: "none",
  margin: 0,
  padding: 0,
});

// Visually hidden but still reachable by assistive tech — carries the
// list-level announcement text (see GateChecklist.tsx's aria-live comment).
export const srOnly = style({
  position: "absolute",
  width: "1px",
  height: "1px",
  padding: 0,
  margin: "-1px",
  overflow: "hidden",
  clip: "rect(0, 0, 0, 0)",
  whiteSpace: "nowrap",
  border: 0,
});

const rowBase = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  borderRadius: vars.radii.md,
  padding: vars.space["3"],
  borderLeft: "4px solid",
});

export const row = rowBase;

export const rowSatisfied = style([
  rowBase,
  {
    borderLeftColor: vars.color.success,
    background: vars.color.successBg,
  },
]);

export const rowPending = style([
  rowBase,
  {
    borderLeftColor: vars.color.textMuted,
    background: vars.color.cardBackground,
  },
]);

export const rowBlocked = style([
  rowBase,
  {
    borderLeftColor: vars.color.error,
    background: vars.color.errorBg,
  },
]);

export const rowConfigError = style([
  rowBase,
  {
    borderLeftColor: vars.color.warning,
    background: vars.color.warningBg,
  },
]);

export const rowHeader = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const rowIcon = style({
  fontWeight: vars.fontWeight.bold,
});

export const rowIconSatisfied = style({ color: vars.color.success });
export const rowIconPending = style({ color: vars.color.textMuted });
export const rowIconBlocked = style({ color: vars.color.error });
export const rowIconConfigError = style({ color: vars.color.warning });

export const rowLabel = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const rowDescription = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const rowActions = style({
  display: "flex",
  gap: vars.space["2"],
  flexWrap: "wrap",
});

const buttonBase = style({
  display: "inline-flex",
  alignItems: "center",
  minHeight: "44px",
  borderRadius: vars.radii.md,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
});

export const approveButton = style([
  buttonBase,
  {
    background: vars.color.primary,
    color: vars.color.primaryText,
    border: "none",
    ":hover": { background: vars.color.primaryHover },
    ":disabled": { opacity: 0.5, cursor: "not-allowed" },
  },
]);

export const rejectButton = style([
  buttonBase,
  {
    background: "none",
    border: `1px solid ${vars.color.borderMuted}`,
    color: vars.color.textSecondary,
    ":hover": { borderColor: vars.color.borderStrong, color: vars.color.textPrimary },
    ":disabled": { opacity: 0.5, cursor: "not-allowed" },
  },
]);

export const fixLink = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.primary,
  fontWeight: vars.fontWeight.medium,
});

export const staleStageNotice = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  padding: vars.space["2"],
  borderRadius: vars.radii.md,
  background: vars.color.surfaceMuted,
});
