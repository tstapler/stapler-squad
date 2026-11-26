"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import type { Session } from "@/gen/session/v1/types_pb";
import { titleFirstLine } from "@/lib/pane/paneUtils";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import { useTrayDismissal } from "@/lib/hooks/useTrayDismissal";
import {
  sheetBackdrop, sheet, sheetHeader, sheetSearch, sheetClose, sheetList, sheetItem, sheetRow, sheetRowTitle, sheetRowMeta, sheetEmpty,
} from "@/styles/pane/mobilePaneTabStrip.css";
import { pickerSheetWide, pickerTitle } from "@/styles/pane/sessionPickerDialog.css";

interface SessionPickerDialogProps {
  sessions: Session[];
  /** Sessions already shown in a pane; picking one moves it here. */
  openSessionIds: ReadonlySet<string>;
  onPick: (sessionId: string) => void;
  onClose: () => void;
}

/** Searchable session chooser used to fill an empty pane. */
export function SessionPickerDialog({ sessions, openSessionIds, onPick, onClose }: SessionPickerDialogProps) {
  const [query, setQuery] = useState("");
  const ref = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const invoker = useRef<HTMLElement | null>(
    typeof document === "undefined" ? null : (document.activeElement as HTMLElement | null),
  );

  useTrayDismissal({ isOpen: true, isSheet: true, modal: false, trayRef: ref, close: onClose });
  useFocusTrap(ref, true, invoker);
  useEffect(() => { searchRef.current?.focus(); }, []);

  const q = query.trim().toLowerCase();
  const visible = useMemo(() => {
    const live = sessions.filter((s) => !s.archivedAt);
    if (!q) return live;
    return live.filter((s) => `${s.title} ${s.branch} ${s.program} ${s.category}`.toLowerCase().includes(q));
  }, [sessions, q]);

  return (
    <div className={sheetBackdrop} onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div ref={ref} className={`${sheet} ${pickerSheetWide}`} role="dialog" aria-modal="true" aria-labelledby="session-picker-title">
        <h2 id="session-picker-title" className={pickerTitle}>Open a session in this pane</h2>
        <div className={sheetHeader}>
          <input
            ref={searchRef}
            type="search"
            className={sheetSearch}
            placeholder="Filter sessions"
            aria-label="Filter sessions"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter" && visible.length > 0) onPick(visible[0].id); }}
          />
          <button type="button" className={sheetClose} aria-label="Close session picker" onClick={onClose}>
            {/* analytics-exempt */}
            ×
          </button>
        </div>
        {visible.length === 0 ? (
          <div className={sheetEmpty}>{q ? `No sessions match “${query}”` : "No sessions yet"}</div>
        ) : (
          <ul className={sheetList} aria-label="Sessions">
            {visible.map((s) => {
              const meta = [s.branch, s.program, s.category].filter(Boolean).join(" · ");
              const open = openSessionIds.has(s.id);
              return (
                <li key={s.id} className={sheetItem}>
                  <button type="button" className={sheetRow} onClick={() => onPick(s.id)}>
                    {/* analytics-exempt */}
                    <span className={sheetRowTitle}>{titleFirstLine(s.title)}</span>
                    {(meta || open) && (
                      <span className={sheetRowMeta}>{[meta, open ? "open in another pane — will move here" : ""].filter(Boolean).join(" · ")}</span>
                    )}
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}
