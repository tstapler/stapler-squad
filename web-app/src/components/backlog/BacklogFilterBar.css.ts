import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const filterBar = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["3"]} ${vars.space["6"]}`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  flexShrink: 0,
  flexWrap: "wrap",
});

export const searchInput = style({
  flex: 1,
  minWidth: "180px",
  maxWidth: "320px",
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.sans,
  outline: "none",
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
  },
  "::placeholder": {
    color: vars.color.placeholderColor,
  },
});

export const groupByLabel = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  marginLeft: "auto",
});

export const showArchivedLabel = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  cursor: "pointer",
  whiteSpace: "nowrap",
});

export const groupBySelect = style({
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.sans,
  cursor: "pointer",
});

export const resetViewButton = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  whiteSpace: "nowrap",
  transition: "background 0.1s ease",
  ":hover": {
    background: vars.color.hoverBackground,
    borderColor: vars.color.borderStrong,
  },
});

export const filterChipGroup = style({
  display: "flex",
  gap: vars.space["1"],
  flexWrap: "wrap",
});

export const filterChip = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderMuted}`,
  background: vars.color.surfaceMuted,
  color: vars.color.textSecondary,
  transition: "background 0.1s ease, border-color 0.1s ease",
  ":hover": {
    background: vars.color.hoverBackground,
    borderColor: vars.color.borderStrong,
  },
});

export const filterChipActive = style({
  background: vars.color.accentBg,
  color: vars.color.primary,
  borderColor: vars.color.primary,
});
