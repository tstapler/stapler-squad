import { style, keyframes, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const panel = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 12,
  padding: 20,
  display: "flex",
  flexDirection: "column",
  gap: 20,
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

export const windowSelector = style({
  display: "flex",
  gap: 6,
  flexWrap: "wrap",
});

export const windowBtn = style({
  background: vars.color.hoverBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 6,
  padding: "5px 14px",
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

export const windowBtnActive = style({
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

export const cards = style({
  display: "grid",
  gridTemplateColumns: "repeat(auto-fill, minmax(130px, 1fr))",
  gap: 12,
});

export const card = style({
  background: vars.color.panelBgSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 10,
  padding: "14px 16px",
  display: "flex",
  flexDirection: "column",
  gap: 2,
});

export const cardAllow = style({
  borderColor: "rgba(34, 197, 94, 0.3)",
  background: "rgba(34, 197, 94, 0.06)",
});

export const cardDeny = style({
  borderColor: "rgba(239, 68, 68, 0.3)",
  background: "rgba(239, 68, 68, 0.06)",
});

export const cardManual = style({
  borderColor: "rgba(234, 179, 8, 0.3)",
  background: "rgba(234, 179, 8, 0.06)",
});

export const cardValue = style({
  fontSize: 28,
  fontWeight: 700,
  color: vars.color.textPrimary,
  lineHeight: 1,
  selectors: {
    [`${cardAllow} &`]: { color: "#22c55e" },
    [`${cardDeny} &`]: { color: "#ef4444" },
    [`${cardManual} &`]: { color: "#eab308" },
  },
});

export const cardLabel = style({
  fontSize: 12,
  fontWeight: 500,
  color: vars.color.textSecondary,
  marginTop: 4,
});

export const cardSub = style({
  fontSize: 11,
  color: vars.color.textSecondary,
  opacity: 0.7,
});

export const loading = style({
  padding: 32,
  textAlign: "center",
  color: vars.color.textSecondary,
  fontSize: 14,
  lineHeight: 1.6,
});

export const empty = style({
  padding: 32,
  textAlign: "center",
  color: vars.color.textSecondary,
  fontSize: 14,
  lineHeight: 1.6,
});

export const emptyHint = style({
  fontSize: 12,
  opacity: 0.7,
});

export const sectionTitle = style({
  margin: "0 0 10px",
  fontSize: 15,
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const tableSection = style({
  display: "flex",
  flexDirection: "column",
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
});

export const th = style({
  padding: "9px 12px",
  textAlign: "left",
  fontWeight: 600,
  color: vars.color.textSecondary,
  background: vars.color.panelBgSecondary,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  whiteSpace: "nowrap",
});

export const thRight = style({
  textAlign: "right",
});

export const td = style({
  padding: "9px 12px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textPrimary,
  verticalAlign: "middle",
});

export const tdRight = style({
  textAlign: "right",
  whiteSpace: "nowrap",
});

export const tdBar = style({
  width: 120,
  minWidth: 80,
});

export const row = style({});

globalStyle(`${row}:last-child td`, { borderBottom: "none" });

export const allowCount = style({
  color: "#22c55e",
  fontWeight: 600,
});

export const denyCount = style({
  color: "#ef4444",
  fontWeight: 600,
});

export const manualCount = style({
  color: "#eab308",
  fontWeight: 600,
});

export const pctLabel = style({
  fontSize: 11,
  color: vars.color.textSecondary,
});

export const toolName = style({
  background: vars.color.terminalBackground,
  borderRadius: 4,
  padding: "1px 6px",
  fontSize: 12,
  fontFamily: vars.font.mono,
  color: vars.color.textPrimary,
});

export const ruleName = style({
  fontSize: 13,
  color: vars.color.textPrimary,
});

export const barTrack = style({
  height: 8,
  background: vars.color.panelBgSecondary,
  borderRadius: 4,
  overflow: "hidden",
});

export const barFill = style({
  height: "100%",
  borderRadius: 4,
  transition: "width 0.3s ease",
  minWidth: 2,
});

export const barTotal = style({
  background: vars.color.primary,
  opacity: 0.7,
});

export const barTool = style({
  background: "#8b5cf6",
  opacity: 0.7,
});

export const barRule = style({
  background: "#22c55e",
  opacity: 0.7,
});

export const barCmd = style({
  background: "#f59e0b",
  opacity: 0.7,
});

export const barPython = style({
  background: "#3b82f6",
  opacity: 0.7,
});

export const barGap = style({
  background: "#f97316",
  opacity: 0.8,
});

export const categoryBadge = style({
  display: "inline-block",
  fontSize: 11,
  padding: "1px 7px",
  borderRadius: 10,
  background: vars.color.panelBgSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textSecondary,
  whiteSpace: "nowrap",
});

export const subSectionTitle = style({
  margin: "14px 0 8px",
  fontSize: 13,
  fontWeight: 600,
  color: vars.color.textSecondary,
  textTransform: "uppercase",
  letterSpacing: "0.04em",
});

export const filterInput = style({
  width: "100%",
  padding: "7px 12px",
  fontSize: 13,
  color: vars.color.textPrimary,
  background: vars.color.panelBgSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 6,
  marginBottom: 8,
  outline: "none",
  boxSizing: "border-box",
  transition: "border-color 0.15s ease",
  selectors: {
    "&::placeholder": {
      color: vars.color.textSecondary,
      opacity: 0.7,
    },
    "&:focus": {
      borderColor: vars.color.primary,
    },
  },
});

export const addRuleLink = style({
  fontSize: 12,
  color: vars.color.primary,
  textDecoration: "none",
  whiteSpace: "nowrap",
  opacity: 0.85,
  selectors: {
    "&:hover": {
      opacity: 1,
      textDecoration: "underline",
    },
  },
});

export const coverageGapHeader = style({
  borderRadius: 10,
  padding: "14px 16px",
  marginBottom: 12,
  border: "1px solid",
});

export const coverageGapHigh = style({
  background: "rgba(249, 115, 22, 0.08)",
  borderColor: "rgba(249, 115, 22, 0.35)",
});

export const coverageGapMed = style({
  background: "rgba(234, 179, 8, 0.08)",
  borderColor: "rgba(234, 179, 8, 0.35)",
});

export const coverageGapLow = style({
  background: "rgba(34, 197, 94, 0.06)",
  borderColor: "rgba(34, 197, 94, 0.25)",
});

export const coverageGapTitleRow = style({
  display: "flex",
  alignItems: "center",
  gap: 8,
  marginBottom: 6,
});

export const coverageGapIcon = style({
  fontSize: 15,
  lineHeight: 1,
});

export const coverageGapTitle = style({
  margin: 0,
  fontSize: 15,
  fontWeight: 600,
  color: vars.color.textPrimary,
  flex: 1,
});

export const coverageGapBadge = style({
  fontSize: 12,
  fontWeight: 600,
  padding: "2px 8px",
  borderRadius: 10,
  background: vars.color.panelBgSecondary,
  color: vars.color.textPrimary,
});

export const coverageGapDesc = style({
  margin: 0,
  fontSize: 13,
  color: vars.color.textSecondary,
  lineHeight: 1.5,
});
