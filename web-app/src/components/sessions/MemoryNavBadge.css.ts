import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const badge = style({
  display: "inline-flex",
  alignItems: "center",
  gap: "0.25rem",
  padding: "0.25rem 0.5rem",
  background: vars.color.warningBg,
  color: vars.color.warningText,
  fontSize: "0.6875rem",
  fontWeight: 600,
  borderRadius: "0.5rem",
  border: `1px solid ${vars.color.warning}`,
  whiteSpace: "nowrap",
  cursor: "default",
});

export const dot = style({
  width: "6px",
  height: "6px",
  borderRadius: "50%",
  background: vars.color.warning,
  flexShrink: 0,
});
