import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const panel = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 12,
  padding: 20,
  display: "flex",
  flexDirection: "column",
  gap: 14,
});

export const header = style({
  display: "flex",
  flexDirection: "column",
  gap: 4,
});

export const title = style({
  margin: 0,
  fontSize: 18,
  fontWeight: 700,
  color: vars.color.textPrimary,
});

export const subtitle = style({
  margin: 0,
  fontSize: 13,
  color: vars.color.textSecondary,
});

export const checkboxRow = style({
  display: "flex",
  alignItems: "flex-start",
  gap: 10,
  fontSize: 13,
  color: vars.color.textPrimary,
});

export const checkboxLabel = style({
  color: vars.color.textSecondary,
});

export const message = style({
  margin: 0,
  fontSize: 13,
  color: vars.color.textSecondary,
});

export const footer = style({
  display: "flex",
  justifyContent: "flex-end",
});

export const installButton = style({
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: 8,
  padding: "6px 14px",
  fontSize: 13,
  fontWeight: 500,
  cursor: "pointer",
  transition: "background 0.15s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.primaryHover,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});
