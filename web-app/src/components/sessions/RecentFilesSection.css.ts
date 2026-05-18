import { style } from "@vanilla-extract/css";
import { vars } from "../../styles/theme.css";

export const container = style({
  borderBottom: `1px solid ${vars.color.borderColor}`,
  paddingBottom: 4,
  marginBottom: 4,
});

export const heading = style({
  fontSize: 11,
  textTransform: "uppercase",
  padding: "4px 8px 2px",
  color: vars.color.textMuted,
  letterSpacing: "0.05em",
  userSelect: "none",
});

export const entry = style({
  display: "flex",
  alignItems: "center",
  gap: 6,
  width: "100%",
  padding: "3px 8px",
  height: 28,
  cursor: "pointer",
  background: "transparent",
  border: "none",
  textAlign: "left",
  borderRadius: 4,
  color: "inherit",
  fontSize: 13,
  selectors: {
    "&:hover": {
      background: vars.color.terminalHoverBg,
    },
  },
});

export const entrySelected = style({
  display: "flex",
  alignItems: "center",
  gap: 6,
  width: "100%",
  padding: "3px 8px",
  height: 28,
  cursor: "pointer",
  background: "var(--selection-bg, rgba(40, 100, 255, 0.25))",
  border: "none",
  textAlign: "left",
  borderRadius: 4,
  color: "inherit",
  fontSize: 13,
  selectors: {
    "&:hover": {
      background: vars.color.terminalHoverBg,
    },
  },
});

export const entryIcon = style({
  flexShrink: 0,
  width: 16,
  textAlign: "center",
  fontSize: 13,
});

export const entryName = style({
  fontWeight: 600,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  flex: 1,
});

export const entryDir = style({
  fontSize: 11,
  color: vars.color.textMuted,
  flexShrink: 0,
  marginLeft: 4,
});
