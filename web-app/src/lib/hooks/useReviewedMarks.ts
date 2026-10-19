"use client";

import { useCallback, useContext, useSyncExternalStore } from "react";
import { AnalyticsContext } from "@/lib/contexts/AnalyticsContext";

const MARKS_KEY = "ssq.background.reviewed";
const FIRST_RUN_KEY = "ssq.background.firstRunSeen";
const MAX_MARKS = 1000;

export type MarkStorage = "local" | "session";

interface Store {
  marks: ReadonlySet<string>;
  firstRunSeen: boolean;
  storage: MarkStorage;
}

let store: Store | null = null;
const listeners = new Set<() => void>();

function read(): Store {
  try {
    const raw = window.localStorage.getItem(MARKS_KEY);
    const marks = new Set<string>(raw ? (JSON.parse(raw) as string[]) : []);
    return { marks, firstRunSeen: window.localStorage.getItem(FIRST_RUN_KEY) === "1", storage: "local" };
  } catch {
    return { marks: new Set(), firstRunSeen: false, storage: "session" };
  }
}

function current(): Store {
  store ??= read();
  return store;
}

function commit(next: Store) {
  let storage: MarkStorage = "local";
  try {
    window.localStorage.setItem(MARKS_KEY, JSON.stringify([...next.marks].slice(-MAX_MARKS)));
    if (next.firstRunSeen) window.localStorage.setItem(FIRST_RUN_KEY, "1");
  } catch {
    storage = "session";
  }
  store = { ...next, storage };
  listeners.forEach((l) => l());
}

/** A mark is per (row, last activity): a newer failure on the same session is unreviewed again. */
export const markKeyOf = (row: { key: string; timestampMs: number }) => `${row.key}@${row.timestampMs}`;

/** Test seam: forget the in-memory copy so the next read goes back to storage, as a reload does. */
export function resetReviewedMarksForTests() {
  store = null;
  listeners.forEach((l) => l());
}

/**
 * Reviewed marks live in this browser only (no server write, ADR-001); when storage throws they
 * last for the page session. State is shared by every caller so the tab badge and the list agree.
 */
export function useReviewedMarks() {
  const snap = useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    current,
    current,
  );
  const analytics = useContext(AnalyticsContext);

  const mark = useCallback(
    (keys: readonly string[], options: { track?: boolean } = {}) => {
      const before = current();
      commit({ ...before, marks: new Set([...before.marks, ...keys]) });
      if (options.track === false) return;
      analytics?.track({
        name: "background_reviewed_mark_total",
        category: "user_action",
        component: "BackgroundActivity",
        labels: { storage: current().storage },
      });
    },
    [analytics],
  );
  const dismissFirstRun = useCallback(() => commit({ ...current(), firstRunSeen: true }), []);

  return {
    isReviewed: (key: string) => snap.marks.has(key),
    mark,
    firstRunSeen: snap.firstRunSeen,
    dismissFirstRun,
    storage: snap.storage,
  };
}

export type ReviewedMarks = ReturnType<typeof useReviewedMarks>;
