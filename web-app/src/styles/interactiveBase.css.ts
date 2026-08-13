import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "./theme.css";

/**
 * Story 6.2 — Shared hover/focus state system.
 *
 * All interactive controls that are not already styled via Button.css.ts should
 * compose from these base styles. They ensure consistent focus rings, theme-aware
 * glow effects, and reduced-motion compliance across the entire UI.
 */

/** Standard focus ring — apply to any focusable element. */
export const focusRing = style({
  selectors: {
    "&:focus-visible": {
      outline: `2px solid ${vars.color.primary}`,
      outlineOffset: "2px",
      // Theme-aware glow halo on focus
      boxShadow: `0 0 0 4px ${vars.color.glowSecondary}`,
    },
  },
});

/** Hover highlight for list rows and clickable surfaces. */
export const hoverHighlight = style({
  transition: "background 120ms ease, border-color 120ms ease",
  "@media": {
    "(prefers-reduced-motion: reduce)": {
      transition: "none",
    },
  },
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
  },
});

/** Interactive button base — combines hover + focus ring + cursor. */
export const interactiveButton = recipe({
  base: [
    focusRing,
    {
      cursor: "pointer",
      border: "none",
      background: "transparent",
      transition: "background 120ms ease, color 120ms ease, box-shadow 120ms ease",
      "@media": {
        "(prefers-reduced-motion: reduce)": {
          transition: "none",
        },
      },
    },
  ],
  variants: {
    variant: {
      ghost: {
        selectors: {
          "&:hover": {
            background: vars.color.hoverBackground,
            color: vars.color.textPrimary,
          },
        },
      },
      primary: {
        background: vars.color.primary,
        color: vars.color.primaryText,
        selectors: {
          "&:hover": {
            background: vars.color.primaryHover,
          },
          "&:active": {
            background: vars.color.primaryActive,
          },
        },
      },
      danger: {
        background: vars.color.errorBg,
        color: vars.color.errorText,
        selectors: {
          "&:hover": {
            background: vars.color.error,
            color: vars.color.primaryText,
          },
        },
      },
    },
  },
  defaultVariants: { variant: "ghost" },
});

/** Glow-on-hover for primary action elements (omnibar open button, CTA buttons, etc.). */
export const glowOnHover = style({
  transition: "box-shadow 200ms ease",
  "@media": {
    "(prefers-reduced-motion: reduce)": {
      transition: "none",
    },
  },
  selectors: {
    "&:hover": {
      boxShadow: `0 0 8px ${vars.color.glowPrimary}, 0 0 16px ${vars.color.glowSecondary}`,
    },
  },
});
