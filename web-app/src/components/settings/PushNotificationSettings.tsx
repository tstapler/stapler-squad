"use client";

import { usePushNotifications } from "@/lib/hooks/usePushNotifications";
import { getApiBaseUrl } from "@/lib/config";
import * as styles from "./PushNotificationSettings.css";

export function PushNotificationSettings() {
  const { isSupported, subscription, isLoading, error, permission, subscribe, unsubscribe } =
    usePushNotifications();

  const isSubscribed = subscription !== null;
  const baseUrl = getApiBaseUrl();

  const handleToggle = async () => {
    if (isSubscribed) {
      await unsubscribe(baseUrl);
    } else {
      await subscribe(baseUrl);
    }
  };

  if (isLoading) {
    return null;
  }

  return (
    <div className={styles.container}>
      <h2 className={styles.title}>Push Notifications</h2>

      {!isSupported && (
        <>
          <span className={`${styles.statusBadge} ${styles.statusUnsupported}`}>
            Not supported
          </span>
          <p className={styles.description}>
            Push notifications are not supported in this browser.
          </p>
        </>
      )}

      {isSupported && permission === "denied" && (
        <>
          <span className={`${styles.statusBadge} ${styles.statusBlocked}`}>Blocked</span>
          <p className={styles.instructions}>
            Notifications are blocked in your browser settings. To enable them, open your
            browser&apos;s site settings for this page and set Notifications to{" "}
            <strong>Allow</strong>.
          </p>
        </>
      )}

      {isSupported && permission !== "denied" && (
        <div className={styles.toggleRow}>
          <input
            id="push-notifications-toggle"
            type="checkbox"
            checked={isSubscribed}
            onChange={handleToggle}
          />
          <label htmlFor="push-notifications-toggle" className={styles.toggleLabel}>
            {isSubscribed ? "Push notifications enabled" : "Enable push notifications"}
          </label>
        </div>
      )}

      {error && <p className={styles.errorText}>{error}</p>}
    </div>
  );
}
