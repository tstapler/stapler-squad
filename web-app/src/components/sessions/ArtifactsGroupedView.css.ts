import { style } from "@vanilla-extract/css";
import { vars } from "../../styles/theme-contract.css";
import { MIN_TOUCH_TARGET } from "@/lib/unfinished/prTouchTokens";

export const groups = style({ display: "flex", flexDirection: "column", gap: vars.space["3"], minWidth: 0 });

export const group = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  minWidth: 0,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: vars.space["2"],
});

export const groupHeader = style({ display: "flex", alignItems: "center", flexWrap: "wrap", gap: vars.space["2"], minWidth: 0 });

export const disclosure = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["2"],
  minHeight: MIN_TOUCH_TARGET,
  minWidth: MIN_TOUCH_TARGET,
  padding: `0 ${vars.space["2"]}`,
  background: "none",
  border: "none",
  cursor: "pointer",
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  textAlign: "left",
  overflowWrap: "anywhere",
  selectors: { "&:focus-visible": { outline: `2px solid ${vars.color.inputFocusBorder}`, outlineOffset: "2px" } },
});

export const count = style({ fontWeight: 400, color: vars.color.textSecondary, fontSize: vars.fontSize.xs });
export const prLink = style({ color: vars.color.textPrimary, fontSize: vars.fontSize.sm, overflowWrap: "anywhere", textDecoration: "underline" });
export const note = style({ margin: 0, fontSize: vars.fontSize.xs, color: vars.color.textSecondary });

export const items = style({ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: vars.space["1"] });

export const item = style({ display: "flex", flexDirection: "column", gap: "2px", minWidth: 0, paddingLeft: vars.space["2"] });
export const itemLine = style({ display: "flex", alignItems: "center", flexWrap: "wrap", gap: vars.space["2"], minWidth: 0 });
export const kind = style({ fontSize: vars.fontSize.xs, color: vars.color.textSecondary, textTransform: "uppercase", letterSpacing: "0.04em" });
export const meta = style({ fontSize: vars.fontSize.xs, color: vars.color.textMuted, overflowWrap: "anywhere" });
export const value = style({ fontFamily: "monospace", fontSize: vars.fontSize.sm, overflowWrap: "anywhere", minWidth: 0 });

export const control = style({
  minHeight: MIN_TOUCH_TARGET,
  minWidth: MIN_TOUCH_TARGET,
  padding: `0 ${vars.space["2"]}`,
  display: "inline-flex",
  alignItems: "center",
  background: "none",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  color: vars.color.textPrimary,
  cursor: "pointer",
  fontFamily: "monospace",
  fontSize: vars.fontSize.sm,
  overflowWrap: "anywhere",
  textAlign: "left",
  textDecoration: "underline",
  selectors: { "&:focus-visible": { outline: `2px solid ${vars.color.inputFocusBorder}`, outlineOffset: "2px" } },
});

export const showAll = style([control, { fontFamily: "inherit", alignSelf: "flex-start" }]);
