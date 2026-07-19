// analytics-exempt
"use client";
// +feature: backlog:board-page

import { useState, useEffect, useCallback, useRef } from "react";
import { useRouter } from "next/navigation";
import { BacklogBoard } from "@/components/backlog/BacklogBoard";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";
import { useNotifications } from "@/lib/contexts/NotificationContext";

const ACTION_SUCCESS_MESSAGES: Record<string, string> = {
  mark_ready: "Marked ready.",
  trigger_triage: "Triage started.",
  spawn_session: "Session started.",
  cancel_triage: "Triage cancelled.",
};

export default function BacklogBoardPage() {
  const { listBacklogItems, transitionStatus, triggerTriage, spawnSessionFromItem, cancelTriage } =
    useBacklogService();
  const { showActionToast } = useNotifications();
  const router = useRouter();
  const [items, setItems] = useState<BacklogItem[]>([]);
  const [loading, setLoading] = useState(true);
  /** itemId -> action key currently in flight for that card. */
  const [pending, setPending] = useState<Record<string, string>>({});

  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const result = await listBacklogItems();
      if (mountedRef.current) setItems(result);
    } finally {
      if (mountedRef.current) setLoading(false);
    }
  }, [listBacklogItems]);

  useEffect(() => {
    void load();
  }, [load]);

  const handleAction = useCallback(
    async (action: string, itemId: string) => {
      if (action === "view_session" || action === "view_review") {
        router.push(`/backlog?item=${itemId}`);
        return;
      }
      setPending((prev) => ({ ...prev, [itemId]: action }));
      const toastKey = `${itemId}:${action}`;
      try {
        switch (action) {
          case "mark_ready":
            await transitionStatus(itemId, "ready");
            break;
          case "trigger_triage":
            await triggerTriage(itemId);
            break;
          case "spawn_session":
            await spawnSessionFromItem(itemId);
            break;
          case "cancel_triage":
            await cancelTriage(itemId);
            break;
          default:
            return;
        }
        showActionToast(ACTION_SUCCESS_MESSAGES[action] ?? "Done.", "success", toastKey);
        await load();
      } catch (e) {
        const msg = e instanceof Error ? e.message : "Action failed.";
        showActionToast(msg, "error", toastKey);
      } finally {
        if (mountedRef.current) {
          setPending((prev) => {
            const next = { ...prev };
            delete next[itemId];
            return next;
          });
        }
      }
    },
    [transitionStatus, triggerTriage, spawnSessionFromItem, cancelTriage, load, router, showActionToast]
  );

  const handleItemClick = useCallback(
    (itemId: string) => {
      router.push(`/backlog?item=${itemId}`);
    },
    [router]
  );

  return (
    <BacklogBoard
      items={items}
      onAction={handleAction}
      onItemClick={handleItemClick}
      isLoading={loading}
      pending={pending}
    />
  );
}
