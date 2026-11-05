import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const row = style({
  display: "flex",
  flexDirection: "column",
  gap: 8,
  padding: "8px 16px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const actionButton = style({
  display: "flex",
  alignItems: "center",
  gap: 8,
  width: "100%",
  minHeight: 44,
  padding: "10px 12px",
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 6,
  fontSize: 14,
  textAlign: "left",
  cursor: "pointer",
  selectors: {
    "&:hover:not(:disabled)": { background: vars.color.hoverBackground },
    "&:focus-visible": { outline: `3px solid ${vars.color.primary}`, outlineOffset: 2 },
    "&:disabled": { cursor: "progress", opacity: 0.7 },
  },
});

export const errorLine = style({
  display: "flex",
  alignItems: "center",
  flexWrap: "wrap",
  gap: 8,
  color: vars.color.error,
  fontSize: 13,
});

export const retryButton = style({
  minHeight: 44,
  minWidth: 44,
  padding: "8px 14px",
  background: "transparent",
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 6,
  cursor: "pointer",
  selectors: {
    "&:focus-visible": { outline: `3px solid ${vars.color.primary}`, outlineOffset: 2 },
  },
});
