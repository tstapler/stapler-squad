import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const detail = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderTop: "none",
  borderBottomLeftRadius: vars.radii.md,
  borderBottomRightRadius: vars.radii.md,
  padding: vars.space["4"],
});

export const statsRow = style({
  display: "flex",
  gap: vars.space["4"],
  marginBottom: vars.space["3"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  flexWrap: "wrap",
});

export const statItem = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["1"],
});

export const added = style({
  color: vars.color.success,
  fontWeight: 600,
  fontFamily: vars.font.mono,
});

export const removed = style({
  color: vars.color.error,
  fontWeight: 600,
  fontFamily: vars.font.mono,
});

export const commitList = style({
  listStyle: "none",
  padding: 0,
  margin: `0 0 ${vars.space["3"]} 0`,
});

export const commitItem = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  padding: `${vars.space["1"]} 0`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  ":last-child": {
    borderBottom: "none",
  },
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const actionRow = style({
  display: "flex",
  gap: vars.space["2"],
  flexWrap: "wrap",
  marginTop: vars.space["3"],
});

export const btn = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  fontWeight: 500,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  color: vars.color.textSecondary,
  transition: "background 0.12s, color 0.12s",
  ":hover": {
    background: vars.color.hoverBackground,
    color: vars.color.textPrimary,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.inputFocusBorder}`,
    outlineOffset: "1px",
  },
});

export const btnPrimary = style([
  btn,
  {
    background: vars.color.primary,
    color: vars.color.textInverse,
    borderColor: vars.color.primary,
    ":hover": {
      background: vars.color.primaryHover,
      color: vars.color.textInverse,
    },
  },
]);

export const summaryBox = style({
  marginTop: vars.space["3"],
  padding: vars.space["3"],
  background: vars.color.accentBg,
  border: `1px solid ${vars.color.borderSubtle}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  lineHeight: 1.6,
});

export const summaryError = style({
  color: vars.color.errorText,
});

export const spinner = style({
  display: "inline-block",
  width: "1em",
  height: "1em",
  border: `2px solid ${vars.color.borderColor}`,
  borderTopColor: vars.color.primary,
  borderRadius: "50%",
  animation: "spin 0.6s linear infinite",
  "@keyframes": {
    spin: {
      from: { transform: "rotate(0deg)" },
      to: { transform: "rotate(360deg)" },
    },
  },
} as Parameters<typeof import("@vanilla-extract/css").style>[0]);

export const noChanges = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  fontStyle: "italic",
});
