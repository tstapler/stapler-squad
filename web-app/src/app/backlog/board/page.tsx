// analytics-exempt
"use client";
// +feature: backlog:board-page

import { useState, useCallback, useRef, useEffect } from "react";
import { useRouter } from "next/navigation";
import { BacklogBoard } from "@/components/backlog/BacklogBoard";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { useStuckBacklogItems } from "@/lib/hooks/useStuckBacklogItems";

const ACTION_SUCCESS_MESSAGES: Record<string, string> = {
  mark_ready: "Marked ready.",
  trigger_triage: "Triage started.",
  spawn_session: "Session started.",
  cancel_triage: "Triage cancelled.",
};

export default function BacklogBoardPage() {
  const { transitionStatus, triggerTriage, spawnSessionFromItem, cancelTriage } = useBacklogService();
  const { showActionToast } = useNotifications();
  const router = useRouter();
  /** itemId -> action key currently in flight for that card. */
  const [pending, setPending] = useState<Record<string, string>>({});
  // Called once here (not per-card) so every card shares one poll instead of
  // N independent 60s polls — see plan.md Task 5.1.1a.
  const { items: stuckItems } = useStuckBacklogItems();

  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const handleAction = useCallback(
    async (action: string, itemId: string) => {
      if (action === "view_session" || action === "view_review") {
        router.push(`/backlog?item=${itemId}`);
        return;
      }
      setPending((prev) => ({ ...prev, [itemId]: action }));
      const toastKey = `${itemId}:${action}`;
      let successMessage = ACTION_SUCCESS_MESSAGES[action] ?? "Done.";
      try {
        switch (action) {
          case "mark_ready":
            await transitionStatus(itemId, "ready");
            break;
          case "trigger_triage":
            await triggerTriage(itemId);
            break;
          case "spawn_session": {
            const resp = await spawnSessionFromItem(itemId);
            if (resp?.queued) successMessage = "At capacity — item queued, will start automatically.";
            break;
          }
          case "cancel_triage":
            await cancelTriage(itemId);
            break;
          default:
            return;
        }
        // No manual refetch: the mutation's server-side effect publishes a
        // BacklogItemEvent that the board's live useWatchBacklogItems stream
        // picks up and applies to the shared store, so the card/column
        // updates itself once that event arrives (Epic 5.2).
        showActionToast(successMessage, "success", toastKey);
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
    [transitionStatus, triggerTriage, spawnSessionFromItem, cancelTriage, router, showActionToast]
  );

  const handleItemClick = useCallback(
    (itemId: string) => {
      router.push(`/backlog?item=${itemId}`);
    },
    [router]
  );

  return (
    <BacklogBoard
      onAction={handleAction}
      onItemClick={handleItemClick}
      pending={pending}
      stuckItems={stuckItems}
    />
  );
}
