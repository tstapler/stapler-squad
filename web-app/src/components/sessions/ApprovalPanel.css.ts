import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const panel = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 8,
  padding: 12,
  display: "flex",
  flexDirection: "column",
  gap: 12,
});

export const header = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
});

export const title = style({
  margin: 0,
  fontSize: 16,
  fontWeight: 700,
  color: vars.color.textPrimary,
  display: "flex",
  alignItems: "center",
  gap: 8,
});

export const countBadge = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: 22,
  height: 22,
  padding: "0 6px",
  borderRadius: 11,
  background: vars.color.warning,
  color: vars.color.primaryText,
  fontSize: 12,
  fontWeight: 700,
});

export const refreshButton = style({
  background: vars.color.hoverBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 6,
  padding: "4px 8px",
  fontSize: 16,
  cursor: "pointer",
  transition: "all 0.2s ease",
  color: vars.color.textSecondary,
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.accentHover,
      transform: "rotate(180deg)",
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const list = style({
  display: "flex",
  flexDirection: "column",
  gap: 8,
  maxHeight: 400,
  overflowY: "auto",
  selectors: {
    "&::-webkit-scrollbar": {
      width: 6,
    },
    "&::-webkit-scrollbar-track": {
      background: vars.color.cardBackground,
    },
    "&::-webkit-scrollbar-thumb": {
      background: vars.color.borderColor,
      borderRadius: 3,
    },
    "&::-webkit-scrollbar-thumb:hover": {
      background: vars.color.textSecondary,
    },
  },
});

export const empty = style({
  padding: 16,
  textAlign: "center",
  color: vars.color.textSecondary,
  fontSize: 14,
});

export const error = style({
  padding: 12,
  textAlign: "center",
  color: vars.color.errorText,
  fontSize: 13,
});

export const retryButton = style({
  marginTop: 8,
  padding: "4px 12px",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 4,
  background: vars.color.hoverBackground,
  color: vars.color.textPrimary,
  fontSize: 12,
  cursor: "pointer",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": {
      background: vars.color.accentHover,
    },
  },
});
