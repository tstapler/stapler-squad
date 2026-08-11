import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const dropdown = style({
  listStyle: "none",
  margin: 0,
  padding: "4px 0",
  maxHeight: 200,
  overflowY: "auto",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
});

export const item = style({
  display: "flex",
  alignItems: "center",
  gap: 8,
  padding: "7px 16px",
  cursor: "pointer",
  fontSize: 13,
  fontFamily: vars.font.mono,
  color: vars.color.textPrimary,
  userSelect: "none",
  selectors: {
    "&:hover": {
      background: vars.color.accentBg,
    },
  },
});

export const itemSelected = style({
  background: vars.color.accentBg,
});

export const icon = style({
  fontSize: 14,
  flexShrink: 0,
  width: 18,
  textAlign: "center",
});

export const name = style({
  flex: 1,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const suffix = style({
  color: vars.color.textMuted,
  flexShrink: 0,
});

export const itemHistory = style({
  color: vars.color.textMuted,
});

export const divider = style({
  height: 1,
  background: vars.color.borderColor,
  margin: "4px 8px",
  padding: 0,
  listStyle: "none",
});

export const loading = style({
  padding: "10px 16px",
  fontSize: 13,
  color: vars.color.textMuted,
  borderBottom: `1px solid ${vars.color.borderColor}`,
});
