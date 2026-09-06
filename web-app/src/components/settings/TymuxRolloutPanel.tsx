"use client";
// +feature: tymux-rollout

import { useState, useEffect, useCallback, useMemo } from "react";
import { createClient } from "@connectrpc/connect";
import { TymuxRolloutService } from "@/gen/session/v1/tymux_rollout_pb";
import { SessionService } from "@/gen/session/v1/session_pb";
import { timestampDate, type Timestamp } from "@bufbuild/protobuf/wkt";
import { getConnectTransport } from "@/lib/api/transport";
import { useAnalytics } from "@/lib/analytics";
// ponytail: reuses StreamHubRolloutPanel's stylesheet — the class names
// (panel, heading, statusRow, ...) are generic, not stream-hub-specific, and
// this panel's markup is deliberately shaped the same way.
import * as styles from "./StreamHubRolloutPanel.css";

interface SessionOverride {
  sessionName: string;
  forceTymux: boolean;
}

// Mirrors session/tmux/tmux.go's toStaplerSquadTmuxNameWithPrefix exactly.
// TymuxSessionOverrides is keyed by the *sanitized* tmux session name, not
// the raw title (see ResolveSessionBackend's doc comment and #162) — typing
// a title with spaces/dots/colons here and sending it unsanitized would
// silently never match at CreateSession time.
function toStaplerSquadSessionName(title: string): string {
  return `staplersquad_${title.replace(/\s+/g, "").replace(/[.:]/g, "_")}`;
}

/**
 * Controls for the tymux-bundled-integration rollout of BackendTymux.
 * Everything here takes effect immediately, config.json-backed: the
 * "tymux" feature flag (off by default) via a live global override, and
 * per-session canary overrides — no process restart required. Mirrors
 * StreamHubRolloutPanel's shape exactly.
 */
export function TymuxRolloutPanel() {
  const { track } = useAnalytics();
  const transport = useMemo(() => getConnectTransport(), []);
  const client = useMemo(() => createClient(TymuxRolloutService, transport), [transport]);
  const sessionClient = useMemo(() => createClient(SessionService, transport), [transport]);

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
    setRehearsalCompletedAt(ts ? timestampDate(ts) : null);
    setOverrides(status.sessionOverrides);
    setGlobalOverride(status.globalOverride);
  }, []);

  const load = useCallback(async () => {
    try {
      const status = await client.getTymuxRolloutStatus({});
      applyStatus(status);
    } catch {
      setError("Failed to load tymux rollout status");
    } finally {
      setLoading(false);
    }
  }, [client, applyStatus]);

  useEffect(() => {
    let cancelled = false;
    void load();
    sessionClient.listSessions({}).then((res) => {
      if (cancelled) return;
      setSessionNames(res.sessions.map((s) => s.title).filter(Boolean));
    }).catch(() => {
      // Non-fatal — the override input still works as free text.
    });
    return () => {
      cancelled = true;
    };
  }, [load, sessionClient]);

  const completeRehearsal = useCallback(async () => {
    track({ name: "tymux_rehearsal_completed", category: "user_action", component: "TymuxRolloutPanel" });
    setBusy(true);
    setError(null);
    try {
      const status = await client.completeTymuxRollbackRehearsal({});
      applyStatus(status);
    } catch {
      setError("Failed to record rollback rehearsal");
    } finally {
      setBusy(false);
    }
  }, [client, applyStatus, track]);

  const addOverride = useCallback(async () => {
    const title = newOverrideName.trim();
    if (!title) return;
    const sessionName = toStaplerSquadSessionName(title);
    track({ name: "tymux_override_added", category: "user_action", component: "TymuxRolloutPanel" });
    setBusy(true);
    setError(null);
    try {
      const status = await client.setTymuxSessionOverride({ sessionName, forceTymux: true });
      applyStatus(status);
      setNewOverrideName("");
    } catch {
      setError("Failed to set session override");
    } finally {
      setBusy(false);
    }
  }, [client, applyStatus, newOverrideName, track]);

  const setGlobalOverrideValue = useCallback(async (forceTymux: boolean | undefined) => {
    track({ name: "tymux_global_override_changed", category: "user_action", component: "TymuxRolloutPanel", labels: { forceTymux: forceTymux === undefined ? "clear" : String(forceTymux) } });
    setBusy(true);
    setError(null);
    try {
      const status = await client.setTymuxGlobalOverride({ forceTymux });
      applyStatus(status);
    } catch {
      setError("Failed to update the global override");
    } finally {
      setBusy(false);
    }
  }, [client, applyStatus, track]);

  const removeOverride = useCallback(async (sessionName: string) => {
    track({ name: "tymux_override_removed", category: "user_action", component: "TymuxRolloutPanel" });
    setBusy(true);
    setError(null);
    try {
      const status = await client.setTymuxSessionOverride({ sessionName });
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
    <section className={styles.panel} data-testid="tymux-rollout-panel">
      <h2 className={styles.heading}>Tymux Backend Rollout</h2>
      <p className={styles.description}>
        Staged rollout for the tymux-backed <code>ProcessManager</code>. The
        &quot;tymux&quot; feature flag defaults to off; the override below takes effect
        immediately for any session created from this point on — no restart required.
      </p>

      {error && <p className={styles.errorMessage} role="alert">{error}</p>}

      <div className={styles.statusRow}>
        <span className={styles.statusLabel}>Global override</span>
        <span className={`${styles.badge} ${globalOverride === true ? styles.badgeEnabled : styles.badgeDisabled}`}>
          {globalOverride === undefined ? "Not set (default: off)" : globalOverride ? "Forced on" : "Forced off"}
        </span>
      </div>
      <div className={styles.addRow}>
        <button
          className={styles.actionButton}
          disabled={busy || globalOverride === true}
          onClick={() => setGlobalOverrideValue(true)}
          data-testid="tymux-global-override-on"
          aria-label="Force tymux on for all sessions"
        >
          Force on for everything
        </button>
        <button
          className={styles.actionButton}
          disabled={busy || globalOverride === false}
          onClick={() => setGlobalOverrideValue(false)}
          data-testid="tymux-global-override-off"
          aria-label="Force tymux off for all sessions"
        >
          Force off for everything
        </button>
        <button
          className={styles.removeButton}
          disabled={busy || globalOverride === undefined}
          onClick={() => setGlobalOverrideValue(undefined)}
          data-testid="tymux-global-override-clear"
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
            data-testid="tymux-complete-rehearsal"
            disabled={busy}
            onClick={completeRehearsal}
          >
            Mark rehearsal complete
          </button>
        )}
      </div>
      <p className={styles.hint}>
        Only mark this after manually verifying: flip a per-session override on, run a
        real session through it, confirm a clean disconnect/reconnect.
      </p>

      <h3 className={styles.subheading}>Per-session canary overrides</h3>
      {overrides.length === 0 ? (
        <p className={styles.description}>No sessions are currently overridden.</p>
      ) : (
        <ul className={styles.overrideList}>
          {overrides.map((o) => (
            <li key={o.sessionName} className={styles.overrideRow} data-testid="tymux-override-row">
              <span className={styles.statusLabel}>{o.sessionName}</span>
              <span className={`${styles.badge} ${o.forceTymux ? styles.badgeEnabled : styles.badgeDisabled}`}>
                {o.forceTymux ? "Forced on" : "Forced off"}
              </span>
              <button
                className={styles.removeButton}
                data-testid="tymux-remove-override"
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
          list="tymux-session-names"
          placeholder="Session title (existing or one you're about to create)"
          aria-label="Session title"
          value={newOverrideName}
          onChange={(e) => setNewOverrideName(e.target.value)}
          data-testid="tymux-override-input"
        />
        <datalist id="tymux-session-names">
          {sessionNames.map((name) => <option key={name} value={name} />)}
        </datalist>
        <button
          className={styles.actionButton}
          disabled={busy || !newOverrideName.trim()}
          onClick={addOverride}
          data-testid="tymux-add-override"
        >
          Force tymux on for session
        </button>
      </div>
      {newOverrideName.trim() && (
        <p className={styles.hint}>
          Stored as <code>{toStaplerSquadSessionName(newOverrideName.trim())}</code> — the
          sanitized tmux session name a session with this title resolves to.
        </p>
      )}
    </section>
  );
}
