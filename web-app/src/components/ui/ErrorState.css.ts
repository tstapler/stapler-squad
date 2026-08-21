import { style, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  minHeight: "400px",
  padding: "2rem",
  "@media": {
    "screen and (max-width: 768px)": {
      minHeight: "300px",
      padding: "1rem",
    },
  },
});

export const content = style({
  maxWidth: "600px",
  width: "100%",
  textAlign: "center",
});

export const icon = style({
  color: vars.color.error,
  marginBottom: "1.5rem",
  display: "flex",
  justifyContent: "center",
});

export const title = style({
  fontSize: "1.5rem",
  fontWeight: 600,
  margin: "0 0 0.5rem 0",
  color: vars.color.textPrimary,
  "@media": {
    "screen and (max-width: 768px)": {
      fontSize: "1.25rem",
    },
  },
});

export const message = style({
  color: vars.color.textSecondary,
  margin: "0 0 2rem 0",
  fontSize: "1rem",
  lineHeight: 1.5,
  "@media": {
    "screen and (max-width: 768px)": {
      fontSize: "0.875rem",
    },
  },
});

export const retryButton = style({
  padding: "0.75rem 1.5rem",
  background: vars.color.primary,
  color: "white",
  border: "none",
  borderRadius: "6px",
  fontSize: "1rem",
  fontWeight: 500,
  cursor: "pointer",
  transition: "background 0.2s, transform 0.1s",
  selectors: {
    "&:hover": {
      background: vars.color.primaryDark,
    },
    "&:active": {
      transform: "scale(0.98)",
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0.625rem 1.25rem",
      fontSize: "0.875rem",
    },
  },
});

export const details = style({
  margin: "2rem 0",
  textAlign: "left",
  background: vars.color.surfaceSubtle,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  overflow: "hidden",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: vars.color.surfaceSubtle,
      borderColor: vars.color.borderSubtle,
    },
  },
});

export const detailsSummary = style({
  padding: "1rem",
  cursor: "pointer",
  fontWeight: 500,
  userSelect: "none",
  transition: "background 0.2s",
  selectors: {
    "&:hover": {
      background: vars.color.surfaceMuted,
    },
  },
  "@media": {
    "(prefers-color-scheme: dark)": {
      selectors: {
        "&:hover": {
          background: vars.color.borderSubtle,
        },
      },
    },
  },
});

export const detailsContent = style({
  padding: "1rem",
  borderTop: `1px solid ${vars.color.borderColor}`,
  "@media": {
    "(prefers-color-scheme: dark)": {
      borderTopColor: vars.color.borderSubtle,
    },
  },
});

export const errorBlock = style({
  marginBottom: "1rem",
  selectors: {
    "&:last-child": {
      marginBottom: 0,
    },
  },
});

globalStyle(`${errorBlock} strong`, {
  display: "block",
  marginBottom: "0.5rem",
  color: vars.color.textPrimary,
});

export const errorText = style({
  background: vars.color.terminalBackground,
  color: vars.color.terminalForeground,
  padding: "1rem",
  borderRadius: "4px",
  overflowX: "auto",
  fontSize: "0.875rem",
  lineHeight: 1.5,
  margin: 0,
  fontFamily: '"Menlo", "Monaco", "Courier New", monospace',
});

export const stackTrace = style({
  background: vars.color.terminalBackground,
  color: vars.color.terminalForeground,
  padding: "1rem",
  borderRadius: "4px",
  overflowX: "auto",
  fontSize: "0.875rem",
  lineHeight: 1.5,
  margin: 0,
  fontFamily: '"Menlo", "Monaco", "Courier New", monospace',
  maxHeight: "300px",
  overflowY: "auto",
});
