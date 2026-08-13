export type NotificationSyncMessage =
  | { type: "NOTIFICATION_DISMISSED"; notificationId: string }
  | { type: "NOTIFICATION_ACKNOWLEDGED"; sessionId: string };

const CHANNEL_NAME = "stapler-squad:notification-sync";

export function createNotificationSyncChannel(): {
  broadcast: (message: NotificationSyncMessage) => void;
  subscribe: (handler: (message: NotificationSyncMessage) => void) => () => void;
} {
  if (typeof window === "undefined") {
    return {
      broadcast: () => {},
      subscribe: () => () => {},
    };
  }

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
