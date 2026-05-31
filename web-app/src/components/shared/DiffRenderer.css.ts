import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// DiffViewer uses a dark code-editor aesthetic regardless of theme.
// Terminal tokens from the theme contract are used for structural colors.

export const container = style({
  display: "flex",
  flexDirection: "column",
  height: "100%",
  background: vars.color.terminalBackground,
  color: vars.color.terminalForeground,
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'source-code-pro', monospace",
});

export const toolbar = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "0.75rem 1rem",
  background: vars.color.terminalHeaderBg,
  borderBottom: `1px solid ${vars.color.terminalBorder}`,
  flexShrink: 0,
  "@media": {
    "(max-width: 768px)": {
      flexDirection: "column",
      alignItems: "stretch",
      gap: "0.75rem",
      padding: "0.5rem 0.75rem",
    },
  },
  selectors: {
    // Left-handed: flip the row so Refresh + view-mode buttons land on the left thumb side
    ':root[data-left-handed] &': {
      flexDirection: "row-reverse",
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
  color: vars.color.terminalForeground,
});

export const additions = style({
  color: vars.color.success,
  fontWeight: 500,
});

export const deletions = style({
  color: vars.color.error,
  fontWeight: 500,
});

export const viewModeToggle = style({
  display: "flex",
  gap: "0.25rem",
  background: vars.color.terminalHoverBg,
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
  color: vars.color.terminalForeground,
  fontSize: "0.875rem",
  cursor: "pointer",
  transition: "background 0.2s, color 0.2s",
  selectors: {
    "&:hover": { background: vars.color.terminalHoverBg },
  },
});

export const viewModeButtonActive = style({
  background: vars.color.primary,
  color: vars.color.primaryText,
});

export const diffContent = style({
  flex: 1,
  overflowY: "auto",
  padding: "1rem",
  selectors: {
    "&::-webkit-scrollbar": { width: "12px", height: "12px" },
    "&::-webkit-scrollbar-track": { background: vars.color.terminalBackground },
    "&::-webkit-scrollbar-thumb": {
      background: vars.color.terminalHoverBg,
      borderRadius: "6px",
      border: `2px solid ${vars.color.terminalBackground}`,
    },
    "&::-webkit-scrollbar-thumb:hover": { background: vars.color.terminalBorder },
  },
  "@media": {
    "(max-width: 768px)": {
      padding: "0.75rem",
    },
  },
});

export const file = style({
  marginBottom: "1.5rem",
  border: `1px solid ${vars.color.terminalBorder}`,
  borderRadius: "6px",
  overflow: "hidden",
});

export const fileHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "0.75rem 1rem",
  background: vars.color.terminalHeaderBg,
  borderBottom: `1px solid ${vars.color.terminalBorder}`,
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
  color: vars.color.terminalForeground,
});

export const fileStats = style({
  display: "flex",
  gap: "0.75rem",
  fontSize: "0.875rem",
});

export const hunk = style({
  borderTop: `1px solid ${vars.color.terminalBorder}`,
});

export const hunkHeader = style({
  padding: "0.5rem 1rem",
  background: vars.color.terminalHoverBg,
  color: vars.color.terminalTextMuted,
  fontSize: "0.875rem",
  fontFamily: "inherit",
});

export const lines = style({
  background: vars.color.terminalBackground,
});

export const line = style({
  display: "flex",
  alignItems: "center",
  fontSize: "13px",
  lineHeight: 1.6,
  borderLeft: "3px solid transparent",
});

export const lineAdd = style({
  background: vars.color.successBg,
  borderLeftColor: vars.color.success,
});

export const lineDelete = style({
  background: vars.color.errorBg,
  borderLeftColor: vars.color.error,
});

export const lineContext = style({
  background: vars.color.terminalBackground,
});

export const lineNumber = style({
  display: "inline-block",
  width: "50px",
  padding: "0 0.75rem",
  textAlign: "right",
  color: vars.color.terminalTextMuted,
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
    "&::-webkit-scrollbar-track": { background: vars.color.terminalBackground },
    "&::-webkit-scrollbar-thumb": {
      background: vars.color.terminalHoverBg,
      borderRadius: "6px",
      border: `2px solid ${vars.color.terminalBackground}`,
    },
    "&::-webkit-scrollbar-thumb:hover": { background: vars.color.terminalBorder },
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
  color: vars.color.terminalTextMuted,
});

export const emptyHint = style({
  marginTop: "0.5rem",
  fontSize: "0.875rem",
  color: vars.color.terminalTextMuted,
});

export const errorState = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  padding: "2rem",
  textAlign: "center",
  color: vars.color.error,
});
