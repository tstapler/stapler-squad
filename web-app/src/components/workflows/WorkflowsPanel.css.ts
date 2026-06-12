import { style } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

export const panel = style({
  display: "flex",
  flexDirection: "column",
  gap: "1.5rem",
});

export const header = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  flexWrap: "wrap",
  gap: "0.75rem",
});

export const titleRow = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.25rem",
});

export const title = style({
  fontSize: vars.fontSize.xl,
  fontWeight: 700,
  color: vars.color.textPrimary,
  margin: 0,
});

export const subtitle = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  margin: 0,
});

export const addButton = style({
  display: "flex",
  alignItems: "center",
  gap: "0.4rem",
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
});

export const error = style({
  padding: `${vars.space[3]} ${vars.space[4]}`,
  background: vars.color.errorBg,
  color: vars.color.error,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
});

export const loading = style({
  padding: vars.space[8],
  textAlign: "center",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});

export const empty = style({
  padding: vars.space[8],
  textAlign: "center",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  border: `1px dashed ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
});

export const tableWrapper = style({
  overflowX: "auto",
  borderRadius: vars.radii.lg,
  border: `1px solid ${vars.color.borderColor}`,
});

export const table = style({
  width: "100%",
  borderCollapse: "collapse",
  fontSize: vars.fontSize.sm,
});

export const th = style({
  padding: `${vars.space[3]} ${vars.space[4]}`,
  textAlign: "left",
  fontWeight: 600,
  color: vars.color.textSecondary,
  background: vars.color.surfaceMuted,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  whiteSpace: "nowrap",
});

export const td = style({
  padding: `${vars.space[3]} ${vars.space[4]}`,
  color: vars.color.textPrimary,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  verticalAlign: "middle",
});

export const row = style({
  ":hover": {
    background: vars.color.hoverBackground,
  },
});

export const slugCell = style({
  fontFamily: "monospace",
  fontSize: vars.fontSize.xs,
  background: vars.color.surfaceMuted,
  padding: `2px ${vars.space[2]}`,
  borderRadius: vars.radii.sm,
  color: vars.color.textSecondary,
  whiteSpace: "nowrap",
});

export const cronBadge = style({
  display: "inline-flex",
  alignItems: "center",
  gap: "0.2rem",
  padding: `2px ${vars.space[2]}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  fontWeight: 500,
});

export const cronEnabled = style({
  background: vars.color.successBg,
  color: vars.color.success,
});

export const cronDisabled = style({
  background: vars.color.surfaceMuted,
  color: vars.color.textMuted,
});

export const actions = style({
  display: "flex",
  gap: "0.5rem",
  alignItems: "center",
});

export const runButton = style({
  padding: `${vars.space[1]} ${vars.space[3]}`,
  background: vars.color.primary,
  color: vars.color.textInverse,
  border: "none",
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  cursor: "pointer",
  transition: vars.transition.fast,
  ":hover": {
    background: vars.color.primaryHover,
  },
});

export const editButton = style({
  padding: `${vars.space[1]} ${vars.space[3]}`,
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: 500,
  cursor: "pointer",
  transition: vars.transition.fast,
  ":hover": {
    background: vars.color.hoverBackground,
    color: vars.color.textPrimary,
  },
});

export const deleteButton = style({
  padding: `${vars.space[1]} ${vars.space[3]}`,
  background: "transparent",
  color: vars.color.error,
  border: `1px solid ${vars.color.error}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: 500,
  cursor: "pointer",
  transition: vars.transition.fast,
  ":hover": {
    background: vars.color.errorBg,
  },
});

export const formOverlay = style({
  position: "fixed",
  inset: 0,
  background: "rgba(0,0,0,0.5)",
  zIndex: zIndex.modal,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  padding: vars.space[4],
});

export const formCard = style({
  background: vars.color.background,
  borderRadius: vars.radii.lg,
  padding: vars.space[6],
  width: "100%",
  maxWidth: "560px",
  maxHeight: "90vh",
  overflowY: "auto",
  boxShadow: vars.shadow.lg,
});
