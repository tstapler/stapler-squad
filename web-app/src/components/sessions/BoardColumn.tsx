"use client";

import { useRef } from "react";
import { useDroppable } from "@dnd-kit/core";
import { useVirtualizer } from "@tanstack/react-virtual";
import type { Session } from "@/gen/session/v1/types_pb";
import type { BoardColumnKey } from "@/lib/board/columns";
import { makeCompositeId } from "@/lib/board/compositeId";
import { mergeRefs } from "@/lib/utils/mergeRefs";
import { BoardCard, type BoardCardProps } from "./BoardCard";
import {
  column,
  columnDropOver,
  columnHeader,
  columnTitle,
  columnCount,
  columnCards,
  emptyColumn,
} from "./BoardColumn.css";

// Rough card height used only to seed the virtualizer's initial range before
// measureElement reports real sizes — SessionCard's actual height varies with content.
const ESTIMATED_CARD_HEIGHT = 220;

export interface BoardColumnProps {
  column: { key: BoardColumnKey; label: string };
  sessions: Session[];
  /**
   * Scopes this column's dnd-kit droppable id (`${rowKey}:${column.key}`) uniquely within the
   * DndContext -- `"__default__"` until Phase 6 wires real swimlane row keys.
   */
  rowKey: string;
  /** Builds the SessionCard-shaped callback props for one session, minus `session`/`rowKey`. */
  getCardProps: (session: Session) => Omit<BoardCardProps, "session" | "rowKey">;
}

/**
 * One board column: header with a count badge, plus a virtualized, independently-scrolling
 * card list. Each BoardColumn owns its own @tanstack/react-virtual instance — never a single
 * board-wide virtualizer — so columns scroll and window their DOM nodes independently.
 */
export function BoardColumn({ column: col, sessions, rowKey, getCardProps }: BoardColumnProps) {
  // The scrollable card-list container below is the same DOM node useDroppable's setNodeRef
  // attaches to via mergeRefs, alongside the virtualizer's own scroll ref.
  const scrollRef = useRef<HTMLDivElement>(null);
  const { setNodeRef, isOver } = useDroppable({ id: makeCompositeId(rowKey, col.key) });

  const virtualizer = useVirtualizer({
    count: sessions.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ESTIMATED_CARD_HEIGHT,
    overscan: 4,
    measureElement: (el) => el.getBoundingClientRect().height,
  });

  return (
    <section className={column} aria-label={`${col.label} column`} data-testid={`board-column-${col.key}`}>
      <div className={columnHeader}>
        <h3 className={columnTitle}>{col.label}</h3>
        <span className={columnCount} aria-label={`${sessions.length} sessions`}>
          {sessions.length}
        </span>
      </div>

      <div
        ref={mergeRefs(scrollRef, setNodeRef)}
        className={isOver ? `${columnCards} ${columnDropOver}` : columnCards}
        role="list"
        aria-label={`${col.label} sessions`}
        data-testid={`board-column-cards-${col.key}`}
      >
        {sessions.length === 0 ? (
          <p className={emptyColumn} data-testid={`board-column-empty-${col.key}`}>
            No sessions
          </p>
        ) : (
          <div
            style={{
              height: virtualizer.getTotalSize(),
              width: "100%",
              position: "relative",
            }}
          >
            {virtualizer.getVirtualItems().map((virtualItem) => {
              const session = sessions[virtualItem.index];
              if (!session) return null;
              return (
                <div
                  key={session.id}
                  role="listitem"
                  ref={virtualizer.measureElement}
                  data-index={virtualItem.index}
                  style={{
                    position: "absolute",
                    top: 0,
                    left: 0,
                    width: "100%",
                    transform: `translateY(${virtualItem.start}px)`,
                  }}
                >
                  <BoardCard session={session} rowKey={rowKey} {...getCardProps(session)} />
                </div>
              );
            })}
          </div>
        )}
      </div>
    </section>
  );
}
