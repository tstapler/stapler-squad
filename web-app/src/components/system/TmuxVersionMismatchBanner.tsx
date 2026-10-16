"use client";
// +feature: ui:tmux-version-mismatch-banner

import { useState, useEffect, useMemo, useCallback, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { TmuxVersionMismatch } from "@/gen/session/v1/session_pb";
import { getApiBaseUrl } from "@/lib/config";
import { SystemBanner } from "@/components/ui/SystemBanner";
import { RestartTmuxServerConfirmDialog } from "./RestartTmuxServerConfirmDialog";

// How often to re-poll for a mismatch clearing (e.g. after a manual fix) or a
// new one appearing. Not urgent — this is a persistent system-health signal,
// not a live event stream — so a slow interval is appropriate.
const POLL_INTERVAL_MS = 60_000;

/**
 * Persistent, dismissible-per-session banner surfacing a tmux client/server
 * version mismatch (session/tmux/version_check.go): control mode is disabled
 * for the affected socket, degrading terminal input/output to slower
 * per-command subprocess calls. Offers a "Restart tmux server" action, gated
 * behind RestartTmuxServerConfirmDialog since it kills every live session on
 * that socket. Renders via the generic SystemBanner primitive.
 */
export function TmuxVersionMismatchBanner() {
  const [mismatches, setMismatches] = useState<TmuxVersionMismatch[]>([]);
  const [dismissed, setDismissed] = useState<Set<string>>(new Set());
  const [confirmTarget, setConfirmTarget] = useState<TmuxVersionMismatch | null>(null);
  const [restarting, setRestarting] = useState(false);
  const [restartError, setRestartError] = useState<string | null>(null);

  const client = useMemo(
    () => createClient(SessionService, createConnectTransport({ baseUrl: getApiBaseUrl() })),
    []
  );

  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const refresh = useCallback(async () => {
    try {
      const res = await client.getTmuxVersionStatus({});
      if (mountedRef.current) setMismatches(res.mismatches);
    } catch {
      // Transient RPC failure — keep showing whatever we already had rather
      // than flashing the banner away.
    }
  }, [client]);

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, POLL_INTERVAL_MS);
    return () => clearInterval(id);
  }, [refresh]);

  const handleConfirmRestart = useCallback(async () => {
    if (!confirmTarget) return;
    setRestarting(true);
    setRestartError(null);
    try {
      await client.restartTmuxServer({ serverSocket: confirmTarget.serverSocket });
      setConfirmTarget(null);
      await refresh();
    } catch (e) {
      setRestartError(e instanceof Error ? e.message : String(e));
    } finally {
      if (mountedRef.current) setRestarting(false);
    }
  }, [client, confirmTarget, refresh]);

  const visible = mismatches.filter((m) => !dismissed.has(m.serverSocket));
  if (visible.length === 0) return null;

  return (
    <>
      {visible.map((m) => (
        <SystemBanner
          key={m.serverSocket}
          id="tmux-version-mismatch"
          severity="error"
          icon="⚠"
          testId="tmux-version-mismatch-banner"
          message={
            <>
              tmux version mismatch on {m.serverSocket || "the default server"} — client {m.clientVersion} vs.
              server {m.serverVersion}. Control mode is disabled; terminal input/output is slower than usual.
              {restartError && <> Restart failed: {restartError}</>}
            </>
          }
          actions={[
            {
              label: "Restart tmux server…",
              variant: "danger",
              testId: "tmux-version-mismatch-restart",
              onClick: () => setConfirmTarget(m),
            },
          ]}
          onDismiss={() => setDismissed((prev) => new Set(prev).add(m.serverSocket))}
        />
      ))}

      {confirmTarget && (
        <RestartTmuxServerConfirmDialog
          sessionCount={confirmTarget.affectedSessionCount}
          clientVersion={confirmTarget.clientVersion}
          serverVersion={confirmTarget.serverVersion}
          busy={restarting}
          onConfirm={handleConfirmRestart}
          onCancel={() => setConfirmTarget(null)}
        />
      )}
    </>
  );
}
