import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const wrapper = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[2],
});

export const modeToggle = style({
  display: "flex",
  gap: vars.space[3],
});

export const modeOption = style({
  display: "flex",
  alignItems: "center",
  gap: "0.3rem",
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  cursor: "pointer",
});

export const notice = style({
  padding: `${vars.space[2]} ${vars.space[3]}`,
  background: vars.color.warningBg,
  color: vars.color.warning,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.xs,
});

const inputBase = style({
  padding: `${vars.space[2]} ${vars.space[3]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  outline: "none",
  transition: vars.transition.fast,
  selectors: {
    "&:focus": {
      borderColor: vars.color.inputFocusBorder,
    },
  },
});

export const simpleRow = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space[2],
});

export const select = style([inputBase, { flex: "1 1 auto", minWidth: "8rem" }]);
export const numberInput = style([inputBase, { width: "5rem" }]);
export const timeInput = style([inputBase, { width: "8rem" }]);
export const rawInput = style([inputBase, { width: "100%", boxSizing: "border-box", fontFamily: "monospace" }]);

export const explanation = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const error = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.error,
});

export const timezoneNote = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  fontStyle: "italic",
});
