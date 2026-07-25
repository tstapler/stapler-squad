import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars, breakpoints } from "@/styles/theme-contract.css";

export const widget = recipe({
  base: {
    display: "flex",
    flexDirection: "column",
    gap: vars.space[3],
  },
  variants: {
    mode: {
      full: {},
      compact: {
        gap: vars.space[2],
        fontSize: vars.fontSize.sm,
      },
    },
  },
  defaultVariants: { mode: "full" },
});

export const liveRegion = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: vars.space[2],
  "@media": {
    [`screen and (max-width: ${breakpoints.sm})`]: {
      order: -1,
    },
  },
});

export const controlsRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  flexWrap: "wrap",
});

export const snapshotTimestamp = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
});

export const neutralNotice = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const refreshButton = style({
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
  selectors: {
    "&:hover": { background: vars.color.hoverBackground },
  },
});

export const viewDiffButton = style({
  display: "inline-flex",
  alignItems: "center",
  minHeight: 44,
  padding: `${vars.space[1]} ${vars.space[3]}`,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  background: "transparent",
  color: vars.color.primary,
  cursor: "pointer",
  fontWeight: vars.fontWeight.medium,
  selectors: {
    "&:hover": { background: vars.color.hoverBackground },
  },
});

export const aggregateStatLine = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const additions = style({ color: vars.color.success });
export const deletions = style({ color: vars.color.errorText });
