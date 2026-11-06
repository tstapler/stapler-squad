import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const radioGroup = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  margin: `${vars.space["4"]} 0`,
  border: "none",
  padding: 0,
});

export const radioLabel = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  minHeight: "44px",
  cursor: "pointer",
});

export const consent = style({
  display: "flex",
  alignItems: "flex-start",
  gap: vars.space["2"],
  padding: vars.space["3"],
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
});

export const errorBanner = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space["2"],
  marginTop: vars.space["4"],
  padding: vars.space["3"],
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
});

export const resultBox = style({
  marginTop: vars.space["4"],
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});
