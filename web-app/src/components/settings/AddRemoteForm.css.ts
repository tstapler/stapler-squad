import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const form = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
});

export const fieldGroup = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const label = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const hint = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  margin: 0,
});

export const input = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  color: vars.color.inputText,
  fontSize: vars.fontSize.sm,
  // Meets the 44px minimum touch target on mobile without changing desktop
  // density (padding alone stays under 44px there).
  minHeight: "44px",
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
    outline: "none",
  },
  ":disabled": {
    opacity: 0.6,
    cursor: "not-allowed",
  },
  "::placeholder": {
    color: vars.color.placeholderColor,
  },
});

export const actionRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  marginTop: vars.space["2"],
  "@media": {
    "(max-width: 600px)": {
      flexDirection: "column-reverse",
    },
  },
});

export const submitBtn = style({
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
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
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
  "@media": {
    "(max-width: 600px)": {
      width: "100%",
    },
  },
});

export const cancelBtn = style({
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  color: vars.color.textPrimary,
  minHeight: "44px",
  ":hover": {
    background: vars.color.hoverBackground,
  },
  "@media": {
    "(max-width: 600px)": {
      width: "100%",
    },
  },
});

export const errorMessage = style({
  color: vars.color.errorText,
  background: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.sm,
});

export const authorizedKeysBlock = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: vars.space["3"],
  background: vars.color.surfaceSubtle,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
});

export const authorizedKeysRow = style({
  display: "flex",
  alignItems: "flex-start",
  gap: vars.space["2"],
});

export const authorizedKeysLine = style({
  flex: 1,
  minWidth: 0,
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textPrimary,
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  padding: vars.space["2"],
  overflowX: "auto",
  whiteSpace: "pre",
});

export const copyBtn = style({
  flexShrink: 0,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
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
});

export const caveat = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.warningText,
  margin: 0,
});
