import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  minHeight: "var(--viewport-height, 100dvh)",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  background: vars.color.background,
  padding: "1rem",
});

export const card = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "12px",
  padding: "2.5rem 2rem",
  width: "100%",
  maxWidth: "400px",
  textAlign: "center",
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  gap: "0.75rem",
});

export const logo = style({
  fontSize: "3rem",
  lineHeight: "1",
  marginBottom: "0.25rem",
});

export const title = style({
  fontSize: "1.5rem",
  fontWeight: 700,
  color: vars.color.textPrimary,
  margin: 0,
});

export const subtitle = style({
  fontSize: "1rem",
  color: vars.color.textSecondary,
  margin: 0,
});

export const hint = style({
  fontSize: "0.85rem",
  color: vars.color.textMuted,
  lineHeight: "1.5",
  margin: 0,
});

export const button = style({
  marginTop: "0.5rem",
  width: "100%",
  padding: "0.75rem 1.5rem",
  background: vars.color.primary,
  color: "#fff",
  border: "none",
  borderRadius: "8px",
  fontSize: "1rem",
  fontWeight: 600,
  cursor: "pointer",
  transition: "opacity 0.15s",
  selectors: {
    "&:hover:not(:disabled)": {
      opacity: 0.85,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const caDivider = style({
  marginTop: "1.25rem",
  borderTop: `1px solid ${vars.color.borderColor}`,
  paddingTop: "1.25rem",
  width: "100%",
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  gap: "0.5rem",
});

export const secondaryButton = style({
  display: "inline-flex",
  alignItems: "center",
  gap: "0.4rem",
  padding: "0.5rem 1rem",
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  fontSize: "0.85rem",
  fontWeight: 500,
  cursor: "pointer",
  textDecoration: "none",
  transition: "border-color 0.15s, color 0.15s",
  selectors: {
    "&:hover": {
      borderColor: vars.color.textMuted,
      color: vars.color.textPrimary,
    },
  },
});

export const error = style({
  color: vars.color.error,
  fontSize: "0.85rem",
  background: "rgba(248, 81, 73, 0.1)",
  border: "1px solid rgba(248, 81, 73, 0.3)",
  borderRadius: "6px",
  padding: "0.5rem 0.75rem",
  width: "100%",
  boxSizing: "border-box",
  textAlign: "left",
  margin: 0,
});
