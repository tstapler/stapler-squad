"use client";

import { useState, useRef, useEffect, KeyboardEvent } from "react";
import {
  container as containerClass,
  error as errorClass,
  suggestions as suggestionsClass,
  suggestion,
  highlighted,
  loading,
} from "./AutocompleteInput.css";

/** Default filter: plain case-insensitive substring match (unchanged pre-existing behavior). */
function defaultFilterFn(query: string, suggestions: string[]): string[] {
  return suggestions.filter((suggestion) =>
    suggestion.toLowerCase().includes(query.toLowerCase())
  );
}

interface AutocompleteInputProps {
  id: string;
  value: string;
  onChange: (value: string) => void;
  onBlur?: () => void;
  placeholder?: string;
  suggestions: string[];
  isLoading?: boolean;
  className?: string;
  error?: boolean;
  disabled?: boolean;
  "data-testid"?: string;
  /** Ranks/filters `suggestions` against the current query. Defaults to substring match. */
  filterFn?: (query: string, suggestions: string[]) => string[];
  /** Maps a suggestion value to its display label. Defaults to the value itself. */
  getLabel?: (value: string) => string;
}

export function AutocompleteInput({
  id,
  value,
  onChange,
  onBlur,
  placeholder,
  suggestions,
  isLoading = false,
  className = "",
  error = false,
  disabled = false,
  "data-testid": dataTestId,
  filterFn = defaultFilterFn,
  getLabel = (v) => v,
}: AutocompleteInputProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [highlightedIndex, setHighlightedIndex] = useState(-1);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLUListElement>(null);

  // Filter (and, when filterFn is fuzzy, rank) suggestions based on input value
  const filteredSuggestions = filterFn(value, suggestions);

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (
        inputRef.current &&
        !inputRef.current.contains(event.target as Node) &&
        listRef.current &&
        !listRef.current.contains(event.target as Node)
      ) {
        setIsOpen(false);
      }
    };

    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  // Scroll highlighted item into view
  useEffect(() => {
    if (listRef.current && highlightedIndex >= 0) {
      const highlightedElement = listRef.current.children[highlightedIndex] as HTMLElement;
      if (highlightedElement) {
        highlightedElement.scrollIntoView({
          block: "nearest",
          behavior: "smooth",
        });
      }
    }
  }, [highlightedIndex]);

  const handleInputChange = (newValue: string) => {
    onChange(newValue);
    setIsOpen(true);
    setHighlightedIndex(-1);
  };

  const handleSuggestionClick = (suggestion: string) => {
    onChange(suggestion);
    setIsOpen(false);
    setHighlightedIndex(-1);
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (!isOpen || filteredSuggestions.length === 0) {
      if (e.key === "ArrowDown" && filteredSuggestions.length > 0) {
        setIsOpen(true);
        setHighlightedIndex(0);
        e.preventDefault();
      }
      return;
    }

    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setHighlightedIndex((prev) =>
          prev < filteredSuggestions.length - 1 ? prev + 1 : prev
        );
        break;

      case "ArrowUp":
        e.preventDefault();
        setHighlightedIndex((prev) => (prev > 0 ? prev - 1 : -1));
        break;

      case "Enter":
        e.preventDefault();
        if (highlightedIndex >= 0 && highlightedIndex < filteredSuggestions.length) {
          handleSuggestionClick(filteredSuggestions[highlightedIndex]);
        }
        break;

      case "Escape":
        e.preventDefault();
        setIsOpen(false);
        setHighlightedIndex(-1);
        break;

      case "Tab":
        // Allow tab to close dropdown but don't prevent default
        setIsOpen(false);
        setHighlightedIndex(-1);
        break;
    }
  };

  const handleFocus = () => {
    if (filteredSuggestions.length > 0) {
      setIsOpen(true);
    }
  };

  const handleBlur = () => {
    onBlur?.();
  };

  return (
    <div className={containerClass}>
      <input
        ref={inputRef}
        id={id}
        type="text"
        value={value}
        onChange={(e) => handleInputChange(e.target.value)}
        onKeyDown={handleKeyDown}
        onFocus={handleFocus}
        onBlur={handleBlur}
        placeholder={placeholder}
        className={`${className} ${error ? errorClass : ""}`}
        disabled={disabled}
        data-testid={dataTestId}
        autoComplete="off"
      />

      {isOpen && filteredSuggestions.length > 0 && (
        <ul ref={listRef} className={suggestionsClass} role="listbox">
          {filteredSuggestions.map((sugg, index) => (
            <li
              key={sugg}
              className={`${suggestion} ${
                index === highlightedIndex ? highlighted : ""
              }`}
              onClick={() => handleSuggestionClick(sugg)}
              onMouseEnter={() => setHighlightedIndex(index)}
              role="option"
              aria-selected={index === highlightedIndex}
            >
              {getLabel(sugg)}
            </li>
          ))}
        </ul>
      )}

      {isOpen && isLoading && (
        <div className={loading}>Loading suggestions...</div>
      )}
    </div>
  );
}
