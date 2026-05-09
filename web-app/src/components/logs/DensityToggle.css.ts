import { style } from "@vanilla-extract/css";

export const container = style({
  display: "flex",
  backgroundColor: "#1a1a1a",
  border: "1px solid #333",
  borderRadius: "4px",
  overflow: "hidden",
});

export const option = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "32px",
  height: "32px",
  padding: "0",
  background: "none",
  border: "none",
  borderRight: "1px solid #333",
  color: "#666",
  cursor: "pointer",
  fontSize: "0.9rem",
  transition: "all 0.15s",
  selectors: {
    "&:last-child": {
      borderRight: "none",
    },
    "&:hover": {
      backgroundColor: "#2a2a2a",
      color: "#999",
    },
  },
});

export const active = style({
  backgroundColor: "#17a2b8",
  color: "#fff",
  selectors: {
    "&:hover": {
      backgroundColor: "#138496",
    },
  },
});
