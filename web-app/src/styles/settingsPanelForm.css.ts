import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

/**
 * Shared "settings panel with a form" chrome — the container/heading/
 * description/form/field/label/input shell common to a settings page that
 * loads config, shows a form, and saves it (e.g. JulesSettings,
 * SlackNotificationSettings). Extracted to keep new settings panels from
 * re-declaring this exact block (jscpd's duplication gate, web-app/.jscpd.json).
 */
export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
  maxWidth: "640px",
});

export const heading = style({
  color: vars.color.textPrimary,
  fontSize: "1.25rem",
  fontWeight: 600,
  margin: 0,
});

export const description = style({
  color: vars.color.textMuted,
  fontSize: "0.875rem",
  margin: 0,
});

export const loadingText = style({
  color: vars.color.textMuted,
});

export const form = style({
  display: "flex",
  flexDirection: "column",
  gap: "1.25rem",
});

export const field = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.5rem",
});

export const label = style({
  color: vars.color.textSecondary,
  fontSize: "0.875rem",
  fontWeight: 600,
});

export const inputRow = style({
  display: "flex",
  gap: "0.5rem",
  alignItems: "center",
});

export const input = style({
  padding: "0.5rem 0.75rem",
  backgroundColor: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: "4px",
  color: vars.color.inputText,
  fontSize: "0.875rem",
  flex: 1,
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.inputFocusBorder,
    },
  },
});
