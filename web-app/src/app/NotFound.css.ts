import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  margin: "auto",
  maxWidth: "600px",
  padding: "2rem 1rem",
  textAlign: "center",
});

export const title = style({
  fontSize: "1.5rem",
  fontWeight: 600,
  margin: "0 0 0.5rem 0",
  color: vars.color.textPrimary,
});

export const message = style({
  color: vars.color.textSecondary,
  margin: "0 0 1.5rem 0",
  overflowWrap: "anywhere",
});

export const pathText = style({
  fontFamily: "monospace",
});

export const actions = style({
  display: "flex",
  gap: "0.75rem",
  justifyContent: "center",
  flexWrap: "wrap",
});

const button = {
  minHeight: "44px",
  padding: "0.625rem 1.25rem",
  borderRadius: "6px",
  fontSize: "1rem",
  fontWeight: 500,
  cursor: "pointer",
  display: "inline-flex",
  alignItems: "center",
  textDecoration: "none",
} as const;

export const homeLink = style({
  ...button,
  background: vars.color.primary,
  color: "white",
  border: "1px solid transparent",
});

export const goBack = style({
  ...button,
  background: "transparent",
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderMuted}`,
});
