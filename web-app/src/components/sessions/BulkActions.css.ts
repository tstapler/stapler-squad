import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const container = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "1rem 1.5rem",
  background: vars.color.cardBackground,
  borderRadius: 8,
  boxShadow: "0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)",
  marginBottom: "1rem",
  gap: "1rem",
  "@media": {
    "screen and (max-width: 768px)": {
      flexDirection: "column",
      alignItems: "stretch",
      padding: "1rem",
    },
  },
});

export const selection = style({
  display: "flex",
  alignItems: "center",
  gap: "1rem",
  "@media": {
    "screen and (max-width: 768px)": {
      justifyContent: "space-between",
    },
  },
});

export const count = style({
  fontWeight: 600,
  color: vars.color.textPrimary,
  fontSize: "0.9375rem",
});

export const selectAllButton = style({
  padding: "0.5rem 1rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 6,
  background: "transparent",
  color: vars.color.textSecondary,
  fontSize: "0.875rem",
  fontWeight: 500,
  cursor: "pointer",
  transition: "all 0.2s",
  selectors: {
    "&:hover": {
      background: vars.color.surfaceSubtle,
      borderColor: vars.color.borderStrong,
    },
  },
});

export const clearButton = style({
  padding: "0.5rem 1rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 6,
  background: "transparent",
  color: vars.color.textSecondary,
  fontSize: "0.875rem",
  fontWeight: 500,
  cursor: "pointer",
  transition: "all 0.2s",
  selectors: {
    "&:hover": {
      background: vars.color.surfaceSubtle,
      borderColor: vars.color.borderStrong,
    },
  },
});

export const actions = style({
  display: "flex",
  gap: "0.75rem",
  "@media": {
    "screen and (max-width: 768px)": {
      flexDirection: "column",
    },
  },
});

export const actionButton = style({
  padding: "0.625rem 1.25rem",
  borderRadius: 6,
  border: "none",
  fontWeight: 500,
  fontSize: "0.9375rem",
  cursor: "pointer",
  transition: "all 0.2s",
  background: vars.color.primary,
  color: "white",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.primaryDark,
      transform: "translateY(-1px)",
      boxShadow: "0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06)",
    },
    "&:active:not(:disabled)": {
      transform: "translateY(0)",
    },
    "&:disabled": {
      opacity: 0.4,
      cursor: "not-allowed",
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      width: "100%",
    },
  },
});

export const danger = style({
  background: vars.color.error,
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.errorDark,
    },
  },
});

export const feedback = style({
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.sm,
  marginBottom: "0.5rem",
});
