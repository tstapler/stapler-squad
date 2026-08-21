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

/** Deep link from the detail page's BlockerChip to the pre-filtered /unfinished tab. */
export const unfinishedLink = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.primary,
  whiteSpace: "nowrap",
});

/** D6 fix (Task 3.1.4g): compact "Pipeline: <name>" badge next to the tracker. */
export const pipelineBadge = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.full,
  border: `1px solid ${vars.color.inputBorder}`,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  whiteSpace: "nowrap",
});
