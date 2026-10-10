// +feature: notification-toast-stack
"use client";

import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { zIndex } from "@/styles/theme.css";
import { NotificationToast } from "@/components/ui/NotificationToast";
import {
  deckPlacement,
  chipRow,
  overflowChip,
  deckHeader,
  deckAction,
  undoBar,
  undoAction,
} from "@/components/ui/NotificationToast.css";
import { useViewport } from "@/components/providers/ViewportProvider";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import { useNotificationState, useNotificationCommands } from "@/lib/contexts/notificationContexts";
import { useNotificationConnectivity } from "@/lib/hooks/useNotificationConnectivity";
import { useAnnounce } from "@/lib/hooks/useAnnounce";
import { useAuditLog } from "@/lib/hooks/useAuditLog";
import type { ToastTimerRegistry } from "@/lib/hooks/useToastTimers";
import { MOVE_UNDO_TIMER_ID } from "@/lib/contexts/toastTray";
import { getStackTopOffset, subscribeStackTopOffset } from "@/lib/utils/toastDock";
import { readTerminalCursor } from "@/lib/terminal/cursorRect";
import { bottomRightFootprint, deckAnchor, type DeckAnchor } from "@/components/ui/toastDeckAnchor";
import { readPinnedCollapseMs } from "@/lib/utils/deckSettings";
import {
  NOTIFICATION_TRAY_V2_FLAG,
  isPinned,
  partitionToasts,
  toastAutoCloseMs,
  toastAutoMinimizeMs,
  toastCapFor,
} from "@/lib/notification-policy";
import type { NotificationData } from "@/lib/types/notification";

const EXIT_ANIMATION_MS = 300;

interface ToastStackProps {
  timers: ToastTimerRegistry;
}

interface ToastSlotProps {
  notification: NotificationData;
  timers: ToastTimerRegistry;
  onRemove: (id: string) => void;
  /** Capped deck: cards sit in flow, and a pinned decision never auto-closes or minimizes. */
  stacked: boolean;
  /** Phone deck: a pinned card with no interaction collapses to a chip so it cannot cover the terminal for long. */
  collapsible?: boolean;
  offlineReason?: string;
}

/**
 * One toast and its timers. Every timer goes through the registry so the stack,
 * and not the card, decides when a toast closes, minimizes or exits.
 */
function ToastSlot({ notification, timers, onRemove, stacked, collapsible = false, offlineReason }: ToastSlotProps) {
  const { id } = notification;
  // The audit-log object is new every render; a ref keeps timer effects from restarting.
  const auditLog = useAuditLog();
  const auditLogRef = useRef(auditLog);
  auditLogRef.current = auditLog;
  const [presented, setPresented] = useState(false);
  const [exiting, setExiting] = useState(false);
  const [minimized, setMinimized] = useState(false);
  const [collapsed, setCollapsed] = useState(false);
  // Once the user expands a collapsed card it stays open: no collapse loop.
  const expandedByUser = useRef(false);
  // True while an Approve or Deny is in flight or its confirm step is open (set by the card).
  const [busy, setBusy] = useState(false);
  // An Approve or Deny that failed keeps the toast up until it is resolved.
  const [actionFailed, setActionFailed] = useState(false);
  const closingRef = useRef(false);
  const pinned = isPinned(notification) || actionFailed;

  const requestClose = useCallback(
    ({ acknowledge = false }: { acknowledge?: boolean } = {}) => {
      if (closingRef.current) return;
      closingRef.current = true;
      setExiting(true);
      auditLogRef.current.logNotificationDismissed(id, notification.sessionId);
      timers.register(id, "exit", EXIT_ANIMATION_MS, () => {
        notification.onDismiss?.();
        if (acknowledge) notification.onAcknowledge?.();
        onRemove(id);
      });
    },
    [id, notification, onRemove, timers],
  );

  const onPresent = useCallback(() => {
    setPresented(true);
    return () => setPresented(false);
  }, []);

  useEffect(() => {
    if (!presented) return;
    // A pinned decision in the capped deck is never timed out: it leaves only by
    // an explicit action (the tray keeps it either way).
    if (stacked && pinned) return;
    const autoClose = toastAutoCloseMs(notification.notificationType);
    if (autoClose > 0) timers.register(id, "close", autoClose, () => requestClose());
    return () => timers.cancel(id, "close");
  }, [presented, stacked, pinned, id, notification.notificationType, requestClose, timers]);

  useEffect(() => {
    // The compact-pill minimize belongs to the legacy corner list; the deck keeps cards whole.
    if (!presented || minimized || stacked) return;
    const autoMinimize = toastAutoMinimizeMs(notification.notificationType);
    if (autoMinimize > 0) timers.register(id, "minimize", autoMinimize, () => setMinimized(true));
    return () => timers.cancel(id, "minimize");
  }, [presented, minimized, stacked, id, notification.notificationType, timers]);

  useEffect(() => {
    if (!presented || !collapsible || !pinned || collapsed || busy || expandedByUser.current) return;
    const collapseMs = readPinnedCollapseMs();
    if (collapseMs === null) return;
    timers.register(id, "collapse", collapseMs, () => setCollapsed(true));
    return () => timers.cancel(id, "collapse");
  }, [presented, collapsible, pinned, collapsed, busy, id, timers]);

  useEffect(() => () => timers.cancel(id), [id, timers]);

  // Hover, focus and touch hold this toast's timers; the remaining time is kept.
  const holdHandlers = useMemo(
    () => ({
      onPointerEnter: () => timers.pause(id, "hover"),
      onPointerLeave: () => timers.resume(id, "hover"),
      onFocus: () => timers.pause(id, "focus"),
      onBlur: () => timers.resume(id, "focus"),
    }),
    [id, timers],
  );

  return (
    <NotificationToast
      notification={notification}
      stacked={stacked}
      compact={collapsible}
      offlineReason={offlineReason}
      collapsed={collapsed}
      onExpandCollapsed={() => {
        expandedByUser.current = true;
        setCollapsed(false);
      }}
      onBusyChange={setBusy}
      onActionFailed={() => setActionFailed(true)}
      holdHandlers={holdHandlers}
      exiting={exiting}
      minimized={minimized}
      onExpand={() => setMinimized(false)}
      onClose={requestClose}
      onPresent={onPresent}
    />
  );
}

/** Speaks each newly arrived toast once; the Announcer coalesces bursts and owns the live regions. */
function useAnnounceArrivals(toasts: NotificationData[]) {
  const { announce, announceArrival } = useAnnounce();
  const seen = useRef<Set<string>>(new Set());

  useEffect(() => {
    const current = new Set(toasts.map((n) => n.id));
    for (const n of toasts) {
      if (seen.current.has(n.id)) continue;
      seen.current.add(n.id);
      const isReceipt = n.notificationType === "undo" || n.metadata?.actionToastKey !== undefined;
      if (isReceipt) {
        announce(n.message, n.notificationType === "error" ? "assertive" : "polite", n.id);
      } else {
        announceArrival({ title: n.title || n.sessionName || n.message, pinned: isPinned(n) });
      }
    }
    seen.current.forEach((id) => current.has(id) || seen.current.delete(id));
  }, [toasts, announce, announceArrival]);
}

interface DeckProps {
  toasts: NotificationData[];
  timers: ToastTimerRegistry;
  onRemove: (id: string) => void;
  onOpenTray: () => void;
}

const MOVE_ALL_LABEL = "Move all notifications to tray";
const ANCHOR_POLL_MS = 250;
const DESKTOP_DECK = { width: 360, inset: { right: 16, bottom: 24 }, fallbackHeight: 120 };

/**
 * Deck controls must not pull focus off the terminal: a mousedown on a button
 * would blur xterm's textarea (and, on a phone, collapse the soft keyboard).
 */
const keepTerminalFocus = { onMouseDown: (e: React.MouseEvent) => e.preventDefault() };

/**
 * Desktop overlap guard (TD-15): re-reads the xterm cursor a few times a second
 * while a deck is showing and moves the deck to the top-right anchor whenever
 * its bottom-right footprint would cover the cursor cell.
 */
function useDesktopDeckAnchor(enabled: boolean, deckRef: React.RefObject<HTMLDivElement | null>): DeckAnchor {
  const [anchor, setAnchor] = useState<DeckAnchor>("bottom-right");
  useEffect(() => {
    if (!enabled) return;
    const check = () => {
      const deck = deckRef.current;
      const footprint = bottomRightFootprint(
        { width: window.innerWidth, height: window.innerHeight },
        { width: DESKTOP_DECK.width, height: deck?.offsetHeight || DESKTOP_DECK.fallbackHeight },
        DESKTOP_DECK.inset,
      );
      setAnchor(deckAnchor(readTerminalCursor(), footprint));
    };
    check();
    const interval = setInterval(check, ANCHOR_POLL_MS);
    return () => clearInterval(interval);
  }, [enabled, deckRef]);
  return enabled ? anchor : "bottom-right";
}

/** "Moved N to tray - Undo", paused while hovered or focused so it cannot vanish under a tap. */
function UndoBar({ count, timers, onUndo }: { count: number; timers: ToastTimerRegistry; onUndo: () => void }) {
  const pause = (reason: string) => timers.pause(MOVE_UNDO_TIMER_ID, reason);
  const resume = (reason: string) => timers.resume(MOVE_UNDO_TIMER_ID, reason);
  // Release any hold when the bar goes away, so the next window cannot start paused.
  useEffect(
    () => () => {
      timers.resume(MOVE_UNDO_TIMER_ID, "hover");
      timers.resume(MOVE_UNDO_TIMER_ID, "focus");
    },
    [timers],
  );
  return (
    <div
      className={undoBar}
      data-testid="toast-undo-bar"
      onPointerEnter={() => pause("hover")}
      onPointerLeave={() => resume("hover")}
      onFocus={() => pause("focus")}
      onBlur={() => resume("focus")}
    >
      <span>Moved {count} to tray</span>
      <button
        type="button"
        className={undoAction}
        data-testid="toast-undo-move"
        onClick={onUndo}
        {...keepTerminalFocus}
      >
        Undo
      </button>
    </div>
  );
}

/** The capped deck (notification_tray_v2): at most `cap` cards and a "+N more" chip for the rest. */
function Deck({ toasts, timers, onRemove, onOpenTray }: DeckProps) {
  const viewport = useViewport();
  const { isOffline } = useNotificationConnectivity();
  const { movedToTray } = useNotificationState();
  const { moveAllToTray, undoMoveToTray } = useNotificationCommands();
  const cap = toastCapFor(viewport);
  const { visible, overflow, pinnedCount } = partitionToasts(toasts, cap);
  const onPhone = !viewport.isInnerScreen;
  const topOffset = useSyncExternalStore(subscribeStackTopOffset, getStackTopOffset, () => null);
  const deckRef = useRef<HTMLDivElement>(null);
  const hasContent = visible.length > 0 || overflow > 0 || movedToTray !== null;
  const desktopAnchor = useDesktopDeckAnchor(!onPhone && hasContent, deckRef);
  if (!hasContent) return null;

  // Phone: top dock under the session tab row when one is published, else above the bottom nav.
  let placement: keyof typeof deckPlacement;
  if (!onPhone) placement = desktopAnchor === "top-right" ? "desktopTopRight" : "desktop";
  else placement = topOffset !== null ? "mobileTop" : "mobileBottom";
  // With the keyboard open the cap is 0: one line, "N notifications - M needs you".
  const chipText =
    cap === 0
      ? `${toasts.length} notification${toasts.length === 1 ? "" : "s"}${pinnedCount > 0 ? ` - ${pinnedCount} needs you` : ""}`
      : `+${overflow} more`;
  const chipLabel = cap === 0 ? `${chipText}, open tray` : `${overflow} more notifications, open tray`;

  const moveAll = (
    <button
      type="button"
      className={deckAction}
      data-testid="toast-move-all-to-tray"
      aria-label={MOVE_ALL_LABEL}
      onClick={() => moveAllToTray()}
      {...keepTerminalFocus}
    >
      Move all to tray ({toasts.length})
    </button>
  );
  const showMoveAll = toasts.length >= 2;
  const undo = movedToTray ? <UndoBar count={movedToTray.count} timers={timers} onUndo={undoMoveToTray} /> : null;

  return (
    <div ref={deckRef} className={deckPlacement[placement]} data-testid="toast-stack" data-placement={placement}>
      {/* Desktop: one header slot, holding either the bulk control or its undo. */}
      {!onPhone && (undo ?? (showMoveAll ? <div className={deckHeader}>{moveAll}</div> : null))}
      {visible.map((notification) => (
        <ToastSlot
          key={notification.id}
          notification={notification}
          timers={timers}
          onRemove={onRemove}
          stacked
          collapsible={onPhone}
          offlineReason={isOffline ? "Offline" : undefined}
        />
      ))}
      {/* Phone: the undo replaces the chip row in place; otherwise chip plus a secondary bulk button. */}
      {onPhone && undo}
      {!(onPhone && undo) && (overflow > 0 || (onPhone && showMoveAll)) && (
        <div className={chipRow}>
          {overflow > 0 && (
            <button
              type="button"
              className={overflowChip}
              data-testid="toast-overflow-chip"
              aria-label={chipLabel}
              onClick={onOpenTray}
              {...keepTerminalFocus}
            >
              {chipText}
            </button>
          )}
          {onPhone && showMoveAll && moveAll}
        </div>
      )}
    </div>
  );
}

/** Legacy corner list: every toast renders, as before the capped deck. */
function LegacyList({ toasts, timers, onRemove }: Omit<DeckProps, "onOpenTray">) {
  return (
    <div
      style={{
        position: "fixed",
        bottom: 0,
        right: 0,
        zIndex: zIndex.toast,
        pointerEvents: "none",
      }}
    >
      {toasts.map((notification) => (
        <div key={notification.id} style={{ pointerEvents: "auto" }}>
          <ToastSlot notification={notification} timers={timers} onRemove={onRemove} stacked={false} />
        </div>
      ))}
    </div>
  );
}

/**
 * Renders the active toasts. The provider owns the queue and the timer registry;
 * this owns presentation, per-toast timers and the cap. With `notification_tray_v2`
 * off it renders the legacy uncapped list.
 */
export function ToastStack({ timers }: ToastStackProps) {
  const { notifications, isPanelOpen } = useNotificationState();
  const { removeNotification, togglePanel } = useNotificationCommands();
  const v2 = useFeatureFlag(NOTIFICATION_TRAY_V2_FLAG);
  useAnnounceArrivals(notifications);

  // An open tray holds every toast timer; closing it resumes them with time preserved.
  useEffect(() => {
    if (isPanelOpen) timers.hold("tray-open");
    else timers.release("tray-open");
    return () => timers.release("tray-open");
  }, [isPanelOpen, timers]);

  return v2 ? (
    <Deck toasts={notifications} timers={timers} onRemove={removeNotification} onOpenTray={togglePanel} />
  ) : (
    <LegacyList toasts={notifications} timers={timers} onRemove={removeNotification} />
  );
}
