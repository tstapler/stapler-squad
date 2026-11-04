"use client";

import { useCallback, useEffect, useRef } from "react";
import { useRouter } from "@/lib/nav";
import { useNotificationCommands } from "@/lib/contexts/notificationContexts";
import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import { BacklogService } from "@/gen/session/v1/backlog_pb";
import { store } from "@/lib/store";
import { upsertItem } from "@/lib/store/backlogItemsSlice";
import { useBacklogService } from "@/lib/hooks/useBacklogService";

const POLL_INTERVAL_MS = 4000;
// Give up on an item whose triage never settles (e.g. the service restarted mid-run).
const MAX_TRACK_MS = 30 * 60 * 1000;

interface Tracked {
  title: string;
  startedAt: number;
  notifiedComplete: boolean;
  notifiedGuidance: boolean;
}

// CreateBacklogItem publishes no live event, so an open list only learns of a
// new item once it is upserted into the shared store (as the backlog page does).
async function hydrateItemIntoStore(itemId: string, signal: AbortSignal): Promise<void> {
  try {
    const client = createClient(
      BacklogService,
      getConnectTransport()
    );
    const resp = await client.getBacklogItem({ itemId }, { signal });
    if (resp.item) store.dispatch(upsertItem(resp.item));
  } catch (err) {
    console.error("[useBacklogCreationNotifier] failed to hydrate new item into the store:", err);
  }
}

/**
 * Follows backlog items created from the compose widget and raises a
 * notification when triage completes or the AI asks the user a question.
 * Only items passed to trackItem are watched, so items from import or the
 * manual form never notify. Tracking is in-memory and per-tab.
 */
export function useBacklogCreationNotifier(): {
  trackItem: (id: string, title: string) => void;
} {
  const router = useRouter();
  const { addNotification } = useNotificationCommands();
  const { getBacklogItem } = useBacklogService();
  const trackedRef = useRef(new Map<string, Tracked>());
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const inFlightRef = useRef(false);
  const mountedRef = useRef(true);
  const abortRef = useRef<AbortController | null>(null);

  const notify = useCallback(
    (id: string, headline: string, title: string, type: "task_complete" | "question" | "error") => {
      addNotification({
        sessionId: "",
        sessionName: "",
        title: `${headline} — ${title}`,
        message: "Open the backlog item to review.",
        notificationType: type,
        onView: () => router.push(`/backlog?item=${id}`),
      });
    },
    [addNotification, router]
  );

  const pollOnce = useCallback(async () => {
    const tracked = trackedRef.current;
    for (const [id, t] of Array.from(tracked)) {
      if (Date.now() - t.startedAt > MAX_TRACK_MS) {
        tracked.delete(id);
        continue;
      }
      let item;
      try {
        item = await getBacklogItem(id);
      } catch {
        continue;
      }
      if (!mountedRef.current) return;
      if (!item) continue;

      if (item.hasPendingGuidanceRequests) {
        if (!t.notifiedGuidance) {
          t.notifiedGuidance = true;
          notify(id, "AI needs your input", t.title, "question");
        }
      } else {
        t.notifiedGuidance = false;
      }

      if (!t.notifiedComplete && item.triageStatus === "completed") {
        t.notifiedComplete = true;
        notify(id, "Triage complete", t.title, "task_complete");
      } else if (!t.notifiedComplete && item.triageStatus === "failed") {
        t.notifiedComplete = true;
        notify(id, "Triage failed", t.title, "error");
      }

      if (t.notifiedComplete && !item.hasPendingGuidanceRequests) tracked.delete(id);
    }
    if (tracked.size === 0 && timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }
  }, [getBacklogItem, notify]);

  const pollRef = useRef(pollOnce);
  useEffect(() => {
    pollRef.current = pollOnce;
  }, [pollOnce]);

  const trackItem = useCallback((id: string, title: string) => {
    if (!mountedRef.current) return;
    if (abortRef.current) void hydrateItemIntoStore(id, abortRef.current.signal);
    trackedRef.current.set(id, { title, startedAt: Date.now(), notifiedComplete: false, notifiedGuidance: false });
    if (timerRef.current) return;
    timerRef.current = setInterval(() => {
      // A slow RPC must not let the next tick start an overlapping pass.
      if (inFlightRef.current) return;
      inFlightRef.current = true;
      void pollRef.current().finally(() => {
        inFlightRef.current = false;
      });
    }, POLL_INTERVAL_MS);
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    abortRef.current = new AbortController();
    return () => {
      mountedRef.current = false;
      abortRef.current?.abort();
      if (timerRef.current) clearInterval(timerRef.current);
      timerRef.current = null;
    };
  }, []);

  return { trackItem };
}
