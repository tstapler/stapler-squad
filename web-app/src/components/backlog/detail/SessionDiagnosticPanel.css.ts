import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const panel = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

// The one-line role="status" state summary at the top of a Structured
// Diagnostic (ux.md Surface 3) — deliberately plain, matching
// StatusBadge.tsx's existing role="status" convention.
export const stateSummary = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textSecondary,
});
