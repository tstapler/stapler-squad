import { style, globalStyle } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

export const panel = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 12,
  padding: 20,
  display: "flex",
  flexDirection: "column",
  gap: 16,
  minHeight: 0,
  flexShrink: 0,
});

export const header = style({
  display: "flex",
  flexDirection: "column",
  gap: 4,
});

export const titleRow = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
});

export const title = style({
  margin: 0,
  fontSize: 24,
  fontWeight: 700,
  color: vars.color.textPrimary,
});

export const subtitle = style({
  margin: 0,
  fontSize: 13,
  color: vars.color.textSecondary,
});

export const refreshButton = style({
  background: vars.color.hoverBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 8,
  padding: "6px 10px",
  fontSize: 16,
  cursor: "pointer",
  color: vars.color.textPrimary,
  transition: "all 0.2s ease",
  selectors: {
    "&:hover:not(:disabled)": { background: vars.color.accentHover },
    "&:disabled": { opacity: 0.5, cursor: "not-allowed" },
  },
});

export const error = style({
  padding: 12,
  background: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: 8,
  color: vars.color.errorText,
  fontSize: 13,
  display: "flex",
  alignItems: "center",
  gap: 10,
});

export const retryButton = style({
  background: vars.color.errorBg,
  border: "none",
  borderRadius: 4,
  padding: "3px 8px",
  cursor: "pointer",
  color: vars.color.error,
  fontSize: 12,
});

export const loading = style({
  padding: 24,
  textAlign: "center",
  color: vars.color.textSecondary,
  fontSize: 14,
});

export const empty = style({
  padding: 24,
  textAlign: "center",
  color: vars.color.textSecondary,
  fontSize: 14,
});

export const tabs = style({
  display: "flex",
  gap: 6,
  flexWrap: "wrap",
});

export const tab = style({
  background: vars.color.hoverBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 6,
  padding: "5px 12px",
  fontSize: 13,
  cursor: "pointer",
  color: vars.color.textSecondary,
  transition: "all 0.15s ease",
  selectors: {
    "&:hover": { background: vars.color.accentHover, color: vars.color.textPrimary },
  },
});

export const tabActive = style({
  background: vars.color.primary,
  borderColor: vars.color.primary,
  color: vars.color.primaryText,
});

export const tableWrapper = style({
  overflowX: "auto",
  overflowY: "auto",
  maxHeight: 520,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 8,
  "@media": {
    "screen and (max-width: 640px)": { display: "none" },
  },
});

export const table = style({
  width: "100%",
  borderCollapse: "collapse",
  fontSize: 13,
});

export const th = style({
  padding: "10px 12px",
  textAlign: "left",
  fontWeight: 600,
  color: vars.color.textSecondary,
  background: vars.color.panelBgSecondary,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  whiteSpace: "nowrap",
  position: "sticky",
  top: 0,
  zIndex: zIndex.tableHeader,
});

export const td = style({
  padding: "10px 12px",
  verticalAlign: "top",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textPrimary,
});

export const tdCenter = style({ textAlign: "center" });

export const row = style({});
globalStyle(`${row}:last-child td`, { borderBottom: "none" });

export const rowDisabled = style({});
globalStyle(`${rowDisabled} td`, { opacity: 0.45 });

export const triggerName = style({
  display: "block",
  fontWeight: 500,
});

export const triggerSlug = style({
  display: "block",
  fontSize: 11,
  fontFamily: vars.font.mono,
  color: vars.color.textSecondary,
  marginTop: 2,
});

// ── Trigger-type badges ────────────────────────────────────────────────────

export const typeBadge = style({
  display: "inline-block",
  padding: "2px 8px",
  borderRadius: 4,
  fontSize: 11,
  fontWeight: 600,
  whiteSpace: "nowrap",
});

export const typeCron = style({ background: vars.color.panelBgSecondary, color: vars.color.textSecondary });
export const typeGithubPush = style({ background: vars.color.accentHover, color: vars.color.primary });
export const typeWebhook = style({ background: vars.color.warningBg, color: vars.color.warning });
export const typeManual = style({ background: vars.color.panelBgSecondary, color: vars.color.textMuted });

// ── 5-state outcome/status badges (research/ux.md §4) ──────────────────────
// Status must never be color-only (WCAG 1.4.1) — every badge below pairs a
// color with a text label, never a bare dot.

export const statusBadge = style({
  display: "inline-flex",
  alignItems: "center",
  gap: 4,
  padding: "2px 8px",
  borderRadius: 4,
  fontSize: 11,
  fontWeight: 600,
  whiteSpace: "nowrap",
});

export const statusFiredSuccess = style({ background: vars.color.successBg, color: vars.color.success });
export const statusFiredFailed = style({ background: vars.color.warningBg, color: vars.color.warning });
export const statusRejected = style({ background: vars.color.errorBg, color: vars.color.error });
export const statusNoMatch = style({ background: vars.color.panelBgSecondary, color: vars.color.textMuted });
export const statusDisabled = style({ background: vars.color.panelBgSecondary, color: vars.color.textMuted, border: `1px solid ${vars.color.borderSubtle}` });

export const lastFired = style({
  fontSize: 12,
  color: vars.color.textSecondary,
});

export const neverFired = style({
  fontSize: 12,
  color: vars.color.textMuted,
  fontStyle: "italic",
});

export const toggle = style({
  border: "none",
  borderRadius: 4,
  padding: "3px 8px",
  fontSize: 11,
  fontWeight: 700,
  cursor: "pointer",
  transition: "all 0.15s ease",
  minWidth: 44,
  minHeight: 28,
  selectors: {
    "&:disabled": { cursor: "default", opacity: 0.5 },
  },
});

export const toggleOn = style({ background: vars.color.successBg, color: vars.color.success });
export const toggleOff = style({ background: vars.color.hoverBackground, color: vars.color.textSecondary });

export const rowActions = style({
  display: "flex",
  gap: 4,
  alignItems: "center",
  flexWrap: "wrap",
});

export const iconButton = style({
  fontSize: "0.75rem",
  padding: "4px 10px",
  minHeight: 32,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 4,
  background: "transparent",
  color: "inherit",
  transition: "all 0.15s ease",
  selectors: {
    "&:hover": { background: vars.color.hoverBackground },
  },
});

export const addButton = style({
  background: vars.color.primary,
  border: "none",
  borderRadius: 8,
  padding: "8px 16px",
  fontSize: 14,
  fontWeight: 600,
  color: vars.color.primaryText,
  cursor: "pointer",
  transition: "opacity 0.15s ease",
  selectors: {
    "&:hover": { opacity: 0.85 },
  },
});

// ── Mobile FAB + card layout ─────────────────────────────────────────────

export const mobileAddFab = style({
  display: "none",
  "@media": {
    "screen and (max-width: 640px)": {
      display: "flex",
      position: "fixed",
      bottom: "calc(var(--bottom-nav-height, 72px) + 16px)",
      right: 16,
      zIndex: zIndex.raised,
      background: vars.color.primary,
      color: vars.color.primaryText,
      border: "none",
      borderRadius: "50%",
      width: 52,
      height: 52,
      fontSize: 24,
      fontWeight: 700,
      alignItems: "center",
      justifyContent: "center",
      cursor: "pointer",
      boxShadow: "0 4px 16px rgba(0,0,0,0.3)",
      transition: "opacity 0.15s ease",
      selectors: {
        "&:hover": { opacity: 0.85 },
        "&:active": { transform: "scale(0.95)" },
      },
    },
  },
});

export const headerButtonsHiddenOnMobile = style({
  "@media": {
    "screen and (max-width: 640px)": { display: "none" },
  },
});

export const cardList = style({
  display: "none",
  "@media": {
    "screen and (max-width: 640px)": {
      display: "flex",
      flexDirection: "column",
      gap: 10,
    },
  },
});

export const card = style({
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 10,
  padding: 12,
  display: "flex",
  flexDirection: "column",
  gap: 8,
  background: vars.color.panelBgSecondary,
});

export const cardTop = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "flex-start",
  gap: 8,
});

export const cardMeta = style({
  display: "flex",
  flexWrap: "wrap",
  gap: 6,
  alignItems: "center",
  fontSize: 12,
  color: vars.color.textSecondary,
});

export const rowCount = style({
  fontSize: 12,
  color: vars.color.textMuted,
  textAlign: "right",
  paddingTop: 6,
});

// ── Execution history (expand-on-demand row detail, Epic 7.2) ────────────

export const historyToggle = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  color: vars.color.primary,
  fontSize: 12,
  padding: "2px 4px",
  textDecoration: "underline",
});

export const historyWrapper = style({
  padding: "8px 0 0 0",
});

export const historyCounter = style({
  fontSize: 12,
  color: vars.color.textSecondary,
  marginBottom: 6,
});

export const historyList = style({
  display: "flex",
  flexDirection: "column",
  gap: 6,
});

export const historyEntry = style({
  display: "flex",
  alignItems: "center",
  gap: 8,
  fontSize: 12,
  padding: "6px 8px",
  borderRadius: 6,
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderSubtle}`,
});

export const historyTimestamp = style({
  color: vars.color.textMuted,
  fontFamily: vars.font.mono,
  fontSize: 11,
  whiteSpace: "nowrap",
});

export const historyError = style({
  color: vars.color.errorText,
  fontSize: 11,
});

export const historySessionLink = style({
  color: vars.color.primary,
  fontSize: 11,
  textDecoration: "underline",
});

export const showNoMatchToggle = style({
  background: "none",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 6,
  padding: "3px 10px",
  fontSize: 11,
  color: vars.color.textSecondary,
  cursor: "pointer",
  selectors: {
    "&:hover": { background: vars.color.hoverBackground },
  },
});

// ── Attribution badge (Epic 7.4, used on session card/detail) ────────────

export const attributionBadge = style({
  display: "inline-flex",
  alignItems: "center",
  gap: 4,
  padding: "2px 8px",
  borderRadius: 4,
  fontSize: 11,
  fontWeight: 500,
  background: vars.color.accentHover,
  color: vars.color.primary,
  textDecoration: "none",
  selectors: {
    "&:hover": { textDecoration: "underline" },
  },
});
