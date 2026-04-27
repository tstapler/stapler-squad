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

export const toggleRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
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
});

export const toggleOn = style({
  background: vars.color.primary,
});

export const list = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const listItem = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
});

export const listItemPath = style({
  flexGrow: 1,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
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
  ":focus-visible": {
    outline: `2px solid ${vars.color.inputFocusBorder}`,
    outlineOffset: "1px",
    borderRadius: vars.radii.sm,
  },
});

export const addRow = style({
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
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.sm,
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
    outline: "none",
  },
  "::placeholder": {
    color: vars.color.placeholderColor,
  },
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
  ":hover": {
    background: vars.color.primaryHover,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.inputFocusBorder}`,
    outlineOffset: "1px",
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
