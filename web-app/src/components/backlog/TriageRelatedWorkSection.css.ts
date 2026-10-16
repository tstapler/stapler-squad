import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const input = style({
  width: "100%",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  backgroundColor: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

export const resultList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  listStyle: "none",
  margin: 0,
  padding: 0,
});

export const resultCard = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  padding: vars.space["2"],
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  textDecoration: "none",
  color: "inherit",
  cursor: "pointer",
  transition: vars.transition.fast,
  ":hover": {
    backgroundColor: vars.color.hoverBackground,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

export const resultTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textPrimary,
});

export const resultMeta = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

// Deliberately plain, non-interactive text — not a separate navigation
// target. Activating anywhere on the card (including over this text)
// navigates to the session's history page.
export const moreMatchesText = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
});

export const snippetText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  margin: 0,
});

export const emptyState = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  fontStyle: "italic",
});

export const errorState = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  fontSize: vars.fontSize.sm,
  color: vars.color.errorText,
});

export const retryButton = style({
  backgroundColor: "transparent",
  color: vars.color.primary,
  border: `1px solid ${vars.color.primary}`,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  transition: vars.transition.fast,
  ":hover": {
    backgroundColor: vars.color.hoverBackground,
  },
});

export const hintText = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});
