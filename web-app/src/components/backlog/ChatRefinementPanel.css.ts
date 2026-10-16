import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const panel = style({
  backgroundColor: vars.color.cardBackground,
  borderLeft: `3px solid ${vars.color.primary}`,
  borderRadius: vars.radii.md,
  padding: vars.space["4"],
  marginBottom: vars.space["4"],
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const heading = style({
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  margin: 0,
});

export const questionBanner = style({
  backgroundColor: vars.color.warningBg,
  borderRadius: vars.radii.sm,
  padding: vars.space["3"],
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const questionText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  margin: 0,
});

export const questionMeta = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const transcript = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  maxHeight: "16rem",
  overflowY: "auto",
});

export const turn = style({
  borderRadius: vars.radii.sm,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.sm,
  whiteSpace: "pre-wrap",
});

export const turnUser = style([
  turn,
  {
    backgroundColor: vars.color.hoverBackground,
    alignSelf: "flex-end",
  },
]);

export const turnAssistant = style([
  turn,
  {
    backgroundColor: vars.color.background,
    border: `1px solid ${vars.color.borderColor}`,
    alignSelf: "flex-start",
  },
]);

export const inputRow = style({
  display: "flex",
  gap: vars.space["2"],
});

export const input = style({
  flex: 1,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.inputBorder}`,
  backgroundColor: vars.color.inputBackground,
  color: vars.color.inputText,
  fontSize: vars.fontSize.sm,
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
    outline: "none",
  },
});

export const sendButton = style({
  backgroundColor: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.sm,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  transition: vars.transition.fast,
  ":disabled": {
    opacity: 0.6,
    cursor: "not-allowed",
  },
});

export const errorText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.errorText,
  margin: 0,
});
