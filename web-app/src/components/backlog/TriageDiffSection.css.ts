import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const diffContainer = style({
  display: "grid",
  gridTemplateColumns: "1fr 1fr",
  gap: vars.space["4"],
  marginTop: vars.space["2"],
});

export const diffColumn = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const columnHeader = style({
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  marginBottom: vars.space["1"],
});

export const emptyState = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  fontStyle: "italic",
});

export const addedItem = style({
  display: "flex",
  gap: vars.space["1"],
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  backgroundColor: vars.color.successBg,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  lineHeight: "1.5",
});

export const removedItem = style({
  display: "flex",
  gap: vars.space["1"],
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  backgroundColor: vars.color.errorBg,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  lineHeight: "1.5",
  textDecoration: "line-through",
});

export const diffPrefix = style({
  flexShrink: 0,
  fontWeight: vars.fontWeight.semibold,
});

export const questionsSection = style({
  marginTop: vars.space["4"],
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const questionsHeading = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textSecondary,
  margin: 0,
});

export const questionItem = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  backgroundColor: vars.color.accentBg,
  borderRadius: vars.radii.sm,
  lineHeight: "1.5",
});
