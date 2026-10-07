import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import { MIN_TOUCH_TARGET } from "@/lib/unfinished/prTouchTokens";

const NARROW = "screen and (max-width: 480px)";

export const nudge = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  minWidth: 0,
});

export const controls = style({
  display: "flex",
  alignItems: "center",
  flexWrap: "wrap",
  gap: vars.space["2"],
  "@media": { [NARROW]: { flexDirection: "column", alignItems: "stretch" } },
});

export const selectLabel = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["2"],
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
});

export const select = style({
  minHeight: MIN_TOUCH_TARGET,
  minWidth: MIN_TOUCH_TARGET,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontSize: vars.fontSize.xs,
  color: vars.color.textPrimary,
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
});

export const askButton = style({
  minHeight: MIN_TOUCH_TARGET,
  minWidth: MIN_TOUCH_TARGET,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  background: vars.color.accentBg,
  color: vars.color.inputFocusBorder,
  border: `1px solid ${vars.color.inputFocusBorder}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  cursor: "pointer",
  overflowWrap: "anywhere",
  selectors: {
    '&[aria-disabled="true"]': { opacity: 0.6, cursor: "not-allowed" },
    '&:not([aria-disabled="true"]):hover': { background: vars.color.accentHover },
  },
});

export const hint = style({
  margin: 0,
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  overflowWrap: "break-word",
});

export const statusRegion = style({
  display: "flex",
  alignItems: "center",
  flexWrap: "wrap",
  gap: vars.space["2"],
  fontSize: vars.fontSize.xs,
  color: vars.color.textPrimary,
  outline: "none",
  selectors: { "&:empty": { display: "none" } },
});

export const alertRegion = style([
  statusRegion,
  { color: vars.color.errorText },
]);

export const openSessionLink = style({
  display: "inline-flex",
  alignItems: "center",
  minHeight: MIN_TOUCH_TARGET,
  minWidth: MIN_TOUCH_TARGET,
  color: vars.color.inputFocusBorder,
  fontWeight: 600,
});
