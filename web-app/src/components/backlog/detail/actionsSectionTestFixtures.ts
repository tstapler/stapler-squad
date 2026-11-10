import type { ManualReviewControls } from "./ActionsSection";

const noop = () => {};

/** The manual-review form closed, with inert callbacks, for ActionsSection tests that do not exercise it. */
export const noManualReview: ManualReviewControls = {
  show: false,
  outcome: "PASS",
  summary: "",
  onOutcomeChange: noop,
  onSummaryChange: noop,
  onSubmit: noop,
  onCancel: noop,
};
