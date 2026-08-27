"use client";

import { GripVertical } from "lucide-react";
import { useDraggable } from "@dnd-kit/core";
import { CSS } from "@dnd-kit/utilities";
import { makeCompositeId } from "@/lib/board/compositeId";
import { SessionCard, type SessionCardProps } from "./SessionCard";
import { cardWrapper, cardBody, cardDragging, dragHandle } from "./BoardCard.css";

// BoardCard accepts the exact SessionCard callback surface (see SessionCard.tsx's
// SessionCardProps) so SessionBoard/BoardColumn can pass it straight through unchanged --
// this wrapper only adds board-specific chrome (the drag handle), it doesn't fork or
// duplicate SessionCard's rendering.
export interface BoardCardProps extends SessionCardProps {
  /**
   * Scopes this card's dnd-kit draggable id (`${rowKey}:${session.id}`) uniquely within the
   * DndContext -- `"__default__"` until Phase 6 wires real swimlane row keys.
   */
  rowKey: string;
}

/**
 * Thin wrapper around SessionCard adding a drag-handle affordance for the board view. The
 * handle (not the whole card) is the dnd-kit drag source, so normal card clicks/scrolling
 * are unaffected.
 */
export function BoardCard({ rowKey, ...sessionCardProps }: BoardCardProps) {
  const { session } = sessionCardProps;
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: makeCompositeId(rowKey, session.id),
  });

  return (
    <div
      ref={setNodeRef}
      className={isDragging ? `${cardWrapper} ${cardDragging}` : cardWrapper}
      data-testid="board-card"
      style={transform ? { transform: CSS.Translate.toString(transform) } : undefined}
    >
      <button
        type="button"
        className={dragHandle}
        aria-label={`Drag ${session.title} to move`}
        {...listeners}
        {...attributes}
      >
        <GripVertical size={16} aria-hidden="true" />
      </button>
      <div className={cardBody}>
        <SessionCard {...sessionCardProps} />
      </div>
    </div>
  );
}
