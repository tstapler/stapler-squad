"use client";

import { useEffect, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import { SessionService, GetNotificationHistoryRequestSchema } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import type { NotificationHistoryItem } from "@/lib/types/notification";

export interface NotificationRecordSummary {
  title: string;
  message: string;
  timestampMs: number;
}

/**
 * Captured title and message for a deep link's `notification=<id>` (Story 5.3, C5): the
 * hydrated history slice first, then one `GetNotificationHistory` call scoped to the
 * session (the RPC has no id filter). `undefined` while resolving, `null` when the
 * record is gone (pruned, cleared, or the link carried no id).
 */
export function useNotificationRecord(
  notificationHistory: readonly NotificationHistoryItem[] | undefined,
  notificationId: string | null,
  sessionId: string | null,
  enabled: boolean,
): NotificationRecordSummary | null | undefined {
  const [fetchedFor, setFetchedFor] = useState<{
    forId: string;
    value: NotificationRecordSummary | null;
  } | null>(null);

  const inSlice = notificationId
    ? (notificationHistory ?? []).find((n) => n.id === notificationId)
    : undefined;

  useEffect(() => {
    if (!enabled || !notificationId || inSlice) return;
    const controller = new AbortController();
    const client = createClient(SessionService, getConnectTransport());
    client
      .getNotificationHistory(
        create(GetNotificationHistoryRequestSchema, { sessionId: sessionId ?? undefined, limit: 100 }),
        { signal: controller.signal },
      )
      .then((res) => {
        if (controller.signal.aborted) return;
        const hit = res.notifications.find((n) => n.id === notificationId);
        setFetchedFor({
          forId: notificationId,
          value: hit
            ? {
                title: hit.title,
                message: hit.message,
                timestampMs: hit.createdAt ? Number(hit.createdAt.seconds) * 1000 : 0,
              }
            : null,
        });
      })
      .catch(() => {
        if (!controller.signal.aborted) setFetchedFor({ forId: notificationId, value: null });
      });
    return () => controller.abort();
  }, [enabled, notificationId, sessionId, inSlice]);

  if (!notificationId) return null;
  if (inSlice) {
    return {
      title: inSlice.title ?? inSlice.sessionName,
      message: inSlice.message,
      timestampMs: inSlice.timestamp,
    };
  }
  // A result fetched for another id must not leak onto this card while its own fetch is in flight.
  return fetchedFor?.forId === notificationId ? fetchedFor.value : undefined;
}
