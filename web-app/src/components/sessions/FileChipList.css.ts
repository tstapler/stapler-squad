import { style } from "@vanilla-extract/css";
import { vars } from "../../styles/theme-contract.css";

export const chipList = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space["2"],
  marginTop: vars.space["2"],
  maxHeight: "120px",
  overflowY: "auto",
});

export const chip = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.md,
  background: vars.color.surfaceSubtle,
  border: `1px solid ${vars.color.borderColor}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  maxWidth: "200px",
  overflow: "hidden",
});

export const chipName = style({
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  minWidth: 0,
});

export const chipThumbnail = style({
  flexShrink: 0,
  width: "20px",
  height: "20px",
  objectFit: "cover",
  borderRadius: "2px",
});

export const chipIconWrapper = style({
  flexShrink: 0,
  display: "flex",
  alignItems: "center",
});

export const chipRemove = style({
  flexShrink: 0,
  marginLeft: vars.space["1"],
  background: "none",
  border: "none",
  cursor: "pointer",
  padding: "0",
  lineHeight: "1",
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.base,
  selectors: {
    "&:hover": {
      color: vars.color.error,
    },
  },
});
