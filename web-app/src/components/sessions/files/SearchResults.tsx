"use client";

import { forwardRef, useCallback, useImperativeHandle, useRef, type KeyboardEvent } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import type { SearchHit } from "@/lib/files/searchClient";
import { useFirstTapActivation } from "@/lib/hooks/useFirstTapActivation";
import { DIRTY_NOTE, accessibleName, type ResultRow } from "./searchRows";
import {
  count as countClass, dirtyNote, groupPath, groupRow, hitText, lineNo, mark, rowBase, tree, treeDimmed, treeInner,
} from "./SearchPanel.css";

export interface SearchResultsHandle {
  /** Brings a row into view (arrow keys, F4 stepping). */
  scrollToKey(key: string): void;
}

export type ActivateVia = "tap" | "enter" | "quickfix";

interface SearchResultsProps {
  rows: readonly ResultRow[];
  /** Results belong to an earlier query: dimmed. */
  stale: boolean;
  /** Streaming in progress. */
  busy: boolean;
  /** Key of the active row, owned by the query input's `aria-activedescendant` on desktop. */
  activeKey: string | null;
  dirtyPaths: ReadonlySet<string>;
  /** `pointer: coarse`: 44 px rows that are focusable. */
  coarse: boolean;
  treeId: string;
  onToggleGroup: (path: string) => void;
  onActivate: (hit: SearchHit, hitIndex: number, via: ActivateVia) => void;
}

export const rowDomId = (treeId: string, key: string) => `${treeId}-${key}`;

/** Splits `text` around the match range (UTF-16 indexes, as the server sends them). */
function Highlighted({ hit }: { hit: SearchHit }) {
  const { text, start, end } = hit;
  if (start < 0 || end > text.length || start >= end) return <>{text}</>;
  return (
    <>
      {text.slice(0, start)}
      <mark className={mark}>{text.slice(start, end)}</mark>
      {text.slice(end)}
    </>
  );
}

/** Virtualized `tree` of file groups and hit rows; every row carries explicit level, setsize and posinset. */
export const SearchResults = forwardRef<SearchResultsHandle, SearchResultsProps>(function SearchResults(p, ref) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const rowHeight = p.coarse ? 44 : 24;
  const virtualizer = useVirtualizer({
    count: p.rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => rowHeight,
    overscan: 8,
    getItemKey: (i) => p.rows[i]?.key ?? i,
  });

  useImperativeHandle(ref, () => ({
    scrollToKey(key) {
      const i = p.rows.findIndex((r) => r.key === key);
      if (i >= 0) virtualizer.scrollToIndex(i, { align: "auto" });
    },
  }), [p.rows, virtualizer]);

  const activateKey = useCallback((key: string, via: ActivateVia) => {
    const row = p.rows.find((r) => r.key === key);
    if (!row) return;
    if (row.kind === "group") p.onToggleGroup(row.path);
    else p.onActivate(row.hit, row.hitIndex, via);
  }, [p]);
  const tap = useFirstTapActivation(useCallback((key: string) => activateKey(key, "tap"), [activateKey]));
  // Rows are focusable on touch devices, where a hardware keyboard may still be attached.
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== "Enter" && e.key !== " ") return;
    const key = (e.target as HTMLElement).closest<HTMLElement>("[data-row-key]")?.dataset.rowKey;
    if (!key) return;
    e.preventDefault();
    activateKey(key, "enter");
  };

  return (
    <div
      ref={scrollRef}
      className={`${tree} ${p.stale ? treeDimmed : ""}`}
      role="tree"
      aria-label="Search results"
      aria-busy={p.busy}
      {...tap}
      onKeyDown={onKeyDown}
      data-testid="search-results"
    >
      <div className={treeInner} style={{ height: virtualizer.getTotalSize() }}>
        {virtualizer.getVirtualItems().map((v) => {
          const row = p.rows[v.index];
          if (!row) return null;
          const style = { transform: `translateY(${v.start}px)`, height: v.size, ...(p.coarse ? { minHeight: 44 } : {}) };
          const common = {
            id: rowDomId(p.treeId, row.key),
            "data-row-key": row.key,
            role: "treeitem" as const,
            "aria-setsize": row.setsize,
            "aria-posinset": row.posinset,
            "aria-selected": p.activeKey === row.key,
            tabIndex: p.coarse ? -1 : undefined,
            style,
          };
          if (row.kind === "group") {
            return (
              <div key={row.key} {...common} className={`${rowBase} ${groupRow}`} aria-level={1} aria-expanded={!row.collapsed} aria-label={accessibleName(row)}>
                <span className={groupPath}>{row.collapsed ? "▸" : "▾"} {row.path}</span>
                <span className={countClass}>{row.count}</span>
              </div>
            );
          }
          const dirty = p.dirtyPaths.has(row.hit.path);
          return (
            <div key={row.key} {...common} className={rowBase} aria-level={2} aria-label={accessibleName(row, dirty)}>
              <span className={lineNo}>{row.hit.line}</span>
              <span className={hitText}><Highlighted hit={row.hit} /></span>
              {dirty && <span className={dirtyNote}>{DIRTY_NOTE}</span>}
            </div>
          );
        })}
      </div>
    </div>
  );
});
