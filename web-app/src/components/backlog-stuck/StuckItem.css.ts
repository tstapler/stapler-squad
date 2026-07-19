import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const card = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space["3"]} ${vars.space["4"]}`,
  cursor: "pointer",
  transition: "background 0.15s, border-color 0.15s",
  outline: "none",
  ":hover": {
    background: vars.color.hoverBackground,
    borderColor: vars.color.borderHover,
  },
  ":focus-visible": {
    borderColor: vars.color.inputFocusBorder,
    boxShadow: `0 0 0 2px ${vars.color.inputFocusBorder}`,
  },
});

export const cardExpanded = style({
  borderBottomLeftRadius: 0,
  borderBottomRightRadius: 0,
  borderBottom: "none",
});

export const cardResolved = style({
  opacity: 0.6,
});

export const header = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  flexWrap: "wrap",
});

export const title = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  fontWeight: 500,
  flexGrow: 1,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  minWidth: "80px",
});

export const duration = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  flexShrink: 0,
});

export const metaRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  marginTop: vars.space["1"],
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  fontFamily: vars.font.mono,
  flexWrap: "wrap",
});

export const otherReasonsBadge = style({
  fontFamily: vars.font.sans,
  color: vars.color.textSecondary,
  cursor: "default",
});

export const resolvedBanner = style({
  padding: vars.space["3"],
  background: vars.color.successBg,
  color: vars.color.success,
  fontSize: vars.fontSize.sm,
  borderRadius: `0 0 ${vars.radii.md} ${vars.radii.md}`,
  border: `1px solid ${vars.color.borderColor}`,
  borderTop: "none",
});

// ── Snooze affordance (Story 5.1.1, design/ux.md Surface 7/10) ──────────────
//
// Reuses the exact hover-reveal + `(hover: none)` always-on pattern already
// established by SessionCard.css.ts's `editTagsButton` — opacity:0 by
// default, opacity:1 on card hover/focus-within/focus-visible, and forced
// opacity:1 under `(hover: none)` so touch/no-hover pointers never lose the
// control behind an unreachable :hover state.

export const snoozeBtn = style({
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  padding: `2px ${vars.space["2"]}`,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  cursor: "pointer",
  lineHeight: 1.5,
  flexShrink: 0,
  opacity: 0,
  transition: "opacity 0.15s, background 0.12s, color 0.12s, border-color 0.12s",
  "@media": {
    "(hover: none)": { opacity: 1 },
  },
  selectors: {
    [`${card}:hover &`]: { opacity: 1 },
    [`${card}:focus-within &`]: { opacity: 1 },
    "&:focus-visible": {
      opacity: 1,
      outline: `2px solid ${vars.color.inputFocusBorder}`,
      outlineOffset: "1px",
    },
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
      borderColor: vars.color.borderHover,
    },
  },
});

/** Applied in addition to `snoozeBtn` when a `(hover: none), (pointer: coarse)`
 * media query match is detected in JS (see StuckItem.tsx's useHoverUnavailable) —
 * forces the kebab affordance permanently visible at a >=44x44px tap target,
 * per design/ux.md Surface 7's touch/no-hover requirement. */
export const snoozeBtnAlwaysOn = style({
  opacity: 1,
  minWidth: "44px",
  minHeight: "44px",
  fontSize: vars.fontSize.lg,
  fontWeight: 700,
});

export const snoozePicker = style({
  marginTop: vars.space["2"],
  padding: vars.space["3"],
  background: vars.color.surfaceSubtle,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  cursor: "default",
});

export const snoozeOptions = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const snoozeOptionLabel = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  cursor: "pointer",
});

export const snoozeErrorRow = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.errorText,
});

export const snoozeActions = style({
  display: "flex",
  justifyContent: "flex-end",
  gap: vars.space["2"],
});

export const snoozeCancelBtn = style({
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  padding: `4px ${vars.space["3"]}`,
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  cursor: "pointer",
  ":hover": {
    background: vars.color.hoverBackground,
  },
});

export const snoozeConfirmBtn = style({
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: `1px solid ${vars.color.primary}`,
  borderRadius: vars.radii.sm,
  padding: `4px ${vars.space["3"]}`,
  fontSize: vars.fontSize.xs,
  cursor: "pointer",
  ":hover": {
    background: vars.color.primaryHover,
  },
  selectors: {
    "&:disabled": {
      opacity: 0.6,
      cursor: "not-allowed",
    },
  },
});
