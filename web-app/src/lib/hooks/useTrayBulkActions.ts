"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import { useClearResultLine, type ClearByIdsResult } from "@/lib/hooks/useClearResultLine";
import { useUndoWindow } from "@/lib/hooks/useUndoWindow";
import type { NotificationHistoryItem } from "@/lib/types/notification";

interface Options {
  notificationHistory: NotificationHistoryItem[];
  isOffline: boolean;
  needsAttention: number;
  scopedMarkReadIds: string[];
  announce: (message: string, politeness?: "polite" | "assertive", key?: string) => void;
  markAsRead: (ids: string[]) => void | Promise<boolean>;
  clearHistoryByIds: (ids: string[], options?: { keepalive?: boolean }) => Promise<ClearByIdsResult>;
  showActionToast: (message: string, type: "success" | "error", key: string) => string | void;
}

export type TrayConfirmKind = "informational" | "history" | null;

/**
 * The tray's bulk actions (Story 4.4): Mark activity read, Clear informational
 * (hidden now, deleted server-side after the undo window), Clear history (inline
 * confirm, no undo) and the kept line. The ids sent are rows the server-sent
 * `isPendingDecision` says are not decisions; the server guards the same
 * predicate again and reports what it kept.
 */
export function useTrayBulkActions(o: Options) {
  const undoWindow = useUndoWindow();
  const { keptLine, applyClearResult, clearKeptLine } = useClearResultLine();
  const [hiddenIds, setHiddenIds] = useState<ReadonlySet<string>>(new Set());
  const [confirm, setConfirm] = useState<TrayConfirmKind>(null);
  const [confirmError, setConfirmError] = useState(false);
  const [clearError, setClearError] = useState<{ label: string; ids: string[] } | null>(null);
  const [markReadError, setMarkReadError] = useState(false);
  const pendingUndoIdsRef = useRef<string[]>([]);
  const { notificationHistory, isOffline, announce, clearHistoryByIds, showActionToast } = o;

  const informationalIds = useMemo(
    () => notificationHistory.filter((n) => n.notificationType !== "auto_approved" && !n.isPendingDecision).map((n) => n.id),
    [notificationHistory],
  );
  const readIds = useMemo(
    () => notificationHistory.filter((n) => n.isRead && !n.isPendingDecision).map((n) => n.id),
    [notificationHistory],
  );
  const decisionCount = useMemo(() => notificationHistory.filter((n) => n.isPendingDecision).length, [notificationHistory]);

  const unhide = useCallback((ids: string[]) => {
    setHiddenIds((prev) => {
      const next = new Set(prev);
      ids.forEach((id) => next.delete(id));
      return next;
    });
  }, []);

  const startUndoableClear = useCallback(
    (label: string, ids: string[]) => {
      if (ids.length === 0 || isOffline) return;
      clearKeptLine();
      setClearError(null);
      pendingUndoIdsRef.current = ids;
      setHiddenIds((prev) => new Set([...prev, ...ids]));
      announce(`${label}. Undo available.`, "polite", "tray-clear");
      undoWindow.start(label, async ({ keepalive }) => {
        try {
          applyClearResult(await clearHistoryByIds(ids, { keepalive }));
        } catch {
          setClearError({ label, ids });
          showActionToast("Could not clear notifications", "error", "tray-clear");
        } finally {
          unhide(ids);
        }
      });
    },
    [announce, applyClearResult, clearHistoryByIds, clearKeptLine, isOffline, showActionToast, undoWindow, unhide],
  );

  const undo = () => {
    const ids = pendingUndoIdsRef.current;
    undoWindow.undo();
    unhide(ids);
  };

  const confirmClearInformational = () => {
    setConfirm(null);
    startUndoableClear(`Cleared ${informationalIds.length}`, informationalIds);
  };

  const confirmClearHistory = async () => {
    setConfirmError(false);
    try {
      const result = await clearHistoryByIds(readIds);
      setConfirm(null);
      applyClearResult(result);
      showActionToast(`Cleared ${result.deleted} notifications`, "success", "tray-clear-history");
    } catch {
      setConfirmError(true);
    }
  };

  const markActivityRead = async () => {
    clearKeptLine();
    setMarkReadError(false);
    const ids = o.scopedMarkReadIds;
    const ok = await o.markAsRead(ids);
    if (ok === false) {
      setMarkReadError(true);
      return;
    }
    announce(
      `${ids.length} marked read${o.needsAttention > 0 ? `, ${o.needsAttention} still needs attention` : ""}`,
      "polite",
      "mark-read",
    );
  };

  const openConfirm = (kind: Exclude<TrayConfirmKind, null>) => {
    setConfirmError(false);
    setConfirm(kind);
  };

  const reset = () => {
    setConfirm(null);
    setConfirmError(false);
  };

  return {
    undoWindow,
    hiddenIds,
    keptLine,
    clearKeptLine,
    confirm,
    confirmError,
    clearError,
    markReadError,
    informationalIds,
    readIds,
    decisionCount,
    startUndoableClear,
    undo,
    confirmClearInformational,
    confirmClearHistory,
    markActivityRead,
    openConfirm,
    cancelConfirm: () => setConfirm(null),
    reset,
  };
}
