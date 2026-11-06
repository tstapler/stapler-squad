import { style } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

const focusRing = {
  outline: `2px solid ${vars.color.primary}`,
  outlineOffset: "2px",
} as const;

// Static by design (nothing moves), so prefers-reduced-motion needs no override.
export const panel = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[3],
  padding: vars.space[3],
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  selectors: {
    '&[data-overlay="true"]': {
      position: "absolute",
      left: 0,
      right: 0,
      zIndex: zIndex.terminalPaneChrome,
      overflowY: "auto",
    },
  },
});

export const fieldset = style({ border: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: vars.space[2] });
export const legend = style({ fontWeight: vars.fontWeight.medium, padding: 0, marginBottom: vars.space[2] });

export const option = style({
  display: "flex",
  alignItems: "flex-start",
  gap: vars.space[3],
  minHeight: "44px",
  minWidth: "44px",
  padding: vars.space[2],
  cursor: "pointer",
});

export const optionInput = style({
  flexShrink: 0,
  width: "24px",
  height: "24px",
  margin: "10px 0 0 0",
  selectors: { "&:focus-visible": focusRing },
});

export const optionText = style({ display: "flex", flexDirection: "column", gap: vars.space[1] });
export const optionLabel = style({ fontWeight: vars.fontWeight.medium });
export const description = style({ color: vars.color.textSecondary });

export const switchRow = style({
  display: "flex",
  alignItems: "flex-start",
  gap: vars.space[3],
  minHeight: "44px",
  padding: vars.space[2],
  cursor: "pointer",
});

export const note = style({ margin: 0, color: vars.color.textSecondary });

export const button = style({
  minHeight: "44px",
  minWidth: "44px",
  padding: `${vars.space[1]} ${vars.space[3]}`,
  background: "transparent",
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderStrong}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  selectors: { "&:focus-visible": focusRing },
});

export const chip = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minHeight: "44px",
  minWidth: "44px",
  padding: `${vars.space[1]} ${vars.space[3]}`,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderStrong}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  whiteSpace: "nowrap",
  flexShrink: 0,
  selectors: {
    "&:focus-visible": focusRing,
    // Misroute cue: heavier border plus a leading "!" in the text, never colour alone.
    '&[data-highlighted="true"]': { borderWidth: "3px", fontWeight: vars.fontWeight.medium },
  },
});

export const hint = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[3],
  padding: vars.space[2],
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderStrong}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
});
