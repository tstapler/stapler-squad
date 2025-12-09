"use client";

import { useState, useCallback, useEffect } from 'react';

const STORAGE_KEY = 'claude-squad-logs-search-history';
const MAX_HISTORY = 10;

interface SearchHistoryOptions {
  /** Maximum number of history entries (default: 10) */
  maxEntries?: number;
  /** Storage key prefix */
  storageKey?: string;
}

export interface SearchHistoryEntry {
  query: string;
  timestamp: number;
}

export function useSearchHistory(options: SearchHistoryOptions = {}) {
  const { maxEntries = MAX_HISTORY, storageKey = STORAGE_KEY } = options;
  const [history, setHistory] = useState<SearchHistoryEntry[]>([]);
  const [isLoaded, setIsLoaded] = useState(false);

  // Load history from localStorage on mount
  useEffect(() => {
    try {
      const stored = localStorage.getItem(storageKey);
      if (stored) {
        const parsed = JSON.parse(stored);
        if (Array.isArray(parsed)) {
          setHistory(parsed);
        }
      }
    } catch (err) {
      console.error('Failed to load search history:', err);
    }
    setIsLoaded(true);
  }, [storageKey]);

  // Save history to localStorage
  const saveHistory = useCallback((entries: SearchHistoryEntry[]) => {
    try {
      localStorage.setItem(storageKey, JSON.stringify(entries));
    } catch (err) {
      console.error('Failed to save search history:', err);
    }
  }, [storageKey]);

  // Add a query to history
  const addToHistory = useCallback((query: string) => {
    if (!query.trim()) return;

    const normalizedQuery = query.trim();

    setHistory(prev => {
      // Remove duplicate if exists
      const filtered = prev.filter(entry => entry.query !== normalizedQuery);

      // Add new entry at the beginning
      const newHistory = [
        { query: normalizedQuery, timestamp: Date.now() },
        ...filtered,
      ].slice(0, maxEntries);

      saveHistory(newHistory);
      return newHistory;
    });
  }, [maxEntries, saveHistory]);

  // Remove a query from history
  const removeFromHistory = useCallback((query: string) => {
    setHistory(prev => {
      const filtered = prev.filter(entry => entry.query !== query);
      saveHistory(filtered);
      return filtered;
    });
  }, [saveHistory]);

  // Clear all history
  const clearHistory = useCallback(() => {
    setHistory([]);
    try {
      localStorage.removeItem(storageKey);
    } catch (err) {
      console.error('Failed to clear search history:', err);
    }
  }, [storageKey]);

  return {
    history,
    isLoaded,
    addToHistory,
    removeFromHistory,
    clearHistory,
  };
}

export default useSearchHistory;
