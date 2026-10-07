import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

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
  // minWidth:0 overrides the flex item's default content-based automatic
  // minimum size; overflowWrap lets long unbreakable tokens (e.g.
  // "cache_read_input_tokens/cache_creation_input_tokens") wrap instead of
  // forcing this row (and every ancestor up to UnfinishedTab's .container)
  // wider than the viewport on mobile.
  minWidth: 0,
  overflowWrap: "break-word",
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

export const prActions = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  marginTop: vars.space["1"],
});

export const openSessionButton = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  background: vars.color.accentBg,
  color: vars.color.inputFocusBorder,
  border: `1px solid ${vars.color.inputFocusBorder}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  cursor: "pointer",
  textDecoration: "none",
  display: "inline-flex",
  alignItems: "center",
  whiteSpace: "nowrap",
  ":hover": {
    background: vars.color.accentHover,
  },
});

export const createSessionButton = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  cursor: "pointer",
  textDecoration: "none",
  display: "inline-flex",
  alignItems: "center",
  whiteSpace: "nowrap",
  ":hover": {
    borderColor: vars.color.primary,
    color: vars.color.primary,
  },
});

export const repoGroupSection = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  paddingLeft: vars.space["4"],
});

export const repoGroupHeader = style({
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
  color: vars.color.textMuted,
  fontWeight: 600,
  padding: `${vars.space["1"]} 0`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  marginTop: vars.space["1"],
});
