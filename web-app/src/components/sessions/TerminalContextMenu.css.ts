import { style } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

export const menu = style({
  position: "fixed",
  zIndex: zIndex.floatingTerminalUI,
  minWidth: "140px",
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  boxShadow: vars.shadow.lg,
  padding: `${vars.space[1]} 0`,
  listStyle: "none",
  margin: 0,
  outline: "none",
});

export const menuItem = style({
  display: "block",
  width: "100%",
  padding: `${vars.space[2]} ${vars.space[3]}`,
  background: "none",
  border: "none",
  textAlign: "left",
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  cursor: "pointer",
  userSelect: "none",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.hoverBackground,
    },
    "&:disabled": {
      color: vars.color.textDisabled,
      cursor: "default",
    },
  },
});
