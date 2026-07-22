import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// Layout only — flex row that wraps on narrow/mobile viewports so the
// StageTracker, BlockerChip, and LivenessLine each drop to their own line
// instead of being clipped or forcing horizontal scroll (design/ux.md
// Surface 1/2 mobile-specific rules, Task 2.1.4b). No color logic
// duplicated from child components.
export const container = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: vars.space["3"],
  padding: `${vars.space["2"]} 0`,
});
