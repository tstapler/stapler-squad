import { style } from "@vanilla-extract/css";

export const container = style({
  position: "relative",
  flex: 1,
  maxWidth: "400px",
});

export const inputWrapper = style({
  position: "relative",
  display: "flex",
  alignItems: "center",
});

export const input = style({
  width: "100%",
  padding: "0.5rem 2rem 0.5rem 0.5rem",
  backgroundColor: "#0a0a0a",
  border: "1px solid #444",
  color: "#e5e5e5",
  borderRadius: "4px",
  fontSize: "0.9rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "border-color 0.2s",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: "#17a2b8",
    },
    "&::placeholder": {
      color: "#666",
    },
  },
});

export const clearButton = style({
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

export const dropdown = style({
  position: "absolute",
  top: "100%",
  left: "0",
  right: "0",
  marginTop: "4px",
  backgroundColor: "#1a1a1a",
  border: "1px solid #333",
  borderRadius: "6px",
  boxShadow: "0 4px 12px rgba(0, 0, 0, 0.4)",
  zIndex: 100,
  overflow: "hidden",
});

export const header = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  padding: "0.5rem 0.75rem",
  borderBottom: "1px solid #333",
  fontSize: "0.75rem",
  color: "#666",
  textTransform: "uppercase",
});

export const clearAllButton = style({
  background: "none",
  border: "none",
  color: "#17a2b8",
  cursor: "pointer",
  fontSize: "0.75rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  textTransform: "uppercase",
  selectors: {
    "&:hover": {
      color: "#ff6b6b",
    },
  },
});

export const items = style({
  maxHeight: "250px",
  overflowY: "auto",
});

export const item = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  padding: "0.5rem 0.75rem",
  cursor: "pointer",
  transition: "background-color 0.15s",
  selectors: {
    "&:hover": {
      backgroundColor: "#2a2a2a",
    },
  },
});

export const itemSelected = style({
  backgroundColor: "#2a2a2a",
});

export const historyIcon = style({
  fontSize: "0.75rem",
  color: "#666",
});

export const query = style({
  flex: 1,
  color: "#e5e5e5",
  fontSize: "0.85rem",
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
});

export const timestamp = style({
  fontSize: "0.7rem",
  color: "#666",
  whiteSpace: "nowrap",
});

export const removeButton = style({
  background: "none",
  border: "none",
  color: "#666",
  cursor: "pointer",
  fontSize: "1rem",
  lineHeight: "1",
  padding: "0.125rem 0.25rem",
  opacity: 0,
  transition: "opacity 0.15s, color 0.15s",
  selectors: {
    [`${item}:hover &`]: {
      opacity: 1,
    },
    "&:hover": {
      color: "#ff6b6b",
    },
  },
});
