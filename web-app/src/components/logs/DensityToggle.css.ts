import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "4px",
  overflow: "hidden",
});

export const option = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "32px",
  height: "32px",
  padding: "0",
  background: "none",
  border: "none",
  borderRight: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textMuted,
  cursor: "pointer",
  fontSize: "0.9rem",
  transition: "all 0.15s",
  selectors: {
    "&:last-child": {
      borderRight: "none",
    },
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
      color: vars.color.textMuted,
    },
  },
});

export const active = style({
  backgroundColor: vars.color.primary,
  color: vars.color.primaryText,
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.primaryHover,
    },
  },
});
