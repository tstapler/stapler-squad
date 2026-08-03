import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// "Show N more" control (useShowMore) — shared with WorkflowHistorySection/
// ProgressHistorySection/SessionsSection, see detailShared.css.ts.
export { showMoreButton } from "./detailShared.css";

// Distinguishes RespawnHistorySection's all-time count from BlockerChip's
// episode-scoped ×N — see the caption this class styles in the component.
export const reconciliationCaption = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  marginTop: `calc(-1 * ${vars.space["1"]})`,
  marginBottom: vars.space["2"],
});
