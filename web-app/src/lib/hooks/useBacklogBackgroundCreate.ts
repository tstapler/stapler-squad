"use client";

import { useCallback } from "react";
import { useRouter } from "next/navigation";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import { useNotificationCommands } from "@/lib/contexts/notificationContexts";

/**
 * Creates a backlog item from free text without blocking the caller, then
 * reports the outcome as a notification (deep-linking to the new item).
 * The returned function never rejects.
 */
export function useBacklogBackgroundCreate() {
  const { createBacklogItemFromChat } = useBacklogService();
  const { addNotification } = useNotificationCommands();
  const router = useRouter();

  return useCallback(
    async (text: string) => {
      const result = await createBacklogItemFromChat(text);
      if (result) {
        const { item } = result;
        addNotification({
          sessionId: "",
          sessionName: "Backlog",
          title: "Backlog item created",
          message: item.title,
          notificationType: "task_complete",
          onView: () => router.push(`/backlog?item=${item.id}`),
        });
      } else {
        addNotification({
          sessionId: "",
          sessionName: "Backlog",
          title: "Backlog item not created",
          message: "Creating the backlog item failed. Your text was not saved — try again.",
          notificationType: "error",
        });
      }
    },
    [createBacklogItemFromChat, addNotification, router]
  );
}
