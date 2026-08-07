import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const panel = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 12,
  padding: 20,
  display: "flex",
  flexDirection: "column",
  gap: 16,
});

export const header = style({
  display: "flex",
  flexDirection: "column",
  gap: 4,
});

export const title = style({
  margin: 0,
  fontSize: 20,
  fontWeight: 700,
  color: vars.color.textPrimary,
});

export const subtitle = style({
  margin: 0,
  fontSize: 13,
  color: vars.color.textSecondary,
});

export const fieldRow = style({
  display: "flex",
  flexDirection: "column",
  gap: 6,
});

export const fieldLabelRow = style({
  display: "flex",
  alignItems: "center",
  gap: 8,
});

export const fieldLabel = style({
  fontSize: 13,
  fontWeight: 500,
  color: vars.color.textPrimary,
});

export const configuredBadge = style({
  display: "inline-block",
  padding: "1px 7px",
  borderRadius: 4,
  fontSize: 10,
  fontWeight: 600,
  background: vars.color.successBg,
  color: vars.color.success,
});

export const notConfiguredBadge = style({
  display: "inline-block",
  padding: "1px 7px",
  borderRadius: 4,
  fontSize: 10,
  fontWeight: 600,
  background: vars.color.panelBgSecondary,
  color: vars.color.textMuted,
});

export const inputRow = style({
  display: "flex",
  gap: 8,
  flexWrap: "wrap",
});

export const input = style({
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: 6,
  padding: "7px 10px",
  fontSize: 13,
  color: vars.color.textPrimary,
  flex: "1 1 260px",
  minWidth: 0,
  selectors: {
    "&:focus": { outline: "none", borderColor: vars.color.primary },
  },
});

export const clearButton = style({
  background: vars.color.hoverBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 6,
  padding: "7px 12px",
  fontSize: 12,
  color: vars.color.textSecondary,
  cursor: "pointer",
  selectors: {
    "&:hover": { background: vars.color.errorBg, color: vars.color.error },
  },
});

export const hint = style({
  fontSize: 11,
  color: vars.color.textMuted,
});

export const saveRow = style({
  display: "flex",
  alignItems: "center",
  gap: 10,
});

export const saveButton = style({
  background: vars.color.primary,
  border: "none",
  borderRadius: 7,
  padding: "8px 18px",
  fontSize: 14,
  fontWeight: 600,
  color: vars.color.primaryText,
  cursor: "pointer",
  selectors: {
    "&:hover:not(:disabled)": { opacity: 0.85 },
    "&:disabled": { opacity: 0.5, cursor: "not-allowed" },
  },
});

export const statusMessage = style({
  fontSize: 12,
  color: vars.color.success,
});

export const errorBanner = style({
  padding: "8px 12px",
  background: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: 6,
  color: vars.color.errorText,
  fontSize: 13,
});
