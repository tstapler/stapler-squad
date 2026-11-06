"use client";

import { useEffect, useRef } from "react";
import type { NamedWindow, WindowId } from "@/lib/window/windowTypes";
import * as styles from "@/styles/window/windowOverview.css";
import { WindowPreviewCard, type SessionTitleLookup } from "./WindowLayoutPreview";

interface WindowOverviewProps {
  windows: NamedWindow[];
  currentWindowId: WindowId;
  getSessionTitle: SessionTitleLookup;
  onSwitch: (id: WindowId) => void;
  onClose: () => void;
}

/** Full-screen grid of every window's layout; click or press 1-9 to jump, Esc to dismiss. */
export function WindowOverview({
  windows,
  currentWindowId,
  getSessionTitle,
  onSwitch,
  onClose,
}: WindowOverviewProps) {
  const panelRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    panelRef.current?.focus();
  }, []);

  const select = (id: WindowId) => {
    onSwitch(id);
    onClose();
  };

  return (
    <div
      className={styles.overlay}
      data-testid="window-overview"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        ref={panelRef}
        className={styles.overlayPanel}
        role="dialog"
        aria-modal="true"
        aria-label="Window overview"
        tabIndex={-1}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.preventDefault();
            onClose();
            return;
          }
          const target = /^[1-9]$/.test(e.key) ? windows[Number(e.key) - 1] : undefined;
          if (target) {
            e.preventDefault();
            select(target.id);
          }
        }}
      >
        <div className={styles.overlayHint}>Click a window or press 1–9 to switch · Esc to close</div>
        <div className={styles.overlayGrid}>
          {windows.map((w, i) => (
            <WindowPreviewCard
              key={w.id}
              window={w}
              position={i + 1}
              isActive={w.id === currentWindowId}
              getSessionTitle={getSessionTitle}
              onSelect={select}
            />
          ))}
        </div>
      </div>
    </div>
  );
}
