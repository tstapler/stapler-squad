import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme-contract.css";

export const header = recipe({
  base: {
    display: "flex",
    flexDirection: "column",
    gap: vars.space[1],
  },
  variants: {
    mode: {
      full: { fontSize: vars.fontSize.base },
      compact: { fontSize: vars.fontSize.sm },
    },
  },
  defaultVariants: { mode: "full" },
});

export const row = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  flexWrap: "wrap",
});

export const icon = style({
  color: vars.color.textSecondary,
  flexShrink: 0,
});

export const branch = style({
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const clean = style({ color: vars.color.success });
export const dirty = style({ color: vars.color.warningText });

export const stat = style({
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.sm,
});

export const worktreeRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  overflowX: "auto",
});

export const worktreePath = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  whiteSpace: "nowrap",
});

export const iconButton = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minHeight: 44,
  minWidth: 44,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  background: "transparent",
  color: vars.color.textSecondary,
  cursor: "pointer",
  flexShrink: 0,
  selectors: {
    "&:hover": { background: vars.color.hoverBackground },
  },
});

export const activeSessions = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[1],
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.sm,
});
