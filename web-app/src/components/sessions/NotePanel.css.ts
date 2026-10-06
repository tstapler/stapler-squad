import { style } from "@vanilla-extract/css";
import { vars } from "../../styles/theme.css";

export const panelContainer = style({
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: vars.space["3"],
  marginTop: vars.space["3"],
});

export const summary = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  cursor: "pointer",
  userSelect: "none",
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  listStyle: "none",
  color: vars.color.textPrimary,
  selectors: {
    "&::-webkit-details-marker": {
      display: "none",
    },
  },
});

export const body = style({
  marginTop: vars.space["2"],
});

export const emptyText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  marginBottom: vars.space["2"],
});

export const addButton = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.primary,
  background: "none",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  cursor: "pointer",
  minHeight: "44px",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
  },
});

export const textarea = style({
  width: "100%",
  minHeight: "120px",
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.mono,
  color: vars.color.inputText,
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  padding: vars.space["2"],
  resize: "vertical",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.inputFocusBorder,
    },
  },
});

export const hint = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  marginTop: vars.space["1"],
});

export const actionsRow = style({
  display: "flex",
  gap: vars.space["2"],
  marginTop: vars.space["2"],
});

export const saveButton = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textInverse,
  background: vars.color.primary,
  border: "none",
  borderRadius: vars.radii.sm,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  cursor: "pointer",
  minHeight: "44px",
  minWidth: "44px",
  selectors: {
    "&:hover": {
      background: vars.color.primaryHover,
    },
    "&:disabled": {
      opacity: 0.6,
      cursor: "not-allowed",
    },
  },
});

export const cancelButton = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  background: "none",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  cursor: "pointer",
  minHeight: "44px",
  minWidth: "44px",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
  },
});

export const editButton = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  fontSize: vars.fontSize.xs,
  color: vars.color.primary,
  background: "none",
  border: "none",
  cursor: "pointer",
  padding: `0 ${vars.space["2"]}`,
  marginTop: vars.space["2"],
  minHeight: "44px",
  minWidth: "44px",
  selectors: {
    "&:hover": {
      textDecoration: "underline",
    },
  },
});

export const errorText = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.errorText,
  marginTop: vars.space["2"],
});

export const renderedHeading = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  margin: `${vars.space["2"]} 0`,
});
