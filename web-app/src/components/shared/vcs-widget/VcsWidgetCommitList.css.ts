import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme-contract.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[1],
});

export const list = recipe({
  base: {
    listStyle: "none",
    margin: 0,
    padding: 0,
    display: "flex",
    flexDirection: "column",
  },
  variants: {
    mode: {
      full: {},
      compact: {},
    },
  },
  defaultVariants: { mode: "full" },
});

export const commitRow = style({
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
});

export const commitButton = style({
  display: "block",
  width: "100%",
  minHeight: 44,
  padding: `${vars.space[1]} ${vars.space[2]}`,
  border: "none",
  background: "transparent",
  textAlign: "left",
  cursor: "pointer",
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
  selectors: {
    "&:hover": { background: vars.color.hoverBackground },
  },
});

export const summaryCollapsed = style({
  display: "block",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const summaryExpanded = style({
  display: "block",
  whiteSpace: "normal",
  wordBreak: "break-word",
});

export const expanded = style({
  padding: `0 ${vars.space[2]} ${vars.space[2]}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  whiteSpace: "normal",
  wordBreak: "break-word",
});

export const showAllButton = style({
  alignSelf: "flex-start",
  minHeight: 44,
  padding: `${vars.space[1]} ${vars.space[2]}`,
  border: "none",
  background: "transparent",
  color: vars.color.primary,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
});
