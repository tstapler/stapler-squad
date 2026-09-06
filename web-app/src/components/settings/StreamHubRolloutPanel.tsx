"use client";
// +feature: stream-hub-rollout

import { useState, useEffect, useCallback, useMemo } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { Timestamp } from "@bufbuild/protobuf/wkt";
import { getConnectTransport } from "@/lib/api/transport";
import { useAnalytics } from "@/lib/analytics";
import * as styles from "./StreamHubRolloutPanel.css";

interface SessionOverride {
  sessionName: string;
  forceHub: boolean;
}

/**
 * Controls for the terminal-multi-connection-streaming hub. Everything here
 * takes effect immediately, config.json-backed, for session connections
 * resolved from that point on — an already-connected session's resolution
 * is cached for its lifetime and can't be moved: per-session canary
 * overrides and a live global override of the "stream_hub" feature flag
 * (on by default), no process restart required.
 */
export function StreamHubRolloutPanel() {
  const { track } = useAnalytics();
  const client = useMemo(() => createClient(SessionService, getConnectTransport()), []);

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [globalOverride, setGlobalOverride] = useState<boolean | undefined>(undefined);
  const [rehearsalCompletedAt, setRehearsalCompletedAt] = useState<Date | null>(null);
  const [overrides, setOverrides] = useState<SessionOverride[]>([]);
  const [sessionNames, setSessionNames] = useState<string[]>([]);
  const [newOverrideName, setNewOverrideName] = useState("");
  const [busy, setBusy] = useState(false);

  const applyStatus = useCallback((status: {
    rollbackRehearsalCompletedAt?: Timestamp;
    sessionOverrides: SessionOverride[];
    globalOverride?: boolean;
  }) => {
    const ts = status.rollbackRehearsalCompletedAt;
    setRehearsalCompletedAt(ts ? new Date(Number(ts.seconds) * 1000) : null);
    setOverrides(status.sessionOverrides);
    setGlobalOverride(status.globalOverride);
  }, []);

  const load = useCallback(async () => {
    try {
      const status = await client.getStreamHubRolloutStatus({});
      applyStatus(status);
    } catch {
      setError("Failed to load stream-hub rollout status");
    } finally {
      setLoading(false);
    }
  }, [client, applyStatus]);

  useEffect(() => {
    void load();
    client.listSessions({}).then((res) => {
      setSessionNames(res.sessions.map((s) => s.title).filter(Boolean));
    }).catch(() => {
      // Non-fatal — the override input still works as free text.
    });
  }, [load, client]);

  const completeRehearsal = useCallback(async () => {
    track({ name: "stream_hub_rehearsal_completed", category: "user_action", component: "StreamHubRolloutPanel" });
    setBusy(true);
    setError(null);
    try {
      const status = await client.completeStreamHubRollbackRehearsal({});
      applyStatus(status);
    } catch {
      setError("Failed to record rollback rehearsal");
    } finally {
      setBusy(false);
    }
  }, [client, applyStatus, track]);

  const addOverride = useCallback(async () => {
    const sessionName = newOverrideName.trim();
    if (!sessionName) return;
    track({ name: "stream_hub_override_added", category: "user_action", component: "StreamHubRolloutPanel" });
    setBusy(true);
    setError(null);
    try {
      const status = await client.setStreamHubSessionOverride({ sessionName, forceHub: true });
      applyStatus(status);
      setNewOverrideName("");
    } catch {
      setError("Failed to set session override");
    } finally {
      setBusy(false);
    }
  }, [client, applyStatus, newOverrideName, track]);

  const setGlobalOverrideValue = useCallback(async (forceHub: boolean | undefined) => {
    track({ name: "stream_hub_global_override_changed", category: "user_action", component: "StreamHubRolloutPanel", labels: { forceHub: forceHub === undefined ? "clear" : String(forceHub) } });
    setBusy(true);
    setError(null);
    try {
      const status = await client.setStreamHubGlobalOverride({ forceHub });
      applyStatus(status);
    } catch {
      setError(
        forceHub === true
          ? "Failed to enable the global override — the rollback rehearsal may not be completed yet"
          : "Failed to update the global override",
      );
    } finally {
      setBusy(false);
    }
  }, [client, applyStatus, track]);

  const removeOverride = useCallback(async (sessionName: string) => {
    track({ name: "stream_hub_override_removed", category: "user_action", component: "StreamHubRolloutPanel" });
    setBusy(true);
    setError(null);
    try {
      const status = await client.setStreamHubSessionOverride({ sessionName });
      applyStatus(status);
    } catch {
      setError("Failed to clear session override");
    } finally {
      setBusy(false);
    }
  }, [client, applyStatus, track]);

  if (loading) {
    return <p className={styles.description}>Loading…</p>;
  }

  return (
    <section className={styles.panel} data-testid="stream-hub-rollout-panel">
      <h2 className={styles.heading}>Stream Hub Rollout</h2>
      <p className={styles.description}>
        Staged rollout for the terminal-multi-connection-streaming hub. The
        &quot;stream_hub&quot; feature flag defaults to on; the override below takes effect
        immediately for any session connection resolved from this point on (an
        already-connected session can&apos;t be moved) — no process restart required.
      </p>

      {error && <p className={styles.errorMessage} role="alert">{error}</p>}

      <div className={styles.statusRow}>
        <span className={styles.statusLabel}>Global override</span>
        <span className={`${styles.badge} ${globalOverride === true ? styles.badgeEnabled : styles.badgeDisabled}`}>
          {globalOverride === undefined ? "Not set (default: on)" : globalOverride ? "Forced on" : "Forced off"}
        </span>
      </div>
      <div className={styles.addRow}>
        <button
          className={styles.actionButton}
          disabled={busy || globalOverride === true}
          onClick={() => setGlobalOverrideValue(true)}
          data-testid="stream-hub-global-override-on"
          aria-label="Force stream hub on for all sessions"
        >
          Force on for everything
        </button>
        <button
          className={styles.actionButton}
          disabled={busy || globalOverride === false}
          onClick={() => setGlobalOverrideValue(false)}
          data-testid="stream-hub-global-override-off"
          aria-label="Force stream hub off for all sessions"
        >
          Force off for everything
        </button>
        <button
          className={styles.removeButton}
          disabled={busy || globalOverride === undefined}
          onClick={() => setGlobalOverrideValue(undefined)}
          data-testid="stream-hub-global-override-clear"
          aria-label="Clear global override, revert to the default"
        >
          Clear override
        </button>
      </div>

      <div className={styles.statusRow}>
        <span className={styles.statusLabel}>Rollback rehearsal</span>
        {rehearsalCompletedAt ? (
          <span className={`${styles.badge} ${styles.badgeEnabled}`}>
            Completed {rehearsalCompletedAt.toLocaleString()}
          </span>
        ) : (
          <button
            className={styles.actionButton}
            data-testid="stream-hub-complete-rehearsal"
            disabled={busy}
            onClick={completeRehearsal}
          >
            Mark rehearsal complete
          </button>
        )}
      </div>
      <p className={styles.hint}>
        Only mark this after manually verifying: flip a per-session override on, use it
        briefly, remove it, confirm a clean reconnect under the legacy path.
      </p>

      <h3 className={styles.subheading}>Per-session canary overrides</h3>
      {overrides.length === 0 ? (
        <p className={styles.description}>No sessions are currently overridden.</p>
      ) : (
        <ul className={styles.overrideList}>
          {overrides.map((o) => (
            <li key={o.sessionName} className={styles.overrideRow} data-testid="stream-hub-override-row">
              <span className={styles.statusLabel}>{o.sessionName}</span>
              <span className={`${styles.badge} ${o.forceHub ? styles.badgeEnabled : styles.badgeDisabled}`}>
                {o.forceHub ? "Forced on" : "Forced off"}
              </span>
              <button
                className={styles.removeButton}
                data-testid="stream-hub-remove-override"
                disabled={busy}
                onClick={() => removeOverride(o.sessionName)}
              >
                Remove
              </button>
            </li>
          ))}
        </ul>
      )}

      <div className={styles.addRow}>
        <input
          className={styles.input}
          list="stream-hub-session-names"
          placeholder="Session name"
          value={newOverrideName}
          onChange={(e) => setNewOverrideName(e.target.value)}
          data-testid="stream-hub-override-input"
        />
        <datalist id="stream-hub-session-names">
          {sessionNames.map((name) => <option key={name} value={name} />)}
        </datalist>
        <button
          className={styles.actionButton}
          disabled={busy || !newOverrideName.trim()}
          onClick={addOverride}
          data-testid="stream-hub-add-override"
        >
          Force hub on for session
        </button>
      </div>
    </section>
  );
}
