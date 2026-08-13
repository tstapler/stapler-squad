import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// Informational (non-error) banner family — deliberately distinct from
// InlineError's assertive red/danger styling (see InlineError.css.ts's
// pillContainer/blockContainer): role="status" aria-live="polite" content
// needs a visually calmer treatment so it doesn't compete with real errors.
export const container = style({
  display: "flex",
  alignItems: "flex-start",
  gap: vars.space["2"],
  padding: vars.space["3"],
  border: `1px solid ${vars.color.primary}`,
  borderRadius: vars.radii.md,
  background: vars.color.accentBg,
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
});

export const icon = style({
  // accentText (not primary) — primary fails WCAG AA (4.09:1) against this
  // component's accentBg background in the light theme; accentText is tuned
  // per-theme to guarantee >=4.5:1 here. See theme.css.ts's accentText notes.
  color: vars.color.accentText,
  flexShrink: 0,
  lineHeight: vars.fontSize.lg,
});

export const body = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  flex: 1,
  minWidth: 0,
});

export const messageText = style({
  color: vars.color.textPrimary,
});

export const actions = style({
  display: "flex",
  gap: vars.space["2"],
  flexWrap: "wrap",
});

export const actionButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  // accentText, not primary — see `icon` above for why.
  color: vars.color.accentText,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  padding: 0,
  textDecoration: "underline",
  ":hover": {
    color: vars.color.primaryHover,
  },
});

export const actionButtonPrimary = style({
  background: vars.color.primary,
  border: "none",
  borderRadius: vars.radii.sm,
  cursor: "pointer",
  color: vars.color.primaryText,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  ":hover": {
    background: vars.color.primaryHover,
  },
});

export const dismissButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  padding: "2px",
  marginLeft: "auto",
  lineHeight: 1,
  minWidth: 24,
  minHeight: 24,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  flexShrink: 0,
});
