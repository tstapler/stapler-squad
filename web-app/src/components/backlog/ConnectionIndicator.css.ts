import { style, styleVariants, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const wrapper = style({
  display: "inline-flex",
  alignItems: "center",
  gap: 6,
  fontSize: "0.75rem",
  fontWeight: 600,
  userSelect: "none",
  whiteSpace: "nowrap",
});

const dotBase = style({
  width: 8,
  height: 8,
  borderRadius: "50%",
  flexShrink: 0,
  display: "inline-block",
});

const spin = keyframes({
  from: { transform: "rotate(0deg)" },
  to: { transform: "rotate(360deg)" },
});

// Shared by "connecting" and "reconnecting" — an actively-retrying ring.
const spinnerBase = style({
  width: 8,
  height: 8,
  borderRadius: "50%",
  flexShrink: 0,
  display: "inline-block",
  border: `2px solid ${vars.color.warning}`,
  borderTopColor: "transparent",
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      animationName: spin,
      animationDuration: "0.8s",
      animationTimingFunction: "linear",
      animationIterationCount: "infinite",
    },
    // ux.md §5 "prefers-reduced-motion" requirement: the pulsing/spinning
    // ring must have a static fallback that still communicates state via
    // color/shape alone, not via omitting the information.
    "(prefers-reduced-motion: reduce)": {
      borderTopColor: vars.color.warning,
    },
  },
});

export const dots = styleVariants({
  live: [dotBase, { background: vars.color.success }],
  connecting: [spinnerBase],
  reconnecting: [spinnerBase],
  // Deliberately distinct from "reconnecting": a static (non-spinning) dot
  // with a visible ring. The idle-staleness backstop (Story 4.2.3, plan.md
  // pre-mortem P1 #1) is a self-healing timer waiting to fire, not an
  // active in-flight retry — folding it into the same spinner would hide
  // exactly the condition that pre-mortem fix exists to surface.
  stale: [dotBase, { background: vars.color.warning, border: `1px solid ${vars.color.warningText}` }],
  polling: [dotBase, { background: vars.color.textMuted }],
});

// Always visible (unlike components/layout/ConnectionIndicator.css.ts's
// label, which hides under 640px) — ux.md §5 cross-cutting UX AC #5
// requires the connection indicator to carry a text label a sighted mobile
// user gets identically to desktop, not color/dot alone.
export const label = style({
  color: vars.color.textSecondary,
});
