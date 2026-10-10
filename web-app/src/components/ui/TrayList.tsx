"use client";

import { useEffect, useRef, useState, type RefObject } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useSwipeToDismiss } from "@/lib/hooks/useSwipeToDismiss";
import type { GroupedNotification, TrayRow } from "@/lib/utils/notificationGrouping";
import { NotificationItem, type NotificationItemProps } from "./NotificationItem";
import {
  groupHeader,
  groupHeaderPinned,
  groupNote,
  groupToggle,
  loadMoreButton,
  swipeRow,
  trayButton,
  virtualRow,
  virtualViewport,
} from "./NotificationPanel.css";

export type ListRow = TrayRow | { kind: "load-more"; key: string };

export type TrayItemProps = Omit<NotificationItemProps, "group" | "removeFromHistory" | "offlineReason">;

interface TrayListProps {
  rows: ListRow[];
  /** Total selectable rows, for aria-setsize. */
  setSize: number;
  itemProps: TrayItemProps;
  /** Visible reason server-mutating controls are disabled; also turns swipe off. */
  offlineReason?: string;
  scrollRef: RefObject<HTMLDivElement | null>;
  /** Height of the non-virtual block (search, filters, cards) above the list in the same scroller. */
  scrollMargin?: number;
  hasMore: boolean;
  loading: boolean;
  onLoadMore: () => void;
  onToggleGroup: (key: string) => void;
  /** Dismiss every record of an informational group (undoable). */
  onDismissGroup: (group: GroupedNotification) => void;
  /** Dismiss every informational row of a session ("Dismiss N informational"). */
  onDismissSession: (sessionKey: string) => void;
  /** Restores a representative id to its group for the ✕ control. */
  groupForId: (id: string) => GroupedNotification | undefined;
}

const HEADER_HEIGHT = 44;
const GROUP_HEIGHT = 140;

function SwipeRow({ onDismiss, disabled, children }: { onDismiss: () => void; disabled: boolean; children: React.ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  const view = useSwipeToDismiss(ref, { onDismiss, disabled });
  return (
    <div
      ref={ref}
      className={swipeRow}
      data-testid="tray-swipe-row"
      style={{
        transform: view.offset ? `translateX(${view.offset}px)` : undefined,
        transition: view.dragging ? "none" : "transform 0.2s ease-out",
      }}
    >
      {children}
    </div>
  );
}

/**
 * The virtualized list (Story 4.3): a flattened header/group model windowed by
 * `@tanstack/react-virtual`, so only the visible rows are in the DOM. Roving
 * tabindex over header and group rows; arrows or `j`/`k` move, Enter opens, `x` dismisses
 * (only while focus is inside the list, WCAG 2.1.4).
 */
export function TrayList({
  rows,
  setSize,
  itemProps,
  offlineReason,
  scrollRef,
  scrollMargin = 0,
  hasMore,
  loading,
  onLoadMore,
  onToggleGroup,
  onDismissGroup,
  onDismissSession,
  groupForId,
}: TrayListProps) {
  const [activeKey, setActiveKey] = useState<string | null>(null);
  const focusKeyRef = useRef<string | null>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: (i) => (rows[i]?.kind === "group" ? GROUP_HEIGHT : HEADER_HEIGHT),
    getItemKey: (i) => rows[i]?.key ?? i,
    overscan: 8,
    scrollMargin,
    initialRect: { width: 400, height: 640 },
  });
  const virtualItems = virtualizer.getVirtualItems();

  // Page in the next slice only when the end of the list is scrolled into view.
  const lastIndex = virtualItems.length > 0 ? virtualItems[virtualItems.length - 1].index : -1;
  useEffect(() => {
    if (hasMore && !loading && lastIndex >= rows.length - 1 && rows.length > 0) onLoadMore();
  }, [hasMore, loading, lastIndex, rows.length, onLoadMore]);

  // After an arrow key scrolls a row into the window, focus it once it is rendered.
  useEffect(() => {
    const key = focusKeyRef.current;
    if (!key) return;
    const el = listRef.current?.querySelector<HTMLElement>(`[data-tray-row="${CSS.escape(key)}"]`);
    if (el) {
      focusKeyRef.current = null;
      el.focus({ preventScroll: true });
    }
  });

  const focusableKeys = rows.map((r) => r.key).filter((k) => k !== "nothing-needs-attention");
  const tabbableKey = activeKey && focusableKeys.includes(activeKey) ? activeKey : focusableKeys[0];

  const moveTo = (key: string | undefined) => {
    if (!key) return;
    const index = rows.findIndex((r) => r.key === key);
    focusKeyRef.current = key;
    setActiveKey(key);
    virtualizer.scrollToIndex(index);
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    const target = e.target as HTMLElement;
    if (target.closest("input, textarea, select")) return;
    const rowEl = target.closest<HTMLElement>("[data-tray-row]");
    const currentKey = rowEl?.dataset.trayRow ?? tabbableKey;
    const at = focusableKeys.indexOf(currentKey ?? "");
    // j and k are single-key shortcuts: they act only here, with focus inside the list (WCAG 2.1.4).
    const plain = !e.ctrlKey && !e.metaKey && !e.altKey;
    if (e.key === "ArrowDown" || (plain && e.key === "j")) {
      e.preventDefault();
      moveTo(focusableKeys[Math.min(at + 1, focusableKeys.length - 1)]);
    } else if (e.key === "ArrowUp" || (plain && e.key === "k")) {
      e.preventDefault();
      moveTo(focusableKeys[Math.max(at - 1, 0)]);
    } else if (e.key === "Home") {
      e.preventDefault();
      moveTo(focusableKeys[0]);
    } else if (e.key === "End") {
      e.preventDefault();
      moveTo(focusableKeys[focusableKeys.length - 1]);
    } else if (rowEl && target === rowEl) {
      const row = rows.find((r) => r.key === rowEl.dataset.trayRow);
      if (!row) return;
      if (row.kind === "header" && !row.pinned && (e.key === " " || e.key === "Enter")) {
        e.preventDefault();
        onToggleGroup(row.key);
      } else if (row.kind === "group" && e.key === "Enter") {
        e.preventDefault();
        rowEl.querySelector<HTMLElement>("a")?.click();
      } else if (row.kind === "group" && e.key === "x" && !row.pinned && !offlineReason) {
        e.preventDefault();
        onDismissGroup(row.group);
      }
    }
  };

  return (
    <div ref={listRef} role="list" data-testid="tray-list" onKeyDown={onKeyDown}>
      <div className={virtualViewport} style={{ height: virtualizer.getTotalSize() }}>
        {virtualItems.map((v) => {
          const row = rows[v.index];
          if (!row) return null;
          const rowProps = {
            "data-tray-row": row.key,
            tabIndex: row.key === tabbableKey ? 0 : -1,
            onFocus: () => setActiveKey(row.key),
          };
          let body: React.ReactNode;
          if (row.kind === "header") {
            body = (
              <div className={`${groupHeader} ${row.pinned ? groupHeaderPinned : ""}`} data-testid={row.pinned ? "tray-needs-attention-header" : "tray-group-header"}>
                {row.pinned ? (
                  <span>
                    {row.label} ({row.count})
                  </span>
                ) : (
                  <button
                    type="button"
                    className={groupToggle}
                    aria-expanded={!row.collapsed}
                    tabIndex={-1}
                    onClick={() => onToggleGroup(row.key)}
                  >
                    <span aria-hidden="true">{row.collapsed ? "▸" : "▾"}</span>
                    <span>
                      {row.label} ({row.count})
                    </span>
                  </button>
                )}
                {!row.pinned && !row.collapsed && (
                  <button
                    type="button"
                    className={trayButton}
                    aria-disabled={offlineReason ? true : undefined}
                    title={offlineReason}
                    onClick={() => !offlineReason && onDismissSession(row.key)}
                  >
                    Dismiss {row.count} informational
                  </button>
                )}
              </div>
            );
          } else if (row.kind === "note") {
            body = <div className={groupNote}>{row.text}</div>;
          } else if (row.kind === "load-more") {
            body = (
              <div className={groupNote}>
                <button type="button" className={loadMoreButton} onClick={onLoadMore} disabled={loading}>
                  {loading ? "Loading..." : "Load more"}
                </button>
              </div>
            );
          } else {
            const item = (
              <NotificationItem
                group={row.group}
                {...itemProps}
                offlineReason={offlineReason}
                removeFromHistory={
                  row.pinned
                    ? undefined
                    : (id) => {
                        const group = groupForId(id);
                        if (group) onDismissGroup(group);
                      }
                }
              />
            );
            body = row.pinned ? (
              <div data-testid="needs-attention-row">{item}</div>
            ) : (
              <SwipeRow disabled={!!offlineReason} onDismiss={() => onDismissGroup(row.group)}>
                {item}
              </SwipeRow>
            );
          }
          const isGroup = row.kind === "group";
          return (
            <div
              key={row.key}
              ref={virtualizer.measureElement}
              data-index={v.index}
              className={virtualRow}
              style={{ transform: `translateY(${v.start - scrollMargin}px)` }}
            >
              <div
                role="listitem"
                {...rowProps}
                aria-posinset={isGroup ? row.posInSet : undefined}
                aria-setsize={isGroup ? setSize : undefined}
                data-testid={isGroup ? "tray-row" : undefined}
              >
                {body}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
