import { style, styleVariants, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// Shared badge shell -- mirrors SessionCard.css.ts's `status` base (padding/
// radius/font sizing), reused across all three connection-state variants
// below rather than duplicated per-variant.
export const badge = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["1"]} 10px`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
});

// statusConnected/statusReconnecting/statusDisconnected: named to match the
// statusPaused/statusPausedDistinct family already established in
// SessionCard.css.ts -- statusXxx names the badge's semantic state, not its
// visual treatment.
export const statusConnected = style({
  background: vars.color.successBg,
  color: vars.color.successText,
});

export const statusReconnecting = style({
  background: vars.color.warningBg,
  color: vars.color.warningText,
  border: `1px solid ${vars.color.warning}`,
});

export const statusDisconnected = style({
  background: vars.color.errorBg,
  color: vars.color.errorText,
  border: `1px solid ${vars.color.error}`,
});

const dotBase = style({
  width: 8,
  height: 8,
  borderRadius: "50%",
  flexShrink: 0,
  display: "inline-block",
});

export const dots = styleVariants({
  connected: [dotBase, { background: vars.color.success }],
  reconnecting: [dotBase, { background: vars.color.warning }],
  disconnected: [dotBase, { background: vars.color.error }],
});

// Spinner shown in place of the static dot while reconnecting -- mirrors
// layout/ConnectionIndicator.css.ts's `spinner` treatment for the
// session-stream indicator's own reconnecting state.
const spin = keyframes({
  from: { transform: "rotate(0deg)" },
  to: { transform: "rotate(360deg)" },
});

export const spinner = style({
  width: 8,
  height: 8,
  borderRadius: "50%",
  flexShrink: 0,
  display: "inline-block",
  border: `2px solid ${vars.color.warning}`,
  borderTopColor: "transparent",
  animation: `${spin} 0.8s linear infinite`,
});

// Visually-hidden persistent aria-live="polite" region -- byte-identical
// shape to layout/ConnectionIndicator.css.ts's ariaLiveRegion, kept as its
// own copy here (rather than a shared import) since the two indicators are
// independent components with no reason to couple their stylesheets.
export const ariaLiveRegion = style({
  position: "absolute",
  width: 1,
  height: 1,
  padding: 0,
  margin: -1,
  overflow: "hidden",
  clip: "rect(0, 0, 0, 0)",
  whiteSpace: "nowrap",
  borderWidth: 0,
});
