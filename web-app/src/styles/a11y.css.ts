import { style } from "@vanilla-extract/css";

/**
 * Visually hides content while keeping it accessible to assistive tech — the
 * standard "sr-only" pattern used for `aria-live` announcer spans throughout
 * the app (ReviewQueuePanel, TriggersPanel, TriggerFormModal, CallbackSettings).
 * Hoisted here so every consumer shares one definition instead of hand-rolling
 * the same inline style object.
 */
export const visuallyHidden = style({
  position: "absolute",
  width: "1px",
  height: "1px",
  padding: 0,
  margin: "-1px",
  overflow: "hidden",
  clipPath: "inset(50%)",
  whiteSpace: "nowrap",
  border: 0,
});
