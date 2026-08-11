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
import {
  overlay,
  menu,
  header,
  menuTitle,
  closeButton,
  contentArea,
  section,
  sectionTitle,
  toggleRow,
  toggleLabel,
  toggleName,
  toggleDescription,
  permissionWarning,
  toggle,
  toggleOn,
  toggleSlider,
  commandList,
  command,
  commandDescription,
  debugLink,
  debugLinkIcon,
  debugLinkContent,
  debugLinkName,
  debugLinkDescription,
  footer,
  doneButton,
  noteInputRow,
  noteInput,
  snapshotButton,
  spinner,
  snapshotResult as snapshotResultClass,
  snapshotResultText,
  snapshotFilePath,
  snapshotError as snapshotErrorClass,
} from "./DebugMenu.css";

interface DebugMenuProps {
  isOpen: boolean;
  onClose: () => void;
}

export function DebugMenu({ isOpen, onClose }: DebugMenuProps) {
  const [terminalDebug, setTerminalDebug] = useState(false);
  const [serverDebugLog, setServerDebugLog] = useState(false);
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

  // Load initial state from localStorage and server
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

    // Fetch current server log level
    if (isOpen) {
      fetch("/api/debug/log-level")
        .then((r) => r.json())
        .then((data: { level: string }) => {
          setServerDebugLog(data.level === "DEBUG");
        })
        .catch(() => {/* ignore — server may not be running with debug endpoint */});
    }
  }, [isOpen]);

  const handleServerDebugLogToggle = async () => {
    const newLevel = serverDebugLog ? "INFO" : "DEBUG";
    try {
      const response = await fetch("/api/debug/log-level", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ level: newLevel }),
      });
      if (response.ok) {
        setServerDebugLog(!serverDebugLog);
        console.log(`Server log level set to ${newLevel}`);
      }
    } catch (err) {
      console.error("Failed to set server log level:", err);
    }
  };

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
    <div className={overlay} onClick={onClose}>
      <div
        className={menu}
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="debug-menu-title"
        ref={menuRef}
      >
        <div className={header}>
          <h2 className={menuTitle} id="debug-menu-title">🛠️ Debug Menu</h2>
          <button
            className={closeButton}
            onClick={onClose}
            aria-label="Close debug menu"
          >
            ×
          </button>
        </div>

        <div className={contentArea}>
          <div className={section}>
            <h3 className={sectionTitle}>Notifications</h3>

            <label className={toggleRow}>
              <div className={toggleLabel}>
                <span className={toggleName}>Session Notifications</span>
                <span className={toggleDescription}>
                  Play sound and show notification when sessions need attention
                  {notificationPermission === "denied" && (
                    <span className={permissionWarning}>
                      {" "}
                      (Browser permission denied)
                    </span>
                  )}
                  {notificationPermission === "unsupported" && (
                    <span className={permissionWarning}>
                      {" "}
                      (Not supported by browser)
                    </span>
                  )}
                </span>
              </div>
              <button
                className={`${toggle} ${notificationsEnabled ? toggleOn : ""}`}
                onClick={handleNotificationToggle}
                role="switch"
                aria-checked={notificationsEnabled}
              >
                <span className={toggleSlider} />
              </button>
            </label>
          </div>

          <div className={section}>
            <h3 className={sectionTitle}>Logging</h3>

            <label className={toggleRow}>
              <div className={toggleLabel}>
                <span className={toggleName}>Server Debug Logging</span>
                <span className={toggleDescription}>
                  Enable verbose DEBUG logs on the server (persists until restart)
                </span>
              </div>
              <button
                className={`${toggle} ${serverDebugLog ? toggleOn : ""}`}
                onClick={handleServerDebugLogToggle}
                role="switch"
                aria-checked={serverDebugLog}
              >
                <span className={toggleSlider} />
              </button>
            </label>

            <label className={toggleRow}>
              <div className={toggleLabel}>
                <span className={toggleName}>Terminal Stream Logging</span>
                <span className={toggleDescription}>
                  Show byte count for each terminal output chunk
                </span>
              </div>
              <button
                className={`${toggle} ${terminalDebug ? toggleOn : ""}`}
                onClick={handleTerminalDebugToggle}
                role="switch"
                aria-checked={terminalDebug}
              >
                <span className={toggleSlider} />
              </button>
            </label>
          </div>

          <div className={section}>
            <h3 className={sectionTitle}>Debug Pages</h3>
            <a
              href="/debug/escape-codes"
              className={debugLink}
              onClick={onClose}
            >
              <span className={debugLinkIcon}>📊</span>
              <div className={debugLinkContent}>
                <span className={debugLinkName}>Escape Code Analytics</span>
                <span className={debugLinkDescription}>
                  Track terminal escape sequences for debugging rendering issues
                </span>
              </div>
            </a>
          </div>

          <div className={section}>
            <h3 className={sectionTitle}>Diagnostics</h3>

            <div className={noteInputRow}>
              <input
                type="text"
                className={noteInput}
                placeholder="Describe the issue (optional)..."
                value={snapshotNote}
                onChange={(e) => setSnapshotNote(e.target.value)}
                maxLength={500}
                aria-label="Snapshot note"
              />
            </div>

            <button
              className={snapshotButton}
              onClick={handleCreateSnapshot}
              disabled={isCapturing}
              aria-label="Capture debug snapshot"
            >
              {isCapturing ? (
                <>
                  <span className={spinner} /> Capturing...
                </>
              ) : (
                <>📸 Capture Debug Snapshot</>
              )}
            </button>

            {snapshotResult && (
              <div className={snapshotResultClass}>
                <div className={snapshotResultText}>
                  ✓ {snapshotResult.summary}
                </div>
                <code className={snapshotFilePath}>
                  {snapshotResult.filePath}
                </code>
              </div>
            )}

            {snapshotError && (
              <div className={snapshotErrorClass}>{snapshotError}</div>
            )}
          </div>

          <div className={section}>
            <h3 className={sectionTitle}>Console Commands</h3>
            <div className={commandList}>
              <code className={command}>
                {`localStorage.setItem("notifications-enabled", "false")`}
              </code>
              <span className={commandDescription}>
                Disable notifications
              </span>
            </div>
            <div className={commandList}>
              <code className={command}>
                {`localStorage.setItem("debug-terminal", "true")`}
              </code>
              <span className={commandDescription}>Enable terminal logging</span>
            </div>
          </div>
        </div>

        <div className={footer}>
          <button className={doneButton} onClick={onClose}>
            Done
          </button>
        </div>
      </div>
    </div>
  );
}
