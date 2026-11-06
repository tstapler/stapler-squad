"use client";

import { useCallback, useEffect, useReducer, useRef } from "react";

interface Options<T> {
  /** True for rows in the tier that must hold still (Needs you). */
  isHeld: (item: T) => boolean;
  /** Called once with the number of queued rows when an unfreeze absorbs them. */
  onMerge?: (count: number) => void;
}

export interface FrozenList<T> {
  items: T[];
  /** Held-tier rows that arrived while frozen and wait behind "N new". */
  queued: number;
  /** Ids of the queued rows, in live order. */
  queuedIds: string[];
  /** The operator asked for the queued rows now ("N new"). */
  merge: () => void;
}

/** Applies the freeze rules to `live` against the rows shown when the freeze began. */
export function freezeAgainst<T extends { id: string }>(before: Map<string, T>, live: T[], isHeld: (item: T) => boolean) {
  const items: T[] = [];
  const queuedIds: string[] = [];
  const liveIds = new Set<string>();
  for (const l of live) {
    liveIds.add(l.id);
    const b = before.get(l.id);
    if (b) items.push(isHeld(b) || isHeld(l) ? b : l);
    else if (isHeld(l)) queuedIds.push(l.id);
    else items.push(l);
  }
  for (const b of before.values()) if (!liveIds.has(b.id) && isHeld(b)) items.push(b);
  return { items, queuedIds };
}

/**
 * Freezes the held tier while `frozen` (design/ux.md 7.1): none of its rows is inserted,
 * removed or reordered, a row resolved meanwhile keeps its last state, and arrivals queue.
 * Rows outside the held tier pass straight through. Unfreezing publishes the live list.
 */
export function useFrozenList<T extends { id: string }>(live: T[], frozen: boolean, { isHeld, onMerge }: Options<T>): FrozenList<T> {
  const [, rerender] = useReducer((n: number) => n + 1, 0);
  const lastLive = useRef<T[]>(live);
  const base = useRef<Map<string, T> | null>(null);
  const queuedRef = useRef(0);
  const wasFrozen = useRef(false);
  const onMergeRef = useRef(onMerge);
  onMergeRef.current = onMerge;

  let items = live;
  let queuedIds: string[] = [];
  if (!frozen) {
    lastLive.current = live;
    base.current = null;
  } else {
    if (!base.current) base.current = new Map(lastLive.current.map((t) => [t.id, t]));
    ({ items, queuedIds } = freezeAgainst(base.current, live, isHeld));
  }

  useEffect(() => {
    if (wasFrozen.current && !frozen && queuedRef.current > 0) onMergeRef.current?.(queuedRef.current);
    wasFrozen.current = frozen;
    queuedRef.current = frozen ? queuedIds.length : 0;
  });

  const liveRef = useRef(live);
  liveRef.current = live;
  const merge = useCallback(() => {
    lastLive.current = liveRef.current;
    base.current = null;
    queuedRef.current = 0;
    rerender();
  }, []);

  return { items, queued: queuedIds.length, queuedIds, merge };
}
