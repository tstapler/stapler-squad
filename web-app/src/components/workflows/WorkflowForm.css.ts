import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const form = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[4],
});

export const formHeader = style({
  marginBottom: vars.space[2],
});

export const formTitle = style({
  fontSize: vars.fontSize.lg,
  fontWeight: 700,
  color: vars.color.textPrimary,
  margin: 0,
});

export const fieldGroup = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.3rem",
});

export const label = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 500,
  color: vars.color.textSecondary,
});

export const required = style({
  color: vars.color.error,
  marginLeft: "0.2rem",
});

export const input = style({
  padding: `${vars.space[2]} ${vars.space[3]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  outline: "none",
  width: "100%",
  boxSizing: "border-box",
  transition: vars.transition.fast,
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
  },
  ":disabled": {
    opacity: 0.6,
    cursor: "not-allowed",
  },
});

export const textarea = style({
  padding: `${vars.space[2]} ${vars.space[3]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  outline: "none",
  width: "100%",
  boxSizing: "border-box",
  transition: vars.transition.fast,
  resize: "vertical",
  minHeight: "80px",
  fontFamily: "monospace",
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
  },
});

export const hint = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  marginTop: "0.15rem",
});

export const checkboxRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  cursor: "pointer",
});

export const row = style({
  display: "grid",
  gridTemplateColumns: "1fr 1fr",
  gap: vars.space[3],
  "@media": {
    "screen and (max-width: 480px)": {
      gridTemplateColumns: "1fr",
    },
  },
});

export const errorBanner = style({
  padding: `${vars.space[2]} ${vars.space[3]}`,
  background: vars.color.errorBg,
  color: vars.color.error,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
});

export const buttonRow = style({
  display: "flex",
  justifyContent: "flex-end",
  gap: vars.space[3],
  marginTop: vars.space[2],
});

export const cancelButton = style({
  padding: `${vars.space[2]} ${vars.space[4]}`,
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: 500,
  cursor: "pointer",
  transition: vars.transition.fast,
  ":hover": {
    background: vars.color.hoverBackground,
    color: vars.color.textPrimary,
  },
});

export const submitButton = style({
  padding: `${vars.space[2]} ${vars.space[4]}`,
  background: vars.color.primary,
  color: vars.color.textInverse,
  border: "none",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  cursor: "pointer",
  transition: vars.transition.fast,
  ":hover": {
    background: vars.color.primaryHover,
  },
  ":disabled": {
    opacity: 0.6,
    cursor: "not-allowed",
  },
});
