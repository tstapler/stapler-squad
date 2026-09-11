import { style, styleVariants } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

const severityColors = {
  // No dedicated "info" tier exists in theme-contract.css.ts (only
  // success/warning/error/critical) -- map to the neutral panel/border/text
  // tokens instead of inventing a new theme-wide token for one banner variant.
  info: { bg: vars.color.panelBgSecondary, border: vars.color.borderColor, text: vars.color.textPrimary },
  warning: { bg: vars.color.warningBg, border: vars.color.warning, text: vars.color.warningText },
  error: { bg: vars.color.errorBg, border: vars.color.error, text: vars.color.errorText },
};

export const banner = styleVariants(severityColors, (c) => ({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: vars.space["3"],
  background: c.bg,
  border: `1px solid ${c.border}`,
  borderRadius: vars.radii.md,
  margin: `0 0 ${vars.space["3"]} 0`,
}));

export const row = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space["3"],
  flexWrap: "wrap",
});

export const text = styleVariants(severityColors, (c) => ({
  color: c.text,
  fontSize: vars.fontSize.sm,
  flex: 1,
  minWidth: "240px",
}));

export const icon = style({
  marginRight: vars.space["1"],
});

export const rowActions = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  flexShrink: 0,
});

export const actionButton = styleVariants(severityColors, (c) => ({
  backgroundColor: c.border,
  color: c.text,
  border: "none",
  borderRadius: vars.radii.sm,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  whiteSpace: "nowrap",
  ":hover": {
    opacity: 0.9,
  },
  ":disabled": {
    opacity: 0.6,
    cursor: "not-allowed",
  },
}));

export const dangerButton = style({
  backgroundColor: vars.color.error,
  color: vars.color.errorText,
  border: "none",
  borderRadius: vars.radii.sm,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  whiteSpace: "nowrap",
  ":hover": {
    opacity: 0.9,
  },
  ":disabled": {
    opacity: 0.6,
    cursor: "not-allowed",
  },
});

export const dismissButton = styleVariants(severityColors, (c) => ({
  background: "none",
  border: "none",
  color: c.text,
  cursor: "pointer",
  fontSize: vars.fontSize.sm,
  padding: `0 ${vars.space["1"]}`,
  opacity: 0.8,
  ":hover": {
    opacity: 1,
  },
}));
