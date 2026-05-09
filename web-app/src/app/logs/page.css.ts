import { style } from "@vanilla-extract/css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  height: "var(--viewport-height, 100dvh)",
  overflow: "hidden",
  padding: "1.5rem",
  backgroundColor: "#0a0a0a",
  color: "#e5e5e5",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
});

export const header = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  marginBottom: "1.5rem",
});

export const headerActions = style({
  display: "flex",
  alignItems: "center",
  gap: "1rem",
});

export const timezone = style({
  padding: "0.5rem 1rem",
  backgroundColor: "#1a1a1a",
  border: "1px solid #333",
  borderRadius: "4px",
  fontSize: "0.85rem",
  color: "#999",
  cursor: "help",
});

export const refreshButton = style({
  padding: "0.5rem 1rem",
  backgroundColor: "#1a1a1a",
  border: "1px solid #333",
  color: "#e5e5e5",
  cursor: "pointer",
  borderRadius: "4px",
  fontSize: "0.9rem",
  transition: "background-color 0.2s",
  selectors: {
    "&:hover": {
      backgroundColor: "#2a2a2a",
    },
  },
});

export const filters = style({
  display: "flex",
  gap: "1rem",
  marginBottom: "1.5rem",
  padding: "1rem",
  backgroundColor: "#1a1a1a",
  borderRadius: "6px",
  border: "1px solid #333",
  flexWrap: "wrap",
});

export const filterGroup = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
});

export const searchInput = style({
  padding: "0.5rem",
  backgroundColor: "#0a0a0a",
  border: "1px solid #444",
  color: "#e5e5e5",
  borderRadius: "4px",
  fontSize: "0.9rem",
  minWidth: "300px",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: "#666",
    },
  },
});

export const searchButton = style({
  padding: "0.5rem 1rem",
  backgroundColor: "#2a2a2a",
  border: "1px solid #444",
  color: "#e5e5e5",
  cursor: "pointer",
  borderRadius: "4px",
  fontSize: "0.9rem",
  transition: "background-color 0.2s",
  selectors: {
    "&:hover": {
      backgroundColor: "#3a3a3a",
    },
  },
});

export const select = style({
  padding: "0.5rem",
  backgroundColor: "#0a0a0a",
  border: "1px solid #444",
  color: "#e5e5e5",
  borderRadius: "4px",
  fontSize: "0.9rem",
  cursor: "pointer",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: "#666",
    },
  },
});

export const loading = style({
  padding: "2rem",
  textAlign: "center",
  backgroundColor: "#1a1a1a",
  borderRadius: "6px",
  border: "1px solid #333",
});

export const error = style({
  padding: "2rem",
  textAlign: "center",
  backgroundColor: "#1a1a1a",
  borderRadius: "6px",
  border: "1px solid #ff6b6b",
  color: "#ff6b6b",
});

export const noLogs = style({
  padding: "3rem 2rem",
  textAlign: "center",
  backgroundColor: "#1a1a1a",
  borderRadius: "6px",
  border: "1px solid #333",
});

export const logsContainer = style({
  flex: 1,
  overflow: "auto",
  backgroundColor: "#0a0a0a",
  borderRadius: "6px",
  border: "1px solid #333",
});

export const logsTable = style({
  width: "100%",
  borderCollapse: "collapse",
  fontSize: "0.85rem",
});

export const timestampColumn = style({
  width: "200px",
});

export const levelColumn = style({
  width: "100px",
});

export const sourceColumn = style({
  width: "200px",
});

export const messageColumn = style({
  flex: 1,
});

export const logRow = style({
  borderBottom: "1px solid #1a1a1a",
  transition: "background-color 0.1s",
  selectors: {
    "&:hover": {
      backgroundColor: "#1a1a1a",
    },
  },
});

export const logRowExpanded = style({
  backgroundColor: "#1a1a1a",
});

export const timestamp = style({
  padding: "0.75rem",
  color: "#999",
  whiteSpace: "nowrap",
});

export const level = style({
  padding: "0.75rem",
  fontWeight: 600,
  textTransform: "uppercase",
  whiteSpace: "nowrap",
});

export const levelDebug = style({ color: "#6c757d" });
export const levelInfo = style({ color: "#17a2b8" });
export const levelWarning = style({ color: "#ffc107" });
export const levelError = style({ color: "#dc3545" });
export const levelFatal = style({ color: "#ff0000", fontWeight: 700 });

export const source = style({
  padding: "0.75rem",
  color: "#666",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  fontSize: "0.8rem",
});

export const message = style({
  padding: "0.75rem",
  color: "#e5e5e5",
  wordBreak: "break-word",
  lineHeight: "1.4",
});

export const loadingMore = style({
  padding: "1.5rem",
  textAlign: "center",
  color: "#999",
  fontSize: "0.9rem",
  backgroundColor: "#1a1a1a",
  borderTop: "1px solid #333",
});

export const endOfLogs = style({
  padding: "1.5rem",
  textAlign: "center",
  color: "#666",
  fontSize: "0.85rem",
  backgroundColor: "#0a0a0a",
  borderTop: "1px solid #333",
  fontStyle: "italic",
});

export const searchWrapper = style({
  position: "relative",
  display: "flex",
  alignItems: "center",
});

export const clearSearch = style({
  position: "absolute",
  right: "8px",
  background: "none",
  border: "none",
  color: "#666",
  cursor: "pointer",
  fontSize: "1.2rem",
  lineHeight: "1",
  padding: "0.25rem",
  transition: "color 0.15s",
  selectors: {
    "&:hover": {
      color: "#ff6b6b",
    },
  },
});

export const expandColumn = style({
  width: "40px",
  textAlign: "center",
});

export const expandCell = style({
  textAlign: "center",
  padding: "0.5rem",
});

export const expandButton = style({
  background: "none",
  border: "none",
  color: "#666",
  cursor: "pointer",
  fontSize: "0.75rem",
  padding: "0.25rem",
  transition: "color 0.15s",
  selectors: {
    "&:hover": {
      color: "#17a2b8",
    },
  },
});

export const actionsColumn = style({
  width: "50px",
  textAlign: "center",
});

export const actionsCell = style({
  textAlign: "center",
  padding: "0.5rem",
});

export const actionButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  fontSize: "0.9rem",
  padding: "0.25rem",
  opacity: 0.5,
  transition: "opacity 0.15s",
  selectors: {
    "&:hover": {
      opacity: 1,
    },
  },
});

export const clickable = style({
  cursor: "pointer",
  position: "relative",
  selectors: {
    "&:hover": {
      textDecoration: "underline",
    },
  },
});

export const filterIcon = style({
  fontSize: "0.7rem",
  marginLeft: "0.25rem",
  opacity: 0,
  transition: "opacity 0.15s",
});

export const expandedRow = style({});

export const logDetail = style({
  padding: "1rem 1.5rem",
});

export const logDetailSection = style({
  marginBottom: "0.75rem",
  display: "flex",
  gap: "0.75rem",
});

export const logDetailMessage = style({
  margin: 0,
  padding: "0.75rem",
  backgroundColor: "#0a0a0a",
  borderRadius: "4px",
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
  fontSize: "0.85rem",
  lineHeight: "1.5",
  overflowX: "auto",
  maxHeight: "300px",
  overflowY: "auto",
});

export const footer = style({
  marginTop: "1rem",
  padding: "0.75rem 1rem",
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  backgroundColor: "#1a1a1a",
  borderRadius: "6px",
  border: "1px solid #333",
  fontSize: "0.85rem",
  color: "#999",
});

export const shortcuts = style({
  display: "flex",
  gap: "0.75rem",
  alignItems: "center",
  fontSize: "0.75rem",
  color: "#666",
});

export const liveTailStatus = style({
  color: "#17a2b8",
  fontWeight: 500,
});

export const searchHistoryWrapper = style({
  minWidth: "300px",
});

export const densityCompact = style({});
export const densityComfortable = style({});
export const densitySpacious = style({});
