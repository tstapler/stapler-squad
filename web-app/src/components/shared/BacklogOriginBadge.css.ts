import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const badge = style({
  display: "inline-flex",
  alignItems: "center",
  gap: 4,
  padding: "4px 10px",
  borderRadius: 12,
  fontSize: 12,
  fontWeight: 600,
  textDecoration: "none",
  cursor: "pointer",
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.surfaceSubtle,
  color: vars.color.textPrimary,
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderHover,
    },
    "&:focus": {
      outline: `2px solid ${vars.color.primary}`,
      outlineOffset: 2,
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      fontSize: 11,
      padding: "3px 8px",
    },
  },
});

export const compact = style({
  padding: "4px 8px",
  fontSize: 11,
});

export const icon = style({
  width: 14,
  height: 14,
  flexShrink: 0,
  "@media": {
    "screen and (max-width: 768px)": {
      width: 12,
      height: 12,
    },
  },
});

export const text = style({
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
  maxWidth: 120,
  textTransform: "capitalize",
});
