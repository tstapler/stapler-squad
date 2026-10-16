import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const tabList = style({
  display: "flex",
  gap: 0,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  overflowX: "auto",
});

export const tab = style({
  display: "inline-flex",
  alignItems: "center",
  gap: "0.5rem",
  minHeight: "44px",
  padding: "0.625rem 1rem",
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textMuted,
  background: "none",
  border: "none",
  borderBottom: "2px solid transparent",
  marginBottom: "-1px",
  cursor: "pointer",
  whiteSpace: "nowrap",
  transition: vars.transition.fast,
  selectors: {
    "&:hover": { color: vars.color.textPrimary },
    // textPrimary, not primary: primary on the page background is 3.58:1 in the default
    // dark theme (caught by the scoped Axe run in tests/e2e/up-next-tabs.spec.ts).
    '&[aria-selected="true"]': {
      color: vars.color.textPrimary,
      fontWeight: vars.fontWeight.semibold,
      borderBottomColor: vars.color.primary,
    },
    "&:focus-visible": {
      outline: `2px solid ${vars.color.primary}`,
      outlineOffset: "-2px",
    },
  },
});

export const badge = style({
  display: "inline-block",
  minWidth: "1.25rem",
  padding: "0 0.375rem",
  borderRadius: "999px",
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  lineHeight: "1.25rem",
  textAlign: "center",
  color: vars.color.errorText,
  background: vars.color.errorBg,
});

export const visuallyHidden = style({
  position: "absolute",
  width: "1px",
  height: "1px",
  margin: "-1px",
  padding: 0,
  overflow: "hidden",
  clip: "rect(0 0 0 0)",
  whiteSpace: "nowrap",
  border: 0,
});

export const panel = style({
  paddingTop: "1rem",
  selectors: {
    "&:focus-visible": {
      outline: `2px solid ${vars.color.primary}`,
      outlineOffset: "2px",
    },
  },
});

export const panelHeading = style({
  margin: 0,
  marginBottom: "0.75rem",
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  selectors: { "&:focus": { outline: "none" } },
});
