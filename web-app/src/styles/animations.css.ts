import { keyframes, style } from "@vanilla-extract/css";
import { vars } from "./theme.css";

/**
 * Story 4.2 — Shared animation keyframes and reusable animation utility classes.
 *
 * All animations are wrapped in prefers-reduced-motion so the system respects
 * the user's accessibility preference (WCAG 2.3.3).
 */

// ── Keyframes ────────────────────────────────────────────────────────────────

export const pulseGlowKeyframes = keyframes({
  "0%, 100%": {
    boxShadow: `0 0 0 0 transparent`,
  },
  "50%": {
    boxShadow: `0 0 8px 2px ${vars.color.glowPrimary}, 0 0 16px 4px ${vars.color.glowSecondary}`,
  },
});

export const glowTextKeyframes = keyframes({
  "0%, 100%": {
    textShadow: `0 0 4px ${vars.color.glowPrimary}`,
  },
  "50%": {
    textShadow: `0 0 12px ${vars.color.glowPrimary}, 0 0 24px ${vars.color.glowSecondary}`,
  },
});

export const slideInFromRight = keyframes({
  from: { transform: "translateX(24px)", opacity: 0 },
  to: { transform: "translateX(0)", opacity: 1 },
});

export const slideInFromBottom = keyframes({
  from: { transform: "translateY(16px)", opacity: 0 },
  to: { transform: "translateY(0)", opacity: 1 },
});

export const fadeIn = keyframes({
  from: { opacity: 0 },
  to: { opacity: 1 },
});

export const terminalBlink = keyframes({
  "0%, 49%": { opacity: 1 },
  "50%, 100%": { opacity: 0 },
});

// ── Reusable utility styles ──────────────────────────────────────────────────

/** Applies the theme-aware glow pulse to any element. */
export const pulseGlow = style({
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      animationName: pulseGlowKeyframes,
      animationDuration: "2.5s",
      animationTimingFunction: "ease-in-out",
      animationIterationCount: "infinite",
    },
  },
});

/** Applies glow text shadow pulse. */
export const glowText = style({
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      animationName: glowTextKeyframes,
      animationDuration: "3s",
      animationTimingFunction: "ease-in-out",
      animationIterationCount: "infinite",
    },
  },
});
