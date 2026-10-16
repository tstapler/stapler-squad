// +feature: notification-tray
"use client";

import { useMemo, useRef, type ReactNode } from "react";
import { useDraggableBell } from "@/lib/hooks/useDraggableBell";
import { useDeckViewport } from "@/lib/contexts/deckViewportContext";
import { useNotificationCommands, useNotificationState } from "@/lib/contexts/notificationContexts";
import { countNeedsAttention } from "@/lib/utils/notificationGrouping";
import { capBadgeCount } from "@/lib/utils/notificationMapping";
import { trayHandle, trayHandleDot, trayQuietBadge } from "./NotificationPanel.css";
import { overflowChip, trayEntry } from "./NotificationToast.css";
import type { TrayAffordance } from "./trayVariant";

/** A mousedown on a button would blur xterm's textarea (and collapse a soft keyboard). */
export const keepTerminalFocus = { onMouseDown: (e: React.MouseEvent) => e.preventDefault() };

const TRAY_ID = "notification-tray";

export function trayEntryLabel(unread: number, needsAttention: number, quiet = false): string {
  const decisions = needsAttention > 0 ? `, ${needsAttention} need attention` : "";
  return `Notifications, ${unread} unread${decisions}${quiet ? ", quiet mode on" : ""}`;
}

function BellIcon() {
  return (
    <svg aria-hidden="true" xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
      <path d="M13.73 21a2 2 0 0 1-3.46 0" />
    </svg>
  );
}

/** Unread count and the "needs attention" count, derived once for every entry point. */
export function useTrayCounts() {
  const { notificationHistory, unreadCount } = useNotificationState();
  const needsAttention = useMemo(() => countNeedsAttention(notificationHistory), [notificationHistory]);
  return { unreadCount, needsAttention };
}

/**
 * The desktop edge handle: a fixed 44px-wide band at 35% from the top (O6),
 * capped "99+" visually with the exact count in its accessible name. It overlays
 * the terminal; nothing reflows. Draggable (see useDraggableBell). On a phone it renders only when the opt-in floating bell is on.
 */
export function TrayHandle() {
  const { isInnerScreen } = useDeckViewport();
  const { isPanelOpen, quietMode } = useNotificationState();
  const { togglePanel } = useNotificationCommands();
  const { unreadCount, needsAttention } = useTrayCounts();
  const { isVirtualKeyboardOpen } = useDeckViewport();
  const ref = useRef<HTMLButtonElement>(null);
  const bell = useDraggableBell(ref, !isInnerScreen);
  // The phone's standing entry is the bottom nav; the floating bell is opt-in and yields to the keyboard.
  if (!isInnerScreen && (!bell.floatingBellEnabled || isVirtualKeyboardOpen)) return null;

  return (
    <button
      ref={ref}
      type="button"
      className={trayHandle}
      data-testid="tray-handle"
      data-open={isPanelOpen}
      aria-expanded={isPanelOpen}
      aria-controls={TRAY_ID}
      aria-keyshortcuts="ArrowUp ArrowDown"
      aria-label={trayEntryLabel(unreadCount, needsAttention, quietMode)}
      data-gesture-ignore
      data-dragging={bell.dragging}
      style={{
        touchAction: "none",
        userSelect: "none",
        WebkitTouchCallout: "none",
        ...bell.style,
      }}
      onClick={() => {
        if (bell.consumeClickSuppression()) return;
        togglePanel();
      }}
      {...bell.handlers}
      {...keepTerminalFocus}
    >
      <BellIcon />
      {quietMode && (
        <span className={trayQuietBadge} data-testid="tray-handle-quiet" aria-hidden="true">
          Quiet
        </span>
      )}
      {unreadCount > 0 && <span data-testid="tray-handle-count">{capBadgeCount(unreadCount)}</span>}
      {needsAttention > 0 && <span className={trayHandleDot} data-testid="tray-handle-dot" aria-hidden="true" />}
    </button>
  );
}

interface TrayEntryChipProps {
  content: TrayAffordance;
  /** Text of the "+N more" row or the 1-line keyboard chip. */
  chipText: string;
  chipLabel: string;
  /** The deck's "Moved N to tray" undo; replaces the chip in place. */
  undo?: ReactNode;
  onOpen: () => void;
}

/**
 * The phone deck's tray entry: the "+N more" row, the keyboard chip, or the undo bar.
 * There is no bell here; the bottom nav's Alerts tab is the standing entry to the tray.
 * Renders nothing when there is nothing to say. Rendered by the deck's dock, never by a page.
 */
export function TrayEntryChip({ content, chipText, chipLabel, undo, onOpen }: TrayEntryChipProps) {
  const { isPanelOpen } = useNotificationState();
  const wide = content === "more-row" || content === "keyboard-chip";
  if (!undo && !wide) return null;

  return (
    <div className={trayEntry} data-testid="tray-entry" data-content={content} data-undo={undo ? "true" : "false"}>
      {undo ?? (
        <button
          type="button"
          className={overflowChip}
          data-testid="toast-overflow-chip"
          aria-expanded={isPanelOpen}
          aria-controls={TRAY_ID}
          aria-label={chipLabel}
          onClick={onOpen}
          {...keepTerminalFocus}
        >
          {chipText}
        </button>
      )}
    </div>
  );
}
