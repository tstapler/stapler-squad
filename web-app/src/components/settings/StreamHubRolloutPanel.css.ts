import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const panel = style({
  marginTop: "2rem",
  paddingTop: "2rem",
  borderTop: `1px solid ${vars.color.borderColor}`,
});

export const heading = style({
  color: vars.color.textPrimary,
  fontSize: "1.125rem",
  fontWeight: 700,
  marginBottom: "0.5rem",
});

export const subheading = style({
  color: vars.color.textPrimary,
  fontSize: "0.9375rem",
  fontWeight: 600,
  margin: "1.25rem 0 0.5rem",
});

export const description = style({
  color: vars.color.textSecondary,
  fontSize: "0.8125rem",
  lineHeight: 1.4,
  marginBottom: "1rem",
});

export const hint = style({
  color: vars.color.textMuted,
  fontSize: "0.75rem",
  lineHeight: 1.4,
  marginTop: "0.25rem",
  marginBottom: "0.5rem",
});

export const statusRow = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: "1rem",
  padding: "0.625rem 0",
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const statusLabel = style({
  color: vars.color.textPrimary,
  fontWeight: 600,
  fontSize: "0.875rem",
});

export const badge = style({
  display: "inline-block",
  padding: "0.125rem 0.5rem",
  borderRadius: "0.25rem",
  fontSize: "0.75rem",
  fontWeight: 600,
});

export const badgeEnabled = style({
  background: vars.color.success,
  color: "white",
});

export const badgeDisabled = style({
  background: vars.color.borderColor,
  color: vars.color.textSecondary,
});

export const errorMessage = style({
  color: vars.color.errorText,
  background: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: vars.radii.md,
  padding: "0.75rem 1rem",
  fontSize: "0.875rem",
  marginBottom: "1rem",
});

export const overrideList = style({
  listStyle: "none",
  margin: 0,
  padding: 0,
});

export const overrideRow = style({
  display: "flex",
  alignItems: "center",
  gap: "0.75rem",
  padding: "0.5rem 0",
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const addRow = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  marginTop: "0.75rem",
});

export const input = style({
  flex: 1,
  padding: "0.5rem 0.75rem",
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
});

export const actionButton = style({
  padding: "0.5rem 0.875rem",
  borderRadius: vars.radii.md,
  border: "none",
  background: vars.color.primary,
  color: vars.color.primaryText,
  fontSize: "0.8125rem",
  fontWeight: 600,
  cursor: "pointer",
  whiteSpace: "nowrap",
  selectors: {
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const removeButton = style({
  marginLeft: "auto",
  padding: "0.25rem 0.625rem",
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  color: vars.color.textSecondary,
  fontSize: "0.75rem",
  cursor: "pointer",
  selectors: {
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});
