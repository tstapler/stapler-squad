import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  maxWidth: "960px",
  margin: "0 auto",
  padding: `${vars.space["4"]} ${vars.space["4"]}`,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
});

export const toolbar = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  flexWrap: "wrap",
});

export const toolbarLeft = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  flexGrow: 1,
});

export const title = style({
  fontSize: vars.fontSize.xl,
  fontWeight: 700,
  color: vars.color.textPrimary,
  margin: 0,
});

export const scanInfo = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const spinner = style({
  display: "inline-block",
  width: "0.75em",
  height: "0.75em",
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

export const toolbarRight = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const btn = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  color: vars.color.textSecondary,
  transition: "background 0.12s",
  ":hover": {
    background: vars.color.hoverBackground,
    color: vars.color.textPrimary,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.inputFocusBorder}`,
    outlineOffset: "1px",
  },
});

export const filterRow = style({
  display: "flex",
  gap: vars.space["2"],
  flexWrap: "wrap",
});

export const chip = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
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

export const chipActive = style({
  background: vars.color.primary,
  color: vars.color.textInverse,
  borderColor: vars.color.primary,
  ":hover": {
    background: vars.color.primaryHover,
    color: vars.color.textInverse,
  },
});

export const empty = style({
  textAlign: "center",
  color: vars.color.textMuted,
  padding: `${vars.space["8"]} 0`,
  fontSize: vars.fontSize.base,
});

export const repoList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["6"],
});
