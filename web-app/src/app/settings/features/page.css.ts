import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  maxWidth: "900px",
  margin: "0 auto",
  padding: "2rem 1.5rem",
});

export const title = style({
  color: vars.color.textPrimary,
  fontSize: "1.5rem",
  fontWeight: 700,
  marginBottom: "0.5rem",
});

export const subtitle = style({
  color: vars.color.textSecondary,
  fontSize: "0.875rem",
  marginBottom: "2rem",
});

export const flagRow = style({
  display: "flex",
  alignItems: "flex-start",
  justifyContent: "space-between",
  gap: "1.5rem",
  padding: "1.25rem 0",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  selectors: {
    "&:last-child": { borderBottom: "none" },
  },
});

export const flagInfo = style({
  flex: 1,
});

export const flagName = style({
  color: vars.color.textPrimary,
  fontWeight: 600,
  fontSize: "0.9375rem",
  textTransform: "capitalize",
  marginBottom: "0.25rem",
});

export const flagDescription = style({
  color: vars.color.textSecondary,
  fontSize: "0.8125rem",
  lineHeight: 1.4,
});

export const toggle = style({
  flexShrink: 0,
  width: "2.75rem",
  height: "1.5rem",
  borderRadius: "9999px",
  border: "none",
  cursor: "pointer",
  transition: "background 0.15s ease",
  position: "relative",
});

export const toggleThumb = style({
  position: "absolute",
  top: "0.1875rem",
  width: "1.125rem",
  height: "1.125rem",
  borderRadius: "50%",
  background: "white",
  transition: "left 0.15s ease",
});

export const badge = style({
  display: "inline-block",
  padding: "0.125rem 0.5rem",
  borderRadius: "0.25rem",
  fontSize: "0.75rem",
  fontWeight: 600,
  marginLeft: "0.5rem",
  verticalAlign: "middle",
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
});

export const emptyMessage = style({
  color: vars.color.textSecondary,
  fontSize: "0.875rem",
  padding: "1.5rem 0",
});
