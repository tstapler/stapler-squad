import { keyframes, style } from "@vanilla-extract/css";
import { vars } from "../../styles/theme-contract.css";

// Both surfaces paint the theme background themselves: the pane behind a tab panel is a fixed dark colour,
// so theme-coloured text on it is unreadable in the light theme.
export const container = style({
  background: vars.color.background,
  height: "100%",
  overflowY: "auto",
  padding: vars.space["4"],
  display: "flex",
  flexDirection: "column",
  gap: vars.space["6"],
});

export const emptyState = style({
  background: vars.color.background,
  flex: 1,
  padding: vars.space["6"],
  color: vars.color.textSecondary,
  textAlign: "center",
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  gap: vars.space["2"],
});

export const section = style({});

export const sectionTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textSecondary,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  marginBottom: vars.space["2"],
});

export const list = style({
  listStyle: "none",
  padding: 0,
  margin: 0,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const link = style({
  color: vars.color.textPrimary,
  textDecoration: "underline",
  fontSize: vars.fontSize.sm,
  selectors: {
    "&:hover": { textDecoration: "underline" },
  },
});

export const sha = style({
  fontFamily: "monospace",
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  marginRight: vars.space["2"],
});

export const shaFull = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  fontFamily: "monospace",
});

export const urlToggleButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
  padding: `${vars.space["1"]} 0`,
  selectors: {
    "&:hover": { textDecoration: "underline" },
  },
});

export const lists = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["6"],
});

export const header = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space["2"],
});

export const statusLine = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const hint = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const actions = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space["2"],
});

// 44px minimum touch target (mobile).
export const actionButton = style({
  minHeight: "44px",
  minWidth: "44px",
  padding: `0 ${vars.space["4"]}`,
  cursor: "pointer",
  color: vars.color.textPrimary,
  background: "none",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  fontSize: vars.fontSize.sm,
  selectors: {
    "&:hover": { textDecoration: "underline" },
    "&:disabled": { cursor: "progress", textDecoration: "none", opacity: 0.7 },
  },
});

export const alert = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space["2"],
  padding: vars.space["3"],
  background: vars.color.errorBg,
  color: vars.color.errorText,
  borderRadius: "6px",
  overflowWrap: "anywhere",
});

const spin = keyframes({ to: { transform: "rotate(360deg)" } });

export const spinner = style({
  display: "inline-block",
  width: "14px",
  height: "14px",
  marginRight: vars.space["2"],
  border: "2px solid currentColor",
  borderTopColor: "transparent",
  borderRadius: "50%",
  verticalAlign: "middle",
  animation: `${spin} 0.8s linear infinite`,
  "@media": { "(prefers-reduced-motion: reduce)": { animation: "none" } },
});
