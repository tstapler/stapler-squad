"use client";

import { useEffect, useRef, useState, type KeyboardEvent } from "react";
import { bucketSymbols } from "@/lib/files/analytics/events";
import { useFilesAnalytics } from "@/lib/files/analytics/filesAnalytics";
import type { OutlineResult, OutlineSymbol } from "@/lib/files/outline";
import { panel, header, heading, filter, list, option, symbolKind, symbolLine, message, actions, textButton } from "./OutlinePanel.css";

export interface OutlinePanelProps {
  result: OutlineResult;
  onSelect: (symbol: OutlineSymbol) => void;
  onClose: () => void;
  onGoToLine: () => void;
}

const UNAVAILABLE_TEXT = {
  large: "Outline unavailable for large files",
  unsupported: "No symbols found for this file type",
  error: "Could not read symbols",
} as const;

/** Symbol picker: type-ahead listbox, or the specific unavailable message with its exits. */
export function OutlinePanel({ result, onSelect, onClose, onGoToLine }: OutlinePanelProps) {
  const analytics = useFilesAnalytics({ component: "FileContentViewer" });
  const [query, setQuery] = useState("");
  const [cursor, setCursor] = useState(0);
  const invoker = useRef<HTMLElement | null>(
    typeof document === "undefined" ? null : (document.activeElement as HTMLElement | null),
  );
  const done = useRef(false);

  const symbols = result.kind === "symbols" ? result.symbols : [];
  const shown = symbols.filter((s) => s.name.toLowerCase().includes(query.toLowerCase()));
  const bucket = bucketSymbols(symbols.length);
  const message_ = result.kind === "unavailable" ? UNAVAILABLE_TEXT[result.reason] : symbols.length === 0 ? UNAVAILABLE_TEXT.unsupported : null;

  // Unavailable and empty outlines are terminal on open; a real list ends on select or close.
  useEffect(() => {
    if (message_ === null) return;
    done.current = true;
    analytics.emit("files.nav.outline", { outcome: result.kind === "unavailable" && result.reason === "large" ? "unavailable_large" : "empty", symbols: bucket });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  useEffect(() => {
    const node = invoker.current;
    return () => node?.focus?.();
  }, []);

  const finish = (outcome: "jumped" | "closed") => {
    if (!done.current) analytics.emit("files.nav.outline", { outcome, symbols: bucket });
    done.current = true;
  };
  const choose = (s: OutlineSymbol) => { finish("jumped"); onSelect(s); };
  const close = () => { finish("closed"); onClose(); };

  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Escape") { e.preventDefault(); close(); }
    else if (e.key === "ArrowDown") { e.preventDefault(); setCursor((c) => Math.min(c + 1, shown.length - 1)); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setCursor((c) => Math.max(c - 1, 0)); }
    else if (e.key === "Enter" && shown[cursor]) { e.preventDefault(); choose(shown[cursor]); }
  };

  return (
    <div className={panel} role="dialog" aria-label="Outline" onKeyDown={onKeyDown}>
      <div className={header}>
        <span className={heading}>Outline{symbols.length ? ` (${symbols.length})` : ""}</span>
        {message_ === null && (
          <input
            className={filter}
            type="text"
            aria-label="Filter symbols"
            autoFocus
            value={query}
            aria-controls="outline-symbols"
            aria-activedescendant={shown[cursor] ? `outline-opt-${cursor}` : undefined}
            onChange={(e) => { setQuery(e.target.value); setCursor(0); }}
          />
        )}
        <button type="button" className={textButton} onClick={close}>Close</button>
      </div>
      {message_ !== null ? (
        <>
          <div className={message} role="status">{message_}</div>
          <div className={actions}>
            <button type="button" className={textButton} onClick={() => { finish("closed"); onGoToLine(); }}>Go to line</button>
          </div>
        </>
      ) : (
        <ul className={list} id="outline-symbols" role="listbox" aria-label="Symbols">
          {shown.map((s, i) => (
            <li
              key={`${s.line}:${s.name}`}
              id={`outline-opt-${i}`}
              role="option"
              aria-selected={i === cursor}
              className={option}
              style={{ paddingLeft: 8 + s.depth * 12 }}
              onMouseEnter={() => setCursor(i)}
              onClick={() => choose(s)}
            >
              <span>{s.name}</span>
              <span className={symbolKind}>{s.kind}</span>
              <span className={symbolLine}>{s.line}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
