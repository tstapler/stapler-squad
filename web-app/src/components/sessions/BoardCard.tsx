"use client";

import { GripVertical } from "lucide-react";
import { useDraggable } from "@dnd-kit/core";
import { CSS } from "@dnd-kit/utilities";
import { makeCompositeId } from "@/lib/board/compositeId";
import { getBoardColumnKey, type BoardColumnKey } from "@/lib/board/columns";
import { SessionCard, type SessionCardProps } from "./SessionCard";
import { MoveToMenu } from "./MoveToMenu";
import { cardWrapper, cardBody, cardControls, cardDragging, dragHandle } from "./BoardCard.css";

// BoardCard accepts the exact SessionCard callback surface (see SessionCard.tsx's
// SessionCardProps) so SessionBoard/BoardColumn can pass it straight through unchanged --
// this wrapper only adds board-specific chrome (the drag handle + MoveToMenu), it doesn't
// fork or duplicate SessionCard's rendering.
export interface BoardCardProps extends SessionCardProps {
  /**
   * Scopes this card's dnd-kit draggable id (`${rowKey}:${session.id}`) uniquely within the
   * DndContext -- `"__default__"` until Phase 6 wires real swimlane row keys.
   */
  rowKey: string;
  /**
   * Non-drag fallback (WCAG SC 2.5.7): moves this card to `toColumn` via the same
   * attemptColumnMove path a completed drag would use. Always wired by SessionBoard -- every
   * card must offer a way to move it without dragging.
   */
  onMoveToColumn?: (toColumn: BoardColumnKey) => void;
}

/**
 * Thin wrapper around SessionCard adding a drag-handle affordance and a MoveToMenu (non-drag
 * fallback) for the board view. The handle (not the whole card) is the dnd-kit drag source, so
 * normal card clicks/scrolling are unaffected.
 */
export function BoardCard({ rowKey, onMoveToColumn, ...sessionCardProps }: BoardCardProps) {
  const { session } = sessionCardProps;
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: makeCompositeId(rowKey, session.id),
  });
  const currentColumn = getBoardColumnKey(session);

  return (
    <div
      ref={setNodeRef}
      className={isDragging ? `${cardWrapper} ${cardDragging}` : cardWrapper}
      data-testid="board-card"
      data-session-id={session.id}
      style={transform ? { transform: CSS.Translate.toString(transform) } : undefined}
    >
      <div className={cardControls}>
        <button
          type="button"
          id={`board-card-drag-handle-${session.id}`}
          className={dragHandle}
          aria-label={`Drag ${session.title} to move`}
          {...listeners}
          {...attributes}
        >
          <GripVertical size={16} aria-hidden="true" />
        </button>
        <MoveToMenu
          session={session}
          currentColumn={currentColumn}
          onMove={(toColumn) => onMoveToColumn?.(toColumn)}
        />
      </div>
      <div className={cardBody}>
        <SessionCard {...sessionCardProps} />
      </div>
    </div>
  );
}
