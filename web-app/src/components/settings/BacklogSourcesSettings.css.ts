import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  maxWidth: "640px",
  display: "flex",
  flexDirection: "column",
  gap: vars.space["6"],
});

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const sectionTitle = style({
  fontSize: vars.fontSize.base,
  fontWeight: 600,
  color: vars.color.textPrimary,
  margin: 0,
});

export const description = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  margin: 0,
});

export const list = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const listItem = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: `${vars.space["3"]}`,
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
});

export const listItemHeader = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const listItemName = style({
  flexGrow: 1,
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const listItemMeta = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
});

export const toggle = style({
  width: "2.5rem",
  height: "1.25rem",
  borderRadius: vars.radii.full,
  border: "none",
  cursor: "pointer",
  background: vars.color.borderColor,
  position: "relative",
  transition: "background 0.2s",
  flexShrink: 0,
});

export const toggleOn = style({
  background: vars.color.primary,
});

export const actionRow = style({
  display: "flex",
  gap: vars.space["2"],
});

export const smallBtn = style({
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  color: vars.color.textPrimary,
  ":hover": {
    background: vars.color.hoverBackground,
  },
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
});

export const removeBtn = style({
  background: "transparent",
  border: "none",
  cursor: "pointer",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  padding: `0 ${vars.space["1"]}`,
  flexShrink: 0,
  ":hover": {
    color: vars.color.errorText,
  },
});

export const historyList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
});

export const form = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const formRow = style({
  display: "flex",
  gap: vars.space["2"],
});

export const input = style({
  flexGrow: 1,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  color: vars.color.inputText,
  fontSize: vars.fontSize.sm,
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
    outline: "none",
  },
  "::placeholder": {
    color: vars.color.placeholderColor,
  },
});

export const select = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  color: vars.color.inputText,
  fontSize: vars.fontSize.sm,
});

export const addBtn = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `1px solid ${vars.color.primary}`,
  background: vars.color.primary,
  color: vars.color.textInverse,
  fontWeight: 500,
  alignSelf: "flex-start",
  ":hover": {
    background: vars.color.primaryHover,
  },
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
});

export const empty = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  fontStyle: "italic",
});

export const errorMessage = style({
  color: vars.color.errorText,
  background: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.sm,
});
