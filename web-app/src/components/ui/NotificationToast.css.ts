import { style, styleVariants, keyframes, globalStyle } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

const ring = keyframes({
  "0%, 100%": { transform: "rotate(0deg)" },
  "10%, 30%, 50%, 70%, 90%": { transform: "rotate(-10deg)" },
  "20%, 40%, 60%, 80%": { transform: "rotate(10deg)" },
});

const enter = keyframes({
  from: { transform: "translateX(450px)", opacity: 0 },
  to: { transform: "translateX(0)", opacity: 1 },
});

export const toast = style({
  position: "fixed",
  bottom: "24px",
  right: "24px",
  width: "380px",
  maxHeight: "calc(var(--viewport-height, 100dvh) - 48px)",
  background: vars.color.modalBackground,
  border: `2px solid var(--priority-color, ${vars.color.primary})`,
  borderRadius: "12px",
  // Story 5.3: Theme-aware glow on toast border
  boxShadow: `0 8px 32px rgba(0, 0, 0, 0.4), 0 0 0 1px ${vars.color.glowSecondary}`,
  zIndex: zIndex.toast,
  overflow: "hidden",
  transform: "translateX(0)",
  opacity: 1,
  // Entrance is a keyframe (not a post-mount class flip) so the card needs no timer.
  animation: `${enter} 0.3s cubic-bezier(0.4, 0, 0.2, 1)`,
  transition: "all 0.3s cubic-bezier(0.4, 0, 0.2, 1)",
  "@media": {
    // On mobile the bottom nav + optional pane tab strip must be cleared.
    // --bottom-nav-height is published by BottomNav; --mobile-pane-tab-strip-height by PaneSplitRenderer.
    "screen and (max-width: 899px)": {
      left: "16px",
      right: "16px",
      width: "auto",
      bottom: "calc(var(--bottom-nav-height, 64px) + var(--mobile-pane-tab-strip-height, 0px) + 12px + max(env(safe-area-inset-bottom, 0px), 0px))",
    },
    "(prefers-reduced-motion: reduce)": {
      animation: "none",
      transition: "none",
    },
  },
});

export const exiting = style({
  transform: "translateX(450px)",
  opacity: 0,
});

export const minimized = style({
  width: "260px",
  maxHeight: "48px",
  overflow: "hidden",
  cursor: "pointer",
  borderRadius: "24px",
  bottom: "16px",
});

export const toastApproval = style({
  width: "480px",
  "@media": {
    "screen and (max-width: 768px)": {
      left: "16px",
      right: "16px",
      width: "auto",
      bottom: "16px",
    },
  },
});

export const header = style({
  display: "flex",
  alignItems: "center",
  gap: "12px",
  padding: "16px 16px 12px 16px",
  background: `linear-gradient(to bottom, var(--priority-color, ${vars.color.primary}), transparent)`,
  backgroundSize: "100% 4px",
  backgroundRepeat: "no-repeat",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  selectors: {
    [`${minimized} &`]: {
      padding: "10px 12px",
      borderBottom: "none",
      background: "none",
    },
  },
});

export const icon = style({
  fontSize: "24px",
  lineHeight: 1,
  flexShrink: 0,
  animation: `${ring} 0.5s ease-in-out`,
  "@media": { "(prefers-reduced-motion: reduce)": { animation: "none" } },
  selectors: {
    [`${minimized} &`]: {
      fontSize: "16px",
      animation: "none",
    },
  },
});

export const titleWrapper = style({
  flex: 1,
  display: "flex",
  flexDirection: "column",
  gap: "4px",
  minWidth: 0,
});

export const titleRow = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
});

globalStyle(`${titleRow} strong`, {
  fontSize: "15px",
  fontWeight: 600,
  color: vars.color.textPrimary,
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
  flex: 1,
  minWidth: 0,
});

globalStyle(`${minimized} ${titleRow} strong`, { fontSize: "13px" });

export const typeLabel = style({
  fontSize: "10px",
  fontWeight: 600,
  textTransform: "uppercase",
  letterSpacing: "0.5px",
  padding: "2px 6px",
  borderRadius: "4px",
  // The darkening layer keeps white text at >= 4.5:1 on every priority colour (Axe color-contrast).
  background: `linear-gradient(rgba(0, 0, 0, 0.4), rgba(0, 0, 0, 0.4)), var(--priority-color, ${vars.color.primary})`,
  color: vars.color.primaryText,
  whiteSpace: "nowrap",
  flexShrink: 0,
  selectors: {
    [`${minimized} &`]: {
      display: "none",
    },
  },
});

export const subtitleRow = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  fontSize: "12px",
  color: vars.color.textMuted,
  selectors: {
    [`${minimized} &`]: {
      display: "none",
    },
  },
});

export const sourceApp = style({
  fontWeight: 500,
  color: vars.color.textSecondary,
});

export const timestamp = style({
  fontSize: "12px",
  color: vars.color.textMuted,
});

export const closeButton = style({
  background: "none",
  border: "none",
  fontSize: "28px",
  lineHeight: 1,
  color: vars.color.textMuted,
  cursor: "pointer",
  padding: 0,
  width: "28px",
  height: "28px",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  borderRadius: "4px",
  transition: "all 0.15s ease",
  flexShrink: 0,
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
    [`${minimized} &`]: {
      fontSize: "18px",
      width: "20px",
      height: "20px",
    },
  },
});

export const body = style({
  padding: "12px 16px",
  overflowY: "auto",
  maxHeight: "300px",
  selectors: {
    [`${minimized} &`]: {
      display: "none",
    },
  },
});

export const message = style({
  margin: 0,
  fontSize: "14px",
  lineHeight: 1.5,
  color: vars.color.textPrimary,
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
});

export const workingDir = style({
  margin: "8px 0 0 0",
  fontSize: "12px",
  color: vars.color.textMuted,
  display: "flex",
  alignItems: "center",
  gap: "4px",
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
});

export const actions = style({
  display: "flex",
  gap: "8px",
  padding: "12px 16px 16px 16px",
  borderTop: `1px solid ${vars.color.borderColor}`,
  selectors: {
    [`${minimized} &`]: {
      display: "none",
    },
  },
});

const baseActionButton = style({
  flex: 1,
  padding: "10px 16px",
  border: "none",
  borderRadius: "6px",
  fontSize: "14px",
  fontWeight: 500,
  cursor: "pointer",
  transition: "all 0.15s ease",
});

export const viewButton = style([baseActionButton, {
  // The darkening layer keeps white text at >= 4.5:1 on every priority colour (Axe color-contrast).
  background: `linear-gradient(rgba(0, 0, 0, 0.4), rgba(0, 0, 0, 0.4)), var(--priority-color, ${vars.color.primary})`,
  color: vars.color.primaryText,
  selectors: {
    "&:hover": {
      filter: "brightness(1.1)",
      transform: "translateY(-1px)",
      boxShadow: "0 4px 8px rgba(0, 0, 0, 0.2)",
    },
  },
}]);

export const dismissButton = style([baseActionButton, {
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderColor}`,
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.textMuted,
    },
  },
}]);

export const focusButton = style([baseActionButton, {
  background: "transparent",
  color: vars.color.primary,
  border: `1px solid ${vars.color.primary}`,
  flex: "0 0 auto",
  selectors: {
    "&:hover": {
      background: vars.color.primary,
      color: vars.color.primaryText,
    },
  },
}]);

export const approveButton = style([baseActionButton, {
  background: vars.color.success,
  color: vars.color.primaryText,
  selectors: {
    "&:hover": {
      background: vars.color.successBg,
      transform: "translateY(-1px)",
      boxShadow: "0 4px 8px rgba(0, 0, 0, 0.2)",
    },
  },
}]);

export const denyButton = style([baseActionButton, {
  background: vars.color.error,
  color: vars.color.primaryText,
  selectors: {
    "&:hover": {
      background: vars.color.errorDark,
      transform: "translateY(-1px)",
      boxShadow: "0 4px 8px rgba(0, 0, 0, 0.2)",
    },
  },
}]);

export const minimizeHint = style({
  display: "none",
});

export const undoButton = style([baseActionButton, {
  background: vars.color.primary,
  color: vars.color.primaryText,
  selectors: {
    "&:hover": {
      filter: "brightness(1.1)",
      transform: "translateY(-1px)",
      boxShadow: "0 4px 8px rgba(0, 0, 0, 0.2)",
    },
    "&:focus-visible": {
      outline: `2px solid ${vars.color.primary}`,
      outlineOffset: "2px",
    },
  },
}]);

// ---------------------------------------------------------------------------
// Capped deck (notification_tray_v2): one fixed flex column, cards in normal flow.
// ---------------------------------------------------------------------------

const deckBase = style({
  position: "fixed",
  zIndex: zIndex.toast,
  display: "flex",
  flexDirection: "column",
  gap: "8px",
  // Only the cards and controls take pointer events; the gaps never block the page.
  pointerEvents: "none",
  boxSizing: "border-box",
});

/**
 * Where the deck docks (ADR-009):
 *  - desktop: bottom-right, 360px wide, clear of the xterm scrollbar;
 *  - mobileTop: under the session tab row (`--mobile-stack-top-offset`, published
 *    by the session layout), never above the safe-area inset;
 *  - mobileBottom: a phone page with no terminal, above the bottom nav.
 */
export const deckPlacement = styleVariants({
  desktop: [
    deckBase,
    {
      bottom: "24px",
      // Clear of the 44px tray handle on the right edge.
      right: "calc(60px + var(--terminal-scrollbar-width, 0px))",
      width: "360px",
      maxHeight: "calc(var(--viewport-height, 100dvh) - 48px)",
    },
  ],
  desktopTopRight: [
    deckBase,
    {
      top: "64px",
      // Clear of the 44px tray handle on the right edge.
      right: "calc(60px + var(--terminal-scrollbar-width, 0px))",
      width: "360px",
      maxHeight: "calc(var(--viewport-height, 100dvh) - 96px)",
    },
  ],
  mobileTop: [
    deckBase,
    {
      top: "calc(max(var(--mobile-stack-top-offset, 0px), env(safe-area-inset-top, 0px)) + 8px)",
      left: "max(16px, env(safe-area-inset-left, 0px))",
      right: "max(16px, env(safe-area-inset-right, 0px))",
      maxHeight: "calc(var(--viewport-height, 100dvh) * 0.4)",
      overflowY: "auto",
    },
  ],
  mobileBottom: [
    deckBase,
    {
      left: "16px",
      right: "16px",
      bottom:
        "calc(var(--bottom-nav-height, 64px) + var(--mobile-pane-tab-strip-height, 0px) + 12px + max(env(safe-area-inset-bottom, 0px), 0px))",
      maxHeight: "calc(var(--viewport-height, 100dvh) * 0.4)",
      overflowY: "auto",
    },
  ],
});

/** A card inside the deck: in flow, filling the deck width. Declared last so it wins over `toast`'s fixed rules. */
export const toastStacked = style({
  position: "relative",
  top: "auto",
  right: "auto",
  bottom: "auto",
  left: "auto",
  width: "100%",
  maxHeight: "none",
  flexShrink: 0,
  pointerEvents: "auto",
  "@media": {
    "screen and (max-width: 899px)": {
      left: "auto",
      right: "auto",
      bottom: "auto",
      width: "100%",
    },
  },
});

export const chipRow = style({
  display: "flex",
  gap: "8px",
  alignItems: "stretch",
  // The phone tray entry is right-aligned and never relocates when its content changes (TH-8).
  justifyContent: "flex-end",
  flexShrink: 0,
  pointerEvents: "auto",
});

const chipBase = {
  minHeight: "44px",
  padding: "0 14px",
  borderRadius: "22px",
  fontSize: "14px",
  fontWeight: 600,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.modalBackground,
  color: vars.color.textPrimary,
  boxShadow: "0 4px 12px rgba(0, 0, 0, 0.3)",
} as const;

export const overflowChip = style({
  ...chipBase,
  flex: 1,
  textAlign: "left",
  selectors: { "&:hover": { background: vars.color.hoverBackground } },
});

export const repeatBadge = style({
  fontSize: "11px",
  fontWeight: 700,
  padding: "1px 6px",
  borderRadius: "10px",
  background: vars.color.cardBackground,
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  flexShrink: 0,
});

export const offlineHint = style({
  fontSize: "12px",
  color: vars.color.textMuted,
  alignSelf: "center",
});

export const deckHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "flex-end",
  gap: "8px",
  pointerEvents: "auto",
  flexShrink: 0,
});

export const deckAction = style({
  ...chipBase,
  borderRadius: "8px",
  selectors: { "&:hover": { background: vars.color.hoverBackground } },
});

/** "Moved N to tray - Undo": takes the header slot on desktop and the chip row's place on a phone. */
export const undoBar = style({
  ...chipBase,
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: "12px",
  pointerEvents: "auto",
  flexShrink: 0,
});

export const undoAction = style({
  minHeight: "44px",
  minWidth: "44px",
  padding: "0 12px",
  borderRadius: "8px",
  border: "none",
  background: vars.color.primary,
  color: vars.color.primaryText,
  fontWeight: 600,
  cursor: "pointer",
});

export const collapsedChip = style({
  display: "block",
  width: "100%",
  minHeight: "44px",
  padding: "0 14px",
  border: "none",
  background: "transparent",
  color: vars.color.textPrimary,
  fontSize: "14px",
  fontWeight: 600,
  textAlign: "left",
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
  cursor: "pointer",
});

// ---------------------------------------------------------------------------
// Swipe row, compact card and phone approval card (Story 3.8)
// ---------------------------------------------------------------------------

/** Wraps a deck card so a swipe has a reveal layer behind it. `pan-y` leaves vertical scrolling to the browser. */
export const swipeRow = style({
  position: "relative",
  flexShrink: 0,
  pointerEvents: "auto",
  touchAction: "pan-y",
});

export const swipeCard = style({
  transition: "transform 0.2s ease-out",
  "@media": {
    "(prefers-reduced-motion: reduce)": { transition: "none" },
  },
});

export const swipeCardDragging = style({ transition: "none" });

export const reveal = style({
  position: "absolute",
  inset: 0,
  display: "flex",
  alignItems: "center",
  gap: "8px",
  padding: "0 20px",
  borderRadius: "12px",
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textPrimary,
  fontSize: "14px",
  fontWeight: 600,
});

export const revealEnd = style({ justifyContent: "flex-end" });

const hit44 = { minHeight: "44px", minWidth: "44px" } as const;

export const closeButtonStacked = style({
  ...hit44,
  background: "none",
  border: "none",
  fontSize: "28px",
  lineHeight: 1,
  color: vars.color.textMuted,
  cursor: "pointer",
  borderRadius: "8px",
  flexShrink: 0,
  selectors: { "&:hover": { background: vars.color.hoverBackground, color: vars.color.textPrimary } },
});

/** Pinned toasts get a tray icon and the visible word "Tray": the control is not a dismissal. */
export const trayButton = style({
  ...hit44,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  gap: "4px",
  padding: "0 8px",
  background: "none",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  color: vars.color.textPrimary,
  fontSize: "13px",
  fontWeight: 600,
  cursor: "pointer",
  flexShrink: 0,
  selectors: { "&:hover": { background: vars.color.hoverBackground } },
});

/** Phone, non-approval: icon, title and one body line left; the primary action and close right. At most 96px. */
export const compactCard = style({
  maxHeight: "96px",
  overflow: "hidden",
});

globalStyle(`${compactCard} [data-slot="body"]`, { padding: "0 16px 8px", maxHeight: "none", overflow: "hidden" });
globalStyle(`${compactCard} [data-slot="body"] p`, {
  display: "-webkit-box",
  WebkitLineClamp: 1,
  WebkitBoxOrient: "vertical",
  overflow: "hidden",
});
globalStyle(`${compactCard} [data-slot="workdir"]`, { display: "none" });
globalStyle(`${compactCard} [data-slot="actions"]`, { padding: "0 16px 8px", borderTop: "none" });

export const commandText = style({
  margin: 0,
  fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
  fontSize: "13px",
  lineHeight: 1.4,
  whiteSpace: "pre-wrap",
  overflowWrap: "anywhere",
  color: vars.color.textPrimary,
  display: "-webkit-box",
  WebkitLineClamp: 4,
  WebkitBoxOrient: "vertical",
  overflow: "hidden",
});

export const commandTextExpanded = style({
  display: "block",
  WebkitLineClamp: "unset",
  overflow: "visible",
  maxHeight: "40vh",
  overflowY: "auto",
});

export const disclosureButton = style({
  ...hit44,
  alignSelf: "flex-start",
  padding: "0 4px",
  background: "none",
  border: "none",
  color: vars.color.primary,
  fontSize: "13px",
  fontWeight: 600,
  cursor: "pointer",
  textAlign: "left",
});

/** Approve and Deny: one decision pair, equal weight, at least 24px between the hit areas. */
export const decisionPair = style({
  display: "flex",
  flexWrap: "wrap",
  gap: "24px",
  width: "100%",
  "@media": {
    "(max-width: 359px)": { flexDirection: "column" },
  },
});

globalStyle(`${decisionPair} > button`, { ...hit44, flex: "1 1 120px" });

export const actionError = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  flexWrap: "wrap",
  width: "100%",
  fontSize: "13px",
  color: vars.color.error,
});

export const inlineAction = style({
  ...hit44,
  padding: "0 12px",
  borderRadius: "8px",
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontWeight: 600,
  cursor: "pointer",
});

export const overflowMenu = style({
  position: "absolute",
  right: "16px",
  bottom: "56px",
  zIndex: 1,
  display: "flex",
  flexDirection: "column",
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  boxShadow: "0 4px 12px rgba(0, 0, 0, 0.3)",
});

export const overflowTrigger = style({
  ...hit44,
  background: "none",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  color: vars.color.textPrimary,
  cursor: "pointer",
  flex: "0 0 auto",
});

export const riskLabel = style({
  fontSize: "11px",
  fontWeight: 700,
  textTransform: "uppercase",
  letterSpacing: "0.5px",
  color: vars.color.textSecondary,
});

/** The single phone tray entry: one mounted node whose content changes in place (D10). */
export const trayEntry = style({
  display: "flex",
  gap: "8px",
  alignItems: "stretch",
  justifyContent: "flex-end",
  flexShrink: 0,
  pointerEvents: "auto",
  minHeight: "44px",
  selectors: {
    // Wide contents (the "+N more" row, the keyboard chip, the undo bar) fill the row.
    '&[data-content="more-row"], &[data-content="keyboard-chip"], &[data-undo="true"]': { flex: 1 },
  },
});

export const trayEntryBell = style({
  ...chipBase,
  width: "44px",
  minWidth: "44px",
  padding: 0,
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  gap: "4px",
  borderRadius: "22px",
  selectors: { "&:hover": { background: vars.color.hoverBackground } },
});

/**
 * Phone: while the tray is open the deck steps aside (it stays mounted, so the entry
 * node is never removed): the tray shows the same notifications and a deck above it
 * would cover the sheet's header and close button.
 */
export const deckBehindTray = style({
  visibility: "hidden",
  pointerEvents: "none",
});

/**
 * Desktop: the side tray is an overlay, so the deck shifts left to stay 16px clear of its
 * left edge instead of hiding (TD-12). The tray is min(400px, 40vw) wide.
 */
export const deckBesideTray = style({
  right: "calc(min(400px, 40vw) + 16px)",
});
