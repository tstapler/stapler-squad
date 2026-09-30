"use client";

import { useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import * as styles from "./TerminalContextMenu.css";

export interface TerminalLinkMenuProps {
  x: number;
  y: number;
  uri: string;
  onOpen: (uri: string) => void;
  onCopy: (uri: string) => void;
  onDismiss: () => void;
}

// Shown on touch devices instead of xterm's default tap-to-open behavior — a bare tap on a
// terminal link (e.g. an internal-only hostname with no public DNS) opens it immediately,
// leaving no way to copy the URL before Chrome navigates away from it.
export function TerminalLinkMenu({ x, y, uri, onOpen, onCopy, onDismiss }: TerminalLinkMenuProps) {
  const menuRef = useRef<HTMLUListElement>(null);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onDismiss();
    };
    const onMouseDown = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) onDismiss();
    };
    const onScroll = () => onDismiss();

    document.addEventListener("keydown", onKeyDown);
    document.addEventListener("mousedown", onMouseDown);
    window.addEventListener("scroll", onScroll, { passive: true, once: true });

    return () => {
      document.removeEventListener("keydown", onKeyDown);
      document.removeEventListener("mousedown", onMouseDown);
      window.removeEventListener("scroll", onScroll);
    };
  }, [onDismiss]);

  const handleOpen = () => {
    onOpen(uri);
    onDismiss();
  };

  const handleCopy = () => {
    onCopy(uri);
    onDismiss();
  };

  return createPortal(
    <ul
      ref={menuRef}
      className={styles.menu}
      style={{ left: x, top: y }}
      data-testid="terminal-link-menu"
      role="menu"
    >
      <li role="none">
        <button className={styles.menuItem} role="menuitem" onClick={handleCopy}>
          Copy link
        </button>
      </li>
      <li role="none">
        <button className={styles.menuItem} role="menuitem" onClick={handleOpen}>
          Open link
        </button>
      </li>
    </ul>,
    document.body
  );
}
