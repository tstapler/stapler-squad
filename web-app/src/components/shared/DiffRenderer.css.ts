import { style } from "@vanilla-extract/css";

// DiffViewer uses a dark code-editor aesthetic regardless of theme.
// Terminal/code colors are not in the theme contract, so we use literal values.

export const container = style({
  display: "flex",
  flexDirection: "column",
  height: "100%",
  background: "#1e1e1e",
  color: "#d4d4d4",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'source-code-pro', monospace",
});

export const toolbar = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "0.75rem 1rem",
  background: "#2d2d30",
  borderBottom: "1px solid #3e3e42",
  flexShrink: 0,
  "@media": {
    "(max-width: 768px)": {
      flexDirection: "column",
      alignItems: "stretch",
      gap: "0.75rem",
      padding: "0.5rem 0.75rem",
    },
  },
});

export const stats = style({
  display: "flex",
  alignItems: "center",
  gap: "1rem",
  fontSize: "0.875rem",
  "@media": {
    "(max-width: 768px)": {
      justifyContent: "center",
    },
  },
});

export const filesChanged = style({
  color: "#cccccc",
});

export const additions = style({
  color: "#4ec9b0",
  fontWeight: 500,
});

export const deletions = style({
  color: "#f48771",
  fontWeight: 500,
});

export const viewModeToggle = style({
  display: "flex",
  gap: "0.25rem",
  background: "#3e3e42",
  borderRadius: "4px",
  padding: "0.25rem",
  "@media": {
    "(max-width: 768px)": {
      justifyContent: "center",
    },
  },
});

export const viewModeButton = style({
  padding: "0.4rem 0.75rem",
  background: "transparent",
  border: "none",
  borderRadius: "3px",
  color: "#cccccc",
  fontSize: "0.875rem",
  cursor: "pointer",
  transition: "background 0.2s, color 0.2s",
  selectors: {
    "&:hover": { background: "#505050" },
  },
});

export const viewModeButtonActive = style({
  background: "#0e639c",
  color: "white",
});

export const diffContent = style({
  flex: 1,
  overflowY: "auto",
  padding: "1rem",
  selectors: {
    "&::-webkit-scrollbar": { width: "12px", height: "12px" },
    "&::-webkit-scrollbar-track": { background: "#1e1e1e" },
    "&::-webkit-scrollbar-thumb": {
      background: "#424242",
      borderRadius: "6px",
      border: "2px solid #1e1e1e",
    },
    "&::-webkit-scrollbar-thumb:hover": { background: "#4e4e4e" },
  },
  "@media": {
    "(max-width: 768px)": {
      padding: "0.75rem",
    },
  },
});

export const file = style({
  marginBottom: "1.5rem",
  border: "1px solid #3e3e42",
  borderRadius: "6px",
  overflow: "hidden",
});

export const fileHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "0.75rem 1rem",
  background: "#2d2d30",
  borderBottom: "1px solid #3e3e42",
  "@media": {
    "(max-width: 768px)": {
      flexDirection: "column",
      alignItems: "flex-start",
      gap: "0.5rem",
    },
  },
});

export const filename = style({
  fontWeight: 600,
  color: "#dcdcaa",
});

export const fileStats = style({
  display: "flex",
  gap: "0.75rem",
  fontSize: "0.875rem",
});

export const hunk = style({
  borderTop: "1px solid #3e3e42",
});

export const hunkHeader = style({
  padding: "0.5rem 1rem",
  background: "#37373d",
  color: "#8c8c8c",
  fontSize: "0.875rem",
  fontFamily: "inherit",
});

export const lines = style({
  background: "#1e1e1e",
});

export const line = style({
  display: "flex",
  alignItems: "center",
  fontSize: "13px",
  lineHeight: 1.6,
  borderLeft: "3px solid transparent",
});

export const lineAdd = style({
  background: "rgba(78, 201, 176, 0.1)",
  borderLeftColor: "#4ec9b0",
});

export const lineDelete = style({
  background: "rgba(244, 135, 113, 0.1)",
  borderLeftColor: "#f48771",
});

export const lineContext = style({
  background: "#1e1e1e",
});

export const lineNumber = style({
  display: "inline-block",
  width: "50px",
  padding: "0 0.75rem",
  textAlign: "right",
  color: "#858585",
  userSelect: "none",
  flexShrink: 0,
  "@media": {
    "(max-width: 768px)": {
      width: "40px",
      padding: "0 0.5rem",
    },
  },
});

export const lineContent = style({
  padding: "0 1rem",
  flex: 1,
  whiteSpace: "pre",
  overflowX: "auto",
  selectors: {
    "&::-webkit-scrollbar": { width: "12px", height: "12px" },
    "&::-webkit-scrollbar-track": { background: "#1e1e1e" },
    "&::-webkit-scrollbar-thumb": {
      background: "#424242",
      borderRadius: "6px",
      border: "2px solid #1e1e1e",
    },
    "&::-webkit-scrollbar-thumb:hover": { background: "#4e4e4e" },
  },
  "@media": {
    "(max-width: 768px)": {
      padding: "0 0.5rem",
      fontSize: "12px",
    },
  },
});

export const loading = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  padding: "2rem",
  textAlign: "center",
});

export const empty = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  padding: "2rem",
  textAlign: "center",
  color: "#8c8c8c",
});

export const emptyHint = style({
  marginTop: "0.5rem",
  fontSize: "0.875rem",
  color: "#9ca3af",
});

export const errorState = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  padding: "2rem",
  textAlign: "center",
  color: "#f48771",
});
