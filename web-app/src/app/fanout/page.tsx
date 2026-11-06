"use client";
// +feature: parallel-fanout

import { Suspense, useCallback, useEffect, useRef, useState } from "react";
import { useSearchParams } from "@/lib/nav";
import { usePageView } from "@/lib/analytics/usePageView";
import { Session, SessionStatus } from "@/gen/session/v1/types_pb";
import { DiffViewer } from "@/components/sessions/DiffViewer";
import { SessionVcsProvider } from "@/lib/contexts/SessionVcsContext";
import { getApiBaseUrl } from "@/lib/config";
import { getErrorMessage } from "@/lib/utils/connectError";
import { listFanoutMembers, pickFanoutWinner } from "@/lib/hooks/useFanoutService";
import * as styles from "./page.css";

const POLL_MS = 5000;

// Mirrors server fanoutWinnerEligibility (session_service_fanout.go); the server is authoritative
// and also rejects an Active session that is still producing output.
const MAYBE_PICKABLE: SessionStatus[] = [
  SessionStatus.ACTIVE,
  SessionStatus.PAUSED,
  SessionStatus.STOPPED,
  SessionStatus.HIBERNATED,
];

function statusLabel(s: Session): string {
  if (s.archivedAt) return "Archived";
  return SessionStatus[s.status]?.toString().toLowerCase() ?? "unknown";
}

function FanoutComparison({ groupId }: { groupId: string }) {
  usePageView();
  const [members, setMembers] = useState<Session[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [confirming, setConfirming] = useState<string | null>(null);
  const [picked, setPicked] = useState<string | null>(null);

  const [pending, setPending] = useState(false);
  const requestSeq = useRef(0);

  // Only the latest request may update state, so a slow poll issued before a pick can't
  // overwrite the post-pick refresh with stale members.
  const refresh = useCallback(async () => {
    const seq = ++requestSeq.current;
    try {
      const next = await listFanoutMembers(groupId);
      if (seq !== requestSeq.current) return;
      setMembers(next);
      setError(null);
    } catch (err) {
      if (seq !== requestSeq.current) return;
      setError(getErrorMessage(err, "Fan-out request failed"));
    }
  }, [groupId]);

  useEffect(() => {
    if (!groupId) return;
    void refresh();
    const t = setInterval(() => void refresh(), POLL_MS);
    return () => {
      clearInterval(t);
      requestSeq.current++; // invalidate in-flight requests after unmount/groupId change
    };
  }, [groupId, refresh]);

  const pick = async (sessionId: string) => {
    if (pending) return;
    setPending(true);
    setError(null);
    try {
      const res = await pickFanoutWinner(groupId, sessionId);
      if (res.failedArchiveSessionIds.length > 0) {
        // Leave picking enabled: repeating the same pick is idempotent and retries the archive.
        await refresh();
        setError(`Winner merged, but ${res.failedArchiveSessionIds.length} other variant(s) could not be archived. Pick the same winner again to retry.`);
      } else {
        setPicked(sessionId);
        await refresh();
      }
    } catch (err) {
      // Includes the structured merge-conflict error (Aborted) — surfaced, base branch unchanged.
      setError(getErrorMessage(err, "Fan-out request failed"));
    } finally {
      setConfirming(null);
      setPending(false);
    }
  };

  if (!groupId) return <div className={styles.page}>Missing groupId.</div>;

  return (
    <div className={styles.page} data-testid="fanout-comparison">
      <div className={styles.header}>
        <h1>Compare variants ({members.length})</h1>
        {picked && <span data-testid="fanout-picked">Winner merged; other variants archived.</span>}
      </div>
      {error && <div role="alert" className={styles.errorText} data-testid="fanout-error">{error}</div>}
      <div className={styles.grid}>
        {members.map((s) => {
          const archived = Boolean(s.archivedAt);
          const canPick = !picked && !archived && MAYBE_PICKABLE.includes(s.status);
          return (
            <section key={s.id} className={styles.column} data-testid={`fanout-column-${s.id}`} aria-label={s.title}>
              <div className={styles.columnHeader}>
                <strong>{s.title}</strong>
                <span className={styles.badge} data-testid={`fanout-status-${s.id}`}>{statusLabel(s)}</span>
              </div>
              {confirming === s.id ? (
                <button data-testid={`fanout-confirm-${s.id}`} disabled={pending} onClick={() => void pick(s.id)}>
                  Confirm: merge this variant and archive the others
                </button>
              ) : (
                <button data-testid={`fanout-pick-${s.id}`} disabled={!canPick || pending} onClick={() => setConfirming(s.id)}>
                  Pick as winner
                </button>
              )}
              <div className={styles.diffPane}>
                <SessionVcsProvider sessionId={s.id} baseUrl={getApiBaseUrl()}>
                  <DiffViewer session={s} />
                </SessionVcsProvider>
              </div>
            </section>
          );
        })}
      </div>
    </div>
  );
}

function FanoutRouteInner() {
  const groupId = useSearchParams().get("groupId") ?? "";
  return <FanoutComparison groupId={groupId} />;
}

export default function FanoutRoute() {
  return (
    <Suspense>
      <FanoutRouteInner />
    </Suspense>
  );
}
