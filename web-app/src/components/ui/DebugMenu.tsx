"use client";

import { useState, useEffect } from "react";
import {
  getNotificationPreference,
  setNotificationPreference,
  requestNotificationPermission,
} from "@/lib/utils/notifications";
import styles from "./DebugMenu.module.css";

interface DebugMenuProps {
  isOpen: boolean;
  onClose: () => void;
}

export function DebugMenu({ isOpen, onClose }: DebugMenuProps) {
  const [terminalDebug, setTerminalDebug] = useState(false);
  const [notificationsEnabled, setNotificationsEnabled] = useState(true);
  const [notificationPermission, setNotificationPermission] = useState<
    NotificationPermission | "unsupported"
  >("default");

  // Load initial state from localStorage
  useEffect(() => {
    if (typeof window !== "undefined") {
      const terminalValue = localStorage.getItem("debug-terminal") === "true";
      setTerminalDebug(terminalValue);

      const notificationValue = getNotificationPreference();
      setNotificationsEnabled(notificationValue);

      // Check notification permission
      if ("Notification" in window) {
        setNotificationPermission(Notification.permission);
      } else {
        setNotificationPermission("unsupported");
      }
    }
  }, [isOpen]);

  const handleTerminalDebugToggle = () => {
    const newValue = !terminalDebug;
    setTerminalDebug(newValue);

    if (typeof window !== "undefined") {
      if (newValue) {
        localStorage.setItem("debug-terminal", "true");
        console.log("✓ Terminal debug logging enabled");
      } else {
        localStorage.removeItem("debug-terminal");
        console.log("✗ Terminal debug logging disabled");
      }
    }
  };

  const handleNotificationToggle = async () => {
    const newValue = !notificationsEnabled;

    // If enabling and we don't have permission, request it
    if (
      newValue &&
      notificationPermission !== "granted" &&
      notificationPermission !== "unsupported"
    ) {
      const granted = await requestNotificationPermission();
      if (granted) {
        setNotificationPermission("granted");
      } else {
        setNotificationPermission("denied");
        return; // Don't enable if permission denied
      }
    }

    setNotificationsEnabled(newValue);
    setNotificationPreference(newValue);

    if (newValue) {
      console.log("🔔 Notifications enabled");
    } else {
      console.log("🔕 Notifications disabled");
    }
  };

  if (!isOpen) return null;

  return (
    <div className={styles.overlay} onClick={onClose}>
      <div className={styles.menu} onClick={(e) => e.stopPropagation()}>
        <div className={styles.header}>
          <h2 className={styles.title}>🛠️ Debug Menu</h2>
          <button
            className={styles.closeButton}
            onClick={onClose}
            aria-label="Close debug menu"
          >
            ×
          </button>
        </div>

        <div className={styles.content}>
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Notifications</h3>

            <label className={styles.toggleRow}>
              <div className={styles.toggleLabel}>
                <span className={styles.toggleName}>Session Notifications</span>
                <span className={styles.toggleDescription}>
                  Play sound and show notification when sessions need attention
                  {notificationPermission === "denied" && (
                    <span className={styles.permissionWarning}>
                      {" "}
                      (Browser permission denied)
                    </span>
                  )}
                  {notificationPermission === "unsupported" && (
                    <span className={styles.permissionWarning}>
                      {" "}
                      (Not supported by browser)
                    </span>
                  )}
                </span>
              </div>
              <button
                className={`${styles.toggle} ${notificationsEnabled ? styles.toggleOn : ""}`}
                onClick={handleNotificationToggle}
                role="switch"
                aria-checked={notificationsEnabled}
              >
                <span className={styles.toggleSlider} />
              </button>
            </label>
          </div>

          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Logging</h3>

            <label className={styles.toggleRow}>
              <div className={styles.toggleLabel}>
                <span className={styles.toggleName}>Terminal Stream Logging</span>
                <span className={styles.toggleDescription}>
                  Show byte count for each terminal output chunk
                </span>
              </div>
              <button
                className={`${styles.toggle} ${terminalDebug ? styles.toggleOn : ""}`}
                onClick={handleTerminalDebugToggle}
                role="switch"
                aria-checked={terminalDebug}
              >
                <span className={styles.toggleSlider} />
              </button>
            </label>
          </div>

          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Console Commands</h3>
            <div className={styles.commandList}>
              <code className={styles.command}>
                localStorage.setItem("notifications-enabled", "false")
              </code>
              <span className={styles.commandDescription}>
                Disable notifications
              </span>
            </div>
            <div className={styles.commandList}>
              <code className={styles.command}>
                localStorage.setItem("debug-terminal", "true")
              </code>
              <span className={styles.commandDescription}>Enable terminal logging</span>
            </div>
          </div>
        </div>

        <div className={styles.footer}>
          <button className={styles.doneButton} onClick={onClose}>
            Done
          </button>
        </div>
      </div>
    </div>
  );
}
