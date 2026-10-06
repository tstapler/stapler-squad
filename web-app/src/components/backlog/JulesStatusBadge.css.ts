import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// Shared badge shell -- mirrors RemoteConnectionIndicator.css.ts's `badge`
// (padding/radius/font sizing), reused across all six phase variants below.
export const badge = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["1"]} 10px`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
});

// Token-only phase variants (Task 3.3.1b) -- every color comes from
// vars.color.*, never a literal hex/rgb value, so both light and dark themes
// (and the other theme.css.ts palettes) render correctly automatically.
export const phaseQueued = style({
  background: vars.color.accentBg,
  color: vars.color.accentText,
});

export const phaseRunning = style({
  background: vars.color.accentBg,
  color: vars.color.accentText,
  border: `1px solid ${vars.color.primary}`,
});

export const phaseNeedsReview = style({
  background: vars.color.warningBg,
  color: vars.color.warningText,
});

export const phaseDone = style({
  background: vars.color.successBg,
  color: vars.color.successText,
});

export const phaseFailed = style({
  background: vars.color.errorBg,
  color: vars.color.errorText,
  border: `1px solid ${vars.color.error}`,
});

// Reconnect-required is deliberately amber (warningBg/warningText/warning
// border), distinct from phaseFailed's red -- validation.md's "icon variant
// distinct from both the Running icon and the red Failed icon (amber)".
export const phaseReconnectRequired = style({
  background: vars.color.warningBg,
  color: vars.color.warningText,
  border: `1px solid ${vars.color.warning}`,
});

const spin = keyframes({
  from: { transform: "rotate(0deg)" },
  to: { transform: "rotate(360deg)" },
});

export const icon = style({
  display: "inline-block",
  lineHeight: 1,
  selectors: {
    [`${phaseRunning} &`]: {
      animation: `${spin} 1.2s linear infinite`,
    },
  },
});

export const secondaryText = style({
  marginLeft: vars.space["2"],
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const updateKeyLink = style({
  marginLeft: vars.space["2"],
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.primary,
  textDecoration: "underline",
});

export const webLink = style({
  display: "block",
  marginTop: vars.space["1"],
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  textDecoration: "none",
  ":hover": {
    textDecoration: "underline",
  },
});

// Visually-hidden persistent aria-live region -- byte-identical shape to
// RemoteConnectionIndicator.css.ts's ariaLiveRegion, kept as its own copy
// per that file's precedent (independent components, no shared import).
export const ariaLiveRegion = style({
  position: "absolute",
  width: 1,
  height: 1,
  padding: 0,
  margin: -1,
  overflow: "hidden",
  clip: "rect(0, 0, 0, 0)",
  whiteSpace: "nowrap",
  borderWidth: 0,
});
