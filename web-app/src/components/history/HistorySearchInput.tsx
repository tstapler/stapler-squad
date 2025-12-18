"use client";

import { useState, useRef, useEffect, useCallback } from "react";
import { useSearchHistory } from "@/lib/hooks/useSearchHistory";
import styles from "./HistorySearchInput.module.css";

interface HistorySearchInputProps {
  /** Current search query */
  value: string;
  /** Called when query changes */
  onChange: (value: string) => void;
  /** Called when search is submitted (Enter pressed) */
  onSubmit?: (value: string) => void;
  /** Placeholder text */
  placeholder?: string;
  /** Whether search is in progress */
  loading?: boolean;
  /** Input ID for accessibility */
  id?: string;
  /** Additional CSS class */
  className?: string;
}

export function HistorySearchInput({
  value,
  onChange,
  onSubmit,
  placeholder = "Search conversation history...",
  loading = false,
  id = "history-search",
  className,
}: HistorySearchInputProps) {
  const [showHistory, setShowHistory] = useState(false);
  const [selectedIndex, setSelectedIndex] = useState(-1);
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const {
    history,
    addToHistory,
    removeFromHistory,
    clearHistory,
  } = useSearchHistory({
    storageKey: "claude-squad-history-search",
    maxEntries: 15,
  });

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setShowHistory(false);
      }
    };

    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const handleFocus = () => {
    if (history.length > 0 && !value) {
      setShowHistory(true);
    }
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const newValue = e.target.value;
    onChange(newValue);
    setSelectedIndex(-1);

    if (!newValue && history.length > 0) {
      setShowHistory(true);
    } else {
      setShowHistory(false);
    }
  };

  const handleSelect = useCallback((query: string) => {
    onChange(query);
    setShowHistory(false);
    setSelectedIndex(-1);
    addToHistory(query);
    onSubmit?.(query);
  }, [onChange, onSubmit, addToHistory]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (!showHistory || history.length === 0) {
      if (e.key === "Enter" && value.trim()) {
        addToHistory(value);
        onSubmit?.(value);
      }
      return;
    }

    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setSelectedIndex((prev) =>
          prev < history.length - 1 ? prev + 1 : 0
        );
        break;
      case "ArrowUp":
        e.preventDefault();
        setSelectedIndex((prev) =>
          prev > 0 ? prev - 1 : history.length - 1
        );
        break;
      case "Enter":
        e.preventDefault();
        if (selectedIndex >= 0 && selectedIndex < history.length) {
          handleSelect(history[selectedIndex].query);
        } else if (value.trim()) {
          addToHistory(value);
          onSubmit?.(value);
          setShowHistory(false);
        }
        break;
      case "Escape":
        setShowHistory(false);
        setSelectedIndex(-1);
        break;
    }
  };

  const handleRemove = (e: React.MouseEvent, query: string) => {
    e.stopPropagation();
    removeFromHistory(query);
  };

  const formatTime = (timestamp: number) => {
    const diff = Date.now() - timestamp;
    const minutes = Math.floor(diff / 60000);
    const hours = Math.floor(diff / 3600000);
    const days = Math.floor(diff / 86400000);

    if (minutes < 1) return "just now";
    if (minutes < 60) return `${minutes}m ago`;
    if (hours < 24) return `${hours}h ago`;
    return `${days}d ago`;
  };

  const handleClear = () => {
    onChange("");
    setShowHistory(history.length > 0);
    inputRef.current?.focus();
  };

  return (
    <div
      className={`${styles.container} ${className || ""}`}
      ref={containerRef}
    >
      <div className={styles.inputWrapper}>
        <span className={styles.searchIcon}>
          {loading ? (
            <span className={styles.spinner} />
          ) : (
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <circle cx="11" cy="11" r="8" />
              <path d="M21 21l-4.35-4.35" />
            </svg>
          )}
        </span>
        <input
          ref={inputRef}
          id={id}
          type="text"
          value={value}
          onChange={handleChange}
          onFocus={handleFocus}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          className={styles.input}
          aria-label="Search conversation history"
          aria-expanded={showHistory}
          aria-controls="search-history-dropdown"
          aria-autocomplete="list"
          autoComplete="off"
        />
        {value && (
          <button
            className={styles.clearButton}
            onClick={handleClear}
            aria-label="Clear search"
            type="button"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M18 6L6 18M6 6l12 12" />
            </svg>
          </button>
        )}
      </div>

      {showHistory && history.length > 0 && (
        <div
          id="search-history-dropdown"
          className={styles.dropdown}
          role="listbox"
        >
          <div className={styles.header}>
            <span>Recent Searches</span>
            <button
              className={styles.clearAllButton}
              onClick={() => {
                clearHistory();
                setShowHistory(false);
              }}
              type="button"
            >
              Clear all
            </button>
          </div>
          <div className={styles.items}>
            {history.map((entry, index) => (
              <div
                key={entry.query}
                className={`${styles.item} ${index === selectedIndex ? styles.selected : ""}`}
                onClick={() => handleSelect(entry.query)}
                role="option"
                aria-selected={index === selectedIndex}
              >
                <span className={styles.historyIcon}>
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <circle cx="12" cy="12" r="10" />
                    <polyline points="12 6 12 12 16 14" />
                  </svg>
                </span>
                <span className={styles.query}>{entry.query}</span>
                <span className={styles.timestamp}>{formatTime(entry.timestamp)}</span>
                <button
                  className={styles.removeButton}
                  onClick={(e) => handleRemove(e, entry.query)}
                  aria-label={`Remove "${entry.query}" from history`}
                  type="button"
                >
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <path d="M18 6L6 18M6 6l12 12" />
                  </svg>
                </button>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

export default HistorySearchInput;
