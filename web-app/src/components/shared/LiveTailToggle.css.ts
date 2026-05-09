import { style, keyframes } from "@vanilla-extract/css";

const pulseKeyframes = keyframes({
  "0%, 100%": {
    opacity: 1,
    transform: "scale(1)",
  },
  "50%": {
    opacity: 0.6,
    transform: "scale(1.2)",
  },
});

export const container = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  position: "relative",
});

export const toggleButton = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  padding: "0.5rem 0.75rem",
  backgroundColor: "#1a1a1a",
  border: "1px solid #444",
  color: "#999",
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "0.85rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "all 0.2s",
  selectors: {
    "&:hover": {
      backgroundColor: "#2a2a2a",
      borderColor: "#666",
      color: "#e5e5e5",
    },
  },
});

export const toggleButtonActive = style({
  backgroundColor: "rgba(23, 162, 184, 0.15)",
  borderColor: "#17a2b8",
  color: "#17a2b8",
  selectors: {
    "&:hover": {
      backgroundColor: "rgba(23, 162, 184, 0.25)",
    },
  },
});

export const indicator = style({
  width: "8px",
  height: "8px",
  borderRadius: "50%",
  backgroundColor: "#666",
  transition: "background-color 0.2s",
});

export const indicatorActive = style({
  backgroundColor: "#17a2b8",
});

export const indicatorPulse = style({
  animation: `${pulseKeyframes} 1.5s ease-in-out infinite`,
});

export const label = style({
  fontWeight: 500,
});

export const pauseButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "32px",
  height: "32px",
  padding: "0",
  backgroundColor: "#1a1a1a",
  border: "1px solid #444",
  color: "#999",
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "0.85rem",
  transition: "all 0.2s",
  selectors: {
    "&:hover": {
      backgroundColor: "#2a2a2a",
      borderColor: "#666",
      color: "#e5e5e5",
    },
    "&:focus": {
      outline: "none",
      borderColor: "#17a2b8",
    },
  },
});

export const settingsButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "32px",
  height: "32px",
  padding: "0",
  backgroundColor: "#1a1a1a",
  border: "1px solid #444",
  color: "#999",
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "0.85rem",
  transition: "all 0.2s",
  selectors: {
    "&:hover": {
      backgroundColor: "#2a2a2a",
      borderColor: "#666",
      color: "#e5e5e5",
    },
    "&:focus": {
      outline: "none",
      borderColor: "#17a2b8",
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
  minWidth: "140px",
  overflow: "hidden",
});

export const dropdownHeader = style({
  padding: "0.5rem 0.75rem",
  fontSize: "0.75rem",
  fontWeight: 600,
  color: "#666",
  textTransform: "uppercase",
  borderBottom: "1px solid #333",
});

export const options = style({
  display: "flex",
  flexDirection: "column",
  padding: "0.25rem",
});

export const option = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "0.5rem 0.75rem",
  background: "none",
  border: "none",
  color: "#e5e5e5",
  cursor: "pointer",
  fontSize: "0.85rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  borderRadius: "4px",
  transition: "background-color 0.15s",
  textAlign: "left",
  width: "100%",
  selectors: {
    "&:hover": {
      backgroundColor: "#2a2a2a",
    },
  },
});

export const optionSelected = style({
  color: "#17a2b8",
});

export const check = style({
  fontSize: "0.8rem",
});

export const lastUpdate = style({
  fontSize: "0.75rem",
  color: "#666",
  whiteSpace: "nowrap",
});
