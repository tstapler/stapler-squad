import { style } from "@vanilla-extract/css";

export const container = style({
  position: "relative",
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
});

export const label = style({
  fontSize: "0.85rem",
  fontWeight: 500,
  color: "#999",
});

export const trigger = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  padding: "0.5rem",
  backgroundColor: "#0a0a0a",
  border: "1px solid #444",
  color: "#e5e5e5",
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "0.9rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "border-color 0.2s",
  minWidth: "120px",
  selectors: {
    "&:hover": {
      borderColor: "#666",
    },
    "&:focus": {
      outline: "none",
      borderColor: "#17a2b8",
    },
  },
});

export const text = style({
  flex: 1,
  textAlign: "left",
});

export const chevron = style({
  fontSize: "0.7rem",
  color: "#666",
});

export const dropdown = style({
  position: "absolute",
  top: "100%",
  left: "0",
  marginTop: "4px",
  backgroundColor: "#1a1a1a",
  border: "1px solid #333",
  borderRadius: "6px",
  boxShadow: "0 4px 12px rgba(0, 0, 0, 0.4)",
  zIndex: 100,
  minWidth: "160px",
  overflow: "hidden",
});

export const actions = style({
  display: "flex",
  gap: "0.5rem",
  padding: "0.5rem",
});

export const actionButton = style({
  flex: 1,
  padding: "0.25rem 0.5rem",
  background: "none",
  border: "1px solid #444",
  color: "#17a2b8",
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "0.75rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "background-color 0.15s",
  selectors: {
    "&:hover": {
      backgroundColor: "#2a2a2a",
    },
  },
});

export const divider = style({
  height: "1px",
  backgroundColor: "#333",
});

export const options = style({
  display: "flex",
  flexDirection: "column",
  padding: "0.5rem",
  maxHeight: "200px",
  overflowY: "auto",
});

export const option = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  padding: "0.5rem",
  cursor: "pointer",
  borderRadius: "4px",
  transition: "background-color 0.15s",
  selectors: {
    "&:hover": {
      backgroundColor: "#2a2a2a",
    },
  },
});

export const checkbox = style({
  width: "16px",
  height: "16px",
  accentColor: "#17a2b8",
  cursor: "pointer",
});

export const optionLabel = style({
  fontSize: "0.85rem",
  fontWeight: 500,
  textTransform: "uppercase",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
});
