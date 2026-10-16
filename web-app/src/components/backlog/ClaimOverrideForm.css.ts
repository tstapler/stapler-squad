import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const form = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  marginTop: vars.space["2"],
});

export const label = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
});

export const textarea = style({
  width: "100%",
  minHeight: "56px",
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

export const hint = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const error = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.errorText,
});

export const actions = style({
  display: "flex",
  gap: vars.space["2"],
});

const button = {
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
} as const;

export const cancelButton = style({
  ...button,
  background: "none",
  border: `1px solid ${vars.color.borderMuted}`,
  color: vars.color.textSecondary,
});

export const confirmButton = style({
  ...button,
  background: vars.color.error,
  border: "none",
  color: vars.color.primaryText,
  selectors: {
    "&:disabled": { opacity: 0.5, cursor: "not-allowed" },
  },
});
