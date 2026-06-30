import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const sectionHeader = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  cursor: "pointer",
  userSelect: "none",
  outline: "none",
  ":hover": {
    background: vars.color.hoverBackground,
  },
  ":focus-visible": {
    boxShadow: `0 0 0 2px ${vars.color.inputFocusBorder}`,
  },
});

export const chevron = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  transition: "transform 0.15s",
  display: "inline-block",
});

export const chevronExpanded = style({
  transform: "rotate(90deg)",
});

export const sectionTitle = style({
  fontWeight: 600,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
});

export const badge = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  background: vars.color.surfaceSubtle,
  borderRadius: vars.radii.full,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  marginLeft: vars.space["2"],
});

export const username = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  marginLeft: "auto",
  fontFamily: vars.font.mono,
});

export const prList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  paddingLeft: vars.space["4"],
});

export const prCard = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space["3"]} ${vars.space["4"]}`,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  transition: "border-color 0.15s",
  ":hover": {
    borderColor: vars.color.borderHover,
  },
});

export const prHeader = style({
  display: "flex",
  alignItems: "flex-start",
  gap: vars.space["3"],
});

export const prTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textPrimary,
  textDecoration: "none",
  flexGrow: 1,
  lineHeight: 1.4,
  ":hover": {
    textDecoration: "underline",
    color: vars.color.primary,
  },
});

export const prMeta = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  flexWrap: "wrap",
});

export const prRepo = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const prBranch = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
});

export const chips = style({
  display: "flex",
  gap: vars.space["2"],
  alignItems: "center",
  flexWrap: "wrap",
  marginLeft: "auto",
});

const chipBase = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  lineHeight: 1.5,
  whiteSpace: "nowrap",
});

export const chipDraft = style([
  chipBase,
  {
    background: vars.color.surfaceSubtle,
    color: vars.color.textMuted,
    border: `1px solid ${vars.color.borderColor}`,
  },
]);

export const chipSuccess = style([
  chipBase,
  {
    background: vars.color.successBg,
    color: vars.color.success,
    border: `1px solid ${vars.color.success}`,
  },
]);

export const chipWarning = style([
  chipBase,
  {
    background: vars.color.warningBg,
    color: vars.color.warningText,
    border: `1px solid ${vars.color.warning}`,
  },
]);

export const chipError = style([
  chipBase,
  {
    background: vars.color.errorBg,
    color: vars.color.errorText,
    border: `1px solid ${vars.color.error}`,
  },
]);

export const worktreeLink = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.primary,
  textDecoration: "none",
  ":hover": {
    textDecoration: "underline",
  },
});

export const empty = style({
  padding: `${vars.space["4"]} ${vars.space["4"]}`,
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});

export const authError = style({
  padding: `${vars.space["3"]} ${vars.space["4"]}`,
  color: vars.color.warningText,
  background: vars.color.warningBg,
  border: `1px solid ${vars.color.warning}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  marginLeft: vars.space["4"],
});
