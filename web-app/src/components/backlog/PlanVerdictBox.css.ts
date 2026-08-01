import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

const cardBase = style({
  borderRadius: vars.radii.md,
  padding: vars.space["3"],
  borderLeft: "4px solid",
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const cardApproved = style([
  cardBase,
  { borderLeftColor: vars.color.success, background: vars.color.successBg },
]);

export const cardPendingReview = style([
  cardBase,
  { borderLeftColor: vars.color.primary, background: vars.color.cardBackground },
]);

export const cardChangesRequested = style([
  cardBase,
  { borderLeftColor: vars.color.warning, background: vars.color.warningBg },
]);

export const cardNoPlan = style([
  cardBase,
  { borderLeftColor: vars.color.textMuted, background: vars.color.cardBackground },
]);

export const cardSkipped = style([
  cardBase,
  { borderLeftColor: vars.color.borderStrong, background: vars.color.surfaceMuted },
]);

export const header = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const iconApproved = style({ color: vars.color.success, fontWeight: vars.fontWeight.bold });
export const iconPendingReview = style({ color: vars.color.primary });
export const iconChangesRequested = style({ color: vars.color.warning });
export const iconNoPlan = style({ color: vars.color.textMuted });
export const iconSkipped = style({ color: vars.color.textSecondary });

export const label = style({
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.bold,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  fontFamily: vars.font.mono,
});

export const labelApproved = style({ color: vars.color.success });
export const labelPendingReview = style({ color: vars.color.primary });
export const labelChangesRequested = style({ color: vars.color.warningText });
export const labelNoPlan = style({ color: vars.color.textMuted });
export const labelSkipped = style({ color: vars.color.textSecondary });

export const reasonText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  whiteSpace: "pre-wrap",
});

export const actions = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  flexWrap: "wrap",
});

const buttonBase = style({
  display: "inline-flex",
  alignItems: "center",
  minHeight: "44px",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontWeight: vars.fontWeight.medium,
});

export const primaryButton = style([
  buttonBase,
  {
    background: vars.color.primary,
    color: vars.color.primaryText,
    border: "none",
    ":hover": { background: vars.color.primaryHover },
    ":disabled": { opacity: 0.5, cursor: "not-allowed" },
  },
]);

export const secondaryButton = style([
  buttonBase,
  {
    background: "none",
    border: `1px solid ${vars.color.borderMuted}`,
    color: vars.color.textSecondary,
    ":hover": { borderColor: vars.color.borderStrong, color: vars.color.textPrimary },
    ":disabled": { opacity: 0.5, cursor: "not-allowed" },
  },
]);

export const form = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const formLabel = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
});

export const formTextarea = style({
  width: "100%",
  minHeight: "72px",
  padding: vars.space["2"],
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.sans,
  resize: "vertical",
  ":focus": {
    outline: "none",
    borderColor: vars.color.inputFocusBorder,
  },
});

export const formActions = style({
  display: "flex",
  gap: vars.space["2"],
});
