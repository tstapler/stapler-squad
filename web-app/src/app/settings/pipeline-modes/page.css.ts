import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  maxWidth: "720px",
  display: "flex",
  flexDirection: "column",
  gap: vars.space["6"],
  padding: vars.space["6"],
});

export const headerRow = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space["3"],
});

export const title = style({
  fontSize: vars.fontSize.lg,
  fontWeight: 600,
  color: vars.color.textPrimary,
  margin: 0,
});

export const description = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  margin: 0,
});

export const newBtn = style({
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
});

export const list = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const listItem = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  padding: vars.space["3"],
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
});

export const listItemDisabled = style({
  opacity: 0.6,
});

export const listItemInfo = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  flexGrow: 1,
  minWidth: 0,
});

export const listItemNameRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const listItemName = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const listItemSlug = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const badge = style({
  fontSize: vars.fontSize.xs,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.full,
  background: vars.color.borderColor,
  color: vars.color.textMuted,
});

export const listItemMeta = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
});

export const actionRow = style({
  display: "flex",
  gap: vars.space["2"],
  flexShrink: 0,
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

export const formOverlay = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
  padding: vars.space["4"],
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
});
