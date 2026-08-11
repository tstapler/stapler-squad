import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const nav = style({
  background: vars.color.background,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  position: "sticky",
  top: 0,
  zIndex: 50,
});

export const container = style({
  maxWidth: "1280px",
  margin: "0 auto",
  padding: "0 1rem",
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  height: "64px",
});

export const brand = style({
  display: "flex",
  alignItems: "center",
});

export const navTitle = style({
  fontSize: "1.25rem",
  fontWeight: 700,
  margin: 0,
  color: vars.color.textPrimary,
});

export const menu = style({
  display: "flex",
  gap: "2rem",
  listStyle: "none",
  margin: 0,
  padding: 0,
  "@media": {
    "screen and (max-width: 768px)": {
      gap: "1rem",
    },
  },
});

export const link = style({
  textDecoration: "none",
  color: vars.color.textPrimary,
  fontWeight: 500,
  padding: "0.5rem 0",
  borderBottom: "2px solid transparent",
  transition: "border-color 0.2s",
  selectors: {
    "&:hover": {
      borderBottomColor: vars.color.textPrimary,
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      fontSize: "0.875rem",
    },
  },
});

export const active = style({
  borderBottomColor: vars.color.primary,
  color: vars.color.primary,
});

export const actions = style({
  display: "flex",
  gap: "1rem",
});

export const createButton = style({
  padding: "0.5rem 1rem",
  background: vars.color.primary,
  color: "white",
  textDecoration: "none",
  borderRadius: "6px",
  fontWeight: 500,
  transition: "background 0.2s",
  selectors: {
    "&:hover": {
      background: vars.color.primaryDark,
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0.375rem 0.75rem",
      fontSize: "0.875rem",
    },
  },
});
