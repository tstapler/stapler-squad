import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars, breakpoints } from "@/styles/theme.css";

// Below this width (matches ViewportProvider's isMobile threshold, <600px),
// controls grow to the WCAG 2.5.5 / Apple HIG 44x44px touch target minimum —
// the 28px/16px desktop sizes below are mouse-precision-tuned and too small
// to tap reliably.
const touchMediaQuery = `(max-width: ${breakpoints.fold})`;

// Visually distinct from mobilePaneTabStrip.css.ts's container (borderTop +
// cardBackground) so the two tablists read as separate UI layers when both
// are visible: borderBottom (this strip sits above the pane strip) and the
// page background instead of the card background.
export const windowTabStrip = style({
  display: "flex",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  background: vars.color.background,
  flexShrink: 0,
  height: "36px",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `0 ${vars.space["2"]}`,
  "@media": {
    [touchMediaQuery]: {
      // Tall enough to contain the 44px-tall touch targets below without clipping.
      height: "44px",
    },
  },
});

// The scrollable row of tabs, separate from windowTabStrip's outer flex
// container so role="tablist" (below) wraps ONLY the tabs — the "+" button
// stays a sibling in the outer container, not a tablist child (see
// WindowTabStrip.tsx's render for why that split matters for
// aria-required-children).
export const windowTabList = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["1"],
  flex: 1,
  minWidth: 0,
  height: "100%",
  overflowX: "auto",
  // Hide scrollbar but allow scrolling
  scrollbarWidth: "none",
  selectors: {
    "&::-webkit-scrollbar": {
      display: "none",
    },
  },
});

export const windowAddButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  height: "28px",
  width: "28px",
  padding: 0,
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  cursor: "pointer",
  fontSize: "16px",
  color: vars.color.textMuted,
  flexShrink: 0,
  marginLeft: "auto",
  transition: "background 100ms, color 100ms",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
  "@media": {
    [touchMediaQuery]: {
      height: "44px",
      width: "44px",
    },
  },
});

export const windowTabWrapper = style({
  position: "relative",
  display: "flex",
  alignItems: "center",
  flexShrink: 0,
  maxWidth: "160px",
});

export const windowTabButton = recipe({
  base: {
    display: "flex",
    alignItems: "center",
    height: "28px",
    padding: `0 ${vars.space["3"]}`,
    background: "transparent",
    border: `1px solid ${vars.color.borderColor}`,
    borderRadius: vars.radii.md,
    cursor: "pointer",
    fontSize: vars.fontSize.xs,
    fontFamily: vars.font.mono,
    whiteSpace: "nowrap",
    overflow: "hidden",
    textOverflow: "ellipsis",
    maxWidth: "160px",
    flexShrink: 0,
    transition: "background 100ms, color 100ms, border-color 100ms",
    "@media": {
      [touchMediaQuery]: {
        height: "44px",
        minWidth: "44px",
      },
    },
  },
  variants: {
    active: {
      true: {
        background: vars.color.primary,
        // primaryText, not textInverse — textInverse is tuned against the page
        // background, not the primary accent; paneSplit.css.ts and
        // panePickerOverlay.css.ts already hit this same axe color-contrast
        // failure (3.75-3.84:1) against vars.color.primary and fixed it the
        // same way.
        color: vars.color.primaryText,
        borderColor: vars.color.primary,
        fontWeight: "bold",
        textDecoration: "underline",
      },
      false: {
        color: vars.color.textSecondary,
        selectors: {
          "&:hover": {
            background: vars.color.hoverBackground,
            color: vars.color.textPrimary,
          },
        },
      },
    },
  },
  defaultVariants: { active: false },
});

export const windowTabCloseButton = recipe({
  base: {
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    height: "16px",
    width: "16px",
    marginLeft: vars.space["1"],
    padding: 0,
    background: "transparent",
    border: "none",
    borderRadius: vars.radii.sm,
    cursor: "pointer",
    fontSize: "12px",
    lineHeight: "16px",
    color: "inherit",
    opacity: 0,
    transition: "opacity 100ms, background 100ms",
    selectors: {
      "&:hover": {
        background: vars.color.hoverBackground,
      },
    },
    "@media": {
      [touchMediaQuery]: {
        height: "44px",
        width: "44px",
      },
    },
  },
  variants: {
    active: {
      true: { opacity: 1 },
      false: {},
    },
  },
  defaultVariants: { active: false },
});

// Surface 10 (design/ux.md): a non-modal, dismiss-once hint anchored below
// the tab strip, not inside it — so it never affects role="tablist"'s
// aria-required-children or the roving-tabindex/keyboard-nav flow above.
export const windowOnboardingHint = style({
  display: "flex",
  alignItems: "flex-start",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  background: vars.color.cardBackground,
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const windowOnboardingHintDismiss = style({
  marginLeft: "auto",
  flexShrink: 0,
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  cursor: "pointer",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
});

export const windowTabInput = style({
  height: "22px",
  width: "100%",
  padding: `0 ${vars.space["1"]}`,
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
});
