import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// Neutral chrome — this is informational, never an error/warning state
// (design/ux.md Surface 1: "Neutral color (not red/amber)").
export const badge = style({
  position: "relative",
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space[1],
  padding: `${vars.space[1]} ${vars.space[2]}`,
  background: vars.color.surfaceMuted,
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "default",
  lineHeight: 1,
});

export const icon = style({
  fontSize: vars.fontSize.sm,
});

// Task 1.4.3c — brief outline-flash pairing the MULTIPLE_VIEWERS blocked
// toast with this badge (design/ux.md Surface 3's ambient/reactive signal
// pairing). aria-hidden in the component -- purely visual reinforcement,
// the toast text already carries the full explanation.
const pulseOutline = keyframes({
  "0%, 100%": { boxShadow: "0 0 0 0 rgba(0,0,0,0)" },
  "50%": { boxShadow: `0 0 0 3px ${vars.color.warning}` },
});

export const pulse = style({
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      animation: `${pulseOutline} 600ms ease-in-out`,
    },
    // ux.md §5 "prefers-reduced-motion" requirement: the pulsing/spinning
    // ring must have a static fallback that still communicates state via
    // color/shape alone, not via omitting the information (matches
    // backlog/ConnectionIndicator.css.ts's spinnerBase pattern).
    "(prefers-reduced-motion: reduce)": {
      boxShadow: `0 0 0 3px ${vars.color.warning}`,
    },
  },
});

// Revealed on hover/focus (Story 4.2.2 Task 4.2.2b) — never a second
// aria-live region, just plain expanded text (design/ux.md Step 3).
export const tooltip = style({
  position: "absolute",
  top: "100%",
  right: 0,
  marginTop: vars.space[1],
  padding: vars.space[2],
  background: vars.color.surfaceMuted,
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.normal,
  width: "220px",
  whiteSpace: "normal",
  textAlign: "left",
  zIndex: 20,
  boxShadow: "0 2px 8px rgba(0,0,0,0.2)",
});
