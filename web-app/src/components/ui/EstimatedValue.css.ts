// +feature: estimated-value
import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

// estimatedValueMarker is the one shared visual treatment for any
// modeled/heuristic number (per-tool cost, activity cost, cache ROI, waste
// score) — see research/pitfalls.md §2 and research/ux.md §4. Muted weight,
// not muted color: the "~" prefix plus tooltip is the actual signal, so the
// number itself must stay legible.
export const estimatedValueMarker = style({
  fontWeight: vars.fontWeight.normal,
  color: vars.color.textSecondary,
  cursor: "help",
  borderBottom: `1px dotted ${vars.color.borderColor}`,
});

// Visually-hidden but screen-reader-visible tooltip text — mirrors
// SessionDetailDrawer.css.ts's srOnly (same visually-hidden pattern, no
// shared base class between these two components yet).
export const srOnly = style({
  position: "absolute",
  width: 1,
  height: 1,
  overflow: "hidden",
  clip: "rect(0,0,0,0)",
});
