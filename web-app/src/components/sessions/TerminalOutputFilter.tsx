"use client";
// +feature: terminal-output-filter

import { useEffect, useRef, useState } from "react";
import { filterLines, MAX_FILTER_RESULTS, type FilterResult } from "@/lib/terminal/bufferFilter";
import * as styles from "./TerminalOutputFilter.css";

const REFRESH_MS = 1000;

interface TerminalOutputFilterProps {
  /** Current terminal buffer as plain lines; called on open, on query change, and every second. */
  getLines: () => string[];
  onClose: () => void;
}

/**
 * Opt-in, display-only filter over the terminal's buffer. It renders its own list
 * and never touches the PTY stream or xterm's buffer.
 */
export function TerminalOutputFilter({ getLines, onClose }: TerminalOutputFilterProps) {
  const [query, setQuery] = useState("");
  const [regex, setRegex] = useState(false);
  const [caseSensitive, setCaseSensitive] = useState(false);
  const [result, setResult] = useState<FilterResult>({ matches: [], truncated: false });
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  useEffect(() => {
    const run = () => setResult(filterLines(getLines(), query, { regex, caseSensitive }));
    run();
    const id = setInterval(run, REFRESH_MS);
    return () => clearInterval(id);
  }, [getLines, query, regex, caseSensitive]);

  const hasQuery = query !== "";

  return (
    <div
      className={styles.panel}
      role="search"
      aria-label="Filter terminal output"
      data-testid="terminal-output-filter"
      onKeyDown={(e) => {
        if (e.key === "Escape") onClose();
      }}
    >
      <div className={styles.header}>
        <input
          ref={inputRef}
          className={styles.input}
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Filter lines…"
          aria-label="Filter text"
          data-testid="terminal-filter-input"
        />
        <label className={styles.option}>
          <input type="checkbox" checked={regex} onChange={(e) => setRegex(e.target.checked)} data-testid="terminal-filter-regex" />
          Regex
        </label>
        <label className={styles.option}>
          <input type="checkbox" checked={caseSensitive} onChange={(e) => setCaseSensitive(e.target.checked)} data-testid="terminal-filter-case" />
          Case
        </label>
        <button type="button" onClick={onClose} aria-label="Close filter" data-testid="terminal-filter-close">
          ✕
        </button>
      </div>

      <div className={styles.status} role="status" data-testid="terminal-filter-status">
        {result.error
          ? `Invalid pattern: ${result.error}`
          : !hasQuery
            ? "Type to filter the terminal's scrollback. Display only — the terminal is unchanged."
            : result.truncated
              ? `Showing the last ${MAX_FILTER_RESULTS} matching lines`
              : `${result.matches.length} matching ${result.matches.length === 1 ? "line" : "lines"}`}
      </div>

      {result.matches.length > 0 && (
        <ul className={styles.results} data-testid="terminal-filter-results">
          {result.matches.map((m) => (
            <li key={m.line} className={styles.row}>
              <span className={styles.lineNumber}>{m.line}</span>
              <span>{m.text}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
