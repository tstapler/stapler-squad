import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const backdrop = style({
  position: "fixed",
  inset: 0,
  background: "rgba(0,0,0,0.5)",
  zIndex: 1000,
  display: "flex",
  alignItems: "flex-start",
  justifyContent: "center",
  paddingTop: "20vh",
});

export const card = style({
  background: vars.color.terminalBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 8,
  width: "min(560px, 90vw)",
  overflow: "hidden",
  boxShadow: "0 8px 32px rgba(0,0,0,0.4)",
});

export const searchInput = style({
  width: "100%",
  border: "none",
  outline: "none",
  padding: "12px 16px",
  fontSize: 14,
  background: "transparent",
  color: "inherit",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  boxSizing: "border-box",
  selectors: {
    "&::placeholder": {
      color: vars.color.textMuted,
    },
  },
});

export const resultsList = style({
  maxHeight: 360,
  overflowY: "auto",
  padding: "4px 0",
});

export const resultItem = style({
  height: 40,
  display: "flex",
  alignItems: "center",
  gap: 8,
  padding: "0 12px",
  cursor: "pointer",
  selectors: {
    "&:hover": {
      background: vars.color.terminalHoverBg,
    },
  },
});

export const resultItemActive = style({
  height: 40,
  display: "flex",
  alignItems: "center",
  gap: 8,
  padding: "0 12px",
  cursor: "pointer",
  background: vars.color.accentBg,
  outline: `1px solid ${vars.color.primary}`,
  outlineOffset: -1,
});

export const resultIcon = style({
  width: 20,
  textAlign: "center",
  flexShrink: 0,
});

export const resultName = style({
  fontWeight: 600,
  fontSize: 13,
  flex: 1,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const resultPath = style({
  fontSize: 11,
  color: vars.color.textMuted,
  flexShrink: 0,
  maxWidth: 200,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const emptyState = style({
  padding: "16px 12px",
  color: vars.color.textMuted,
  fontSize: 13,
  textAlign: "center",
});
