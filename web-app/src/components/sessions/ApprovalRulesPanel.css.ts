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
    "&:hover:not(:disabled)": {
      background: vars.color.accentHover,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const analyticsBar = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: 12,
  padding: "10px 14px",
  background: vars.color.panelBgSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 8,
  fontSize: 13,
  "@media": {
    "screen and (max-width: 640px)": {
      flexDirection: "column",
      alignItems: "flex-start",
    },
  },
});

export const analyticsTotal = style({
  color: vars.color.textPrimary,
  fontWeight: 700,
  fontSize: 14,
});

export const analyticsRate = style({
  fontWeight: 600,
  padding: "2px 8px",
  borderRadius: 4,
});

export const rateAllow = style({
  background: vars.color.successBg,
  color: vars.color.success,
});

export const rateManual = style({
  background: vars.color.warningBg,
  color: vars.color.warning,
});

export const analyticsTopTool = style({
  color: vars.color.textSecondary,
  fontSize: 12,
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
    "&:hover": {
      background: vars.color.accentHover,
      color: vars.color.textPrimary,
    },
  },
});

export const tabActive = style({
  background: vars.color.primary,
  borderColor: vars.color.primary,
  color: vars.color.primaryText,
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

export const tableWrapper = style({
  overflowX: "auto",
  overflowY: "auto",
  maxHeight: 480,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 8,
});

export const table = style({
  width: "100%",
  borderCollapse: "collapse",
  fontSize: 13,
  "@media": {
    "screen and (max-width: 640px)": {
      fontSize: 12,
    },
  },
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
  "@media": {
    "screen and (max-width: 640px)": {
      padding: 8,
    },
  },
});

export const td = style({
  padding: "10px 12px",
  verticalAlign: "top",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textPrimary,
  "@media": {
    "screen and (max-width: 640px)": {
      padding: 8,
    },
  },
});

export const tdCenter = style({
  textAlign: "center",
});

export const row = style({});

globalStyle(`${row}:last-child td`, { borderBottom: "none" });

export const rowDisabled = style({});

globalStyle(`${rowDisabled} td`, { opacity: 0.45 });

export const ruleName = style({
  display: "block",
  fontWeight: 500,
});

export const ruleReason = style({
  display: "block",
  fontSize: 11,
  color: vars.color.textSecondary,
  marginTop: 2,
});

export const ruleAlt = style({
  display: "block",
  fontSize: 11,
  color: vars.color.success,
  marginTop: 2,
});

export const matchInfo = style({
  display: "flex",
  flexWrap: "wrap",
  gap: 4,
});

export const matchChip = style({
  background: vars.color.terminalBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 4,
  padding: "1px 6px",
  fontSize: 11,
  fontFamily: vars.font.mono,
  color: vars.color.textPrimary,
  maxWidth: 200,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const decisionBadge = style({
  display: "inline-block",
  padding: "2px 8px",
  borderRadius: 4,
  fontSize: 11,
  fontWeight: 600,
  whiteSpace: "nowrap",
});

export const decisionAllow = style({
  background: vars.color.successBg,
  color: vars.color.success,
});

export const decisionDeny = style({
  background: vars.color.errorBg,
  color: vars.color.error,
});

export const decisionEscalate = style({
  background: vars.color.warningBg,
  color: vars.color.warning,
});

export const sourceBadge = style({
  display: "inline-block",
  padding: "2px 8px",
  borderRadius: 4,
  fontSize: 11,
  fontWeight: 500,
  background: vars.color.panelBgSecondary,
  color: vars.color.textSecondary,
  whiteSpace: "nowrap",
});

export const toggle = style({
  border: "none",
  borderRadius: 4,
  padding: "3px 8px",
  fontSize: 11,
  fontWeight: 700,
  cursor: "pointer",
  transition: "all 0.15s ease",
  selectors: {
    "&:disabled": {
      cursor: "default",
      opacity: 0.5,
    },
  },
});

export const toggleOn = style({
  background: vars.color.successBg,
  color: vars.color.success,
});

export const toggleOff = style({
  background: vars.color.hoverBackground,
  color: vars.color.textSecondary,
});

export const deleteButton = style({
  background: "none",
  border: "none",
  color: vars.color.textSecondary,
  cursor: "pointer",
  fontSize: 14,
  padding: "2px 6px",
  borderRadius: 4,
  transition: "all 0.15s ease",
  selectors: {
    "&:hover": {
      background: vars.color.errorBg,
      color: vars.color.error,
    },
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
    "&:hover": {
      opacity: 0.85,
    },
  },
});

export const form = style({
  background: vars.color.panelBgSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 10,
  padding: 16,
  display: "flex",
  flexDirection: "column",
  gap: 14,
});

export const formTitle = style({
  margin: 0,
  fontSize: 16,
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const formError = style({
  padding: "8px 12px",
  background: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: 6,
  color: vars.color.errorText,
  fontSize: 13,
});

export const formGrid = style({
  display: "grid",
  gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))",
  gap: 12,
  "@media": {
    "screen and (max-width: 640px)": {
      gridTemplateColumns: "1fr",
    },
  },
});

export const label = style({
  display: "flex",
  flexDirection: "column",
  gap: 4,
  fontSize: 13,
  fontWeight: 500,
  color: vars.color.textSecondary,
});

export const input = style({
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: 6,
  padding: "6px 10px",
  fontSize: 13,
  color: vars.color.textPrimary,
  transition: "border-color 0.15s ease",
  width: "100%",
  boxSizing: "border-box",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
    },
  },
});

export const select = style({
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: 6,
  padding: "6px 10px",
  fontSize: 13,
  color: vars.color.textPrimary,
  transition: "border-color 0.15s ease",
  width: "100%",
  boxSizing: "border-box",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
    },
  },
});

export const formActions = style({
  display: "flex",
  gap: 10,
});

export const saveButton = style({
  background: vars.color.primary,
  border: "none",
  borderRadius: 7,
  padding: "8px 18px",
  fontSize: 14,
  fontWeight: 600,
  color: vars.color.primaryText,
  cursor: "pointer",
  transition: "opacity 0.15s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      opacity: 0.85,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const cancelButton = style({
  background: vars.color.hoverBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 7,
  padding: "8px 18px",
  fontSize: 14,
  color: vars.color.textSecondary,
  cursor: "pointer",
  transition: "all 0.15s ease",
  selectors: {
    "&:hover": {
      background: vars.color.accentHover,
      color: vars.color.textPrimary,
    },
  },
});

// ── Generate suggestions button row ───────────────────────────────────────────

export const generateButtonRow = style({
  display: "flex",
  alignItems: "center",
  gap: 8,
  flexWrap: "wrap",
});

export const generateButton = style({
  background: vars.color.panelBgSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 8,
  padding: "6px 14px",
  fontSize: 13,
  fontWeight: 500,
  color: vars.color.textPrimary,
  cursor: "pointer",
  transition: "all 0.15s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.accentHover,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const cancelGenerateButton = style({
  background: "none",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 8,
  padding: "6px 14px",
  fontSize: 13,
  color: vars.color.textSecondary,
  cursor: "pointer",
  transition: "all 0.15s ease",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
});

export const generateErrorBanner = style({
  padding: "8px 12px",
  background: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: 8,
  color: vars.color.errorText,
  fontSize: 13,
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: 10,
});

export const dismissErrorButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  color: vars.color.error,
  fontSize: 16,
  padding: "0 4px",
  lineHeight: 1,
  selectors: {
    "&:hover": {
      opacity: 0.7,
    },
  },
});

export const suggestionsContainer = style({
  display: "flex",
  flexDirection: "column",
  gap: 12,
});

// ── Command-sample generate section ───────────────────────────────────────────

export const commandSampleDetails = style({
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 8,
  padding: "10px 14px",
  background: vars.color.panelBgSecondary,
  fontSize: 13,
});

export const commandSampleSummary = style({
  cursor: "pointer",
  fontWeight: 500,
  color: vars.color.textSecondary,
  userSelect: "none",
  display: "flex",
  alignItems: "center",
  gap: 6,
  padding: "2px 0",
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
    },
    "&::marker": {
      display: "none",
    },
    "&::-webkit-details-marker": {
      display: "none",
    },
  },
  "::before": {
    content: '"▶"',
    fontSize: 10,
    transition: "transform 0.2s ease",
    display: "inline-block",
  },
});

// Rotate the ▶ caret when the details element is open.
globalStyle(`details[open] > summary${commandSampleSummary}::before`, {
  transform: "rotate(90deg)",
});

export const commandSampleBody = style({
  display: "flex",
  flexDirection: "column",
  gap: 8,
  marginTop: 10,
});

export const commandSampleTextarea = style({
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: 6,
  padding: "8px 10px",
  fontSize: 13,
  color: vars.color.textPrimary,
  fontFamily: vars.font.mono,
  resize: "vertical",
  minHeight: 60,
  width: "100%",
  boxSizing: "border-box",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
    },
  },
});

export const commandSampleActions = style({
  display: "flex",
  alignItems: "center",
  gap: 8,
});

export const aiGeneratedBadge = style({
  display: "inline-block",
  padding: "2px 10px",
  borderRadius: 4,
  fontSize: 11,
  fontWeight: 600,
  background: vars.color.warningBg,
  color: vars.color.warning,
  border: `1px solid ${vars.color.warning}`,
});

// ── Add Rule Modal ────────────────────────────────────────────────────────────
// Portal, overlay, and ARIA are handled by the shared Modal/ModalContent component.
// This class overrides only the properties that differ from ModalContent's defaults:
// wider max-width, taller max-height, no internal padding (modalHeader/modalBody handle it),
// and a flex-column layout so the header stays fixed and the body scrolls.
// The "&&" selector doubles specificity to reliably beat the base `content` class from
// Modal.css.ts regardless of CSS bundle injection order.
export const ruleModalContent = style({
  selectors: {
    "&&": {
      background: vars.color.cardBackground,
      width: "calc(100vw - 32px)",
      maxWidth: 720,
      maxHeight: "90vh",
      padding: 0,
      paddingBottom: 0,
      overflowY: "hidden",
      display: "flex",
      flexDirection: "column",
      boxShadow: "0 20px 60px rgba(0, 0, 0, 0.5)",
    },
  },
});

export const modalHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "16px 20px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  flexShrink: 0,
  gap: 12,
});

export const modalTitleRow = style({
  display: "flex",
  alignItems: "center",
  gap: 10,
});

export const modalBody = style({
  overflowY: "auto",
  padding: "16px 20px",
  flex: 1,
  display: "flex",
  flexDirection: "column",
  gap: 14,
});

export const modalCloseButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  color: vars.color.textSecondary,
  fontSize: 20,
  lineHeight: 1,
  padding: "4px 8px",
  borderRadius: 6,
  flexShrink: 0,
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
});

// ── Form sections (progressive disclosure in modal) ───────────────────────────

export const formSection = style({
  display: "flex",
  flexDirection: "column",
  gap: 12,
});

export const formSectionHeader = style({
  display: "flex",
  alignItems: "center",
  gap: 8,
  fontSize: 11,
  fontWeight: 700,
  letterSpacing: "0.06em",
  textTransform: "uppercase",
  color: vars.color.textMuted,
  paddingBottom: 4,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
});

// ── Priority hint text ────────────────────────────────────────────────────────

export const priorityHint = style({
  fontSize: 11,
  color: vars.color.textMuted,
  marginTop: 2,
  lineHeight: 1.4,
});

// ── Built-in enabled badge (static, non-interactive) ─────────────────────────

export const builtInBadge = style({
  display: "inline-block",
  padding: "3px 8px",
  borderRadius: 4,
  fontSize: 11,
  fontWeight: 500,
  background: vars.color.panelBgSecondary,
  color: vars.color.textMuted,
  border: `1px solid ${vars.color.borderSubtle}`,
  cursor: "default",
  whiteSpace: "nowrap",
});

// ── Mobile FAB (Floating Action Button) for "+ Add Rule" ─────────────────────

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

// On mobile, hide the header add-rule button and generate-suggestions button.
// They are replaced by the FAB and a less prominent location respectively.
export const headerButtonsHiddenOnMobile = style({
  "@media": {
    "screen and (max-width: 640px)": {
      display: "none",
    },
  },
});

// ── Tab label: show abbreviated text on mobile ────────────────────────────────

export const tabLabelShort = style({
  display: "none",
  "@media": {
    "screen and (max-width: 640px)": {
      display: "inline",
    },
  },
});

export const tabLabelFull = style({
  "@media": {
    "screen and (max-width: 640px)": {
      display: "none",
    },
  },
});

// ── Search bar ───────────────────────────────────────────────────────────────

export const searchBar = style({
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: 7,
  padding: "7px 12px",
  fontSize: 13,
  color: vars.color.textPrimary,
  width: "100%",
  boxSizing: "border-box",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
    },
    "&::placeholder": {
      color: vars.color.textMuted,
    },
  },
});

// ── Sortable column header ────────────────────────────────────────────────────

export const thSortable = style({
  cursor: "pointer",
  userSelect: "none",
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
    },
  },
});

// ── Hit count badge ───────────────────────────────────────────────────────────

export const hitBadge = style({
  display: "inline-block",
  padding: "2px 7px",
  borderRadius: 4,
  fontSize: 11,
  fontWeight: 600,
  background: vars.color.panelBgSecondary,
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  fontFamily: vars.font.mono,
});

export const hitBadgeActive = style({
  background: vars.color.successBg,
  color: vars.color.success,
  borderColor: "transparent",
});

// ── Config file source badge ──────────────────────────────────────────────────

export const configFileBadge = style({
  display: "inline-block",
  padding: "2px 8px",
  borderRadius: 4,
  fontSize: 11,
  fontWeight: 500,
  background: "rgba(59, 130, 246, 0.12)",
  color: vars.color.primary,
  border: "1px solid rgba(59, 130, 246, 0.3)",
  whiteSpace: "nowrap",
});

// ── Export-to-config-file inline action button ────────────────────────────────

export const exportConfigButton = style({
  background: "none",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 4,
  padding: "2px 7px",
  fontSize: 11,
  color: vars.color.textSecondary,
  cursor: "pointer",
  whiteSpace: "nowrap",
  transition: "all 0.15s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      background: "rgba(59, 130, 246, 0.12)",
      color: vars.color.primary,
      borderColor: "rgba(59, 130, 246, 0.3)",
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

// ── Config file path hint ─────────────────────────────────────────────────────

export const configFileHint = style({
  fontSize: 11,
  color: vars.color.textMuted,
  fontFamily: vars.font.mono,
  padding: "6px 10px",
  background: vars.color.panelBgSecondary,
  border: `1px solid ${vars.color.borderSubtle}`,
  borderRadius: 6,
  marginTop: 4,
});

// ── Row count indicator below table ──────────────────────────────────────────

export const rowCount = style({
  fontSize: 12,
  color: vars.color.textMuted,
  textAlign: "right",
  paddingTop: 6,
});
