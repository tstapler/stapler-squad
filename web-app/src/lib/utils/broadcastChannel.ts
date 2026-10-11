export type BulkDismissKind = "dismissed" | "moved";

export type NotificationSyncMessage =
  | { type: "NOTIFICATION_DISMISSED"; notificationId: string }
  | { type: "NOTIFICATION_ACKNOWLEDGED"; sessionId: string }
  /**
   * An id-set bulk action in another tab. "moved" only takes toasts off the deck
   * (history untouched); "dismissed" also drops the rows the other tab cleared.
   * Applying it twice, or with ids the receiver never saw, changes nothing.
   */
  | { type: "NOTIFICATIONS_BULK_DISMISSED"; kind: BulkDismissKind; ids: string[] };

const CHANNEL_NAME = "stapler-squad:notification-sync";
const STORAGE_KEY = "stapler-squad:notification-sync";

interface SyncChannel {
  broadcast: (message: NotificationSyncMessage) => void;
  subscribe: (handler: (message: NotificationSyncMessage) => void) => () => void;
}

/**
 * Fallback for browsers without BroadcastChannel: a localStorage write, which the
 * `storage` event delivers to every other tab (never the writer).
 */
function createStorageChannel(): SyncChannel {
  return {
    broadcast: (message) => {
      try {
        // A nonce makes a repeat of an identical message a change, so it fires again.
        window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ nonce: Math.random(), message }));
        window.localStorage.removeItem(STORAGE_KEY);
      } catch {
        // Storage blocked: cross-tab sync is best effort; history refresh still converges.
      }
    },
    subscribe: (handler) => {
      const listener = (event: StorageEvent) => {
        if (event.key !== STORAGE_KEY || !event.newValue) return;
        try {
          handler((JSON.parse(event.newValue) as { message: NotificationSyncMessage }).message);
        } catch {
          // Ignore a malformed payload.
        }
      };
      window.addEventListener("storage", listener);
      return () => window.removeEventListener("storage", listener);
    },
  };
}

export function createNotificationSyncChannel(): SyncChannel {
  if (typeof window === "undefined") {
    return {
      broadcast: () => {},
      subscribe: () => () => {},
    };
  }
  if (typeof BroadcastChannel === "undefined") return createStorageChannel();

  const channel = new BroadcastChannel(CHANNEL_NAME);

  return {
    broadcast: (message) => channel.postMessage(message),
    subscribe: (handler) => {
      const listener = (event: MessageEvent<NotificationSyncMessage>) => {
        handler(event.data);
      };
      channel.addEventListener("message", listener);
      return () => {
        channel.removeEventListener("message", listener);
        channel.close();
      };
    },
  };
}
