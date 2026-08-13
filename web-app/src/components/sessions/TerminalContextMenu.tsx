"use client";

import { useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import * as styles from "./TerminalContextMenu.css";

export interface TerminalContextMenuProps {
  x: number;
  y: number;
  hasSelection: boolean;
  onCopy: () => void;
  onSelectAll: () => void;
  onPaste?: () => void;
  onDismiss: () => void;
}

export function TerminalContextMenu({
  x,
  y,
  hasSelection,
  onCopy,
  onSelectAll,
  onPaste,
  onDismiss,
}: TerminalContextMenuProps) {
  const menuRef = useRef<HTMLUListElement>(null);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        onDismiss();
      }
    };
    const onMouseDown = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onDismiss();
      }
    };
    const onScroll = () => {
      onDismiss();
    };

    document.addEventListener("keydown", onKeyDown);
    document.addEventListener("mousedown", onMouseDown);
    window.addEventListener("scroll", onScroll, { passive: true, once: true });

    return () => {
      document.removeEventListener("keydown", onKeyDown);
      document.removeEventListener("mousedown", onMouseDown);
      window.removeEventListener("scroll", onScroll);
    };
  }, [onDismiss]);

  const handleCopy = () => {
    onCopy();
    onDismiss();
  };

  const handleSelectAll = () => {
    onSelectAll();
    onDismiss();
  };

  const handlePaste = () => {
    onPaste?.();
    onDismiss();
  };

  const hasPasteSupport =
    typeof navigator !== "undefined" && typeof navigator.clipboard?.readText === "function";

  return createPortal(
    <ul
      ref={menuRef}
      className={styles.menu}
      style={{ left: x, top: y }}
      data-testid="terminal-context-menu"
      role="menu"
    >
      <li role="none">
        <button
          className={styles.menuItem}
          role="menuitem"
          disabled={!hasSelection}
          onPointerDown={(e) => e.preventDefault()}
          onClick={handleCopy}
        >
          Copy
        </button>
      </li>
      <li role="none">
        <button
          className={styles.menuItem}
          role="menuitem"
          onClick={handleSelectAll}
        >
          Select All
        </button>
      </li>
      {onPaste && hasPasteSupport && (
        <li role="none">
          <button
            className={styles.menuItem}
            role="menuitem"
            onClick={handlePaste}
          >
            Paste
          </button>
        </li>
      )}
    </ul>,
    document.body
  );
}
