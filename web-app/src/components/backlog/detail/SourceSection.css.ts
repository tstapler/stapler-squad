import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const link = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.primary,
  textDecoration: "none",
  alignSelf: "flex-start",
  ":hover": {
    textDecoration: "underline",
  },
});

export const labels = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space["1"],
});

// Mirrors GitHubIssuePicker.css.ts's labelBadge token-for-token (same
// vars.* references) — see that file for the canonical definition this
// intentionally duplicates rather than importing across components.
export const labelBadge = style({
  display: "inline-block",
  padding: `1px ${vars.space["1"]}`,
  borderRadius: vars.radii.sm,
  fontSize: "10px",
  fontWeight: 500,
  background: vars.color.hoverBackground,
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  maxWidth: "80px",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});
