"use client";

import { useCallback, useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import { useNotificationCommands } from "@/lib/contexts/notificationContexts";
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

  const notify = useCallback(
    (id: string, headline: string, title: string, type: "task_complete" | "question" | "error") => {
      addNotification({
        sessionId: "",
        sessionName: "",
        title: `${headline} — ${title}`,
        message: title,
        notificationType: type,
        onView: () => router.push(`/backlog?item=${id}`),
      });
    },
    [addNotification, router]
  );

  const poll = useCallback(async () => {
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

  const pollRef = useRef(poll);
  pollRef.current = poll;

  const trackItem = useCallback((id: string, title: string) => {
    trackedRef.current.set(id, { title, startedAt: Date.now(), notifiedComplete: false, notifiedGuidance: false });
    if (!timerRef.current) {
      timerRef.current = setInterval(() => void pollRef.current(), POLL_INTERVAL_MS);
    }
  }, []);

  useEffect(
    () => () => {
      if (timerRef.current) clearInterval(timerRef.current);
      timerRef.current = null;
    },
    []
  );

  return { trackItem };
}
