import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  overflowX: "auto",
});

export const table = style({
  width: "100%",
  borderCollapse: "collapse",
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
});

export const thead = style({
  position: "sticky",
  top: 0,
  zIndex: 1,
  backgroundColor: vars.color.cardBackground,
});

export const th = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  textAlign: "left",
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  whiteSpace: "nowrap",
});

export const tr = style({
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  ":hover": {
    backgroundColor: vars.color.hoverBackground,
  },
});

export const trMangled = style({
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  backgroundColor: vars.color.errorBg,
  ":hover": {
    backgroundColor: vars.color.errorBg,
    opacity: 0.9,
  },
});

export const td = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  verticalAlign: "middle",
});

export const codeCell = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  maxWidth: "200px",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const mangledBadge = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  backgroundColor: vars.color.errorBg,
  color: vars.color.errorText,
});

export const loadMoreRow = style({
  display: "flex",
  justifyContent: "center",
  padding: vars.space["4"],
});

export const loadMoreButton = style({
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
  backgroundColor: vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  ":hover": {
    backgroundColor: vars.color.hoverBackground,
  },
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
});

export const emptyState = style({
  padding: vars.space["8"],
  textAlign: "center",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});

export const loadingState = style({
  padding: vars.space["4"],
  textAlign: "center",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});
