"use client";

import { forwardRef } from "react";
import type { Session } from "@/gen/session/v1/types_pb";
import type { LeafPane, PaneId } from "@/lib/pane/paneTypes";
import { mobileTabStrip, mobileTabButton, mobileAddPaneButton } from "@/styles/pane/mobilePaneTabStrip.css";

interface MobilePaneTabStripProps {
  leaves: LeafPane[];
  focusedPaneId: PaneId;
  sessions: Session[];
  onFocus: (paneId: PaneId) => void;
  onAddPane?: () => void;
}

export const MobilePaneTabStrip = forwardRef<HTMLDivElement, MobilePaneTabStripProps>(
  function MobilePaneTabStrip({
    leaves,
    focusedPaneId,
    sessions,
    onFocus,
    onAddPane,
  }, ref) {
  if (leaves.length <= 1) return null;

  return (
    <div ref={ref} className={mobileTabStrip} role="tablist" aria-label="Pane switcher">
      {leaves.map((l) => {
        const session = l.sessionId
          ? sessions.find((s) => s.id === l.sessionId) ?? null
          : null;
        const label = session ? session.title : l.viewKind === "session-list" ? "Sessions" : "Empty";
        const isActive = l.id === focusedPaneId;

        return (
          <button
            key={l.id}
            role="tab"
            aria-selected={isActive}
            className={mobileTabButton({ active: isActive })}
            onClick={() => onFocus(l.id)}
            title={label}
          >
            {label}
          </button>
        );
      })}
      {onAddPane && (
        <button
          className={mobileAddPaneButton}
          onClick={onAddPane}
          title="Add new pane"
          aria-label="Add new pane"
        >
          +
        </button>
      )}
    </div>
  );
});

