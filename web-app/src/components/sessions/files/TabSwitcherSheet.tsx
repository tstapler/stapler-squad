"use client";

import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import type { Buffer } from "@/lib/files/bufferReducer";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import { useTrayDismissal } from "@/lib/hooks/useTrayDismissal";
import { accessibleName, baseName, isDirty, labelOf } from "./tabLabels";
import {
  backdrop, sheet, sheetHeader, heading, list, row, rowActive, rowMain, rowTitle, rowPath, stateWord, dirtyMark,
  iconButton, notice, empty, textButton, rowMenu, menuItem,
} from "./TabSwitcherSheet.css";

export const TARGET_PX = 44;
const LONG_PRESS_MS = 500;

export interface TabSwitcherSheetProps {
  buffers: readonly Buffer[];
  activeIndex: number;
  /** Set when the tab cap refused an open: pinned to the top, with the closable rows' Close highlighted (U-TAB-10). */
  refusal: string | null;
  onSelect: (path: string) => void;
  /** `button` is the row's ×; `context_menu` is the row menu's Close. */
  onClose: (path: string, method: "button" | "context_menu") => void;
  onPin: (path: string) => void;
  onUnpin: (path: string) => void;
  onCloseOthers: (path: string) => void;
  onCopyPath: (path: string) => void;
  onDismiss: () => void;
  onGoToFiles: () => void;
}

const stateOf = (b: Buffer) => (isDirty(b) ? "unsaved" : b.pinned ? "pinned" : "preview");

/** Full-width bottom sheet listing the open buffers; it owns one history entry so system Back dismisses it. */
export function TabSwitcherSheet(p: TabSwitcherSheetProps) {
  const ref = useRef<HTMLDivElement>(null);
  const invoker = useRef<HTMLElement | null>(typeof document === "undefined" ? null : (document.activeElement as HTMLElement | null));
  const [menuFor, setMenuFor] = useState<string | null>(null);
  const pressTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const longPressed = useRef(false);

  useTrayDismissal({ isOpen: true, isSheet: true, modal: false, trayRef: ref, close: p.onDismiss });
  useFocusTrap(ref, true, invoker);
  useEffect(() => () => { if (pressTimer.current) clearTimeout(pressTimer.current); }, []);

  const startPress = (path: string) => {
    longPressed.current = false;
    pressTimer.current = setTimeout(() => { longPressed.current = true; setMenuFor(path); }, LONG_PRESS_MS);
  };
  const endPress = () => { if (pressTimer.current) clearTimeout(pressTimer.current); };
  const onMenuKey = (e: KeyboardEvent) => {
    if (e.key !== "Escape" || !menuFor) return;
    e.stopPropagation();
    setMenuFor(null);
  };

  return (
    <div className={backdrop} data-testid="tab-switcher-backdrop" onClick={(e) => { if (e.target === e.currentTarget) p.onDismiss(); }}>
      <div ref={ref} className={sheet} role="dialog" aria-modal="true" aria-label="Open files" data-testid="tab-switcher-sheet" onKeyDown={onMenuKey}>
        <div className={sheetHeader}>
          <h2 className={heading}>Open files ({p.buffers.length})</h2>
          <button type="button" className={iconButton} style={{ minWidth: TARGET_PX }} aria-label="Close tab list" onClick={p.onDismiss}>×</button>
        </div>
        {p.refusal && <div className={notice} role="note">{p.refusal}</div>}
        {p.buffers.length === 0 ? (
          <div className={empty}>
            <span>No open files - open one from Files</span>
            <button type="button" className={textButton} onClick={p.onGoToFiles}>Go to Files</button>
          </div>
        ) : (
          <ul className={list} aria-label="Open files">
            {p.buffers.map((b, i) => {
              const label = labelOf(b.path, p.buffers);
              const closable = !isDirty(b);
              return (
                <li key={b.path} className={`${row} ${i === p.activeIndex ? rowActive : ""}`} data-testid={`tab-sheet-row-${b.path}`} style={{ minHeight: TARGET_PX }}>
                  <button
                    type="button"
                    className={rowMain}
                    style={{ minHeight: TARGET_PX }}
                    aria-label={accessibleName(b, p.buffers)}
                    aria-current={i === p.activeIndex ? "true" : undefined}
                    onPointerDown={() => startPress(b.path)}
                    onPointerUp={endPress}
                    onPointerLeave={endPress}
                    onPointerCancel={endPress}
                    onContextMenu={(e) => { e.preventDefault(); setMenuFor(b.path); }}
                    onClick={() => { if (longPressed.current) { longPressed.current = false; return; } p.onSelect(b.path); }}
                    onDoubleClick={() => p.onPin(b.path)}
                  >
                    <span className={rowTitle} aria-hidden="true">
                      {isDirty(b) && <span className={dirtyMark} />}
                      {b.pinned && !isDirty(b) && <span>📌</span>}
                      <span>{label}</span>
                      <span className={stateWord}>({stateOf(b)})</span>
                    </span>
                    <span className={rowPath} aria-hidden="true"><bdi>{b.path}</bdi></span>
                  </button>
                  <button type="button" className={iconButton} style={{ minHeight: TARGET_PX }} aria-label={`Actions for ${label}`} aria-haspopup="menu" aria-expanded={menuFor === b.path} onClick={() => setMenuFor(menuFor === b.path ? null : b.path)}>⋯</button>
                  <button
                    type="button"
                    className={iconButton}
                    style={{ minHeight: TARGET_PX }}
                    data-highlight={p.refusal && closable ? "true" : undefined}
                    aria-label={p.refusal && closable ? `Close ${label}, frees a slot` : `Close ${label}`}
                    onClick={() => p.onClose(b.path, "button")}
                  >×</button>
                  {menuFor === b.path && (
                    <RowMenu
                      buffer={b}
                      count={p.buffers.length}
                      onDone={() => setMenuFor(null)}
                      actions={{ pin: p.onPin, unpin: p.onUnpin, close: (path) => p.onClose(path, "context_menu"), closeOthers: p.onCloseOthers, copy: p.onCopyPath }}
                      label={baseName(b.path)}
                    />
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}

interface RowMenuProps {
  buffer: Buffer;
  count: number;
  label: string;
  onDone: () => void;
  actions: Record<"pin" | "unpin" | "close" | "closeOthers" | "copy", (path: string) => void>;
}

function RowMenu({ buffer, count, label, onDone, actions }: RowMenuProps) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => { ref.current?.querySelector<HTMLElement>("[role=menuitem]")?.focus(); }, []);
  const items: [string, () => void, boolean?][] = [
    [buffer.pinned ? "Unpin" : "Pin", () => (buffer.pinned ? actions.unpin : actions.pin)(buffer.path), buffer.pinned && isDirty(buffer)],
    ["Close", () => actions.close(buffer.path)],
    ["Close others", () => actions.closeOthers(buffer.path), count < 2],
    ["Copy path", () => actions.copy(buffer.path)],
  ];
  return (
    <div ref={ref} className={rowMenu} role="menu" aria-label={`${label} actions`}>
      {items.map(([name, run, disabled]) => (
        <button key={name} type="button" role="menuitem" className={menuItem} style={{ minHeight: TARGET_PX }} disabled={disabled} onClick={() => { run(); onDone(); }}>
          {name}
        </button>
      ))}
    </div>
  );
}
