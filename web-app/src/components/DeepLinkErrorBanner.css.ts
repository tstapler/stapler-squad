/* Reuses backlog/InlineError.tsx's existing error tokens for the "alert"
 * cases and the existing warning tokens for the "status" cases — no new
 * colors introduced (ux.md AC "wrong workspace" mental model, Story 5.1). */
import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const sharedContainer = {
  display: "flex",
  alignItems: "flex-start",
  gap: vars.space["2"],
  borderRadius: vars.radii.md,
  padding: vars.space["3"],
  fontSize: vars.fontSize.sm,
} as const;

export const alertContainer = style({
  ...sharedContainer,
  border: `1px solid ${vars.color.error}`,
  background: vars.color.errorBg,
  color: vars.color.errorText,
});

export const statusContainer = style({
  ...sharedContainer,
  border: `1px solid ${vars.color.warning}`,
  background: vars.color.warningBg,
  color: vars.color.warningText,
});

export const icon = style({
  color: "inherit",
  flexShrink: 0,
});

export const headline = style({
  fontWeight: vars.fontWeight.semibold,
});

export const body = style({
  margin: `${vars.space["1"]} 0 0`,
  fontSize: vars.fontSize.sm,
  color: "inherit",
});

export const actions = style({
  display: "flex",
  gap: vars.space["2"],
  flexWrap: "wrap",
  marginTop: vars.space["2"],
});

export const primaryActionButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  color: vars.color.primary,
  fontSize: vars.fontSize.sm,
  padding: 0,
  textDecoration: "underline",
  ":hover": {
    color: vars.color.primaryHover,
  },
});

export const secondaryActionButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  color: "inherit",
  fontSize: vars.fontSize.sm,
  padding: 0,
  textDecoration: "underline",
});
