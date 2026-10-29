import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const page = style({ padding: vars.space["4"], maxWidth: "48rem", margin: "0 auto" });
export const title = style({ fontSize: vars.fontSize.xl, color: vars.color.textPrimary, margin: `0 0 ${vars.space["4"]}` });
export const count = style({ fontSize: vars.fontSize.sm, color: vars.color.textSecondary });
export const muted = style({ color: vars.color.textMuted });
export const error = style({ color: vars.color.errorText });
export const list = style({ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: vars.space["3"] });
export const card = style({
  border: `1px solid ${vars.color.borderColor}`,
  borderLeft: `3px solid ${vars.color.warningText}`,
  borderRadius: vars.radii.lg,
  background: vars.color.cardBackground,
  padding: vars.space["4"],
});
export const question = style({ margin: `0 0 ${vars.space["2"]}`, color: vars.color.textPrimary });
export const itemLink = style({ fontSize: vars.fontSize.sm });
export const actions = style({ display: "flex", flexWrap: "wrap", gap: vars.space["2"], marginTop: vars.space["3"] });
export const actionButton = style({
  minHeight: "44px",
  padding: `0 ${vars.space["4"]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  color: vars.color.textPrimary,
  cursor: "pointer",
});
