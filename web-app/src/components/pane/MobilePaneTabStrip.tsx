"use client";

import { forwardRef, useMemo, useRef, useState } from "react";
import { DndContext, PointerSensor, TouchSensor, useDraggable, useDroppable, useSensor, useSensors, type DragEndEvent } from "@dnd-kit/core";
import { CSS } from "@dnd-kit/utilities";
import type { Session } from "@/gen/session/v1/types_pb";
import type { LeafPane, PaneId } from "@/lib/pane/paneTypes";
import { titleFirstLine } from "@/lib/pane/paneUtils";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import { useTrayDismissal } from "@/lib/hooks/useTrayDismissal";
import {
  mobileTabStrip, mobileSwitcherButton, mobileSwitcherLabel, mobileSwitcherCount, mobileAddPaneButton,
  sheetBackdrop, sheet, sheetHeader, sheetSearch, sheetClose, sheetList, sheetItem, sheetHandle, sheetRow, sheetRowTitle, sheetRowMeta, sheetEmpty,
} from "@/styles/pane/mobilePaneTabStrip.css";

interface MobilePaneTabStripProps {
  leaves: LeafPane[];
  focusedPaneId: PaneId;
  sessions: Session[];
  onFocus: (paneId: PaneId) => void;
  onAddPane?: () => void;
  /** Moves the pane `fromPaneId` to the position of `toPaneId`. Omit to hide the drag handles. */
  onReorder?: (fromPaneId: PaneId, toPaneId: PaneId) => void;
}

interface PaneEntry {
  id: PaneId;
  label: string;
  meta: string;
}

function entryFor(leaf: LeafPane, sessions: Session[]): PaneEntry {
  const session = leaf.sessionId ? sessions.find((s) => s.id === leaf.sessionId) ?? null : null;
  if (!session) {
    return { id: leaf.id, label: leaf.viewKind === "session-list" ? "Sessions" : "Empty", meta: "" };
  }
  // First line only: a multi-paragraph title would otherwise dominate the row.
  const label = titleFirstLine(session.title);
  const meta = [session.branch, session.program, session.category].filter(Boolean).join(" · ");
  return { id: leaf.id, label, meta };
}

function SortableRow({ entry, selected, canDrag, onPick }: { entry: PaneEntry; selected: boolean; canDrag: boolean; onPick: () => void }) {
  const drag = useDraggable({ id: entry.id, disabled: !canDrag });
  const drop = useDroppable({ id: entry.id });
  return (
    <li
      ref={drop.setNodeRef}
      role="presentation"
      className={sheetItem}
      data-over={drop.isOver && !drag.isDragging ? "true" : "false"}
      data-dragging={drag.isDragging ? "true" : "false"}
      style={{ transform: CSS.Translate.toString(drag.transform) }}
    >
      <button type="button" role="option" aria-selected={selected} className={sheetRow} onClick={onPick}>
        {/* analytics-exempt */}
        <span className={sheetRowTitle}>{entry.label}</span>
        {entry.meta && <span className={sheetRowMeta}>{entry.meta}</span>}
      </button>
      {canDrag && (
        <button
          type="button"
          ref={drag.setNodeRef}
          className={sheetHandle}
          aria-label={`Drag to reorder ${entry.label}`}
          {...drag.attributes}
          {...drag.listeners}
        >
          {/* analytics-exempt */}
          ⠿
        </button>
      )}
    </li>
  );
}

export const MobilePaneTabStrip = forwardRef<HTMLDivElement, MobilePaneTabStripProps>(
  function MobilePaneTabStrip({ leaves, focusedPaneId, sessions, onFocus, onAddPane, onReorder }, ref) {
    const [open, setOpen] = useState(false);
    const [query, setQuery] = useState("");
    const sheetRef = useRef<HTMLDivElement>(null);
    const invoker = useRef<HTMLElement | null>(null);

    const entries = useMemo(() => leaves.map((l) => entryFor(l, sessions)), [leaves, sessions]);
    const q = query.trim().toLowerCase();
    const visible = q ? entries.filter((e) => `${e.label} ${e.meta}`.toLowerCase().includes(q)) : entries;

    const sensors = useSensors(
      useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
      useSensor(TouchSensor, { activationConstraint: { delay: 150, tolerance: 8 } }),
    );
    // Reordering a filtered subset would be ambiguous (where do hidden rows go?), so only the full list drags.
    const canDrag = !!onReorder && !q;
    const handleDragEnd = (e: DragEndEvent) => {
      if (e.over && e.active.id !== e.over.id) onReorder?.(String(e.active.id), String(e.over.id));
    };

    const close = () => { setOpen(false); setQuery(""); };

    useTrayDismissal({ isOpen: open, isSheet: true, modal: false, trayRef: sheetRef, close });
    useFocusTrap(sheetRef, open, invoker);

    if (leaves.length <= 1) return null;

    const active = entries.find((e) => e.id === focusedPaneId) ?? entries[0];

    return (
      <div ref={ref} className={mobileTabStrip}>
        <button
          type="button"
          className={mobileSwitcherButton}
          aria-haspopup="dialog"
          aria-label={`Switch pane, current: ${active.label}`}
          title={active.label}
          onClick={(e) => { invoker.current = e.currentTarget; setOpen(true); }}
        >
          {/* analytics-exempt */}
          <span className={mobileSwitcherLabel}>{active.label}</span>
          <span className={mobileSwitcherCount}>{leaves.length} ▴</span>
        </button>
        {onAddPane && (
          // analytics-exempt
          <button type="button" className={mobileAddPaneButton} onClick={onAddPane} title="Add new pane" aria-label="Add new pane">
            +
          </button>
        )}
        {open && (
          // analytics-exempt
          <div className={sheetBackdrop} onClick={(e) => { if (e.target === e.currentTarget) close(); }}>
            <div ref={sheetRef} className={sheet} role="dialog" aria-modal="true" aria-label="Switch pane">
              <div className={sheetHeader}>
                <input
                  type="search"
                  className={sheetSearch}
                  placeholder="Filter panes"
                  aria-label="Filter panes"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                />
                <button type="button" className={sheetClose} aria-label="Close pane list" onClick={close}>
                  {/* analytics-exempt */}
                  ×
                </button>
              </div>
              {visible.length === 0 ? (
                <div className={sheetEmpty}>No panes match “{query}”</div>
              ) : (
                <DndContext sensors={sensors} onDragEnd={handleDragEnd}>
                  <ul className={sheetList} role="listbox" aria-label="Panes">
                    {visible.map((e) => (
                      <SortableRow
                        key={e.id}
                        entry={e}
                        selected={e.id === focusedPaneId}
                        canDrag={canDrag}
                        onPick={() => { onFocus(e.id); close(); }}
                      />
                    ))}
                  </ul>
                </DndContext>
              )}
            </div>
          </div>
        )}
      </div>
    );
  },
);
