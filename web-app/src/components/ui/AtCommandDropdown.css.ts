import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const dropdown = style({
  listStyle: "none",
  margin: 0,
  padding: "4px 0",
  maxHeight: 240,
  overflowY: "auto",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
});

export const item = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  cursor: "pointer",
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
  fontSize: vars.fontSize.sm,
  flexShrink: 0,
  width: "1.25rem",
  textAlign: "center",
});

export const slug = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.sm,
  color: vars.color.accentHover,
  flexShrink: 0,
  minWidth: "8rem",
});

export const name = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  flex: 1,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const description = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  flexShrink: 0,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  maxWidth: "28ch",
});

export const empty = style({
  padding: `${vars.space["3"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  borderBottom: `1px solid ${vars.color.borderColor}`,
});
