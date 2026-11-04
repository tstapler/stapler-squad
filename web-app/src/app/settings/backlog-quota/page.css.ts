import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const TWO_COLUMN_QUERY = "(min-width: 640px)";
const TOUCH_TARGET = "44px";

export const container = style({
  maxWidth: "720px",
  flex: 1,
  minHeight: 0,
  overflowY: "auto",
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
  padding: vars.space["6"],
  margin: "0 auto",
  width: "100%",
  boxSizing: "border-box",
});

export const title = style({
  fontSize: vars.fontSize.lg,
  fontWeight: 600,
  color: vars.color.textPrimary,
  margin: 0,
});

export const staticLine = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  margin: 0,
});

export const warningBox = style({
  display: "flex",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  border: `1px solid ${vars.color.warning}`,
  borderRadius: vars.radii.md,
  background: vars.color.warningBg,
  color: vars.color.warningText,
  fontSize: vars.fontSize.sm,
});

export const form = style({
  display: "grid",
  gridTemplateColumns: "1fr",
  gap: vars.space["4"],
  "@media": {
    [TWO_COLUMN_QUERY]: { gridTemplateColumns: "1fr 1fr" },
  },
});

export const fieldRow = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  minWidth: 0,
});

export const switchRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  minHeight: TOUCH_TARGET,
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
});

export const switchInput = style({
  width: "1.5rem",
  height: "1.5rem",
  flexShrink: 0,
});

export const label = style({
  display: "block",
  fontSize: vars.fontSize.sm,
  fontWeight: 500,
  color: vars.color.textPrimary,
});

export const input = style({
  minHeight: TOUCH_TARGET,
  boxSizing: "border-box",
  width: "100%",
  padding: `0 ${vars.space["3"]}`,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  fontSize: "1rem",
  scrollMarginBottom: "6rem",
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

export const inputInvalid = style({
  borderColor: vars.color.error,
});

export const hint = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  margin: 0,
});

export const fieldError = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.errorText,
  margin: 0,
});

export const fullWidth = style({
  gridColumn: "1 / -1",
});

export const actions = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  scrollMarginBottom: "6rem",
  "@media": {
    [TWO_COLUMN_QUERY]: { flexDirection: "row", alignItems: "center" },
  },
});

export const button = style({
  minHeight: TOUCH_TARGET,
  padding: `0 ${vars.space["4"]}`,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
  selectors: {
    '&[aria-disabled="true"], &:disabled': { opacity: 0.6, cursor: "not-allowed" },
  },
});

export const primaryButton = style([
  button,
  {
    background: vars.color.primary,
    borderColor: vars.color.primary,
    color: vars.color.primaryText,
    fontWeight: 500,
  },
]);

export const status = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  minHeight: "1.25rem",
});

export const errorBox = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  border: `1px solid ${vars.color.error}`,
  borderRadius: vars.radii.md,
  background: vars.color.errorBg,
  color: vars.color.errorText,
  fontSize: vars.fontSize.sm,
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

export const errorList = style({
  margin: 0,
  paddingLeft: vars.space["4"],
});

export const errorActions = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space["2"],
});

export const errorLink = style({
  background: "none",
  border: "none",
  padding: 0,
  color: vars.color.errorText,
  textDecoration: "underline",
  cursor: "pointer",
  font: "inherit",
  textAlign: "left",
});
