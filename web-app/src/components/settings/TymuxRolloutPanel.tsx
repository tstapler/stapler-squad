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
import { RolloutStatusSection, RolloutOverrideList } from "./RolloutStatusSection";

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
  const [migrateTitle, setMigrateTitle] = useState("");
  const [migrateStatus, setMigrateStatus] = useState<string | null>(null);

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

  // kibitzer:ignore replace-magic-literal -- this is a TS union type literal (the two valid
  // ProcessManagerBackend values), not a repeated data literal; the other flagged occurrences
  // (testIdPrefix/subjectLabel props below) are unrelated pre-existing JSX attributes.
  const migrateSessionBackend = useCallback(async (backendOverride: "tmux" | "tymux") => {
    const id = migrateTitle.trim();
    if (!id) return;
    track({ name: "tymux_session_migrated", category: "user_action", component: "TymuxRolloutPanel", labels: { backendOverride } });
    setBusy(true);
    setError(null);
    setMigrateStatus(null);
    try {
      await sessionClient.restartSession({ id, preserveOutput: true, backendOverride });
      setMigrateStatus(`"${id}" restarted on ${backendOverride}.`);
    } catch {
      setError(`Failed to migrate "${id}" to ${backendOverride}`);
    } finally {
      setBusy(false);
    }
  }, [sessionClient, migrateTitle, track]);

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

      <RolloutStatusSection
        testIdPrefix="tymux"
        subjectLabel="tymux"
        notSetLabel="Not set (default: off)"
        error={error}
        globalOverride={globalOverride}
        onSetGlobalOverride={setGlobalOverrideValue}
        busy={busy}
        rehearsalCompletedAt={rehearsalCompletedAt}
        onCompleteRehearsal={completeRehearsal}
        overrides={overrides.map((o) => ({ sessionName: o.sessionName, forced: o.forceTymux }))}
        onRemoveOverride={removeOverride}
      />
      <p className={styles.hint}>
        Only mark this after manually verifying: flip a per-session override on, run a
        real session through it, confirm a clean disconnect/reconnect.
      </p>

      <RolloutOverrideList
        testIdPrefix="tymux"
        overrides={overrides.map((o) => ({ sessionName: o.sessionName, forced: o.forceTymux }))}
        busy={busy}
        onRemoveOverride={removeOverride}
      />

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

      <h3 className={styles.heading}>Migrate a live session</h3>
      <p className={styles.description}>
        Restarts an existing session under the target backend right now — kills the
        current process, rebuilds it on tmux or tymux, and resumes. There is no live
        hand-off between backends (they&apos;re different daemons), so this is a restart,
        not an in-place swap.
      </p>
      <div className={styles.addRow}>
        <input
          className={styles.input}
          list="tymux-session-names"
          placeholder="Session title to migrate"
          aria-label="Session title to migrate"
          value={migrateTitle}
          onChange={(e) => setMigrateTitle(e.target.value)}
          data-testid="tymux-migrate-input"
        />
        <button
          className={styles.actionButton}
          disabled={busy || !migrateTitle.trim()}
          onClick={() => migrateSessionBackend("tymux")}
          data-testid="tymux-migrate-to-tymux"
        >
          Restart on tymux
        </button>
        <button
          className={styles.actionButton}
          disabled={busy || !migrateTitle.trim()}
          onClick={() => migrateSessionBackend("tmux")}
          data-testid="tymux-migrate-to-tmux"
        >
          Restart on tmux
        </button>
      </div>
      {migrateStatus && <p className={styles.hint}>{migrateStatus}</p>}
    </section>
  );
}
