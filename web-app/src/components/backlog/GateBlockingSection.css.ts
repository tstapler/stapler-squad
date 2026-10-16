import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

/** Stacks the frozen-snapshot notice above N GateChecklist blocks (ADR-005 Decision 5). */
export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});
