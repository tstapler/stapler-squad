"use client";

import { useEffect, useRef } from "react";
import { registry, type Shortcut } from "./shortcutRegistry";
import { useNotificationCommands, useNotificationState } from "@/lib/contexts/notificationContexts";
import { MAX_REPLY_OPTIONS, replyAvailability } from "@/lib/reply/replyQuestion";

const GROUP = "Notifications";
const REPLY_CLICK_ATTEMPTS = 30;

/** Clicks the Reply card's own numbered button so the audited send path is the one that runs. */
function clickReplyOption(n: number, attempt = 0): void {
  const btn = document.querySelector<HTMLElement>(`[data-testid="reply-option-${n}"]`);
  if (btn) {
    btn.click();
    return;
  }
  if (attempt < REPLY_CLICK_ATTEMPTS) requestAnimationFrame(() => clickReplyOption(n, attempt + 1));
}

/**
 * Registers the notification tray's actions under the command bar's `n` branch.
 * Reply options 1..N depend on which question is pending, so they are provided
 * lazily (registry.registerLazy) when the bar navigates into `n`, not at mount.
 */
export function useNotificationShortcuts(): void {
  const state = useNotificationState();
  const commands = useNotificationCommands();
  const stateRef = useRef(state);
  stateRef.current = state;
  const commandsRef = useRef(commands);
  commandsRef.current = commands;

  useEffect(() => {
    const defs: Array<[string, string, string, () => void]> = [
      ["notifications.seq.toggle-tray", "Toggle notification tray", "t", () => commandsRef.current.togglePanel()],
      [
        "notifications.seq.mark-all-read",
        "Mark all notifications read",
        "m",
        () => {
          const ids = stateRef.current.notificationHistory.filter((n) => !n.isRead).map((n) => n.id);
          if (ids.length > 0) void commandsRef.current.markAsRead(ids);
        },
      ],
      [
        "notifications.seq.quiet-mode",
        "Toggle Quiet mode",
        "q",
        () => commandsRef.current.setQuietMode(!stateRef.current.quietMode),
      ],
      ["notifications.seq.move-all", "Move all toasts to tray", "a", () => commandsRef.current.moveAllToTray()],
    ];
    const cleanups = defs.map(([id, label, key, action]) =>
      registry.register(id, { label, context: "global", sequence: ["n", key], group: GROUP, action })
    );

    cleanups.push(
      registry.registerLazy("notifications.reply", ["n"], () => {
        // Newest unread replyable question across all sessions.
        const candidates = stateRef.current.notificationHistory
          .filter((n) => !n.isRead && n.notificationType === "question")
          .sort((a, b) => b.timestamp - a.timestamp);
        for (const n of candidates) {
          const availability = replyAvailability(n);
          if (availability?.kind !== "replyable") continue;
          return availability.options.slice(0, MAX_REPLY_OPTIONS).map((label, i): { id: string; shortcut: Shortcut } => ({
            id: `notifications.seq.reply.${i + 1}`,
            shortcut: {
              label: `Reply ${i + 1}: ${label}`,
              context: "global",
              sequence: ["n", "r", String(i + 1)],
              group: GROUP,
              action: () => {
                if (!stateRef.current.isPanelOpen) commandsRef.current.togglePanel();
                clickReplyOption(i + 1);
              },
            },
          }));
        }
        return [];
      })
    );

    return () => cleanups.forEach((c) => c());
  }, []);
}
