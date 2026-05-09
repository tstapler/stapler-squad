import { style } from "@vanilla-extract/css";

export const container = style({
  display: "flex",
  flexWrap: "wrap",
  gap: "0.5rem",
  alignItems: "center",
  marginBottom: "1rem",
});

export const pill = style({
  display: "inline-flex",
  alignItems: "center",
  gap: "0.25rem",
  padding: "0.25rem 0.5rem",
  backgroundColor: "#1a1a1a",
  border: "1px solid #444",
  borderRadius: "16px",
  fontSize: "0.8rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
});

export const label = style({
  color: "#999",
});

export const value = style({
  color: "#e5e5e5",
  fontWeight: 500,
});

export const removeButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "16px",
  height: "16px",
  marginLeft: "0.25rem",
  padding: "0",
  background: "none",
  border: "none",
  color: "#666",
  cursor: "pointer",
  borderRadius: "50%",
  fontSize: "1rem",
  lineHeight: "1",
  transition: "color 0.15s, background-color 0.15s",
  selectors: {
    "&:hover": {
      color: "#ff6b6b",
      backgroundColor: "rgba(255, 107, 107, 0.1)",
    },
    "&:focus": {
      outline: "none",
      boxShadow: "0 0 0 2px rgba(23, 162, 184, 0.3)",
    },
  },
});

export const clearAllButton = style({
  padding: "0.25rem 0.5rem",
  background: "none",
  border: "none",
  color: "#17a2b8",
  cursor: "pointer",
  fontSize: "0.8rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "color 0.15s",
  selectors: {
    "&:hover": {
      color: "#1dc3d8",
      textDecoration: "underline",
    },
    "&:focus": {
      outline: "none",
      textDecoration: "underline",
    },
  },
});
