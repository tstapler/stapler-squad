import { style, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  height: "100%",
  minHeight: 0,
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  fontSize: "0.85rem",
});

export const toolbar = style({
  display: "flex",
  alignItems: "center",
  gap: "0.75rem",
  padding: "0.75rem",
  borderBottom: `1px solid ${vars.color.terminalBorder}`,
  flexWrap: "wrap",
});

export const searchInput = style({
  padding: "0.4rem 0.6rem",
  backgroundColor: vars.color.terminalBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  color: vars.color.terminalForeground,
  borderRadius: "4px",
  fontSize: "0.85rem",
  minWidth: "200px",
  fontFamily: "inherit",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.inputFocusBorder,
    },
  },
});

export const refreshButton = style({
  padding: "0.4rem 0.75rem",
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textPrimary,
  cursor: "pointer",
  borderRadius: "4px",
  fontSize: "0.85rem",
  transition: "background-color 0.15s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const status = style({
  padding: "1.5rem",
  textAlign: "center",
  color: vars.color.textMuted,
});

export const statusError = style({
  padding: "1rem",
  color: vars.color.error,
  backgroundColor: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: "4px",
  margin: "0.75rem",
});

export const empty = style({
  padding: "2rem",
  textAlign: "center",
  color: vars.color.textMuted,
});

export const tableWrapper = style({
  flex: 1,
  overflow: "auto",
  minHeight: 0,
});

export const table = style({
  width: "100%",
  borderCollapse: "collapse",
});

globalStyle(`${table} thead`, {
  position: "sticky",
  top: 0,
  backgroundColor: vars.color.cardBackground,
  zIndex: 1,
});

globalStyle(`${table} th`, {
  padding: "0.5rem 0.75rem",
  textAlign: "left",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  fontWeight: 600,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  fontSize: "0.75rem",
});

export const colTimestamp = style({ width: "130px" });
export const colLevel = style({ width: "80px" });
export const colSource = style({ width: "160px" });
export const colMessage = style({ flex: 1 });

export const row = style({
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  transition: "background-color 0.1s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const timestamp = style({
  padding: "0.4rem 0.75rem",
  color: vars.color.textMuted,
  whiteSpace: "nowrap",
});

export const level = style({
  padding: "0.4rem 0.75rem",
  fontWeight: 600,
  whiteSpace: "nowrap",
});

export const source = style({
  padding: "0.4rem 0.75rem",
  color: vars.color.textMuted,
  fontSize: "0.8rem",
});

export const message = style({
  padding: "0.4rem 0.75rem",
  wordBreak: "break-word",
});

export const levelDebug = style({ color: vars.color.textMuted });
export const levelInfo = style({ color: vars.color.primary });
export const levelWarning = style({ color: vars.color.warning });
export const levelError = style({ color: vars.color.error });
export const levelFatal = style({ color: vars.color.error });

export const loadMoreButton = style({
  display: "block",
  width: "100%",
  padding: "0.75rem",
  backgroundColor: vars.color.cardBackground,
  border: "none",
  borderTop: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textMuted,
  cursor: "pointer",
  fontSize: "0.85rem",
  transition: "background-color 0.15s",
  selectors: {
    "&:hover:not(:disabled)": {
      backgroundColor: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "default",
    },
  },
});
