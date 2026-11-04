import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const chip = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  borderRadius: vars.radii.sm,
  padding: `0 ${vars.space["1"]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  height: "20px",
  maxWidth: "100%",
  minWidth: 0,
  background: vars.color.errorBg,
  color: vars.color.errorText,
  border: `1px solid ${vars.color.error}`,
  ":focus-visible": {
    outline: `2px solid ${vars.color.error}`,
    outlineOffset: "1px",
  },
});

export const label = style({
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  maxWidth: "20ch",
});

export const more = style({
  flexShrink: 0,
  opacity: 0.8,
});

export const interactive = style({ cursor: "pointer" });
