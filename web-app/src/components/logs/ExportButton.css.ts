import { style } from "@vanilla-extract/css";

export const container = style({
  position: "relative",
});

export const button = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  padding: "0.5rem 0.75rem",
  backgroundColor: "#1a1a1a",
  border: "1px solid #444",
  color: "#e5e5e5",
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "0.85rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "all 0.2s",
  selectors: {
    "&:hover:not(:disabled)": {
      backgroundColor: "#2a2a2a",
      borderColor: "#666",
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const dropdown = style({
  position: "absolute",
  top: "100%",
  right: "0",
  marginTop: "4px",
  backgroundColor: "#1a1a1a",
  border: "1px solid #333",
  borderRadius: "6px",
  boxShadow: "0 4px 12px rgba(0, 0, 0, 0.4)",
  zIndex: 100,
  minWidth: "180px",
  overflow: "hidden",
});

export const option = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  padding: "0.625rem 0.75rem",
  background: "none",
  border: "none",
  color: "#e5e5e5",
  cursor: "pointer",
  fontSize: "0.85rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  width: "100%",
  textAlign: "left",
  transition: "background-color 0.15s",
  selectors: {
    "&:hover": {
      backgroundColor: "#2a2a2a",
    },
  },
});

export const icon = style({
  fontSize: "0.9rem",
  width: "20px",
  textAlign: "center",
});

export const optionLabel = style({
  fontWeight: 500,
});

export const optionDesc = style({
  fontSize: "0.75rem",
  color: "#666",
  marginLeft: "auto",
});

export const divider = style({
  height: "1px",
  backgroundColor: "#333",
  margin: "0.25rem 0",
});
