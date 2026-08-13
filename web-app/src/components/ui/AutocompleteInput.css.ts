import { style, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  position: "relative",
  width: "100%",
});

globalStyle(`${container} input`, {
  width: "100%",
  padding: "0.625rem 0.875rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  fontSize: "0.9375rem",
  transition: "border-color 0.2s, box-shadow 0.2s",
  background: vars.color.inputBackground,
  color: vars.color.textPrimary,
});

globalStyle(`${container} input:focus`, {
  outline: "none",
  borderColor: vars.color.primary,
  boxShadow: "0 0 0 3px rgba(0, 112, 243, 0.1)",
});

globalStyle(`${container} input.error`, {
  borderColor: vars.color.error,
});

globalStyle(`${container} input.error:focus`, {
  boxShadow: "0 0 0 3px rgba(239, 68, 68, 0.1)",
});

globalStyle(`${container} input:disabled`, {
  opacity: 0.6,
  cursor: "not-allowed",
  background: vars.color.surfaceMuted,
});

export const error = style({
  borderColor: `${vars.color.error} !important` as "inherit",
});

export const suggestions = style({
  position: "absolute",
  top: "calc(100% + 4px)",
  left: 0,
  right: 0,
  maxHeight: "240px",
  overflowY: "auto",
  background: vars.color.background,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  boxShadow: "0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06)",
  listStyle: "none",
  margin: 0,
  padding: "0.25rem",
  zIndex: 1000,
  selectors: {
    "&::-webkit-scrollbar": {
      width: "8px",
    },
    "&::-webkit-scrollbar-track": {
      background: vars.color.surfaceMuted,
      borderRadius: "4px",
    },
    "&::-webkit-scrollbar-thumb": {
      background: vars.color.borderStrong,
      borderRadius: "4px",
    },
    "&::-webkit-scrollbar-thumb:hover": {
      background: vars.color.textMuted,
    },
  },
});

export const suggestion = style({
  padding: "0.625rem 0.875rem",
  cursor: "pointer",
  borderRadius: "4px",
  transition: "background-color 0.15s",
  color: vars.color.textPrimary,
  fontSize: "0.9375rem",
  selectors: {
    "&:hover": {
      background: vars.color.surfaceMuted,
    },
  },
});

export const highlighted = style({
  background: vars.color.surfaceMuted,
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: vars.color.textPrimary,
    },
  },
});

export const loading = style({
  position: "absolute",
  top: "calc(100% + 4px)",
  left: 0,
  right: 0,
  padding: "0.75rem",
  background: vars.color.background,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  boxShadow: "0 4px 6px -1px rgba(0, 0, 0, 0.1)",
  color: vars.color.textSecondary,
  fontSize: "0.875rem",
  textAlign: "center",
  zIndex: 1000,
});
