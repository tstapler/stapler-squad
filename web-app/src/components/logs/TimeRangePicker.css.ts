import { style, globalStyle } from "@vanilla-extract/css";

export const container = style({
  position: "relative",
  display: "inline-block",
});

export const trigger = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  padding: "0.5rem 0.75rem",
  backgroundColor: "#1a1a1a",
  border: "1px solid #333",
  color: "#e5e5e5",
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "0.9rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "border-color 0.2s, background-color 0.2s",
  selectors: {
    "&:hover": {
      backgroundColor: "#2a2a2a",
      borderColor: "#444",
    },
    "&:focus": {
      outline: "none",
      borderColor: "#17a2b8",
      boxShadow: "0 0 0 2px rgba(23, 162, 184, 0.2)",
    },
  },
});

export const icon = style({
  fontSize: "1rem",
});

export const label = style({
  flex: 1,
  textAlign: "left",
  minWidth: "120px",
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
  minWidth: "200px",
  overflow: "hidden",
});

export const presets = style({
  display: "flex",
  flexDirection: "column",
  padding: "0.5rem",
});

export const presetButton = style({
  padding: "0.5rem 0.75rem",
  background: "none",
  border: "none",
  color: "#e5e5e5",
  textAlign: "left",
  cursor: "pointer",
  borderRadius: "4px",
  fontSize: "0.85rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "background-color 0.15s",
  selectors: {
    "&:hover": {
      backgroundColor: "#2a2a2a",
    },
    "&:focus": {
      outline: "none",
      boxShadow: "inset 0 0 0 2px rgba(23, 162, 184, 0.5)",
    },
  },
});

export const presetButtonActive = style({
  backgroundColor: "#17a2b8",
  color: "#fff",
});

export const divider = style({
  height: "1px",
  backgroundColor: "#333",
  margin: "0.25rem 0",
});

export const customButton = style({
  width: "100%",
  padding: "0.5rem 0.75rem",
  background: "none",
  border: "none",
  color: "#17a2b8",
  textAlign: "left",
  cursor: "pointer",
  fontSize: "0.85rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "background-color 0.15s",
  selectors: {
    "&:hover": {
      backgroundColor: "#2a2a2a",
    },
  },
});

export const customRange = style({
  padding: "0.75rem",
});

export const customHeader = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  marginBottom: "0.75rem",
  fontSize: "0.85rem",
  color: "#999",
});

export const backButton = style({
  background: "none",
  border: "none",
  color: "#17a2b8",
  cursor: "pointer",
  fontSize: "0.85rem",
  padding: "0",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  selectors: {
    "&:hover": {
      textDecoration: "underline",
    },
  },
});

export const customInputs = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.5rem",
  marginBottom: "0.75rem",
});

export const inputGroup = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.25rem",
});

globalStyle(`${inputGroup} span`, { fontSize: "0.75rem", color: "#999" });

export const dateInput = style({
  padding: "0.5rem",
  backgroundColor: "#0a0a0a",
  border: "1px solid #444",
  color: "#e5e5e5",
  borderRadius: "4px",
  fontSize: "0.85rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: "#17a2b8",
    },
  },
});

export const applyButton = style({
  width: "100%",
  padding: "0.5rem",
  backgroundColor: "#17a2b8",
  border: "none",
  color: "#fff",
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "0.85rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "background-color 0.2s",
  selectors: {
    "&:hover:not(:disabled)": {
      backgroundColor: "#138496",
    },
    "&:disabled": {
      backgroundColor: "#333",
      color: "#666",
      cursor: "not-allowed",
    },
  },
});
