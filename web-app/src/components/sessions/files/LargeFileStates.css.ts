import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const states = style({ display: "flex", flexDirection: "column", flexShrink: 0 });

export const chip = style({
  display: "inline-flex",
  alignItems: "center",
  gap: "6px",
  padding: "2px 8px",
  fontSize: "12px",
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderMuted}`,
  color: vars.color.textSecondary,
});
