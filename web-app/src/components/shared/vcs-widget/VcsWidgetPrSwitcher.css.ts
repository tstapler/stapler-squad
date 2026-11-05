import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import { MIN_TOUCH_TARGET } from "@/lib/unfinished/prTouchTokens";

export const container = style({ display: "flex", flexDirection: "column", gap: vars.space["1"], minWidth: 0 });
export const row = style({ display: "flex", alignItems: "center", flexWrap: "wrap", gap: vars.space["2"], fontSize: vars.fontSize.sm });
export const segments = style({ display: "inline-flex", gap: vars.space["1"], flexWrap: "wrap" });

export const segment = style({
  minHeight: MIN_TOUCH_TARGET,
  padding: `0 ${vars.space["3"]}`,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: vars.color.surfaceSubtle,
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  ":focus-visible": { outline: `2px solid ${vars.color.inputFocusBorder}`, outlineOffset: "2px" },
});
export const segmentSelected = style({ background: vars.color.primary, color: vars.color.onPrimaryFill, borderColor: vars.color.primaryDark, fontWeight: 700 });

export const select = style({
  minHeight: MIN_TOUCH_TARGET,
  maxWidth: "100%",
  padding: `0 ${vars.space["2"]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: "16px",
});

export const target = style({ margin: 0, fontSize: vars.fontSize.xs, color: vars.color.textSecondary, overflowWrap: "anywhere" });
