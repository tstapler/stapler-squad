import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const warning = style({
  display: "flex",
  alignItems: "flex-start",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  background: vars.color.warningBg,
  color: vars.color.warningText,
  overflowWrap: "anywhere",
  minWidth: 0,
});

export const disclosureToggle = style({
  minHeight: "44px",
  padding: `0 ${vars.space["2"]}`,
  background: "transparent",
  border: "none",
  color: vars.color.textPrimary,
  font: "inherit",
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  textAlign: "left",
});

export const flagList = style({ listStyle: "none", margin: 0, padding: 0 });

export const flagItem = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: vars.space["2"],
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
});

export const flagName = style({ fontFamily: vars.font.mono, overflowWrap: "anywhere" });
