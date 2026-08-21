import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const spin = keyframes({
  to: { transform: "rotate(360deg)" },
});

export const filterBar = style({
  display: "flex",
  flexDirection: "column",
  gap: "12px",
  marginBottom: "20px",

  "@media": {
    "(max-width: 768px)": {
      gap: "10px",
    },
  },
});

export const searchContainer = style({
  display: "flex",
  gap: "10px",
  alignItems: "center",

  "@media": {
    "(max-width: 768px)": {
      flexWrap: "wrap",
    },
  },
});

export const searchInput = style({
  flex: 1,
  padding: "10px 14px",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontSize: "14px",

  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
      boxShadow: "0 0 0 2px rgba(59, 130, 246, 0.2)",
    },
    "&::placeholder": {
      color: vars.color.textMuted,
    },
  },

  "@media": {
    "(max-width: 768px)": {
      minWidth: "200px",
    },
  },
});

export const searchButton = style({
  display: "flex",
  alignItems: "center",
  gap: "6px",
});

export const spinnerSmall = style({
  width: "14px",
  height: "14px",
  border: "2px solid rgba(255, 255, 255, 0.3)",
  borderTopColor: "white",
  borderRadius: "50%",
  animation: `${spin} 0.8s linear infinite`,
});

export const filters = style({
  display: "flex",
  gap: "10px",
  flexWrap: "wrap",
  alignItems: "center",

  "@media": {
    "(max-width: 768px)": {
      gap: "8px",
    },
  },
});

export const select = style({
  padding: "8px 12px",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontSize: "13px",
  cursor: "pointer",
  minWidth: "140px",

  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
    },
  },

  "@media": {
    "(max-width: 768px)": {
      minWidth: "120px",
      fontSize: "12px",
    },
  },
});

export const sortOrderButton = style({
  padding: "8px 12px",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontSize: "16px",
  cursor: "pointer",
  transition: "all 0.2s",

  selectors: {
    "&:hover": {
      background: vars.color.borderColor,
    },
  },
});

export const searchModeToggle = style({
  display: "flex",
  gap: 0,
  borderRadius: "8px",
  overflow: "hidden",
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  flexShrink: 0,
});

export const searchModeButton = style({
  padding: "8px 16px",
  fontSize: "13px",
  fontWeight: "500",
  color: vars.color.textSecondary,
  background: "transparent",
  border: "none",
  cursor: "pointer",
  transition: "all 0.15s ease",
  display: "flex",
  alignItems: "center",
  gap: "6px",

  selectors: {
    "&:not(:last-child)": {
      borderRight: `1px solid ${vars.color.borderColor}`,
    },
  },
});

export const searchModeButtonActive = style({
  background: vars.color.primary,
  color: "white",
});

export const fullTextSearchInput = style({
  flex: 1,
});
