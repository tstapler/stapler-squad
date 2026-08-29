"use client";

import type { Session } from "@/gen/session/v1/types_pb";
import { BOARD_COLUMNS, type BoardColumnKey } from "@/lib/board/columns";
import { BoardColumn } from "./BoardColumn";
import type { BoardCardProps } from "./BoardCard";
import { swimlane, swimlaneLabel, swimlaneRow } from "./BoardSwimlane.css";

export interface BoardSwimlaneProps {
  /** `GroupedSessions.groupKey` -- scopes every column/card composite id uniquely within this row. */
  rowKey: string;
  /** `GroupedSessions.displayName` -- the row label (e.g. a branch name, tag, or category). */
  displayName: string;
  /** Sessions already bucketed into the 4 board columns for this row only. */
  buckets: Record<BoardColumnKey, Session[]>;
  getCardProps: (session: Session) => Omit<BoardCardProps, "session" | "rowKey">;
}

/**
 * One swimlane row: a row label plus one BoardColumn per BoardColumnKey. Reuses BoardColumn
 * unmodified -- it doesn't need to know it's inside a swimlane, since `rowKey` already scopes
 * its dnd-kit ids (`${rowKey}:${column.key}`) uniquely across every row in the DndContext.
 */
export function BoardSwimlane({ rowKey, displayName, buckets, getCardProps }: BoardSwimlaneProps) {
  return (
    <section className={swimlane} aria-label={`${displayName} swimlane`} data-testid={`board-swimlane-${rowKey}`}>
      <h2 className={swimlaneLabel}>{displayName}</h2>
      <div className={swimlaneRow}>
        {BOARD_COLUMNS.map((col) => (
          <BoardColumn
            key={col.key}
            column={col}
            rowKey={rowKey}
            sessions={buckets[col.key]}
            getCardProps={getCardProps}
          />
        ))}
      </div>
    </section>
  );
}
