import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
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

export const toggleButton = style([
  buttonBase,
  {
    background: "none",
    border: `1px solid ${vars.color.borderMuted}`,
    color: vars.color.textSecondary,
    ":hover": {
      borderColor: vars.color.borderStrong,
      color: vars.color.textPrimary,
    },
    ":disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
]);

export const submitButton = style([
  buttonBase,
  {
    background: vars.color.primary,
    color: vars.color.primaryText,
    border: "none",
    ":hover": {
      background: vars.color.primaryHover,
    },
    ":disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
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
