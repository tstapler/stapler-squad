import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const button = style({
  minWidth: "44px",
  minHeight: "44px",
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  padding: 0,
  background: "transparent",
  border: "none",
  color: vars.color.textSecondary,
  cursor: "pointer",
  selectors: { '&[aria-expanded="true"]': { color: vars.color.textPrimary } },
});

export const description = style({
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.sm,
  overflowWrap: "anywhere",
  flexBasis: "100%",
});
