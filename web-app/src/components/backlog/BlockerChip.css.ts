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

// Interactive retry states — the button reuses getStuckReasonClass()'s color
// styling verbatim (see BlockerChip.tsx); these only add affordance/feedback
// on top of it, mirroring StuckItem.css.ts's retry button conventions.
export const wrapper = style({
  display: "inline-flex",
  flexDirection: "column",
  alignItems: "flex-start",
  gap: vars.space["1"],
});

export const errorText = style({
  color: vars.color.errorText,
  fontSize: vars.fontSize.sm,
});
