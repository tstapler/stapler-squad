"use client";

import { useState, useCallback } from "react";

const STORAGE_KEY = "omnibar:path-history";
const MAX_ENTRIES = 50;
const MAX_RESULTS = 10;

export interface PathHistoryEntry {
  path: string;
  count: number;
  lastUsed: number; // epoch ms
}

function recencyScore(lastUsed: number): number {
  const age = Date.now() - lastUsed;
  const hour = 3_600_000;
  const day = 86_400_000;
  const week = 7 * day;
  const month = 30 * day;
  if (age < hour) return 1.0;
  if (age < day) return 0.8;
  if (age < week) return 0.6;
  if (age < month) return 0.4;
  return 0.2;
}

function entryScore(e: PathHistoryEntry): number {
  return recencyScore(e.lastUsed) + Math.log1p(e.count);
}

function loadFromStorage(): PathHistoryEntry[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    return JSON.parse(raw) as PathHistoryEntry[];
  } catch {
    return [];
  }
}

function persistToStorage(entries: PathHistoryEntry[]): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(entries));
  } catch {
    // Ignore storage errors (private browsing, quota exceeded)
  }
}

/** Remove all saved history (for testing / clear-history UX). */
export function clearPathHistory(): void {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // ignore
  }
}

export function usePathHistory() {
  const [entries, setEntries] = useState<PathHistoryEntry[]>(() =>
    loadFromStorage()
  );

  /**
   * Return stored paths that start with `prefix`, sorted by score desc.
   * Excludes exact matches (the user already typed that path).
   */
  const getMatching = useCallback(
    (prefix: string): PathHistoryEntry[] => {
      if (!prefix) return [];
      return entries
        .filter((e) => e.path.startsWith(prefix) && e.path !== prefix)
        .sort((a, b) => entryScore(b) - entryScore(a))
        .slice(0, MAX_RESULTS);
    },
    [entries]
  );

  /**
   * Return all stored paths sorted by score desc, limited to `limit` entries.
   * Used for "Recent Repos" empty-state display when no query is active.
   */
  const getAll = useCallback(
    (limit: number = MAX_RESULTS): PathHistoryEntry[] => {
      return [...entries]
        .sort((a, b) => entryScore(b) - entryScore(a))
        .slice(0, limit);
    },
    [entries]
  );

  /** Record a path submission. Increments count if already stored. */
  const save = useCallback((path: string): void => {
    setEntries((prev) => {
      const copy = prev.map((e) => ({ ...e }));
      const existing = copy.find((e) => e.path === path);
      const now = Date.now();
      if (existing) {
        existing.count += 1;
        existing.lastUsed = now;
      } else {
        copy.push({ path, count: 1, lastUsed: now });
      }
      copy.sort((a, b) => entryScore(b) - entryScore(a));
      const trimmed = copy.slice(0, MAX_ENTRIES);
      persistToStorage(trimmed);
      return trimmed;
    });
  }, []);

  return { getMatching, getAll, save };
}
