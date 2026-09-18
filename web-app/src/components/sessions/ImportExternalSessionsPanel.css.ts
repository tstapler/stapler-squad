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
});

export const titleRow = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  gap: 12,
  flexWrap: "wrap",
});

export const title = style({
  margin: 0,
  fontSize: 20,
  fontWeight: 700,
  color: vars.color.textPrimary,
});

export const subtitle = style({
  margin: "4px 0 0",
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
    "&:hover": { background: vars.color.accentHover },
  },
});

export const empty = style({
  padding: 32,
  textAlign: "center",
  color: vars.color.textSecondary,
  fontSize: 14,
  lineHeight: 1.6,
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

export const td = style({
  padding: "9px 12px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textPrimary,
  verticalAlign: "middle",
});

export const row = style({});

globalStyle(`${row}:last-child td`, { borderBottom: "none" });

export const checkboxTh = style({
  width: 32,
  padding: "6px 0 6px 10px",
  textAlign: "center",
});

export const checkboxTd = style({
  width: 32,
  padding: "0 0 0 10px",
  textAlign: "center",
  verticalAlign: "middle",
});

export const sourceBadge = style({
  fontSize: 11,
  fontWeight: 600,
  padding: "2px 8px",
  borderRadius: 999,
  background: vars.color.panelBgSecondary,
  color: vars.color.textSecondary,
});

export const pathText = style({
  fontFamily: "monospace",
  fontSize: 12,
  color: vars.color.textSecondary,
});

export const bulkActionBar = style({
  display: "flex",
  alignItems: "center",
  gap: 10,
  padding: "8px 12px",
  marginBottom: 8,
  background: vars.color.panelBgSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 8,
  fontSize: 13,
  color: vars.color.textSecondary,
});

export const bulkActionCount = style({
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const bulkImportBtn = style({
  fontSize: 12,
  fontWeight: 500,
  color: vars.color.primaryText,
  background: vars.color.primary,
  border: "none",
  borderRadius: 5,
  padding: "4px 12px",
  cursor: "pointer",
  transition: "background 0.15s ease",
  selectors: {
    "&:hover": { background: vars.color.primaryHover },
  },
});

export const bulkClearBtn = style({
  fontSize: 12,
  fontWeight: 500,
  color: vars.color.textSecondary,
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 5,
  padding: "4px 10px",
  cursor: "pointer",
  selectors: {
    "&:hover": { color: vars.color.textPrimary },
  },
});

export const rowImportBtn = style({
  fontSize: 12,
  fontWeight: 500,
  color: vars.color.primaryText,
  background: vars.color.primary,
  border: "none",
  borderRadius: 5,
  padding: "4px 10px",
  cursor: "pointer",
  selectors: {
    "&:hover": { background: vars.color.primaryHover },
  },
});
