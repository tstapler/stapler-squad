import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const hint = style({
  display: "flex",
  alignItems: "center",
  gap: "0.75rem",
  padding: "0.5rem 0",
  "@media": {
    "screen and (max-width: 768px)": {
      gap: "0.5rem",
    },
  },
});

export const keys = style({
  display: "flex",
  alignItems: "center",
  gap: "0.25rem",
});

export const key = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "2rem",
  padding: "0.25rem 0.5rem",
  fontFamily: '"SF Mono", "Monaco", "Inconsolata", "Roboto Mono", monospace',
  fontSize: "0.75rem",
  fontWeight: 500,
  lineHeight: 1,
  color: vars.color.textPrimary,
  background: vars.color.surfaceMuted,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "4px",
  boxShadow: `0 1px 0 0 ${vars.color.borderSubtle}`,
  "@media": {
    "screen and (max-width: 768px)": {
      minWidth: "1.5rem",
      padding: "0.25rem 0.375rem",
      fontSize: "0.625rem",
    },
    "(prefers-color-scheme: dark)": {
      background: vars.color.surfaceSubtle,
      borderColor: vars.color.borderSubtle,
      boxShadow: `0 1px 0 0 ${vars.color.borderSubtle}`,
    },
  },
});

export const separator = style({
  color: vars.color.textMuted,
  fontSize: "0.875rem",
  margin: "0 0.125rem",
});

export const description = style({
  color: vars.color.textSecondary,
  fontSize: "0.875rem",
  "@media": {
    "screen and (max-width: 768px)": {
      fontSize: "0.75rem",
    },
  },
});

export const hintsContainer = style({
  padding: "1rem",
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: vars.color.cardBackground,
      borderColor: vars.color.borderSubtle,
    },
  },
});

export const title = style({
  margin: "0 0 1rem 0",
  fontSize: "1rem",
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const hints = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.25rem",
});
