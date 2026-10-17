"use client";

import { useCallback, useEffect, useRef, useState, type KeyboardEvent, type MouseEvent } from "react";
import type { Buffer, BufferNotice } from "@/lib/files/bufferReducer";
import { bucketTabs } from "@/lib/files/analytics/events";
import { useFilesAnalytics } from "@/lib/files/analytics/filesAnalytics";
import {
  bar, tabList, tab as tabClass, tabLabel, pinGlyph, dirtyDot, closeButton, overflowButton, menu, menuItem,
  tabListDialog, listRow, toast, toastAction, srOnly,
} from "./OpenTabsBar.css";

const TOAST_MS = 8_000;

export interface OpenTabsBarProps {
  buffers: readonly Buffer[];
  activeIndex: number;
  notice: BufferNotice | null;
  /** Id of the tabpanel the strip controls. */
  panelId: string;
  onActivate: (path: string) => void;
  onClose: (path: string) => void;
  onPin: (path: string) => void;
  onUnpin: (path: string) => void;
  onMove: (path: string, delta: -1 | 1) => void;
  onCloseOthers: (path: string) => void;
  onCopyPath: (path: string) => void;
  onReveal: (path: string) => void;
  onUndoEvict: () => void;
}

const isDirty = (b: Buffer) => b.status === "loaded" && b.dirty;
const baseName = (path: string) => path.slice(path.lastIndexOf("/") + 1);

/** Basename, with the parent directory added only when two open tabs would read the same. */
function labelOf(path: string, all: readonly Buffer[]): string {
  const base = baseName(path);
  if (all.filter((b) => baseName(b.path) === base).length < 2) return base;
  const parts = path.split("/");
  return parts.length > 1 ? `${parts[parts.length - 2]}/${base}` : base;
}

function accessibleName(b: Buffer, all: readonly Buffer[]): string {
  const label = labelOf(b.path, all);
  if (isDirty(b)) return `${label}, unsaved`;
  return `${label}, ${b.pinned ? "pinned" : "preview"}`;
}

type Via = "strip" | "keyboard";
interface MenuState { path: string; x: number }

export function OpenTabsBar(p: OpenTabsBarProps) {
  const { buffers, activeIndex, notice, panelId } = p;
  const analytics = useFilesAnalytics({ component: "OpenTabsBar" });
  const tabRefs = useRef(new Map<string, HTMLElement>());
  const [menuState, setMenuState] = useState<MenuState | null>(null);
  const [listOpen, setListOpen] = useState(false);
  const [shown, setShown] = useState<BufferNotice | null>(null);
  const [paused, setPaused] = useState(false);

  const focusTab = (path: string) => tabRefs.current.get(path)?.focus();
  const activate = useCallback((path: string, via: Via) => {
    analytics.emit("files.tab.switch", { via });
    p.onActivate(path);
  }, [analytics, p]);
  const close = useCallback((path: string, method: "button" | "keyboard" | "context_menu") => {
    const b = buffers.find((x) => x.path === path);
    analytics.emit("files.tab.close", { dirty: b && isDirty(b) ? "true" : "false", method, open_tabs: bucketTabs(buffers.length) });
    p.onClose(path);
  }, [analytics, buffers, p]);

  // The reducer clears its notice on the next action; the bar keeps showing it until dismissed or timed out.
  useEffect(() => {
    if (!notice) return;
    setShown(notice);
    analytics.emit("files.tab.evict", { outcome: notice.kind === "evicted" ? "evicted_clean_preview" : "refused_pinned_dirty" });
    if (notice.kind === "refused") setListOpen(true);
  }, [notice, analytics]);
  useEffect(() => {
    if (shown?.kind !== "evicted" || paused) return;
    const t = setTimeout(() => setShown(null), TOAST_MS);
    return () => clearTimeout(t);
  }, [shown, paused]);

  const active = buffers[activeIndex];
  useEffect(() => {
    if (active) tabRefs.current.get(active.path)?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
  }, [active?.path]); // eslint-disable-line react-hooks/exhaustive-deps

  const onKeyDown = (e: KeyboardEvent<HTMLElement>, b: Buffer, index: number) => {
    if (e.altKey && e.shiftKey && (e.key === "ArrowLeft" || e.key === "ArrowRight")) {
      e.preventDefault();
      p.onMove(b.path, e.key === "ArrowLeft" ? -1 : 1);
      return;
    }
    if (e.key === "ContextMenu" || (e.shiftKey && e.key === "F10")) {
      e.preventDefault();
      setMenuState({ path: b.path, x: 8 });
      return;
    }
    if (e.key === "Delete") {
      e.preventDefault();
      const neighbor = buffers[index + 1] ?? buffers[index - 1];
      close(b.path, "keyboard");
      if (neighbor) {
        focusTab(neighbor.path);
        if (index === activeIndex) p.onActivate(neighbor.path);
      }
      return;
    }
    const target = e.key === "ArrowRight" ? (index + 1) % buffers.length
      : e.key === "ArrowLeft" ? (index - 1 + buffers.length) % buffers.length
      : e.key === "Home" ? 0 : e.key === "End" ? buffers.length - 1 : -1;
    if (target < 0) return;
    e.preventDefault();
    focusTab(buffers[target].path);
    activate(buffers[target].path, "keyboard");
  };

  const openMenu = (e: MouseEvent, path: string) => {
    e.preventDefault();
    setMenuState({ path, x: Math.max(8, e.clientX - e.currentTarget.getBoundingClientRect().left) });
  };

  const message = shown?.kind === "evicted"
    ? `Closed ${baseName(shown.path)} to stay within ${shown.cap} tabs`
    : shown?.kind === "refused" ? "Too many tabs open - close one" : "";

  return (
    <div className={bar} data-testid="files-tab-bar">
      <div className={tabList} role="tablist" aria-label="Open files">
        {buffers.map((b, i) => (
          <div
            key={b.path}
            ref={(el) => { if (el) tabRefs.current.set(b.path, el); else tabRefs.current.delete(b.path); }}
            role="tab"
            id={`files-tab-${i}`}
            className={tabClass}
            data-testid={`files-tab-${b.path}`}
            data-preview={!b.pinned && !isDirty(b) ? "true" : undefined}
            aria-selected={i === activeIndex}
            aria-controls={panelId}
            aria-label={accessibleName(b, buffers)}
            title={b.path}
            tabIndex={i === activeIndex || (activeIndex < 0 && i === 0) ? 0 : -1}
            onClick={() => activate(b.path, "strip")}
            onDoubleClick={() => p.onPin(b.path)}
            onAuxClick={(e) => { if (e.button === 1) { e.preventDefault(); close(b.path, "button"); } }}
            onContextMenu={(e) => openMenu(e, b.path)}
            onKeyDown={(e) => onKeyDown(e, b, i)}
          >
            {b.pinned && !isDirty(b) && <span className={pinGlyph} aria-hidden="true">📌</span>}
            {isDirty(b) && <span className={dirtyDot} data-testid={`files-tab-dirty-dot-${b.path}`} aria-hidden="true" />}
            <span className={tabLabel} data-tab-label>{labelOf(b.path, buffers)}</span>
            <button
              type="button"
              className={closeButton}
              tabIndex={-1}
              aria-label={`Close ${labelOf(b.path, buffers)}`}
              onClick={(e) => { e.stopPropagation(); close(b.path, "button"); }}
            >
              ×
            </button>
          </div>
        ))}
      </div>
      <button
        type="button"
        className={overflowButton}
        aria-label="List open tabs"
        aria-haspopup="dialog"
        aria-expanded={listOpen}
        onClick={() => setListOpen((o) => !o)}
      >
        ▾
      </button>

      {menuState && (
        <TabMenu
          path={menuState.path}
          x={menuState.x}
          buffers={buffers}
          onDismiss={() => { setMenuState(null); focusTab(menuState.path); }}
          actions={{
            close: () => close(menuState.path, "context_menu"),
            closeOthers: () => p.onCloseOthers(menuState.path),
            pin: () => p.onPin(menuState.path),
            unpin: () => p.onUnpin(menuState.path),
            move: (d) => p.onMove(menuState.path, d),
            copy: () => p.onCopyPath(menuState.path),
            reveal: () => p.onReveal(menuState.path),
          }}
        />
      )}

      {listOpen && (
        <div className={tabListDialog} role="dialog" aria-label="Open tabs" onKeyDown={(e) => { if (e.key === "Escape") setListOpen(false); }}>
          {shown?.kind === "refused" && <div role="note">{message}</div>}
          {buffers.map((b) => (
            <div key={b.path} className={listRow}>
              <button type="button" className={menuItem} onClick={() => { activate(b.path, "strip"); setListOpen(false); }}>
                {accessibleName(b, buffers)}
              </button>
              {b.pinned && !isDirty(b) && <button type="button" className={menuItem} onClick={() => p.onUnpin(b.path)}>Unpin {labelOf(b.path, buffers)}</button>}
              <button type="button" className={menuItem} aria-label={`Close ${labelOf(b.path, buffers)} from list`} onClick={() => close(b.path, "button")}>×</button>
            </div>
          ))}
        </div>
      )}

      <div className={srOnly} role="status" aria-live="polite">{message}</div>
      {shown?.kind === "evicted" && (
        <div
          className={toast}
          data-testid="files-tab-toast"
          role="group"
          aria-label="Tab closed"
          onMouseEnter={() => setPaused(true)}
          onMouseLeave={() => setPaused(false)}
          onFocus={() => setPaused(true)}
          onBlur={() => setPaused(false)}
        >
          <span>{message}</span>
          <button type="button" className={toastAction} onClick={() => { analytics.emit("files.tab.evict_undo"); p.onUndoEvict(); setShown(null); }}>
            Undo
          </button>
          <button type="button" className={toastAction} aria-label="Dismiss" onClick={() => setShown(null)}>×</button>
        </div>
      )}
      {shown?.kind === "refused" && !listOpen && (
        <div className={toast} role="group" aria-label="Tab refused">
          <span>{message}</span>
          <button type="button" className={toastAction} aria-label="Dismiss" onClick={() => setShown(null)}>×</button>
        </div>
      )}
    </div>
  );
}

interface TabMenuProps {
  path: string;
  x: number;
  buffers: readonly Buffer[];
  onDismiss: () => void;
  actions: {
    close: () => void; closeOthers: () => void; pin: () => void; unpin: () => void;
    move: (delta: -1 | 1) => void; copy: () => void; reveal: () => void;
  };
}

function TabMenu({ path, x, buffers, onDismiss, actions }: TabMenuProps) {
  const ref = useRef<HTMLDivElement>(null);
  const buffer = buffers.find((b) => b.path === path);
  const index = buffers.findIndex((b) => b.path === path);
  useEffect(() => {
    ref.current?.querySelector<HTMLElement>("[role=menuitem]")?.focus();
    const onDown = (e: globalThis.MouseEvent) => { if (!ref.current?.contains(e.target as Node)) onDismiss(); };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [onDismiss]);
  if (!buffer) return null;
  const items: [string, () => void, boolean?][] = [
    ["Close", actions.close],
    ["Close others", actions.closeOthers, buffers.length < 2],
    [buffer.pinned ? "Unpin" : "Pin", buffer.pinned ? actions.unpin : actions.pin, isDirty(buffer) && buffer.pinned],
    ["Move left", () => actions.move(-1), index === 0],
    ["Move right", () => actions.move(1), index === buffers.length - 1],
    ["Copy path", actions.copy],
    ["Reveal in tree", actions.reveal],
  ];
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Escape") { e.preventDefault(); onDismiss(); return; }
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
    e.preventDefault();
    const nodes = [...(ref.current?.querySelectorAll<HTMLElement>("[role=menuitem]:not([disabled])") ?? [])];
    const at = nodes.indexOf(document.activeElement as HTMLElement);
    nodes[(at + (e.key === "ArrowDown" ? 1 : -1) + nodes.length) % nodes.length]?.focus();
  };
  return (
    <div ref={ref} className={menu} style={{ left: x }} role="menu" aria-label={`${baseName(path)} tab actions`} onKeyDown={onKeyDown}>
      {items.map(([label, run, disabled]) => (
        <button key={label} type="button" role="menuitem" className={menuItem} disabled={disabled} onClick={() => { run(); onDismiss(); }}>
          {label}
        </button>
      ))}
    </div>
  );
}
