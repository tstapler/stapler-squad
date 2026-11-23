"use client";

import { forwardRef, useMemo, useRef, useState } from "react";
import type { Session } from "@/gen/session/v1/types_pb";
import type { LeafPane, PaneId } from "@/lib/pane/paneTypes";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import { useTrayDismissal } from "@/lib/hooks/useTrayDismissal";
import {
  mobileTabStrip, mobileSwitcherButton, mobileSwitcherLabel, mobileSwitcherCount, mobileAddPaneButton,
  sheetBackdrop, sheet, sheetHeader, sheetSearch, sheetClose, sheetList, sheetRow, sheetRowTitle, sheetRowMeta, sheetEmpty,
} from "@/styles/pane/mobilePaneTabStrip.css";

interface MobilePaneTabStripProps {
  leaves: LeafPane[];
  focusedPaneId: PaneId;
  sessions: Session[];
  onFocus: (paneId: PaneId) => void;
  onAddPane?: () => void;
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
  const label = session.title.split("\n")[0].trim() || session.title;
  const meta = [session.branch, session.program, session.category].filter(Boolean).join(" · ");
  return { id: leaf.id, label, meta };
}

export const MobilePaneTabStrip = forwardRef<HTMLDivElement, MobilePaneTabStripProps>(
  function MobilePaneTabStrip({ leaves, focusedPaneId, sessions, onFocus, onAddPane }, ref) {
    const [open, setOpen] = useState(false);
    const [query, setQuery] = useState("");
    const sheetRef = useRef<HTMLDivElement>(null);
    const invoker = useRef<HTMLElement | null>(null);

    const entries = useMemo(() => leaves.map((l) => entryFor(l, sessions)), [leaves, sessions]);
    const q = query.trim().toLowerCase();
    const visible = q ? entries.filter((e) => `${e.label} ${e.meta}`.toLowerCase().includes(q)) : entries;

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
                <ul className={sheetList} role="listbox" aria-label="Panes">
                  {visible.map((e) => (
                    <li key={e.id} role="presentation">
                      <button
                        type="button"
                        role="option"
                        aria-selected={e.id === focusedPaneId}
                        className={sheetRow}
                        onClick={() => { onFocus(e.id); close(); }}
                      >
                        {/* analytics-exempt */}
                        <span className={sheetRowTitle}>{e.label}</span>
                        {e.meta && <span className={sheetRowMeta}>{e.meta}</span>}
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </div>
        )}
      </div>
    );
  },
);
