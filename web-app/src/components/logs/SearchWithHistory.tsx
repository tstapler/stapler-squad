"use client";

import { useState, useRef, useEffect, useCallback } from 'react';
import { useSearchHistory, type SearchHistoryEntry } from '@/lib/hooks/useSearchHistory';
import styles from './SearchWithHistory.module.css';

interface SearchWithHistoryProps {
  /** Current search value */
  value: string;
  /** Change handler */
  onChange: (value: string) => void;
  /** Placeholder text */
  placeholder?: string;
  /** Input ID */
  id?: string;
  /** Additional class name */
  className?: string;
  /** Called when search is submitted */
  onSubmit?: (value: string) => void;
}

export function SearchWithHistory({
  value,
  onChange,
  placeholder = 'Search logs...',
  id = 'search',
  className,
  onSubmit,
}: SearchWithHistoryProps) {
  const [showHistory, setShowHistory] = useState(false);
  const [selectedIndex, setSelectedIndex] = useState(-1);
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const { history, addToHistory, removeFromHistory, clearHistory } = useSearchHistory();

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setShowHistory(false);
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  // Handle input focus
  const handleFocus = () => {
    if (history.length > 0 && !value) {
      setShowHistory(true);
    }
  };

  // Handle input change
  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const newValue = e.target.value;
    onChange(newValue);
    setSelectedIndex(-1);

    // Show history when input is empty and there's history
    if (!newValue && history.length > 0) {
      setShowHistory(true);
    } else {
      setShowHistory(false);
    }
  };

  // Handle selecting a history item
  const handleSelect = useCallback((query: string) => {
    onChange(query);
    setShowHistory(false);
    setSelectedIndex(-1);
    onSubmit?.(query);
  }, [onChange, onSubmit]);

  // Handle keyboard navigation
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (!showHistory || history.length === 0) {
      if (e.key === 'Enter' && value.trim()) {
        addToHistory(value);
        onSubmit?.(value);
      }
      return;
    }

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        setSelectedIndex(prev =>
          prev < history.length - 1 ? prev + 1 : 0
        );
        break;
      case 'ArrowUp':
        e.preventDefault();
        setSelectedIndex(prev =>
          prev > 0 ? prev - 1 : history.length - 1
        );
        break;
      case 'Enter':
        e.preventDefault();
        if (selectedIndex >= 0 && selectedIndex < history.length) {
          handleSelect(history[selectedIndex].query);
        } else if (value.trim()) {
          addToHistory(value);
          onSubmit?.(value);
          setShowHistory(false);
        }
        break;
      case 'Escape':
        setShowHistory(false);
        setSelectedIndex(-1);
        break;
    }
  };

  // Handle removing an item from history
  const handleRemove = (e: React.MouseEvent, query: string) => {
    e.stopPropagation();
    removeFromHistory(query);
  };

  // Format timestamp for display
  const formatTime = (timestamp: number) => {
    const diff = Date.now() - timestamp;
    const minutes = Math.floor(diff / 60000);
    const hours = Math.floor(diff / 3600000);
    const days = Math.floor(diff / 86400000);

    if (minutes < 1) return 'just now';
    if (minutes < 60) return `${minutes}m ago`;
    if (hours < 24) return `${hours}h ago`;
    return `${days}d ago`;
  };

  // Clear search
  const handleClear = () => {
    onChange('');
    setShowHistory(history.length > 0);
    inputRef.current?.focus();
  };

  return (
    <div
      className={`${styles.container} ${className || ''}`}
      ref={containerRef}
    >
      <div className={styles.inputWrapper}>
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
          aria-label="Search logs"
          aria-expanded={showHistory}
          aria-controls="search-history"
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
            ×
          </button>
        )}
      </div>

      {showHistory && history.length > 0 && (
        <div
          id="search-history"
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
                className={`${styles.item} ${index === selectedIndex ? styles.selected : ''}`}
                onClick={() => handleSelect(entry.query)}
                role="option"
                aria-selected={index === selectedIndex}
              >
                <span className={styles.historyIcon}>🕐</span>
                <span className={styles.query}>{entry.query}</span>
                <span className={styles.timestamp}>{formatTime(entry.timestamp)}</span>
                <button
                  className={styles.removeButton}
                  onClick={(e) => handleRemove(e, entry.query)}
                  aria-label={`Remove "${entry.query}" from history`}
                  type="button"
                >
                  ×
                </button>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

export default SearchWithHistory;
