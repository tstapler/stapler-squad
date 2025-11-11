"use client";

import { useCallback, useRef } from "react";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_connect";
import { LogUserInteractionRequest } from "@/gen/session/v1/session_pb";
import { UserInteractionEvent_InteractionType } from "@/gen/session/v1/events_pb";
import { getApiBaseUrl } from "@/lib/config";

interface UseAuditLogOptions {
  baseUrl?: string;
  enabled?: boolean; // Allow disabling audit logging globally
}

interface AuditLogEntry {
  sessionId?: string;
  interactionType: UserInteractionEvent_InteractionType;
  context?: string;
  notificationId?: string;
  metadata?: Record<string, string>;
}

export function useAuditLog(options: UseAuditLogOptions = {}) {
  const { baseUrl = getApiBaseUrl(), enabled = true } = options;

  const clientRef = useRef<any>(null);

  // Initialize gRPC client on first use
  const getClient = useCallback(() => {
    if (!clientRef.current) {
      const transport = createConnectTransport({
        baseUrl,
      });
      clientRef.current = createPromiseClient(SessionService, transport);
    }
    return clientRef.current;
  }, [baseUrl]);

  // Log user interaction to server
  const logInteraction = useCallback(
    async (entry: AuditLogEntry): Promise<boolean> => {
      if (!enabled) {
        return false; // Silently skip if disabled
      }

      try {
        const client = getClient();
        const request = new LogUserInteractionRequest({
          sessionId: entry.sessionId,
          interactionType: entry.interactionType,
          context: entry.context,
          notificationId: entry.notificationId,
          metadata: entry.metadata,
        });

        const response = await client.logUserInteraction(request);
        return response.success;
      } catch (error) {
        console.error("Failed to log user interaction:", error);
        return false;
      }
    },
    [enabled, getClient]
  );

  // Convenience methods for common notification interactions
  const logNotificationPanelOpened = useCallback(() => {
    return logInteraction({
      interactionType:
        UserInteractionEvent_InteractionType.NOTIFICATION_PANEL_OPENED,
      context: "User opened notification panel",
    });
  }, [logInteraction]);

  const logNotificationPanelClosed = useCallback(() => {
    return logInteraction({
      interactionType:
        UserInteractionEvent_InteractionType.NOTIFICATION_PANEL_CLOSED,
      context: "User closed notification panel",
    });
  }, [logInteraction]);

  const logNotificationViewed = useCallback(
    (notificationId: string, sessionId?: string) => {
      return logInteraction({
        sessionId,
        interactionType:
          UserInteractionEvent_InteractionType.NOTIFICATION_VIEWED,
        context: "User viewed notification",
        notificationId,
      });
    },
    [logInteraction]
  );

  const logNotificationDismissed = useCallback(
    (notificationId: string, sessionId?: string) => {
      return logInteraction({
        sessionId,
        interactionType:
          UserInteractionEvent_InteractionType.NOTIFICATION_DISMISSED,
        context: "User dismissed notification toast",
        notificationId,
      });
    },
    [logInteraction]
  );

  const logNotificationMarkedRead = useCallback(
    (notificationId: string, sessionId?: string) => {
      return logInteraction({
        sessionId,
        interactionType:
          UserInteractionEvent_InteractionType.NOTIFICATION_MARKED_READ,
        context: "User marked notification as read",
        notificationId,
      });
    },
    [logInteraction]
  );

  const logNotificationMarkedAllRead = useCallback(
    (count: number) => {
      return logInteraction({
        interactionType:
          UserInteractionEvent_InteractionType.NOTIFICATION_MARKED_ALL_READ,
        context: "User marked all notifications as read",
        metadata: {
          count: count.toString(),
        },
      });
    },
    [logInteraction]
  );

  const logNotificationRemoved = useCallback(
    (notificationId: string, sessionId?: string) => {
      return logInteraction({
        sessionId,
        interactionType:
          UserInteractionEvent_InteractionType.NOTIFICATION_REMOVED,
        context: "User removed notification from history",
        notificationId,
      });
    },
    [logInteraction]
  );

  const logNotificationHistoryCleared = useCallback(
    (count: number) => {
      return logInteraction({
        interactionType:
          UserInteractionEvent_InteractionType.NOTIFICATION_HISTORY_CLEARED,
        context: "User cleared notification history",
        metadata: {
          count: count.toString(),
        },
      });
    },
    [logInteraction]
  );

  const logNotificationSessionViewed = useCallback(
    (notificationId: string, sessionId: string) => {
      return logInteraction({
        sessionId,
        interactionType:
          UserInteractionEvent_InteractionType.NOTIFICATION_SESSION_VIEWED,
        context: "User clicked notification to view session",
        notificationId,
      });
    },
    [logInteraction]
  );

  return {
    // Generic method
    logInteraction,

    // Convenience methods for notifications
    logNotificationPanelOpened,
    logNotificationPanelClosed,
    logNotificationViewed,
    logNotificationDismissed,
    logNotificationMarkedRead,
    logNotificationMarkedAllRead,
    logNotificationRemoved,
    logNotificationHistoryCleared,
    logNotificationSessionViewed,
  };
}
