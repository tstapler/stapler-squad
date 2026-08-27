import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const cardWrapper = style({
  display: "flex",
  alignItems: "flex-start",
  gap: vars.space["1"],
});

// Applied to the dragged card itself while a drag is in flight (dnd-kit's isDragging).
export const cardDragging = style({
  opacity: 0.5,
});

export const cardBody = style({
  flex: 1,
  minWidth: 0,
});

// Stacks the drag handle above the MoveToMenu trigger -- both are non-drag-surface controls
// that live in the same narrow rail to the left of the card body.
export const cardControls = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  gap: vars.space["1"],
  flexShrink: 0,
});

// touchAction is set on the handle only (not cardWrapper/cardBody) so the card itself keeps
// normal touch scrolling — only this element becomes a drag surface once Phase 3 wires
// useDraggable. cursor: grab is applied now for the same reason: no retrofit later.
// 44x44px meets the minimum touch target (backlog AC9, mirrors MoveToMenu.css.ts's
// menuTrigger) -- padding does the sizing work, the icon itself stays small (16px).
export const dragHandle = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  flexShrink: 0,
  width: "44px",
  height: "44px",
  padding: 0,
  border: "none",
  background: "transparent",
  color: vars.color.textMuted,
  cursor: "grab",
  touchAction: "none",
});
