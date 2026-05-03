"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import { registry, ShortcutContext, Shortcut } from "@/lib/shortcuts/shortcutRegistry";
import { Kbd } from "./Kbd";
import {
  backdrop,
  dialog,
  dialogHeader,
  dialogTitle,
  closeButton,
  searchInput,
  scrollArea,
  contextSection,
  contextHeading,
  shortcutRow,
  shortcutLabel,
  emptyMessage,
} from "./KeyboardShortcutOverlay.css";

const CONTEXT_LABELS: Record<ShortcutContext, string> = {
  global: "Global",
  "session-list": "Session List",
  approval: "Approval Review",
  terminal: "Terminal",
};

interface KeyboardShortcutOverlayProps {
  isOpen: boolean;
  onClose: () => void;
}

export function KeyboardShortcutOverlay({ isOpen, onClose }: KeyboardShortcutOverlayProps) {
  const [query, setQuery] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);

  // Register Escape to close while overlay is open
  useEffect(() => {
    if (!isOpen) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        onClose();
      }
    };
    document.addEventListener("keydown", handler, { capture: true });
    return () => document.removeEventListener("keydown", handler, { capture: true });
  }, [isOpen, onClose]);

  // Focus search on open
  useEffect(() => {
    if (isOpen) {
      setQuery("");
      requestAnimationFrame(() => searchRef.current?.focus());
    }
  }, [isOpen]);

  // Focus trap
  useEffect(() => {
    if (!isOpen || !dialogRef.current) return;
    const focusable = dialogRef.current.querySelectorAll<HTMLElement>(
      'button, input, [tabindex]:not([tabindex="-1"])'
    );
    const first = focusable[0];
    const last = focusable[focusable.length - 1];

    const trap = (e: KeyboardEvent) => {
      if (e.key !== "Tab") return;
      if (e.shiftKey) {
        if (document.activeElement === first) {
          e.preventDefault();
          last?.focus();
        }
      } else {
        if (document.activeElement === last) {
          e.preventDefault();
          first?.focus();
        }
      }
    };
    document.addEventListener("keydown", trap);
    return () => document.removeEventListener("keydown", trap);
  }, [isOpen]);

  const allShortcuts = registry.getAll();
  const loweredQuery = query.toLowerCase();

  const filteredByContext: Partial<Record<ShortcutContext, Shortcut[]>> = {};
  const contexts: ShortcutContext[] = ["global", "session-list", "approval", "terminal"];

  for (const ctx of contexts) {
    const filtered = allShortcuts[ctx].filter((s) =>
      !query || s.label.toLowerCase().includes(loweredQuery)
    );
    if (filtered.length > 0) {
      filteredByContext[ctx] = filtered;
    }
  }

  const totalVisible = Object.values(filteredByContext).reduce((acc, arr) => acc + arr.length, 0);

  if (!isOpen) return null;

  return (
    <div className={backdrop} onClick={onClose} aria-hidden="false">
      <div
        ref={dialogRef}
        className={dialog}
        role="dialog"
        aria-label="Keyboard shortcuts"
        aria-modal="true"
        onClick={(e) => e.stopPropagation()}
      >
        <div className={dialogHeader}>
          <h2 className={dialogTitle}>Keyboard Shortcuts</h2>
          <button className={closeButton} onClick={onClose} aria-label="Close">
            ✕
          </button>
        </div>

        <input
          ref={searchRef}
          className={searchInput}
          type="search"
          placeholder="Search shortcuts..."
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          aria-label="Search keyboard shortcuts"
        />

        <div className={scrollArea}>
          {totalVisible === 0 ? (
            <p className={emptyMessage}>No shortcuts match &ldquo;{query}&rdquo;</p>
          ) : (
            contexts.map((ctx) => {
              const shortcuts = filteredByContext[ctx];
              if (!shortcuts || shortcuts.length === 0) return null;
              return (
                <section key={ctx} className={contextSection} aria-label={CONTEXT_LABELS[ctx]}>
                  <h3 className={contextHeading}>{CONTEXT_LABELS[ctx]}</h3>
                  {shortcuts.map((s) => (
                    <div key={s.label} className={shortcutRow}>
                      <span className={shortcutLabel}>{s.label}</span>
                      <Kbd size="sm">
                        {[
                          s.modifiers?.meta && "⌘",
                          s.modifiers?.ctrl && "Ctrl",
                          s.modifiers?.shift && "⇧",
                          s.modifiers?.alt && "⌥",
                          s.key,
                        ]
                          .filter(Boolean)
                          .join("+")}
                      </Kbd>
                    </div>
                  ))}
                </section>
              );
            })
          )}
        </div>
      </div>
    </div>
  );
}
