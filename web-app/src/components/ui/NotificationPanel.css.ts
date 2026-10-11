import { style, styleVariants, keyframes, globalStyle } from "@vanilla-extract/css";
import { vars, breakpoints, zIndex } from "@/styles/theme.css";

const fadeIn = keyframes({
  from: { opacity: 0 },
  to: { opacity: 1 },
});

export const overlay = style({
  position: "fixed",
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  backgroundColor: vars.color.overlayBackground,
  zIndex: zIndex.slideOver,
  animation: `${fadeIn} 0.3s ease-out`,
});

export const panel = style({
  position: "fixed",
  top: 0,
  right: 0,
  bottom: 0,
  width: "400px",
  maxWidth: "90vw",
  backgroundColor: vars.color.background,
  boxShadow: "-2px 0 10px rgba(0, 0, 0, 0.2)",
  zIndex: zIndex.slideOver,
  transform: "translateX(100%)",
  transition: "transform 0.3s ease-out",
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
  "@media": {
    [`screen and (max-width: ${breakpoints.md})`]: {
      width: "100%",
      maxWidth: "100vw",
    },
  },
});

export const panelOpen = style({
  transform: "translateX(0)",
});

export const header = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  flexWrap: "wrap",
  gap: vars.space[2],
  padding: "1rem 1.25rem",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  backgroundColor: vars.color.cardBackground,
  flexShrink: 0,
  "@media": {
    [`screen and (max-width: ${breakpoints.md})`]: {
      padding: "0.75rem 1rem",
    },
  },
  selectors: {
    // The peek sheet is ~220px tall: the header must not spend it all (see headerActions below).
    '[data-sheet="peek"] &': { padding: "4px 8px", gap: "0 8px" },
  },
});

export const title = style({
  fontSize: "1.25rem",
  fontWeight: 600,
  margin: 0,
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  color: vars.color.textPrimary,
  "@media": {
    [`screen and (max-width: ${breakpoints.md})`]: {
      fontSize: "1.125rem",
    },
  },
  selectors: {
    '[data-sheet="peek"] &': { fontSize: "1rem" },
  },
});

export const unreadBadge = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "1.5rem",
  height: "1.5rem",
  padding: "0 0.5rem",
  backgroundColor: vars.color.error,
  // Darkened so white text stays >= 4.5:1 on the error colour (Axe color-contrast).
  backgroundImage: "linear-gradient(rgba(0, 0, 0, 0.25), rgba(0, 0, 0, 0.25))",
  color: "white",
  borderRadius: "12px",
  fontSize: "0.75rem",
  fontWeight: 600,
});

export const headerActions = style({
  display: "flex",
  alignItems: "center",
  flexWrap: "wrap",
  gap: "0.5rem",
});

/*
 * Peek sheet: the actions join the header's own wrap so the close button can sit on the title row
 * (visually only; DOM and tab order are unchanged). Four 44px+ controls never fit beside the title at
 * 320px, and a header that wraps to three rows leaves the 220px sheet no room for the list.
 */
globalStyle(`[data-sheet="peek"] ${headerActions}`, { display: "contents" });
globalStyle(`[data-sheet="peek"] ${headerActions} > *`, { order: 1 });

export const markAllButton = style({
  padding: "0.5rem 0.75rem",
  backgroundColor: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  cursor: "pointer",
  fontSize: "0.875rem",
  color: vars.color.textPrimary,
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const clearButton = style({
  padding: "0.5rem 0.75rem",
  backgroundColor: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  cursor: "pointer",
  fontSize: "0.875rem",
  color: vars.color.error,
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const closeButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "2rem",
  height: "2rem",
  backgroundColor: "transparent",
  border: "none",
  borderRadius: "50%",
  cursor: "pointer",
  fontSize: "1.5rem",
  color: vars.color.textSecondary,
  transition: "background-color 0.2s ease",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

globalStyle(`[data-sheet="peek"] ${headerActions} > ${closeButton}`, { order: 0, marginLeft: "auto" });

export const content = style({
  flex: 1,
  overflowY: "auto",
  overflowX: "hidden",
  padding: 0,
});

export const empty = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  padding: "2rem",
  textAlign: "center",
  color: vars.color.textSecondary,
});

export const emptyIcon = style({
  fontSize: "4rem",
  marginBottom: "1rem",
  opacity: 0.3,
});

export const emptyText = style({
  fontSize: "1.125rem",
  fontWeight: 500,
  margin: "0 0 0.5rem 0",
  color: vars.color.textPrimary,
});

export const emptySubtext = style({
  fontSize: "0.875rem",
  margin: 0,
  color: vars.color.textSecondary,
});

export const list = style({
  display: "flex",
  flexDirection: "column",
});

export const item = style({
  padding: "1rem 1.25rem",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  borderLeft: "3px solid var(--priority-color)",
  backgroundColor: vars.color.background,
  overflow: "hidden",
  minWidth: 0,
  transition: "background-color 0.2s ease",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
  "@media": {
    [`screen and (max-width: ${breakpoints.md})`]: {
      padding: "0.875rem 1rem",
    },
  },
});

export const unread = style({
  backgroundColor: "rgba(0, 112, 243, 0.07)",
});

export const read = style({
  opacity: 0.7,
});

export const itemHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  marginBottom: "0.5rem",
});

export const itemTitle = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  fontSize: "0.875rem",
  color: vars.color.textPrimary,
  flexWrap: "wrap",
  flex: 1,
  minWidth: 0,
});

globalStyle(`${itemTitle} strong`, {
  overflowWrap: "break-word",
  wordBreak: "break-word",
  flex: "0 1 auto",
  minWidth: 0,
});

export const typeIcon = style({
  fontSize: "1rem",
  flexShrink: 0,
});

export const typeLabel = style({
  fontSize: "0.625rem",
  fontWeight: 600,
  textTransform: "uppercase",
  letterSpacing: "0.3px",
  padding: "2px 4px",
  borderRadius: "3px",
  color: "white",
  // Darkens the inline priority colour so white text stays >= 4.5:1 (Axe color-contrast).
  backgroundImage: "linear-gradient(rgba(0, 0, 0, 0.4), rgba(0, 0, 0, 0.4))",
  whiteSpace: "nowrap",
  flexShrink: 0,
});

export const itemContext = style({
  fontSize: "0.75rem",
  color: vars.color.textSecondary,
  marginBottom: "0.5rem",
  fontWeight: 500,
});

export const itemWorkingDir = style({
  fontSize: "0.75rem",
  color: vars.color.textSecondary,
  marginBottom: "0.5rem",
  display: "flex",
  alignItems: "center",
  gap: "0.25rem",
  overflowWrap: "break-word",
  wordBreak: "break-word",
});

export const itemActions = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
});

export const focusButton = style({
  padding: "0.375rem 0.5rem",
  backgroundColor: "transparent",
  color: vars.color.primary,
  border: `1px solid ${vars.color.primary}`,
  borderRadius: "6px",
  cursor: "pointer",
  fontSize: "0.75rem",
  fontWeight: 500,
  transition: "all 0.2s ease",
  whiteSpace: "nowrap",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.primary,
      color: vars.color.onPrimaryFill,
    },
  },
});

export const unreadDot = style({
  display: "inline-block",
  width: "8px",
  height: "8px",
  backgroundColor: vars.color.primary,
  borderRadius: "50%",
});

export const removeButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "44px",
  minHeight: "44px",
  backgroundColor: "transparent",
  border: "none",
  borderRadius: "50%",
  cursor: "pointer",
  fontSize: "1rem",
  color: vars.color.textMuted,
  transition: "all 0.2s ease",
  opacity: 0,
  flexShrink: 0,
  selectors: {
    [`${item}:hover &`]: {
      opacity: 1,
    },
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
  "@media": {
    [`screen and (max-width: ${breakpoints.md})`]: {
      opacity: 1,
    },
  },
});

export const itemMessage = style({
  margin: "0 0 0.75rem 0",
  fontSize: "0.875rem",
  lineHeight: 1.4,
  color: vars.color.textPrimary,
});

export const itemFooter = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: "0.5rem",
  flexWrap: "wrap",
});

export const timestamp = style({
  fontSize: "0.75rem",
  color: vars.color.textMuted,
});

export const viewButton = style({
  padding: "0.375rem 0.75rem",
  backgroundColor: vars.color.primary,
  color: vars.color.onPrimaryFill,
  border: "none",
  borderRadius: "6px",
  cursor: "pointer",
  fontSize: "0.75rem",
  fontWeight: 500,
  transition: "background-color 0.2s ease",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.primaryHover,
    },
  },
});

export const loadMore = style({
  display: "flex",
  justifyContent: "center",
  padding: "1rem",
});

export const loadMoreButton = style({
  padding: "0.5rem 1.5rem",
  backgroundColor: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  cursor: "pointer",
  fontSize: "0.875rem",
  color: vars.color.textPrimary,
  transition: "all 0.2s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      backgroundColor: vars.color.hoverBackground,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const incompleteSearchNotice = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: "0.5rem",
  padding: "0.5rem 0.75rem",
  margin: "0 0 0.5rem",
  fontSize: "0.8rem",
  color: vars.color.textSecondary,
  backgroundColor: "rgba(0, 0, 0, 0.04)",
  borderLeft: `2px solid ${vars.color.warning}`,
  borderRadius: "0 4px 4px 0",
});

export const incompleteSearchNoticeButton = style({
  padding: "0.2rem 0.6rem",
  fontSize: "0.75rem",
  fontWeight: 500,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "4px",
  background: "transparent",
  color: vars.color.primary,
  cursor: "pointer",
  selectors: {
    "&:hover:not(:disabled)": {
      borderColor: vars.color.primary,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const itemSubtitle = style({
  fontSize: "0.8rem",
  fontWeight: 500,
  color: vars.color.textSecondary,
  marginBottom: "0.125rem",
});

export const approvalDetails = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.25rem",
  margin: "0.375rem 0",
  padding: "0.5rem 0.625rem",
  backgroundColor: "rgba(0, 0, 0, 0.04)",
  borderLeft: `2px solid ${vars.color.warning}`,
  borderRadius: "0 4px 4px 0",
  fontSize: "0.8rem",
});

export const approvalTool = style({
  color: vars.color.textSecondary,
  fontWeight: 500,
});

export const approvalCommand = style({
  fontFamily: "'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace",
  fontSize: "0.78rem",
  color: vars.color.textPrimary,
  background: "transparent",
  whiteSpace: "pre-wrap",
  wordBreak: "break-all",
  padding: 0,
});

export const approvalCwd = style({
  color: vars.color.textMuted,
  fontSize: "0.75rem",
});

export const approveButton = style({
  padding: "0.25rem 0.625rem",
  fontSize: "0.78rem",
  fontWeight: 600,
  border: `1px solid ${vars.color.success}`,
  borderRadius: "4px",
  background: "transparent",
  color: vars.color.success,
  cursor: "pointer",
  transition: "background-color 0.15s, color 0.15s, opacity 0.15s",
  selectors: {
    "&:hover:not(:disabled)": {
      backgroundColor: vars.color.success,
      color: vars.color.onSuccessFill,
    },
    "&:disabled": {
      opacity: 0.45,
      cursor: "not-allowed",
    },
  },
  "@media": {
    [`screen and (max-width: ${breakpoints.md})`]: {
      padding: "0.75rem 1rem",
      minHeight: "44px",
    },
  },
});

export const denyButton = style({
  padding: "0.25rem 0.625rem",
  fontSize: "0.78rem",
  fontWeight: 600,
  border: `1px solid ${vars.color.error}`,
  borderRadius: "4px",
  background: "transparent",
  color: vars.color.error,
  cursor: "pointer",
  transition: "background-color 0.15s, color 0.15s, opacity 0.15s",
  selectors: {
    "&:hover:not(:disabled)": {
      backgroundColor: vars.color.error,
      color: vars.color.onErrorFill,
    },
    "&:disabled": {
      opacity: 0.45,
      cursor: "not-allowed",
    },
  },
  "@media": {
    [`screen and (max-width: ${breakpoints.md})`]: {
      padding: "0.75rem 1rem",
      minHeight: "44px",
    },
  },
});

export const ciBlockedRow = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "flex-start",
  gap: "0.25rem",
});

export const ciBlockedText = style({
  fontSize: "0.75rem",
  color: vars.color.warningText,
});

export const ciBlockedLink = style({
  fontSize: "0.75rem",
  color: vars.color.primary,
  textDecoration: "underline",
});

export const resolvedBadge = style({
  display: "inline-flex",
  alignItems: "center",
  padding: "0.2rem 0.5rem",
  fontSize: "0.75rem",
  fontWeight: 600,
  borderRadius: "4px",
  border: "1px solid transparent",
  whiteSpace: "nowrap",
});

export const countBadge = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "1.25rem",
  height: "1.25rem",
  padding: "0 0.375rem",
  backgroundColor: vars.color.textSecondary,
  color: vars.color.background,
  borderRadius: "10px",
  fontSize: "0.6875rem",
  fontWeight: 600,
  marginLeft: "0.25rem",
});

export const filterBar = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.5rem",
  padding: "0.75rem 1.25rem",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  backgroundColor: vars.color.cardBackground,
  flexShrink: 0,
});

export const searchRow = style({
  display: "flex",
  gap: "0.5rem",
  alignItems: "center",
});

export const searchInput = style({
  width: "100%",
  padding: "0.5rem 0.75rem",
  fontSize: "0.875rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  backgroundColor: vars.color.background,
  color: vars.color.textPrimary,
  outline: "none",
  transition: "border-color 0.15s ease, box-shadow 0.15s ease",
  boxSizing: "border-box",
  selectors: {
    "&::placeholder": {
      color: vars.color.textMuted,
    },
    "&:focus": {
      borderColor: vars.color.primary,
      boxShadow: `0 0 0 2px color-mix(in srgb, ${vars.color.primary} 20%, transparent)`,
    },
  },
  "@media": {
    [`screen and (max-width: ${breakpoints.md})`]: {
      minHeight: "44px",
    },
  },
});

export const searchClearButton = style({
  flexShrink: 0,
  padding: "0.375rem 0.625rem",
  fontSize: "0.75rem",
  fontWeight: 500,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  background: "transparent",
  color: vars.color.textSecondary,
  cursor: "pointer",
  selectors: {
    "&:hover": {
      borderColor: vars.color.primary,
      color: vars.color.primary,
    },
  },
  "@media": {
    [`screen and (max-width: ${breakpoints.md})`]: {
      minHeight: "44px",
      minWidth: "44px",
    },
  },
});

export const filterPills = style({
  display: "flex",
  gap: "0.375rem",
  flexWrap: "wrap",
});

export const filterPill = style({
  padding: "0.25rem 0.625rem",
  fontSize: "0.75rem",
  fontWeight: 500,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "12px",
  background: "transparent",
  color: vars.color.textSecondary,
  cursor: "pointer",
  transition: "all 0.15s ease",
  whiteSpace: "nowrap",
  selectors: {
    "&:hover": {
      borderColor: vars.color.primary,
      color: vars.color.primary,
    },
  },
  "@media": {
    [`screen and (max-width: ${breakpoints.md})`]: {
      minHeight: "44px",
      padding: "0.5rem 0.75rem",
    },
  },
});

export const filterPillActive = style({
  backgroundColor: vars.color.primary,
  borderColor: vars.color.primary,
  color: vars.color.onPrimaryFill,
});

// Exclude-style (negative) filter pill — e.g. "hide backlog items" — visually
// distinct from the inclusion-only filterPillActive so users can tell at a
// glance which pills are narrowing vs. subtracting from the list.
export const filterPillExcludeActive = style({
  backgroundColor: vars.color.error,
  borderColor: vars.color.error,
  color: vars.color.onErrorFill,
});

// Auto-handled (auto_approved) collapsible section

export const autoHandledSection = style({
  borderTop: `1px solid ${vars.color.borderColor}`,
  backgroundColor: vars.color.cardBackground,
  flexShrink: 0,
});

export const autoHandledHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  width: "100%",
  padding: "0.625rem 1.25rem",
  backgroundColor: "transparent",
  border: "none",
  cursor: "pointer",
  fontSize: "0.8125rem",
  fontWeight: 500,
  color: vars.color.textSecondary,
  textAlign: "left",
  transition: "background-color 0.15s ease",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const autoHandledHeaderLeft = style({
  display: "flex",
  alignItems: "center",
  gap: "0.375rem",
});

export const autoHandledBadge = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "1.25rem",
  height: "1.25rem",
  padding: "0 0.375rem",
  backgroundColor: vars.color.textSecondary,
  color: vars.color.background,
  borderRadius: "10px",
  fontSize: "0.6875rem",
  fontWeight: 600,
});

export const autoHandledChevron = style({
  fontSize: "0.625rem",
  color: vars.color.textMuted,
  transition: "transform 0.2s ease",
  flexShrink: 0,
});

export const autoHandledChevronOpen = style({
  transform: "rotate(180deg)",
});

export const autoHandledList = style({
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
  maxHeight: "40vh",
  overflowY: "auto",
});

export const autoHandledItem = style({
  display: "flex",
  alignItems: "flex-start",
  gap: "0.5rem",
  padding: "0.5rem 1.25rem",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  fontSize: "0.8125rem",
  color: vars.color.textSecondary,
  opacity: 0.75,
  selectors: {
    "&:last-child": {
      borderBottom: "none",
    },
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
      opacity: 1,
    },
  },
});

export const autoHandledDecision = style({
  fontSize: "0.75rem",
  fontWeight: 600,
  flexShrink: 0,
  marginTop: "0.0625rem",
});

export const autoHandledContent = style({
  flex: 1,
  minWidth: 0,
});

export const autoHandledTitle = style({
  fontWeight: 500,
  color: vars.color.textPrimary,
  overflowWrap: "break-word",
  wordBreak: "break-word",
});

export const autoHandledMeta = style({
  fontSize: "0.75rem",
  color: vars.color.textMuted,
  display: "flex",
  gap: "0.5rem",
  flexWrap: "wrap",
  marginTop: "0.125rem",
});

export const autoHandledTimestamp = style({
  fontSize: "0.75rem",
  color: vars.color.textMuted,
  flexShrink: 0,
  marginTop: "0.0625rem",
});

// "Needs a decision" section (Task 3.1.2b) — always-expanded top tier, plus
// its calm/hidden-by-filter empty states and staleness indicator.

export const needsDecisionSection = style({
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const needsDecisionHeadingRow = style({
  display: "flex",
  alignItems: "baseline",
  flexWrap: "wrap",
  gap: "0.5rem",
  padding: "0.75rem 1.25rem 0.25rem",
});

export const needsDecisionHeading = style({
  margin: 0,
  fontSize: "0.75rem",
  fontWeight: 700,
  letterSpacing: "0.4px",
  textTransform: "uppercase",
  color: vars.color.textSecondary,
});

// Mirrors ReviewQueuePanel.css.ts's stalenessIndicator/stalenessRetry (Task
// 3.2.1d) so the "Last updated <Xm ago> · Retry" affordance reads identically
// on both surfaces (Task 3.1.2h, AC38).
export const stalenessIndicator = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  whiteSpace: "nowrap",
});

export const stalenessRetry = style({
  background: "none",
  border: "none",
  padding: 0,
  color: vars.color.primary,
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  cursor: "pointer",
  textDecoration: "underline",
});

export const needsDecisionEmpty = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  padding: "1.5rem 1.25rem",
  textAlign: "center",
  color: vars.color.textSecondary,
  gap: "0.25rem",
});

export const needsDecisionEmptyIcon = style({
  fontSize: "1.75rem",
});

export const needsDecisionEmptyText = style({
  fontSize: "0.9375rem",
  fontWeight: 600,
  margin: 0,
  color: vars.color.textPrimary,
});

export const needsDecisionEmptySubtext = style({
  fontSize: "0.8125rem",
  margin: 0,
  color: vars.color.textSecondary,
});

export const needsDecisionClearFilterButton = style({
  marginTop: "0.25rem",
  padding: "0.375rem 0.75rem",
  fontSize: "0.8125rem",
  fontWeight: 500,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  background: "transparent",
  color: vars.color.primary,
  cursor: "pointer",
  selectors: {
    "&:hover": {
      borderColor: vars.color.primary,
    },
  },
});

// ---------------------------------------------------------------------------
// Notification tray (notification_tray_v2): non-modal, transform-only motion.
// Closed trays are `visibility: hidden`, so they leave the tab order and the
// accessibility tree without a layout change.
// ---------------------------------------------------------------------------

const trayBase = style({
  position: "fixed",
  zIndex: zIndex.slideOver,
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
  boxSizing: "border-box",
  backgroundColor: vars.color.background,
  color: vars.color.textPrimary,
  boxShadow: "0 0 12px rgba(0, 0, 0, 0.25)",
  visibility: "hidden",
  transition: "transform 0.25s ease-out, visibility 0s linear 0.25s",
  "@media": {
    "(prefers-reduced-motion: reduce)": { transition: "none" },
  },
  selectors: {
    '&[data-state="open"]': { visibility: "visible", transitionDelay: "0s" },
  },
});

/** Desktop: a right-edge overlay that covers the terminal without resizing it. */
export const trayVariant = styleVariants({
  sideOverlay: [
    trayBase,
    {
      top: 0,
      right: 0,
      bottom: 0,
      width: "min(400px, 40vw)",
      transform: "translateX(100%)",
      borderLeft: `1px solid ${vars.color.borderColor}`,
      selectors: { '&[data-state="open"]': { transform: "translateX(0)" } },
    },
  ],
  bottomSheet: [
    trayBase,
    {
      left: 0,
      right: 0,
      bottom: "max(var(--keyboard-height, 0px), var(--bottom-nav-height, 0px))",
      // TS-1: peek stays inside 22-28% of the viewport; a pixel floor broke that below 786px.
      height: "calc(var(--viewport-height, 100dvh) * 0.27)",
      maxHeight: "var(--viewport-height, 100dvh)",
      transform: "translateY(100%)",
      borderTop: `1px solid ${vars.color.borderColor}`,
      borderTopLeftRadius: "12px",
      borderTopRightRadius: "12px",
      // The bottom nav's measured height already includes the inset; pad only what it does not cover (TS-7).
      paddingBottom: "max(0px, calc(env(safe-area-inset-bottom, 0px) - var(--bottom-nav-height, 0px)))",
      selectors: {
        '&[data-state="open"]': { transform: "translateY(0)" },
        '&[data-sheet="expanded"]': { height: "calc(var(--viewport-height, 100dvh) * 0.85)" },
      },
    },
  ],
  /** Soft keyboard open on a phone: anchored under the tab row so the keyboard never covers it. */
  topSheet: [
    trayBase,
    {
      left: 0,
      right: 0,
      top: "max(var(--mobile-stack-top-offset, 0px), env(safe-area-inset-top, 0px))",
      height: "calc(var(--viewport-height, 100dvh) * 0.5)",
      maxHeight:
        "calc(var(--viewport-height, 100dvh) - max(var(--mobile-stack-top-offset, 0px), env(safe-area-inset-top, 0px)))",
      transform: "translateY(-100%)",
      borderBottom: `1px solid ${vars.color.borderColor}`,
      borderBottomLeftRadius: "12px",
      borderBottomRightRadius: "12px",
      selectors: { '&[data-state="open"]': { transform: "translateY(0)" } },
    },
  ],
  /** Phone on its side: a right-hand column that honors the notch insets. */
  landscapePanel: [
    trayBase,
    {
      top: 0,
      right: 0,
      bottom: 0,
      width: "min(360px, 50vw)",
      paddingRight: "env(safe-area-inset-right, 0px)",
      paddingTop: "env(safe-area-inset-top, 0px)",
      paddingBottom: "env(safe-area-inset-bottom, 0px)",
      transform: "translateX(100%)",
      borderLeft: `1px solid ${vars.color.borderColor}`,
      selectors: { '&[data-state="open"]': { transform: "translateX(0)" } },
    },
  ],
});

/**
 * Pin tray (opt-in, >= 900px): the open tray becomes a layout column, so the
 * terminal narrows instead of being covered. The one mode that resizes the
 * terminal, and only once per toggle through the existing fit() path.
 */
globalStyle('html[data-tray-pinned="true"] #main-content', {
  marginRight: "min(400px, 40vw)",
});

export const sheetScrim = style({
  position: "fixed",
  inset: 0,
  zIndex: zIndex.slideOver,
  background: "transparent",
});

export const sheetGrabberRow = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  flexShrink: 0,
  gap: "8px",
  padding: "0 8px",
});

/** Only this strip drags the sheet; list content never does (C12). */
export const sheetGrabber = style({
  flex: 1,
  minHeight: "44px",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  touchAction: "none",
  cursor: "grab",
  background: "transparent",
  border: "none",
  selectors: {
    "&::before": {
      content: '""',
      width: "40px",
      height: "4px",
      borderRadius: "2px",
      background: vars.color.textMuted,
    },
  },
});

export const trayButton = style({
  minWidth: "44px",
  minHeight: "44px",
  padding: "0 12px",
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  gap: "6px",
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  cursor: "pointer",
  selectors: {
    "&:hover:not([aria-disabled='true'])": { backgroundColor: vars.color.hoverBackground },
    "&[aria-disabled='true']": { opacity: 0.55, cursor: "not-allowed" },
    "&[aria-pressed='true']": { backgroundColor: vars.color.hoverBackground, borderColor: vars.color.primary },
    "&:focus-visible": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "2px" },
  },
});

export const trayAttention = style({
  fontSize: "0.8125rem",
  fontWeight: 600,
  color: vars.color.errorText,
});

export const trayBanner = style({
  padding: "8px 16px",
  fontSize: "0.8125rem",
  background: vars.color.cardBackground,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textPrimary,
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: "8px",
  flexShrink: 0,
});

export const trayUndoBar = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: "12px",
  padding: "4px 16px",
  background: vars.color.modalBackground,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  flexShrink: 0,
  position: "sticky",
  top: 0,
});

export const trayMenu = style({
  position: "absolute",
  top: "calc(100% + 4px)",
  right: 0,
  minWidth: "260px",
  padding: "4px",
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  boxShadow: "0 8px 24px rgba(0, 0, 0, 0.3)",
  zIndex: zIndex.raised,
  display: "flex",
  flexDirection: "column",
});

export const trayMenuItem = style({
  minHeight: "44px",
  padding: "6px 12px",
  textAlign: "left",
  background: "transparent",
  border: "none",
  borderRadius: "6px",
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  cursor: "pointer",
  display: "flex",
  flexDirection: "column",
  justifyContent: "center",
  selectors: {
    "&:hover:not([aria-disabled='true'])": { backgroundColor: vars.color.hoverBackground },
    "&:focus-visible": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "-2px" },
    "&[aria-disabled='true']": { opacity: 0.55, cursor: "not-allowed" },
  },
});

export const trayMenuCaption = style({ fontSize: "0.75rem", color: vars.color.textSecondary });

export const trayMenuDivider = style({
  height: "1px",
  margin: "4px 0",
  background: vars.color.borderColor,
});

export const trayConfirm = style({
  margin: "8px 16px",
  padding: "12px",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  background: vars.color.cardBackground,
  display: "flex",
  flexDirection: "column",
  gap: "8px",
  flexShrink: 0,
});

export const trayConfirmActions = style({ display: "flex", gap: "8px", justifyContent: "flex-end" });

export const traySettings = style({
  margin: "8px 16px",
  padding: "8px 12px",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  display: "flex",
  flexDirection: "column",
  gap: "8px",
  flexShrink: 0,
});

export const traySettingsRow = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: "8px",
  fontSize: "0.875rem",
});

export const traySelect = style({
  minHeight: "44px",
  padding: "0 8px",
  background: vars.color.background,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
});

export const virtualViewport = style({
  position: "relative",
  width: "100%",
});

export const virtualRow = style({
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
});

export const groupHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: "8px",
  padding: "0 12px 0 16px",
  minHeight: "44px",
  background: vars.color.cardBackground,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  fontSize: "0.8125rem",
  fontWeight: 600,
  textTransform: "uppercase",
  letterSpacing: "0.3px",
});

export const groupHeaderPinned = style({
  borderLeft: `4px solid ${vars.color.error}`,
});

export const groupToggle = style({
  flex: 1,
  minHeight: "44px",
  display: "flex",
  alignItems: "center",
  gap: "8px",
  textAlign: "left",
  background: "transparent",
  border: "none",
  color: "inherit",
  font: "inherit",
  textTransform: "inherit",
  letterSpacing: "inherit",
  cursor: "pointer",
  selectors: { "&:focus-visible": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "-2px" } },
});

export const groupNote = style({
  padding: "12px 16px",
  fontSize: "0.8125rem",
  color: vars.color.textSecondary,
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const swipeRow = style({
  position: "relative",
  touchAction: "pan-y",
  willChange: "transform",
});

export const trayFooter = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "0 16px",
  minHeight: "44px",
  borderTop: `1px solid ${vars.color.borderColor}`,
  flexShrink: 0,
  selectors: {
    // The peek sheet spends its height on the list; the footer link appears when expanded.
    '[data-sheet="peek"] &': { display: "none" },
  },
});

export const newPill = style({
  position: "sticky",
  top: 8,
  alignSelf: "center",
  zIndex: zIndex.raised,
  minHeight: "44px",
  padding: "0 16px",
  borderRadius: "22px",
  border: "none",
  background: vars.color.primary,
  color: vars.color.primaryText,
  fontWeight: 600,
  cursor: "pointer",
});

export const trayHandle = style({
  position: "fixed",
  top: "35%",
  right: 0,
  width: "44px",
  minHeight: "88px",
  zIndex: zIndex.slideOver,
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  gap: "4px",
  padding: "8px 0",
  background: vars.color.modalBackground,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRight: "none",
  borderRadius: "8px 0 0 8px",
  boxShadow: "-2px 2px 8px rgba(0, 0, 0, 0.25)",
  cursor: "pointer",
  fontSize: "0.75rem",
  fontWeight: 700,
  transition: "right 0.25s ease-out",
  "@media": { "(prefers-reduced-motion: reduce)": { transition: "none" } },
  selectors: {
    '&[data-open="true"]': { right: "min(400px, 40vw)" },
    "&:hover": { backgroundColor: vars.color.hoverBackground },
    "&:focus-visible": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "2px" },
  },
});

/** Text indicator that Quiet mode is on (TQ-4); never color alone. */
export const trayQuietBadge = style({
  fontSize: "0.625rem",
  fontWeight: 700,
  lineHeight: 1,
  letterSpacing: "0.02em",
  textTransform: "uppercase",
});

/** Fixed fallback shown when the tray throws while rendering (T-TY-20); sits beside the handle. */
export const trayErrorLink = style({
  position: "fixed",
  right: "16px",
  bottom: "16px",
  zIndex: zIndex.slideOver,
  display: "inline-flex",
  alignItems: "center",
  minHeight: "44px",
  padding: "0 12px",
  background: vars.color.modalBackground,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  fontSize: "0.8125rem",
  textDecoration: "underline",
  selectors: { "&:focus-visible": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "2px" } },
});

export const trayHandleDot = style({
  width: "10px",
  height: "10px",
  borderRadius: "50%",
  background: vars.color.error,
  border: `2px solid ${vars.color.modalBackground}`,
});

/**
 * Every control in the v2 tray meets the 44px target size (TS-6, TL-4, XA-2), including
 * the row actions and filter pills shared with the Notifications page, without
 * resizing them there.
 */
globalStyle('[data-notification-tray="v2"] :is(button, select, input[type="search"])', {
  minHeight: "44px",
  minWidth: "44px",
});

globalStyle('[data-notification-tray="v2"] a[href]', {
  display: "inline-flex",
  alignItems: "center",
  minHeight: "44px",
  minWidth: "44px",
});
