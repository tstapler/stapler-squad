// +feature: notification-tray
"use client";

import { useMemo, type ReactNode } from "react";
import { useDeckViewport } from "@/lib/contexts/deckViewportContext";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import { useNotificationCommands, useNotificationState } from "@/lib/contexts/notificationContexts";
import { NOTIFICATION_TRAY_V2_FLAG } from "@/lib/notification-policy";
import { countNeedsAttention } from "@/lib/utils/notificationGrouping";
import { capBadgeCount } from "@/lib/utils/notificationMapping";
import { trayHandle, trayHandleDot, trayQuietBadge } from "./NotificationPanel.css";
import { overflowChip, trayEntry, trayEntryBell } from "./NotificationToast.css";
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
 * the terminal; nothing reflows. Not rendered on a phone (the entry chip is) or
 * with `notification_tray_v2` off.
 */
export function TrayHandle() {
  const v2 = useFeatureFlag(NOTIFICATION_TRAY_V2_FLAG);
  const { isInnerScreen } = useDeckViewport();
  const { isPanelOpen, quietMode } = useNotificationState();
  const { togglePanel } = useNotificationCommands();
  const { unreadCount, needsAttention } = useTrayCounts();
  if (!v2 || !isInnerScreen) return null;

  return (
    <button
      type="button"
      className={trayHandle}
      data-testid="tray-handle"
      data-open={isPanelOpen}
      aria-expanded={isPanelOpen}
      aria-controls={TRAY_ID}
      aria-label={trayEntryLabel(unreadCount, needsAttention, quietMode)}
      onClick={() => togglePanel()}
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
  /** The deck's "Moved N to tray" undo, rendered inside the entry with a trailing bell. */
  undo?: ReactNode;
  onOpen: () => void;
}

/**
 * The one phone tray entry (D10). A single wrapper stays mounted while `content`
 * changes inside it, so no frame shows two entries or none and nothing relocates.
 * Rendered by the deck's dock, never by a page.
 */
export function TrayEntryChip({ content, chipText, chipLabel, undo, onOpen }: TrayEntryChipProps) {
  const { isPanelOpen, quietMode } = useNotificationState();
  const { unreadCount, needsAttention } = useTrayCounts();
  const bellProps = {
    type: "button" as const,
    className: trayEntryBell,
    "data-testid": "tray-entry-open",
    "aria-expanded": isPanelOpen,
    "aria-controls": TRAY_ID,
    "aria-label": trayEntryLabel(unreadCount, needsAttention, quietMode),
    onClick: onOpen,
    ...keepTerminalFocus,
  };
  const bell = (
    <button {...bellProps}>
      <BellIcon />
      {quietMode && (
        <span className={trayQuietBadge} data-testid="tray-entry-quiet" aria-hidden="true">
          Quiet
        </span>
      )}
      {unreadCount > 0 && <span>{capBadgeCount(unreadCount)}</span>}
    </button>
  );
  const wide = content === "more-row" || content === "keyboard-chip";

  return (
    <div className={trayEntry} data-testid="tray-entry" data-content={content} data-undo={undo ? "true" : "false"}>
      {undo ? (
        <>
          {undo}
          {bell}
        </>
      ) : wide ? (
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
      ) : (
        bell
      )}
    </div>
  );
}
