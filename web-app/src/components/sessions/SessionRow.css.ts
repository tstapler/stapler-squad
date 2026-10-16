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
  // Default fallback (no JS): checkbox | dot | name+path | agent | memory | actions.
  // elapsed is no longer a grid column — it renders as a second line inside nameCell (Epic 1.2).
  gridTemplateColumns: "24px 8px 1fr 20px auto auto",
  // flex-start (not center): name/path/chips are never truncated and can
  // wrap to several lines, so a short sibling column (agent icon, actions)
  // centered against that height would float in a large empty gap — see
  // the session-list-wasted-space branch's screenshot. Top-aligning keeps
  // those columns flush with the first line instead.
  alignItems: "flex-start",
  gap: vars.space["2"],
  padding: "6px 12px",
  // Floor, not a cap (Epic 2.1 Story 2.1.1): wrapped 2-3 line name/path
  // content is allowed to grow the row taller than this; this style sets
  // no fixed or capped block-size property.
  minHeight: "38px",
  cursor: "pointer",
  borderRadius: vars.radii.sm,
  listStyle: "none",
  position: "relative",
  // Story 2.1.2: query against the row's own width (not an ancestor scroll/
  // virtualizer wrapper — pitfalls.md §2) so the NARROW breakpoint below
  // reacts to a collapsed sidebar/narrow pane, not the browser viewport.
  containerType: "inline-size",
  containerName: "sessionRow",
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      transition: vars.transition.fast,
    },
  },
  ":hover": {
    background: vars.color.hoverBackground,
  },
});

// Genuinely narrower than the sidebar's fixed ~280px default width, so this
// only matches a collapsed/mobile-narrow layout — never the everyday case.
// Per Story 2.1.2's Resolution Note, this drives purely visual tweaks only
// (font size, chip wrapping); the single truncation-budget constant used at
// every width lives in SessionRow.tsx, not here, and this breakpoint never
// touches it.
const NARROW = "(max-width: 200px)";

export const nameCell = style({
  minWidth: 0,
  display: "flex",
  flexDirection: "column",
  justifyContent: "center",
  gap: "2px",
});

/** Second row inside nameCell: the path text, on its own — see `chipsLine` for the status/GitHub/backlog chips. */
export const pathLine = style({
  display: "flex",
  alignItems: "center",
  gap: "4px",
  minWidth: 0,
});

/**
 * Third row inside nameCell: status/GitHub/backlog chips, separate from the
 * path text so a long (unbounded, wrapping) path doesn't vertically center
 * these short chips against its full height and leave a large empty gap
 * around them. Smaller font size always, since these are secondary/
 * glanceable info, not primary content — but never below vars.fontSize.xs,
 * the theme's documented WCAG-minimum legible size (see theme.css.ts).
 */
export const chipsLine = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: "4px",
  minWidth: 0,
  fontSize: vars.fontSize.xs,
});

export const statusDot = style({
  width: "8px",
  height: "8px",
  borderRadius: vars.radii.full,
  flexShrink: 0,
  // Row is `alignItems: flex-start` (see row's comment) so a tall wrapped
  // name/path doesn't drag this down into a centered-in-a-huge-gap look;
  // this nudges the dot down to sit level with the name text's first line
  // instead of the row's bare top edge.
  marginTop: "6px",
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
    '&[data-status="crashed"]': {
      background: vars.color.error,
    },
    // Distinct from "crashed" (vars.color.error) per plan.md's Pattern
    // Decisions table -- a failed-before-running creation and a
    // crashed-after-running session are different enough states to warrant
    // different colors, matching SessionCard.tsx's statusCreationFailed
    // token (also vars.color.warning-family, not the error palette).
    '&[data-status="failed"]': {
      background: vars.color.warning,
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
  // Story 2.1.1: wrap instead of ellipsis-clipping; overflowWrap is the
  // safety net for a single unbroken token that's still too long to fit.
  overflowWrap: "anywhere",
  // Story 2.1.2: below NARROW, shrink slightly — visual-only, no truncation
  // budget change (see this file's NARROW comment).
  "@container": {
    [`sessionRow ${NARROW}`]: {
      fontSize: vars.fontSize.xs,
    },
  },
});

export const agentIcon = style({
  fontSize: vars.fontSize.sm,
  flexShrink: 0,
  display: "flex",
  alignItems: "center",
  // See statusDot's comment — row is alignItems: flex-start.
  marginTop: "4px",
});

export const path = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  minWidth: 0,
  // Path is never length-truncated (session-list-wasted-space); this is the
  // only thing preventing a single unbroken opaque segment (hash/UUID) from
  // overflowing the row instead of wrapping.
  overflowWrap: "anywhere",
});

export const elapsed = style({
  fontSize: "11px",
  color: vars.color.textMuted,
  fontVariantNumeric: "tabular-nums",
  minWidth: "32px",
  textAlign: "right",
});

/** Second line beneath name/path holding the elapsed time — no longer a grid cell (Epic 1.2). */
export const elapsedSecondLine = style({
  display: "flex",
  alignItems: "center",
  gap: "4px",
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  marginTop: "2px",
});

// Below this row width, Resume/Pause + the ··· overflow no longer fit
// alongside the name/path column without squeezing it — see the
// session-list-wasted-space branch's screenshot. Wider than NARROW (200px)
// since the actions need real room to render as a legible row of their own.
const ACTIONS_NARROW = "(max-width: 340px)";

export const actions = style({
  display: "flex",
  gap: vars.space["1"],
  alignItems: "center",
  "@container": {
    [`sessionRow ${ACTIONS_NARROW}`]: {
      // Span every column so the grid's auto-placement bumps this item onto
      // its own implicit row below name/path/chips, right-aligned there.
      gridColumn: "1 / -1",
      justifyContent: "flex-end",
    },
  },
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
  "@media": {
    // Ensure 44px minimum touch target on coarse-pointer devices (WCAG 2.5.5)
    "(pointer: coarse)": {
      minHeight: 44,
      minWidth: 44,
      padding: "10px",
    },
  },
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

/** Inline note indicator in the name/path cell — icon-only (unlike SessionCard's
 * pill) since the row layout has no room for badge text without breaking the
 * single-line grid. */
export const noteIndicator = style({
  fontSize: vars.fontSize.sm,
  flexShrink: 0,
  display: "flex",
  alignItems: "center",
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

/**
 * Checkbox cell — always occupies the reserved 24px column; visibility is
 * CSS-driven. `alignSelf: stretch` overrides the row's `alignItems: flex-start`
 * so this cell (and its click handler, see SessionRow.tsx) spans the full
 * row height instead of just the 16px button — a mouse click landing in the
 * cell's padding, not exactly on the button, would otherwise fall through to
 * the row's own onClick (opening the session instead of selecting it).
 */
export const checkboxCell = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  alignSelf: "stretch",
  cursor: "pointer",
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

/** Persistent Failed-state message — row-layout equivalent of SessionCard.tsx's
 *  failure-message row (design/ux.md Surface 3: the toast in Epic 5.3 is
 *  transient, this line is not). Third line in nameCell, only rendered when
 *  session.status === FAILED. Text color mirrors SessionCard.tsx's message
 *  wrapper (var(--text-secondary)); the icon itself uses SessionCard.css's
 *  failureMessageIcon token (imported, not redefined) for the warning color. */
export const failureMessageLine = style({
  display: "flex",
  alignItems: "center",
  gap: "4px",
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});
