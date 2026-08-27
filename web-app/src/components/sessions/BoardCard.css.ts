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

// touchAction is set on the handle only (not cardWrapper/cardBody) so the card itself keeps
// normal touch scrolling — only this element becomes a drag surface once Phase 3 wires
// useDraggable. cursor: grab is applied now for the same reason: no retrofit later.
export const dragHandle = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  flexShrink: 0,
  width: "20px",
  height: "20px",
  marginTop: vars.space["2"],
  padding: 0,
  border: "none",
  background: "transparent",
  color: vars.color.textMuted,
  cursor: "grab",
  touchAction: "none",
});
