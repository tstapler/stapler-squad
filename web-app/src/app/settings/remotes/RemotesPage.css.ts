import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  maxWidth: "720px",
  height: "100%",
  overflowY: "auto",
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
  "@media": {
    "(max-width: 600px)": {
      flexDirection: "column",
      alignItems: "stretch",
    },
  },
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
  minHeight: "44px",
  ":hover": {
    background: vars.color.primaryHover,
  },
});

export const list = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const emptyState = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "flex-start",
  gap: vars.space["3"],
  padding: vars.space["6"],
  background: vars.color.cardBackground,
  border: `1px dashed ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
});

export const listItem = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  padding: vars.space["3"],
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  "@media": {
    "(max-width: 600px)": {
      flexDirection: "column",
      alignItems: "stretch",
    },
  },
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
  alignItems: "baseline",
  gap: vars.space["2"],
  flexWrap: "wrap",
});

export const listItemName = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const listItemMeta = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
});

export const listItemStatus = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
});

export const actionRow = style({
  display: "flex",
  gap: vars.space["2"],
  flexShrink: 0,
  alignItems: "center",
  "@media": {
    "(max-width: 600px)": {
      justifyContent: "flex-end",
    },
  },
});

export const confirmText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
});

export const smallBtn = style({
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  color: vars.color.textPrimary,
  minHeight: "44px",
  minWidth: "44px",
  ":hover": {
    background: vars.color.hoverBackground,
  },
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
});

export const deleteBtn = style([
  smallBtn,
  {
    border: `1px solid ${vars.color.error}`,
    color: vars.color.errorText,
    ":hover": {
      background: vars.color.errorBg,
    },
  },
]);

export const confirmDeleteBtn = style([
  smallBtn,
  {
    border: `2px solid ${vars.color.errorDark}`,
    background: vars.color.errorDark,
    color: vars.color.textInverse,
    fontWeight: 700,
  },
]);

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
