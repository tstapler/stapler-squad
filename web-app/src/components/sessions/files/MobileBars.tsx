"use client";

import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import type { PaneState } from "@/lib/hooks/useMobilePane";
import {
  topBar, bottomBar, barButton, bottomItem, split, barLabel, title, fileName, dirtyDot, popover, popoverItem, srOnly,
} from "./MobileFilesLayout.css";

export const NARROW_BAR_PX = 360;

export interface MenuItem { label: string; run: () => void; disabled?: boolean }

/** A button that opens a small menu; Esc and outside taps close it and return focus to the button. */
function MenuButton({ label, glyph, items, from }: { label: string; glyph: string; items: MenuItem[]; from: "top" | "bottom" }) {
  const [open, setOpen] = useState(false);
  const button = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    menu.current?.querySelector<HTMLElement>("[role=menuitem]:not([aria-disabled=true])")?.focus();
    const onDown = (e: Event) => { if (!menu.current?.contains(e.target as Node) && e.target !== button.current) setOpen(false); };
    document.addEventListener("pointerdown", onDown);
    return () => document.removeEventListener("pointerdown", onDown);
  }, [open]);
  const close = () => { setOpen(false); button.current?.focus(); };
  const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") { e.stopPropagation(); close(); } };
  return (
    <>
      <button ref={button} type="button" className={barButton} aria-label={label} aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen((o) => !o)}>{glyph}</button>
      {open && (
        <div ref={menu} className={popover} data-from={from} role="menu" aria-label={label} onKeyDown={onKey}>
          {items.map((it) => (
            <button
              key={it.label}
              type="button"
              role="menuitem"
              className={popoverItem}
              aria-disabled={it.disabled || undefined}
              onClick={() => { if (it.disabled) return; setOpen(false); it.run(); }}
            >
              {it.label}
            </button>
          ))}
        </div>
      )}
    </>
  );
}

export interface TopBarProps {
  pane: PaneState;
  /** Pane the editor returns to, for the arrow's accessible name. */
  origin: PaneState;
  path: string | null;
  dirty: boolean;
  tabCount: number;
  overflow: MenuItem[];
  onBack: () => void;
  onTabs: () => void;
}

export function MobileTopBar({ pane, origin, path, dirty, tabCount, overflow, onBack, onTabs }: TopBarProps) {
  const heading = pane === "files" ? "Files" : pane === "search" ? "Search" : path ?? "No file open";
  const backLabel = pane === "search" ? "Back to Files" : origin === "search" ? "Back to results" : "Back to Files";
  return (
    <div className={topBar} data-testid="mobile-top-bar">
      {pane !== "files" && <button type="button" className={barButton} aria-label={backLabel} onClick={() => onBack()}>←</button>}
      <div className={title} style={pane === "files" ? { paddingLeft: 12 } : undefined}>
        {pane === "editor" && dirty && <span className={dirtyDot} aria-hidden="true" />}
        <span className={fileName}><bdi>{heading}</bdi></span>
        {pane === "editor" && dirty && <span className={srOnly}>unsaved</span>}
      </div>
      <button type="button" className={barButton} aria-label={`Open files, ${tabCount}`} aria-haspopup="dialog" onClick={onTabs}>[{tabCount}]</button>
      {pane === "editor" && overflow.length > 0 && <MenuButton label="More actions" glyph="⋯" items={overflow} from="top" />}
    </div>
  );
}

export interface BottomBarProps {
  pane: PaneState;
  width: number;
  hasFile: boolean;
  canBack: boolean;
  canForward: boolean;
  onFiles: () => void;
  onSearch: () => void;
  onQuickOpen: () => void;
  onGoToLine: () => void;
  onBack: () => void;
  onForward: () => void;
}

export function MobileBottomBar(p: BottomBarProps) {
  const narrow = p.width < NARROW_BAR_PX;
  const step = (label: string, glyph: string, enabled: boolean, run: () => void, why: string) => (
    <button
      type="button"
      className={barButton}
      style={{ flex: 1 }}
      aria-label={label}
      aria-disabled={!enabled}
      title={enabled ? label : why}
      onClick={() => { if (enabled) run(); }}
    >
      {glyph}
    </button>
  );
  const item = (label: string, glyph: string, run: () => void, extra: { current?: boolean; disabled?: boolean } = {}) => (
    <button
      type="button"
      className={`${barButton} ${bottomItem}`}
      aria-label={label}
      aria-current={extra.current ? "true" : undefined}
      aria-disabled={extra.disabled || undefined}
      onClick={() => { if (!extra.disabled) run(); }}
    >
      <span aria-hidden="true">{glyph}</span>
      {!narrow && <span className={barLabel} aria-hidden="true">{label}</span>}
    </button>
  );
  const back = step("Previous location", "◀", p.canBack, p.onBack, "No earlier location");
  return (
    <nav className={bottomBar} aria-label="Files actions" data-testid="mobile-bottom-bar">
      {item("Files", "▤", p.onFiles, { current: p.pane === "files" })}
      {item("Search", "⌕", p.onSearch, { current: p.pane === "search" })}
      {item("Quick open", "⌘", p.onQuickOpen)}
      {narrow ? (
        <>
          <div className={split}>{back}</div>
          <MenuButton
            label="More"
            glyph="⋯"
            from="bottom"
            items={[
              { label: "Go to line", run: p.onGoToLine, disabled: !p.hasFile },
              { label: "Next location", run: p.onForward, disabled: !p.canForward },
            ]}
          />
        </>
      ) : (
        <>
          {item("Go to line", "#", p.onGoToLine, { disabled: !p.hasFile })}
          <div className={split} role="group" aria-label="Location history">
            {back}
            {step("Next location", "▶", p.canForward, p.onForward, "No later location")}
          </div>
        </>
      )}
    </nav>
  );
}
