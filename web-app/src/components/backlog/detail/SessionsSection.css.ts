import { style } from "@vanilla-extract/css";
import { vars, breakpoints } from "@/styles/theme.css";

// "Show N more" control (useShowMore, Task 3.1.4c2) — shared with
// ProgressHistorySection and WorkflowHistorySection, see detailShared.css.ts.
export { showMoreButton } from "./detailShared.css";

/**
 * Story 4.1.4: takes the same flex slot `sessionLink`'s <a> occupied for a
 * real session row, sized to fill the row's main axis while leaving room for
 * the sibling delete button (`sessionRowMain` is a flex row).
 */
export const diagnosticRowWrapper = style({
  flex: 1,
  minWidth: 0,
});

export const diagnosticRowTitle = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  minWidth: 0,
  flex: 1,
});

/**
 * Steer toggle button (Story 2.2.2, ADR-002) — rendered inline next to the
 * existing Delete button in a "work"/"review" row. ≥44x44px touch target
 * per ux.md's mobile note; unlike sessionDeleteBtn this isn't a destructive
 * action, so it uses the neutral/primary palette, not the error one.
 */
export const sessionSteerBtn = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minHeight: 44,
  minWidth: 44,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  color: vars.color.textSecondary,
  cursor: "pointer",
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  flexShrink: 0,
  ":hover": {
    borderColor: vars.color.primary,
    color: vars.color.primary,
  },
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

/**
 * Inline steer composer (Story 2.2.2, Task 2.2.2c/d) — single-line input +
 * Send/Cancel, mirroring TriageDiffSection's answerForm/answerActions shape
 * (Gap 1's same disclosure pattern). Stacks full-width below breakpoints.sm
 * instead of wrapping the input/buttons awkwardly on one line, via a plain
 * media query on the container's flex-direction (no inline style, per
 * docs/reference/css-architecture.md).
 */
export const steerComposer = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["2"]}`,
  "@media": {
    [`screen and (max-width: ${breakpoints.sm})`]: {
      flexDirection: "column",
      alignItems: "stretch",
    },
  },
});

export const steerInput = style({
  flex: 1,
  minWidth: 0,
  minHeight: 44,
  padding: vars.space["2"],
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.sans,
  ":focus": {
    outline: "none",
    borderColor: vars.color.inputFocusBorder,
  },
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
});

export const steerSubmitButton = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minHeight: 44,
  backgroundColor: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.md,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  transition: vars.transition.fast,
  flexShrink: 0,
  ":hover": {
    backgroundColor: vars.color.primaryHover,
  },
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
  "@media": {
    [`screen and (max-width: ${breakpoints.sm})`]: {
      width: "100%",
    },
  },
});

export const steerCancelButton = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minHeight: 44,
  backgroundColor: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  transition: vars.transition.fast,
  flexShrink: 0,
  ":hover": {
    backgroundColor: vars.color.hoverBackground,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
  "@media": {
    [`screen and (max-width: ${breakpoints.sm})`]: {
      width: "100%",
    },
  },
});

export const steerError = style({
  color: vars.color.error,
  fontSize: vars.fontSize.xs,
});

/**
 * Empty-state nudge shown when an item has no linked sessions (AC0 fix) —
 * mirrors BacklogEmptyState.tsx's FooterNudge treatment for visual
 * consistency rather than inventing new markup/CSS.
 */
export const emptyState = style({
  padding: vars.space["4"],
  textAlign: "center",
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
});
