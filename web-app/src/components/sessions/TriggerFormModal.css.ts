import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// Layout/chrome (form/label/input/select/formGrid/formActions/save/cancel/error) is
// reused from ApprovalRulesPanel.css.ts — same cross-file reuse pattern ImportRulesModal
// already uses for ruleModalContent/modalHeader/modalBody/modalCloseButton. This file only
// holds styles genuinely new to the trigger-type-conditional form.

// Shared "sr-only" style for aria-live announcer spans — see a11y.css.ts.
export { visuallyHidden } from "@/styles/a11y.css";

// ── Inline-layout replacements (css-architecture.md: no `style={{ ... }}` for
// static layout) ────────────────────────────────────────────────────────────

export const checkboxLabel = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  fontSize: 13,
});

export const promptTextarea = style({
  resize: "vertical",
  fontFamily: vars.font.mono,
});

export const formActionsSpaced = style({
  marginTop: vars.space["4"],
});

export const secretBoxLegend = style({
  padding: 0,
});

export const typeSelector = style({
  display: "flex",
  gap: 8,
  flexWrap: "wrap",
});

export const typeOption = style({
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 8,
  padding: "8px 14px",
  fontSize: 13,
  fontWeight: 500,
  cursor: "pointer",
  background: "transparent",
  color: vars.color.textSecondary,
  transition: "all 0.15s ease",
  minHeight: 40,
  selectors: {
    "&:hover": { background: vars.color.hoverBackground },
  },
});

export const typeOptionActive = style({
  background: vars.color.primary,
  borderColor: vars.color.primary,
  color: vars.color.primaryText,
});

export const fieldset = style({
  border: `1px solid ${vars.color.borderSubtle}`,
  borderRadius: 8,
  padding: "12px 14px",
  margin: 0,
  display: "flex",
  flexDirection: "column",
  gap: 10,
});

export const legend = style({
  fontSize: 11,
  fontWeight: 700,
  letterSpacing: "0.06em",
  textTransform: "uppercase",
  color: vars.color.textMuted,
  padding: "0 4px",
});

export const fieldError = style({
  fontSize: 11,
  color: vars.color.errorText,
  marginTop: 2,
});

export const fieldHint = style({
  fontSize: 11,
  color: vars.color.textMuted,
  marginTop: 2,
  lineHeight: 1.4,
});

// ── Webhook / GitHub secret field ──────────────────────────────────────────

export const secretBox = style({
  display: "flex",
  flexDirection: "column",
  gap: 6,
});

export const secretRow = style({
  display: "flex",
  gap: 8,
  alignItems: "center",
  flexWrap: "wrap",
});

export const secretValue = style({
  fontFamily: vars.font.mono,
  fontSize: 12,
  padding: "6px 10px",
  background: vars.color.terminalBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 6,
  color: vars.color.textPrimary,
  wordBreak: "break-all",
  flex: "1 1 200px",
  minWidth: 0,
});

export const secretMasked = style({
  fontFamily: vars.font.mono,
  fontSize: 12,
  padding: "6px 10px",
  background: vars.color.panelBgSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 6,
  color: vars.color.textMuted,
  flex: "1 1 200px",
});

export const secretButton = style({
  background: vars.color.hoverBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 6,
  padding: "6px 12px",
  fontSize: 12,
  fontWeight: 500,
  color: vars.color.textPrimary,
  cursor: "pointer",
  minHeight: 32,
  transition: "all 0.15s ease",
  selectors: {
    "&:hover": { background: vars.color.accentHover },
  },
});

export const secretWarning = style({
  fontSize: 11,
  color: vars.color.warning,
  lineHeight: 1.4,
});

export const secretCopiedNotice = style({
  fontSize: 11,
  color: vars.color.success,
});

export const secretCopyErrorNotice = style({
  fontSize: 11,
  color: vars.color.errorText,
});
