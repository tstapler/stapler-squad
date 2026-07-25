// +feature: rules-hook-status
"use client";

import { useState, useEffect, useMemo, useCallback, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getApiBaseUrl } from "@/lib/config";
import { useAnalytics } from "@/lib/analytics";
import * as styles from "./HookStatusPanel.css";

export function HookStatusPanel() {
  const { track } = useAnalytics();
  const [installRules, setInstallRules] = useState(true);
  const [installNotifications, setInstallNotifications] = useState(true);
  const [rulesAvailable, setRulesAvailable] = useState(true);
  const [notificationsAvailable, setNotificationsAvailable] = useState(true);
  const [rulesInstalled, setRulesInstalled] = useState(false);
  const [notificationsInstalled, setNotificationsInstalled] = useState(false);
  const [hookBusy, setHookBusy] = useState(false);
  const [hookMessage, setHookMessage] = useState<string | null>(null);

  const client = useMemo(
    () => createClient(SessionService, createConnectTransport({ baseUrl: getApiBaseUrl() })),
    []
  );

  const mountedRef = useRef(true);
  const seededRef = useRef(false);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const refreshHookStatus = useCallback(async () => {
    try {
      const res = await client.getHookStatus({});
      if (!mountedRef.current) return;
      setRulesInstalled(res.rulesInstalled);
      setNotificationsInstalled(res.notificationsInstalled);
      setRulesAvailable(res.rulesAvailable);
      setNotificationsAvailable(res.notificationsAvailable);
      if (!seededRef.current) {
        seededRef.current = true;
        setInstallRules(res.rulesAvailable && !res.rulesInstalled);
        setInstallNotifications(res.notificationsAvailable && !res.notificationsInstalled);
      }
    } catch {
      if (mountedRef.current) setHookMessage("Could not check hook status.");
    }
  }, [client]);

  useEffect(() => {
    void refreshHookStatus();
  }, [refreshHookStatus]);

  const handleInstallHooks = async () => {
    track({
      name: "hook_status_install",
      category: "user_action",
      component: "HookStatusPanel",
      labels: { installRules: String(installRules), installNotifications: String(installNotifications) },
    });
    setHookBusy(true);
    setHookMessage(null);
    try {
      const res = await client.installHooks({ installRules, installNotifications });
      if (!mountedRef.current) return;
      if (res.status) {
        setRulesInstalled(res.status.rulesInstalled);
        setNotificationsInstalled(res.status.notificationsInstalled);
      }
      setHookMessage(res.messages.join(" ") || "Hooks updated.");
    } catch {
      if (mountedRef.current) {
        setHookMessage("Failed to install hooks.");
      }
    } finally {
      if (mountedRef.current) setHookBusy(false);
    }
  };

  return (
    <section className={styles.panel} data-testid="hook-status-panel">
      <div className={styles.header}>
        <h2 className={styles.title}>Claude Code Hooks</h2>
        <p className={styles.subtitle}>
          Enforce these approval rules and get notifications outside the app too.
        </p>
      </div>

      <label className={styles.checkboxRow}>
        <input
          type="checkbox"
          checked={installRules}
          disabled={!rulesAvailable || rulesInstalled || hookBusy}
          onChange={(e) => setInstallRules(e.target.checked)}
        />
        <span className={styles.checkboxLabel}>
          Enable rule enforcement
          {rulesInstalled
            ? " (already installed)"
            : !rulesAvailable
              ? " (ssq-hooks not installed — run `make install`)"
              : " — gate tool calls through your rules"}
        </span>
      </label>

      <label className={styles.checkboxRow}>
        <input
          type="checkbox"
          checked={installNotifications}
          disabled={!notificationsAvailable || notificationsInstalled || hookBusy}
          onChange={(e) => setInstallNotifications(e.target.checked)}
        />
        <span className={styles.checkboxLabel}>
          Enable notifications
          {notificationsInstalled
            ? " (already installed)"
            : !notificationsAvailable
              ? " (ssq-hook-handler not found)"
              : " — chimes and alerts when Claude needs you"}
        </span>
      </label>

      {hookMessage && <p className={styles.message}>{hookMessage}</p>}

      {(installRules || installNotifications) &&
        (!rulesInstalled || !notificationsInstalled) && (
          <div className={styles.footer}>
            <button
              className={styles.installButton}
              onClick={handleInstallHooks}
              disabled={hookBusy}
              data-testid="hook-status-install-button"
            >
              {hookBusy ? "Installing…" : "Install"}
            </button>
          </div>
        )}
    </section>
  );
}
