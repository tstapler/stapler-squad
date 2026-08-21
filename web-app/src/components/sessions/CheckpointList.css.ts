import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: 0,
});

export const header = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textSecondary,
  marginBottom: vars.space["2"],
  textTransform: "uppercase",
  letterSpacing: "0.05em",
});

export const emptyState = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  padding: `${vars.space["2"]} 0`,
  fontStyle: "italic",
});

export const list = style({
  listStyle: "none",
  margin: 0,
  padding: 0,
  display: "flex",
  flexDirection: "column",
  gap: "1px",
});

export const item = style({
  display: "flex",
  alignItems: "flex-start",
  justifyContent: "space-between",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.surfaceSubtle,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderSubtle}`,
  transition: "background 0.1s ease",

  ":hover": {
    background: vars.color.hoverBackground,
  },
});

export const itemInfo = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  minWidth: 0,
  flex: 1,
});

export const itemLabel = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textPrimary,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const itemMeta = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  flexWrap: "wrap",
});

export const timestamp = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const pill = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `1px ${vars.space["1"]}`,
  background: vars.color.accentBg,
  color: vars.color.textSecondary,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
  letterSpacing: "0.02em",
});

export const deleteButton = style({
  flexShrink: 0,
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  width: "20px",
  height: "20px",
  background: "transparent",
  border: "none",
  borderRadius: vars.radii.sm,
  color: vars.color.textMuted,
  cursor: "pointer",
  fontSize: "1rem",
  lineHeight: 1,
  transition: "background 0.1s ease, color 0.1s ease",
  padding: 0,

  ":hover": {
    background: vars.color.errorBg,
    color: vars.color.error,
  },
});

export const showMoreButton = style({
  marginTop: vars.space["2"],
  padding: 0,
  background: "none",
  border: "none",
  color: vars.color.primary,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  textDecoration: "underline",

  ":hover": {
    color: vars.color.primaryHover,
  },
});
