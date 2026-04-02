"use client";

import { useEffect, useState, useCallback } from "react";

declare global {
  interface ServiceWorkerRegistration {
    showNotification: (title: string, options?: NotificationOptions) => Promise<void>;
  }
}

interface PushSubscriptionJSON {
  endpoint: string;
  keys: {
    p256dh: string;
    auth: string;
  };
}

interface PushSubscriptionData {
  endpoint: string;
  keys: {
    p256dh: string;
    auth: string;
  };
}

interface UsePushNotificationsOptions {
  onNotification?: (notification: { title: string; body: string; data?: unknown }) => void;
}

export function usePushNotifications({ onNotification }: UsePushNotificationsOptions = {}) {
  const [isSupported, setIsSupported] = useState(false);
  const [subscription, setSubscription] = useState<PushSubscription | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const checkSupport = () => {
      const supported = "Notification" in window && "serviceWorker" in navigator && "PushManager" in window;
      setIsSupported(supported);
      setIsLoading(false);
    };
    checkSupport();
  }, []);

  const requestPermission = useCallback(async (): Promise<NotificationPermission> => {
    if (!isSupported) {
      setError("Push notifications not supported");
      return "denied";
    }

    try {
      const permission = await Notification.requestPermission();
      return permission;
    } catch (err) {
      setError("Failed to request notification permission");
      return "denied";
    }
  }, [isSupported]);

  const subscribe = useCallback(async (baseUrl: string): Promise<PushSubscription | null> => {
    if (!isSupported) {
      setError("Push notifications not supported");
      return null;
    }

    try {
      const registration = await navigator.serviceWorker.register("/push-sw.js");
      
      await navigator.serviceWorker.ready;

      const existingSubscription = await registration.pushManager.getSubscription();
      if (existingSubscription) {
        setSubscription(existingSubscription);
        return existingSubscription;
      }

const vapidPublicKey = await fetch(`${baseUrl}/api/push/vapid-key`).then(r => r.text());

const newSubscription = await registration.pushManager.subscribe({
  userVisibleOnly: true,
  // @ts-ignore - PushManager.subscribe accepts Uint8Array but types are strict
  applicationServerKey: urlBase64ToUint8Array(vapidPublicKey) as unknown as ArrayBufferView,
});

      await fetch(`${baseUrl}/api/push/subscribe`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(newSubscription.toJSON()),
      });

      setSubscription(newSubscription);
      return newSubscription;
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to subscribe";
      setError(message);
      return null;
    }
  }, [isSupported]);

  const unsubscribe = useCallback(async (baseUrl: string): Promise<boolean> => {
    if (!subscription) return true;

    try {
      await subscription.unsubscribe();
      
      await fetch(`${baseUrl}/api/push/unsubscribe`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ endpoint: subscription.endpoint }),
      });

      setSubscription(null);
      return true;
    } catch (err) {
      setError("Failed to unsubscribe");
      return false;
    }
  }, [subscription]);

  useEffect(() => {
    if (!isSupported || typeof window === "undefined") return;

    const setupServiceWorker = async () => {
      try {
        const registration = await navigator.serviceWorker.register("/push-sw.js");
        
        registration.addEventListener("updatefound", () => {
          const newWorker = registration.installing;
          if (newWorker) {
            newWorker.addEventListener("statechange", () => {
              if (newWorker.state === "installed" && navigator.serviceWorker.controller) {
                newWorker.postMessage({ type: "SKIP_WAITING" });
              }
            });
          }
        });

        const existingSubscription = await registration.pushManager.getSubscription();
        if (existingSubscription) {
          setSubscription(existingSubscription);
        }
      } catch (err) {
        console.error("Service worker registration failed:", err);
      }
    };

    setupServiceWorker();
  }, [isSupported]);

  return {
    isSupported,
    subscription,
    isLoading,
    error,
    permission: typeof window !== "undefined" ? Notification.permission : "default",
    requestPermission,
    subscribe,
    unsubscribe,
  };
}

function urlBase64ToUint8Array(base64String: string): Uint8Array {
  const padding = "=".repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, "+").replace(/_/g, "/");
  
  const rawData = window.atob(base64);
  const outputArray = new Uint8Array(rawData.length);
  
  for (let i = 0; i < rawData.length; ++i) {
    outputArray[i] = rawData.charCodeAt(i);
  }
  
  return outputArray;
}
