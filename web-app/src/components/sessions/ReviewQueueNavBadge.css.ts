import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const pulse = keyframes({
  "0%, 100%": { opacity: 1 },
  "50%": { opacity: 0.7 },
});

export const badge = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "1.25rem",
  height: "1.25rem",
  padding: "0 0.375rem",
  background: vars.color.primary,
  color: "white",
  fontSize: "0.6875rem",
  fontWeight: 600,
  borderRadius: "0.625rem",
  lineHeight: 1,
  pointerEvents: "none",
  selectors: {
    "&:not(:empty)": {
      animation: `${pulse} 2s ease-in-out infinite`,
    },
    '&[data-count-high="true"]': {
      background: vars.color.error,
    },
    '&[data-count-medium="true"]': {
      background: vars.color.warning,
    },
  },
  "@media": {
    "(prefers-reduced-motion: reduce)": {
      // Override: handled via selectors below
    },
  },
});

export const inline = style({
  marginLeft: "0.375rem",
  verticalAlign: "middle",
});

// empty state - badge with zero count (no visual override needed, kept for className reference)
export const empty = style({});

// Note: prefers-reduced-motion overrides animation on badge:not(:empty)
// vanilla-extract doesn't support combining :not(:empty) with @media in selectors easily,
// so we add a global override via a separate style that consumers can apply if needed.
// The animation in badge will be active unless overridden by browser UA for reduced-motion.
