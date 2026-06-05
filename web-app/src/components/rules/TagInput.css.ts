import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

const highlightFade = keyframes({
  "0%": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "1px" },
  "100%": { outline: "2px solid transparent", outlineOffset: "1px" },
});

export const container = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space["1"],
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  background: vars.color.inputBackground,
  cursor: "text",
  minHeight: "38px",
  alignItems: "center",
  ":focus-within": {
    borderColor: vars.color.inputFocusBorder,
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "1px",
  },
});

export const containerPrefilled = style({
  animation: `${highlightFade} 2s ease-out forwards`,
});

export const chip = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  background: vars.color.primary,
  color: vars.color.textInverse,
  fontFamily: vars.font.mono,
  lineHeight: "1.4",
  whiteSpace: "nowrap",
});

export const chipDisabled = style({
  background: vars.color.surfaceSubtle,
  color: vars.color.textMuted,
});

export const chipRemove = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  padding: "0 2px",
  color: "inherit",
  opacity: 0.7,
  fontSize: vars.fontSize.sm,
  lineHeight: 1,
  ":hover": { opacity: 1 },
});

export const input = style({
  flex: 1,
  minWidth: "120px",
  border: "none",
  outline: "none",
  background: "transparent",
  color: vars.color.inputText,
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.mono,
  padding: "2px 0",
  "::placeholder": { color: vars.color.placeholderColor },
});

export const helperText = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  marginTop: vars.space["1"],
});
