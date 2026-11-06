import { keyframes, style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const spin = keyframes({ to: { transform: "rotate(360deg)" } });

export const wait = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  gap: "10px",
  padding: "24px 16px",
  color: vars.color.textSecondary,
  fontSize: "14px",
  textAlign: "center",
});

export const spinner = style({
  width: "20px",
  height: "20px",
  borderRadius: "50%",
  border: `2px solid ${vars.color.borderMuted}`,
  borderTopColor: vars.color.primary,
  animation: `${spin} 0.8s linear infinite`,
  "@media": { "(prefers-reduced-motion: reduce)": { animation: "none" } },
});

export const failureText = style({ color: vars.color.errorText });
export const actions = style({ display: "flex", gap: "8px", flexWrap: "wrap", justifyContent: "center" });
export const actionButton = style({
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: vars.radii.sm,
  background: "transparent",
  color: vars.color.textPrimary,
  font: "inherit",
  padding: "4px 12px",
  cursor: "pointer",
  "@media": { "(pointer: coarse)": { minHeight: "44px", minWidth: "44px" } },
});
