"use client";

import { useState, useEffect, useRef } from "react";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import {
  getNotificationPreference,
  setNotificationPreference,
  requestNotificationPermission,
} from "@/lib/utils/notifications";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getApiBaseUrl } from "@/lib/config";
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

  const [snapshotNote, setSnapshotNote] = useState("");
  const [isCapturing, setIsCapturing] = useState(false);
  const [snapshotResult, setSnapshotResult] = useState<{
    filePath: string;
    summary: string;
  } | null>(null);
  const [snapshotError, setSnapshotError] = useState<string | null>(null);

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  useFocusTrap(menuRef, isOpen);

  useEffect(() => {
    const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
    clientRef.current = createClient(SessionService, transport);
  }, []);

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

  const handleCreateSnapshot = async () => {
    if (!clientRef.current) return;
    setIsCapturing(true);
    setSnapshotError(null);
    setSnapshotResult(null);

    try {
      const response = await clientRef.current.createDebugSnapshot({
        note: snapshotNote || undefined,
      });
      setSnapshotResult({
        filePath: response.filePath,
        summary: response.summary,
      });
      setSnapshotNote("");
    } catch (err) {
      setSnapshotError(
        err instanceof Error ? err.message : "Failed to capture snapshot"
      );
    } finally {
      setIsCapturing(false);
    }
  };

  if (!isOpen) return null;

  return (
    <div className={styles.overlay} onClick={onClose}>
      <div
        className={styles.menu}
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="debug-menu-title"
        ref={menuRef}
      >
        <div className={styles.header}>
          <h2 className={styles.title} id="debug-menu-title">🛠️ Debug Menu</h2>
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
            <h3 className={styles.sectionTitle}>Debug Pages</h3>
            <a
              href="/debug/escape-codes"
              className={styles.debugLink}
              onClick={onClose}
            >
              <span className={styles.debugLinkIcon}>📊</span>
              <div className={styles.debugLinkContent}>
                <span className={styles.debugLinkName}>Escape Code Analytics</span>
                <span className={styles.debugLinkDescription}>
                  Track terminal escape sequences for debugging rendering issues
                </span>
              </div>
            </a>
          </div>

          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Diagnostics</h3>

            <div className={styles.noteInputRow}>
              <input
                type="text"
                className={styles.noteInput}
                placeholder="Describe the issue (optional)..."
                value={snapshotNote}
                onChange={(e) => setSnapshotNote(e.target.value)}
                maxLength={500}
                aria-label="Snapshot note"
              />
            </div>

            <button
              className={styles.snapshotButton}
              onClick={handleCreateSnapshot}
              disabled={isCapturing}
              aria-label="Capture debug snapshot"
            >
              {isCapturing ? (
                <>
                  <span className={styles.spinner} /> Capturing...
                </>
              ) : (
                <>📸 Capture Debug Snapshot</>
              )}
            </button>

            {snapshotResult && (
              <div className={styles.snapshotResult}>
                <div className={styles.snapshotResultText}>
                  ✓ {snapshotResult.summary}
                </div>
                <code className={styles.snapshotFilePath}>
                  {snapshotResult.filePath}
                </code>
              </div>
            )}

            {snapshotError && (
              <div className={styles.snapshotError}>{snapshotError}</div>
            )}
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
