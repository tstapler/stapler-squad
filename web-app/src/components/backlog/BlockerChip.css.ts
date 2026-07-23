import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// Layout-only addition to the shared chip styling — colors/backgrounds are
// never redefined here. The per-StuckReason color convention lives solely in
// stuckReason.css.ts; BlockerChip reuses getStuckReasonClass() directly for
// that (see BlockerChip.tsx), matching the same reuse discipline
// BacklogItemBadge.tsx/status.ts already applies for status vocabulary.
export const duration = style({
  opacity: 0.75,
  fontWeight: vars.fontWeight.normal,
  marginLeft: vars.space["1"],
});
