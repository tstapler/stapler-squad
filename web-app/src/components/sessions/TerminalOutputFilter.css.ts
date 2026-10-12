import { style } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

export const panel = style({
  position: "absolute",
  top: 0,
  left: 0,
  right: 0,
  zIndex: zIndex.terminalPaneChrome,
  display: "flex",
  flexDirection: "column",
  maxHeight: "50%",
  background: vars.color.cardBackground,
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const header = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: vars.space["2"],
});

export const input = style({
  flex: 1,
  minWidth: 0,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontSize: vars.fontSize.sm,
});

export const option = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["1"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  whiteSpace: "nowrap",
});

export const status = style({
  padding: `0 ${vars.space["2"]} ${vars.space["1"]}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const results = style({
  margin: 0,
  padding: 0,
  listStyle: "none",
  overflowY: "auto",
  fontFamily: "monospace",
  fontSize: vars.fontSize.sm,
});

export const row = style({
  display: "flex",
  gap: vars.space["2"],
  padding: `0 ${vars.space["2"]}`,
  whiteSpace: "pre-wrap",
  wordBreak: "break-all",
});

export const lineNumber = style({
  flexShrink: 0,
  minWidth: "5ch",
  textAlign: "right",
  color: vars.color.textMuted,
});
