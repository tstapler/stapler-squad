import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

// Pulse animation for running status dot — only active when reduced motion is not requested
const pulseOpacity = keyframes({
  "0%": { opacity: 1 },
  "50%": { opacity: 0.4 },
  "100%": { opacity: 1 },
});

export const row = style({
  display: "grid",
  // gridTemplateColumns is set via inline style in SessionRow based on visibleColumns.
  // Default fallback (no JS): dot | name+path | agent | memory | elapsed | actions.
  gridTemplateColumns: "24px 8px 1fr 20px auto 32px auto",
  alignItems: "center",
  gap: vars.space["2"],
  padding: "6px 12px",
  minHeight: "38px",
  cursor: "pointer",
  borderRadius: vars.radii.sm,
  listStyle: "none",
  position: "relative",
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      transition: vars.transition.fast,
    },
  },
  ":hover": {
    background: vars.color.hoverBackground,
  },
});

export const nameCell = style({
  minWidth: 0,
  display: "flex",
  flexDirection: "column",
  justifyContent: "center",
  gap: "2px",
});

/** Second row inside nameCell: path + substatus chip inline */
export const pathLine = style({
  display: "flex",
  alignItems: "center",
  gap: "4px",
  minWidth: 0,
  overflow: "hidden",
});

export const statusDot = style({
  width: "8px",
  height: "8px",
  borderRadius: vars.radii.full,
  flexShrink: 0,
  selectors: {
    '&[data-status="running"]': {
      background: vars.color.statusDot.running,
    },
    '&[data-status="paused"]': {
      background: vars.color.statusDot.paused,
    },
    '&[data-status="idle"]': {
      background: vars.color.statusDot.idle,
    },
    '&[data-status="loading"]': {
      background: vars.color.statusDot.idle,
    },
    '&[data-status="needs-approval"]': {
      background: vars.color.primary,
    },
    '&[data-status="paused-session"]': {
      background: vars.color.warningText,
    },
    '&[data-status="hibernated"]': {
      background: vars.color.statusDot.idle,
    },
  },
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      selectors: {
        '&[data-status="running"]': {
          animationName: pulseOpacity,
          animationDuration: "2s",
          animationIterationCount: "infinite",
          animationTimingFunction: "ease-in-out",
        },
        '&[data-status="needs-approval"]': {
          animationName: pulseOpacity,
          animationDuration: "1.2s",
          animationIterationCount: "infinite",
          animationTimingFunction: "ease-in-out",
        },
      },
    },
  },
});

export const name = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const agentIcon = style({
  fontSize: vars.fontSize.sm,
  flexShrink: 0,
  display: "flex",
  alignItems: "center",
});

export const path = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  minWidth: 0,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const elapsed = style({
  fontSize: "11px",
  color: vars.color.textMuted,
  fontVariantNumeric: "tabular-nums",
  minWidth: "32px",
  textAlign: "right",
});

export const actions = style({
  display: "flex",
  gap: vars.space["1"],
  alignItems: "center",
});

/** Primary action button (Resume/Pause) — hidden unless hovering or session needs attention */
export const primaryActionWrapper = style({
  display: "flex",
  opacity: 0,
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      transition: vars.transition.fast,
    },
    // Touch devices have no hover — always show primary action
    "(hover: none)": {
      opacity: 1,
    },
  },
  selectors: {
    [`${row}:hover &`]: {
      opacity: 1,
    },
    [`${row}:focus-within &`]: {
      opacity: 1,
    },
    [`${row}[data-actions-visible="true"] &`]: {
      opacity: 1,
    },
  },
});

/** Inline action button (Resume/Pause text button) used in row context */
export const inlineActionButton = style({
  padding: "2px 8px",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  background: vars.color.surfaceSubtle,
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  whiteSpace: "nowrap",
  lineHeight: 1.5,
  "@media": {
    // Ensure 44px minimum touch target on coarse-pointer devices (WCAG 2.5.5)
    "(pointer: coarse)": {
      padding: "10px 14px",
      minHeight: 44,
    },
  },
  ":hover": {
    background: vars.color.hoverBackground,
    borderColor: vars.color.borderHover,
  },
});

/** Compact overflow (···) button for inline row use — no border, icon-sized */
export const rowOverflowButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  color: vars.color.textMuted,
  padding: "2px 5px",
  borderRadius: vars.radii.sm,
  lineHeight: 1,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  ":hover": {
    color: vars.color.textPrimary,
    background: vars.color.hoverBackground,
  },
});

export const actionButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  color: vars.color.textMuted,
  padding: "2px 4px",
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  lineHeight: 1,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  ":hover": {
    color: vars.color.textPrimary,
    background: vars.color.hoverBackground,
  },
});

export const memoryBadge = style({
  display: "inline-flex",
  alignItems: "center",
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  fontVariantNumeric: "tabular-nums",
  justifyContent: "flex-end",
});

export const memoryBadgeWarning = style({
  color: vars.color.warning,
  fontWeight: 600,
});

export const memoryBadgeHigh = style({
  color: vars.color.error,
  fontWeight: 700,
});

export const diffBadge = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  fontSize: vars.fontSize.xs,
  fontVariantNumeric: "tabular-nums",
  justifyContent: "flex-end",
});

export const branchCell = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  maxWidth: "120px",
});

// Only applied when RSS > 500 MB — uses a background tint rather than a
// border-inline-start so it doesn't collide with the active/paused left accents.
export const rowMemoryPressure = style({
  background: `color-mix(in srgb, ${vars.color.warningBg} 40%, transparent)`,
});

/** Applied to <li> when session is paused — inline-start border distinguishes paused rows
 *  without reducing opacity, which would drop the elapsed-time text below WCAG AA contrast.
 *  Uses border-inline-start so it flips correctly in RTL layouts. */
export const rowPaused = style({
  borderInlineStart: `2px solid ${vars.color.warningText}`,
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      transition: vars.transition.base,
    },
  },
});

const rowActivePulse = keyframes({
  "0%": { borderLeftColor: vars.color.primary },
  "50%": { borderLeftColor: `${vars.color.primary}66` },
  "100%": { borderLeftColor: vars.color.primary },
});

/** Applied to <li> when subStatus === PROCESSING — pulsing inline-start border makes active
 *  sessions scannable at a glance. Pulse is disabled for reduced-motion users.
 *  Uses border-inline-start so it flips correctly in RTL layouts. */
export const rowActive = style({
  borderInlineStart: `3px solid ${vars.color.primary}`,
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      animationName: rowActivePulse,
      animationDuration: "2s",
      animationIterationCount: "infinite",
      animationTimingFunction: "ease-in-out",
    },
  },
});

/** Name + chip row inside nameCell — extracted from inline style in SessionRow.tsx */
export const nameRow = style({
  display: "flex",
  alignItems: "center",
  gap: "6px",
  minWidth: 0,
});

/** Muted clock icon prefix for the elapsed column — makes the column self-labeling */
export const elapsedIcon = style({
  marginInlineEnd: "3px",
  opacity: 0.45,
  fontSize: "9px",
  fontStyle: "normal",
});

export const groupHeader = style({
  height: "24px",
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  paddingLeft: "8px",
  paddingTop: "8px",
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  listStyle: "none",
});

/** Checkbox cell — always occupies the reserved 24px column; visibility is CSS-driven. */
export const checkboxCell = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  visibility: "hidden",
  pointerEvents: "none",
  selectors: {
    // Desktop hover reveal
    [`${row}:hover &`]: {
      visibility: "visible",
      pointerEvents: "auto",
    },
    // Always visible when select mode is active (all devices)
    [`[data-select-mode="true"] &`]: {
      visibility: "visible",
      pointerEvents: "auto",
    },
  },
  // Touch devices: CSS :hover never fires on tap, so make checkboxes permanently visible.
  "@media": {
    "(hover: none)": {
      visibility: "visible",
      pointerEvents: "auto",
    },
  },
});

/** Custom checkbox button rendered inside checkboxCell. */
export const checkboxButton = style({
  width: "16px",
  height: "16px",
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.surfaceSubtle,
  cursor: "pointer",
  padding: 0,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  selectors: {
    '&[aria-checked="true"]': {
      background: vars.color.primary,
      borderColor: vars.color.primary,
    },
    '&[aria-checked="true"]::after': {
      content: '"✓"',
      color: "white",
      fontSize: "10px",
      lineHeight: 1,
    },
  },
  "@media": {
    "(pointer: coarse)": {
      width: "44px",
      height: "44px",
    },
  },
});

/** Applied to the row when it is in the selected set — background tint distinct from active/paused accents. */
export const rowSelected = style({
  background: "var(--session-selected-bg)",
});
