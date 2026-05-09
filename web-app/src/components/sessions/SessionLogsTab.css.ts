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
  border: "1px solid #444",
  color: vars.color.terminalForeground,
  borderRadius: "4px",
  fontSize: "0.85rem",
  minWidth: "200px",
  fontFamily: "inherit",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: "#666",
    },
  },
});

export const refreshButton = style({
  padding: "0.4rem 0.75rem",
  backgroundColor: "#1a1a1a",
  border: "1px solid #333",
  color: "#e5e5e5",
  cursor: "pointer",
  borderRadius: "4px",
  fontSize: "0.85rem",
  transition: "background-color 0.15s",
  selectors: {
    "&:hover": {
      backgroundColor: "#2a2a2a",
    },
  },
});

export const status = style({
  padding: "1.5rem",
  textAlign: "center",
  color: "#999",
});

export const statusError = style({
  padding: "1rem",
  color: "#ff6b6b",
  backgroundColor: "rgba(255, 107, 107, 0.1)",
  border: "1px solid #ff6b6b",
  borderRadius: "4px",
  margin: "0.75rem",
});

export const empty = style({
  padding: "2rem",
  textAlign: "center",
  color: "#666",
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
  backgroundColor: "#1a1a1a",
  zIndex: 1,
});

globalStyle(`${table} th`, {
  padding: "0.5rem 0.75rem",
  textAlign: "left",
  borderBottom: "1px solid #333",
  fontWeight: 600,
  color: "#999",
  textTransform: "uppercase",
  fontSize: "0.75rem",
});

export const colTimestamp = style({ width: "130px" });
export const colLevel = style({ width: "80px" });
export const colSource = style({ width: "160px" });
export const colMessage = style({ flex: 1 });

export const row = style({
  borderBottom: "1px solid #111",
  transition: "background-color 0.1s",
  selectors: {
    "&:hover": {
      backgroundColor: "#1a1a1a",
    },
  },
});

export const timestamp = style({
  padding: "0.4rem 0.75rem",
  color: "#999",
  whiteSpace: "nowrap",
});

export const level = style({
  padding: "0.4rem 0.75rem",
  fontWeight: 600,
  whiteSpace: "nowrap",
});

export const source = style({
  padding: "0.4rem 0.75rem",
  color: "#888",
  fontSize: "0.8rem",
});

export const message = style({
  padding: "0.4rem 0.75rem",
  wordBreak: "break-word",
});

export const levelDebug = style({ color: "#6c757d" });
export const levelInfo = style({ color: "#17a2b8" });
export const levelWarning = style({ color: "#ffc107" });
export const levelError = style({ color: "#dc3545" });
export const levelFatal = style({ color: "#ff0000" });

export const loadMoreButton = style({
  display: "block",
  width: "100%",
  padding: "0.75rem",
  backgroundColor: "#1a1a1a",
  border: "none",
  borderTop: "1px solid #333",
  color: "#999",
  cursor: "pointer",
  fontSize: "0.85rem",
  transition: "background-color 0.15s",
  selectors: {
    "&:hover:not(:disabled)": {
      backgroundColor: "#2a2a2a",
      color: "#e5e5e5",
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "default",
    },
  },
});
