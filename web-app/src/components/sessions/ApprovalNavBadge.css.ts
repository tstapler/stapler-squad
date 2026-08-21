import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const pulse = keyframes({
  "0%, 100%": { opacity: 1 },
  "50%": { opacity: 0.7 },
});

export const badge = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "1.25rem",
  height: "1.25rem",
  padding: "0 0.375rem",
  background: vars.color.warning,
  color: vars.color.primaryText,
  fontSize: "0.6875rem",
  fontWeight: 600,
  borderRadius: "0.625rem",
  lineHeight: 1,
  pointerEvents: "none",
  animation: `${pulse} 1.5s ease-in-out infinite`,
});

export const inline = style({
  marginLeft: "0.375rem",
  verticalAlign: "middle",
});
