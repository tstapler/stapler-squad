// +feature: session-artifacts
"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { Session, SessionArtifacts, SessionPullRequest } from "@/gen/session/v1/types_pb";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import { formatRelativeTime } from "@/lib/utils/datetime";
import { useAnnounce } from "@/lib/hooks/useAnnounce";
import { useRescanArtifacts } from "@/lib/hooks/useRescanArtifacts";
import { ArtifactsGroupedView } from "./ArtifactsGroupedView";
import * as styles from "./ArtifactsTab.css";

interface ArtifactsTabProps {
  session: Session;
  /** Replaces the rescan RPC (tests); must reject on failure. */
  onRescan?: () => Promise<void> | void;
}

type ScanState = "not_linked" | "scanning" | "none_found" | "has_data" | "failed";

/** How long "Scanning" may last before the tab offers a way out (UX-S7-4). */
export const SCAN_SLOW_AFTER_MS = 60_000;

function hasContent(a: SessionArtifacts | undefined): boolean {
  return (
    (a?.records?.length ?? 0) > 0 ||
    (a?.prUrls?.length ?? 0) > 0 ||
    (a?.commitShas?.length ?? 0) > 0 ||
    (a?.externalUrls?.length ?? 0) > 0
  );
}

/** Servers that predate scan_state send none: derive it from the data. */
function resolveState(a: SessionArtifacts | undefined): ScanState {
  if (!a) return "scanning";
  if (a.scanState) return a.scanState as ScanState;
  return hasContent(a) ? "has_data" : "none_found";
}

function parsePRDisplay(url: string): string {
  const m = url.match(/github\.com\/([\w.-]+\/[\w.-]+)\/pull\/(\d+)/);
  return m ? `${m[1]}#${m[2]}` : url;
}

// safeHref ensures only http/https URLs are used as link targets (N-5 fix).
const safeHref = (url: string) => /^https?:\/\//i.test(url) ? url : "#";

function ArtifactLists({ artifacts, pullRequests }: { artifacts: SessionArtifacts; pullRequests: readonly SessionPullRequest[] }) {
  const [urlsExpanded, setUrlsExpanded] = useState(false);
  // Older scans and servers send no records: keep the flat v1 lists (UX-S7 "v1 blobs").
  if ((artifacts.records?.length ?? 0) > 0) return <ArtifactsGroupedView artifacts={artifacts} pullRequests={pullRequests} />;
  return (
    <div className={styles.lists}>
      {(artifacts.prUrls?.length ?? 0) > 0 && (
        <section className={styles.section}>
          <h3 className={styles.sectionTitle}>Pull Requests</h3>
          <ul className={styles.list}>
            {artifacts.prUrls.map((url) => (
              <li key={url}>
                <a
                  href={safeHref(url)}
                  target="_blank"
                  rel="noopener noreferrer"
                  className={styles.link}
                >
                  {parsePRDisplay(url)}
                </a>
              </li>
            ))}
          </ul>
        </section>
      )}
      {(artifacts.commitShas?.length ?? 0) > 0 && (
        <section className={styles.section}>
          <h3 className={styles.sectionTitle}>Commits</h3>
          <ul className={styles.list}>
            {artifacts.commitShas.map((sha) => (
              <li key={sha}>
                <code className={styles.sha}>{sha.slice(0, 7)}</code>
                <span className={styles.shaFull}>{sha}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
      {(artifacts.externalUrls?.length ?? 0) > 0 && (
        <section className={styles.section}>
          <h3 className={styles.sectionTitle}>External URLs</h3>
          {urlsExpanded ? (
            <>
              <ul className={styles.list}>
                {artifacts.externalUrls.map((url) => (
                  <li key={url}>
                    <a
                      href={safeHref(url)}
                      target="_blank"
                      rel="noopener noreferrer"
                      className={styles.link}
                    >
                      {url.length > 60 ? url.slice(0, 60) + "…" : url}
                    </a>
                  </li>
                ))}
              </ul>
              <button
                className={styles.urlToggleButton}
                onClick={() => setUrlsExpanded(false)}
              >{/* analytics-exempt */}
                Hide URLs
              </button>
            </>
          ) : (
            <button
              className={styles.urlToggleButton}
              onClick={() => setUrlsExpanded(true)}
            >{/* analytics-exempt */}
              Show {artifacts.externalUrls.length} external URL
              {artifacts.externalUrls.length !== 1 ? "s" : ""}
            </button>
          )}
        </section>
      )}
    </div>
  );
}

function StatusLine({ artifacts }: { artifacts: SessionArtifacts | undefined }) {
  const [, setTick] = useState(0);
  const scannedAt = artifacts?.lastScannedAt;
  useEffect(() => {
    if (!scannedAt) return;
    const id = setInterval(() => setTick((n) => n + 1), 60_000);
    return () => clearInterval(id);
  }, [scannedAt]);
  if (!scannedAt) return null;
  return (
    <span className={styles.statusLine}>
      Last scanned {formatRelativeTime(timestampDate(scannedAt).getTime())}
    </span>
  );
}

function ActionButton({ label, onClick, busy = false }: { label: string; onClick?: () => void; busy?: boolean }) {
  if (!onClick) return null;
  return (
    <button
      type="button"
      className={styles.actionButton}
      onClick={onClick}
      disabled={busy}
      aria-busy={busy}
    >
      {/* analytics-exempt */}
      {busy && <span className={styles.spinner} aria-hidden="true" />}
      {busy ? "Rescanning..." : label}
    </button>
  );
}

function scanKey(a: SessionArtifacts | undefined): string {
  return a?.lastScannedAt ? `${a.lastScannedAt.seconds}.${a.lastScannedAt.nanos}` : "";
}

function describeResult(a: SessionArtifacts | undefined): string {
  const flat = (a?.prUrls?.length ?? 0) + (a?.commitShas?.length ?? 0) + (a?.externalUrls?.length ?? 0);
  const n = (a?.records?.length ?? 0) || flat;
  return n > 0 ? `Scan finished: ${n} artifact${n === 1 ? "" : "s"}` : "No artifacts found";
}

/**
 * Tracks one rescan: pending from the click until the server publishes a new
 * scan timestamp (every rescan stamps one), then announces the result.
 */
function useRescan(artifacts: SessionArtifacts | undefined, rescanRpc: () => Promise<void> | void) {
  const { announce } = useAnnounce();
  const key = scanKey(artifacts);
  const [pending, setPending] = useState<{ baseline: string } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const latest = useRef(artifacts);
  latest.current = artifacts;

  useEffect(() => {
    if (pending && key !== pending.baseline) {
      announce(describeResult(latest.current), "polite", "artifacts-rescan");
      setPending(null);
    }
  }, [key, pending, announce]);

  const start = useCallback(async () => {
    if (pending) return;
    setError(null);
    setPending({ baseline: key });
    try {
      await rescanRpc();
    } catch (err) {
      setPending(null);
      setError(err instanceof Error ? err.message : "Rescan failed");
    }
  }, [pending, key, rescanRpc]);

  return { rescanning: pending !== null, error, start, clearError: () => setError(null) };
}

export function ArtifactsTab({ session, onRescan: onRescanOverride }: ArtifactsTabProps) {
  const artifacts = session.artifacts;
  const pullRequests = session.pullRequests ?? [];
  const state = resolveState(artifacts);
  const rescanRpc = useRescanArtifacts(session.id);
  const rescan = useRescan(artifacts, onRescanOverride ?? rescanRpc);
  const onRescan = rescan.start;
  const [slow, setSlow] = useState(false);
  const [cancelled, setCancelled] = useState(false);
  const [alertDismissed, setAlertDismissed] = useState(false);

  useEffect(() => {
    setSlow(false);
    setCancelled(false);
    setAlertDismissed(false);
    if (state !== "scanning") return;
    const id = setTimeout(() => setSlow(true), SCAN_SLOW_AFTER_MS);
    return () => clearTimeout(id);
  }, [state]);

  const rescanError = rescan.error && (
    <div role="alert" className={styles.alert}>
      <span>Rescan failed: {rescan.error}</span>
      <div className={styles.actions}>
        <ActionButton label="Retry" onClick={onRescan} busy={rescan.rescanning} />
        <ActionButton label="Dismiss" onClick={rescan.clearError} />
      </div>
    </div>
  );

  const header = (
    <div className={styles.header}>
      <StatusLine artifacts={artifacts} />
      {(state === "none_found" || state === "has_data") && (
        <ActionButton label="Rescan" onClick={onRescan} busy={rescan.rescanning} />
      )}
    </div>
  );

  if (state === "not_linked") {
    return (
      <div className={styles.emptyState}>
        {artifacts && !artifacts.transcriptSupported ? (
          <span>
            This session&apos;s program does not produce a transcript, so there is nothing to scan.
          </span>
        ) : (
          <span>
            This session has no linked transcript yet, so there is nothing to scan. It links
            automatically once the session writes its first message.
          </span>
        )}
      </div>
    );
  }

  if (state === "scanning") {
    if (slow && !cancelled) {
      return (
        <div className={styles.emptyState} role="status">
          {rescanError}
          <span>Taking longer than usual.</span>
          <div className={styles.actions}>
            <ActionButton label="Rescan" onClick={onRescan} busy={rescan.rescanning} />
            <ActionButton label="Cancel waiting" onClick={() => setCancelled(true)} />
          </div>
        </div>
      );
    }
    if (cancelled) {
      return artifacts && hasContent(artifacts) ? (
        <div className={styles.container}>
          <ArtifactLists artifacts={artifacts} pullRequests={pullRequests} />
        </div>
      ) : (
        <div className={styles.emptyState}>
          <span>Not waiting for the scan any more. It may still finish; reopen this tab to check.</span>
        </div>
      );
    }
    return (
      <div className={styles.emptyState} role="status" aria-busy="true">
        <span>Scanning transcript...</span>
      </div>
    );
  }

  if (state === "none_found") {
    return (
      <div className={styles.container}>
        {rescanError}
        {header}
        <div className={styles.emptyState}>
          <span>No artifacts found in this session&apos;s conversation history.</span>
          <span className={styles.hint}>
            Artifacts are PRs, commits and links the agent produced.
          </span>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      {state === "failed" && !alertDismissed && (
        <div role="alert" className={styles.alert}>
          <span>
            Scan failed.{artifacts && hasContent(artifacts) ? " Showing the last good results." : ""}
          </span>
          <div className={styles.actions}>
            <ActionButton label="Retry" onClick={onRescan} busy={rescan.rescanning} />
            <ActionButton label="Dismiss" onClick={() => setAlertDismissed(true)} />
          </div>
        </div>
      )}
      {rescanError}
      {header}
      {artifacts && hasContent(artifacts) ? (
        <ArtifactLists artifacts={artifacts} pullRequests={pullRequests} />
      ) : (
        <div className={styles.emptyState}>
          <span>No artifacts found in this session&apos;s conversation history.</span>
        </div>
      )}
    </div>
  );
}
