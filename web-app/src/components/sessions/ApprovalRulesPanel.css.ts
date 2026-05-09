import { style, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const panel = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 12,
  padding: 20,
  display: "flex",
  flexDirection: "column",
  gap: 16,
  minHeight: 0,
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
  color: vars.color.textSecondary,
  fontWeight: 500,
});

export const analyticsRate = style({
  fontWeight: 600,
  padding: "2px 8px",
  borderRadius: 4,
});

export const rateAllow = style({
  background: "rgba(34, 197, 94, 0.15)",
  color: "#22c55e",
});

export const rateManual = style({
  background: "rgba(234, 179, 8, 0.15)",
  color: "#eab308",
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
  color: "#fff",
});

export const error = style({
  padding: 12,
  background: "rgba(239, 68, 68, 0.1)",
  border: "1px solid rgba(239, 68, 68, 0.3)",
  borderRadius: 8,
  color: "#ef4444",
  fontSize: 13,
  display: "flex",
  alignItems: "center",
  gap: 10,
});

export const retryButton = style({
  background: "rgba(239, 68, 68, 0.2)",
  border: "none",
  borderRadius: 4,
  padding: "3px 8px",
  cursor: "pointer",
  color: "#ef4444",
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
  color: "#22c55e",
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
  background: "rgba(34, 197, 94, 0.15)",
  color: "#22c55e",
});

export const decisionDeny = style({
  background: "rgba(239, 68, 68, 0.15)",
  color: "#ef4444",
});

export const decisionEscalate = style({
  background: "rgba(234, 179, 8, 0.15)",
  color: "#eab308",
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
  background: "rgba(34, 197, 94, 0.2)",
  color: "#22c55e",
});

export const toggleOff = style({
  background: "rgba(255,255,255,0.06)",
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
      background: "rgba(239, 68, 68, 0.15)",
      color: "#ef4444",
    },
  },
});

export const formSection = style({
  borderTop: `1px solid ${vars.color.borderColor}`,
  paddingTop: 16,
});

export const addButton = style({
  background: vars.color.primary,
  border: "none",
  borderRadius: 8,
  padding: "8px 16px",
  fontSize: 14,
  fontWeight: 600,
  color: "#fff",
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
  background: "rgba(239, 68, 68, 0.1)",
  border: "1px solid rgba(239, 68, 68, 0.3)",
  borderRadius: 6,
  color: "#ef4444",
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
  color: "#fff",
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
