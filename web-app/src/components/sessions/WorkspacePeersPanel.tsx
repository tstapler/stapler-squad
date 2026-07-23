"use client";

// +feature: workspace-peers-panel
import { useMemo } from "react";
import { useAppSelector } from "@/lib/store";
import { selectAllSessions } from "@/lib/store/sessionsSlice";
import { Session, SessionStatus } from "@/gen/session/v1/types_pb";
import {
  panelContainer,
  heading,
  peerList,
  peerItem,
  peerTitleRow,
  peerTitle,
  peerMeta,
  peerGoal,
  lifecycleChipBase,
  lifecycleChipVariants,
} from "./WorkspacePeersPanel.css";

// Mirrors session.goalStaleThreshold (session/workspace_peers.go) — a goal is
// considered stale (peer "stuck") after this long without an update.
const GOAL_STALE_THRESHOLD_MS = 30 * 60 * 1000;

export type PeerLifecycle = "active" | "stuck" | "gone";

/**
 * Derives a peer's lifecycle from two independent signals: whether the session process
 * is confirmed dead (status === STOPPED, ponytail: a proxy for tmux liveness — this repo's
 * orphan-sweep reconciliation keeps Status roughly in sync with reality; a fully
 * authoritative check would need a server round-trip via list_workspace_peers) and whether
 * its goal hasn't been touched in a while. Exported for direct unit testing.
 */
export function peerLifecycle(peer: Session, nowMs: number): PeerLifecycle {
  if (peer.status === SessionStatus.STOPPED) return "gone";
  const updatedAt = peer.goal?.updatedAt;
  if (updatedAt) {
    const updatedMs = Number(updatedAt.seconds) * 1000;
    if (nowMs - updatedMs > GOAL_STALE_THRESHOLD_MS) return "stuck";
  }
  return "active";
}

const LIFECYCLE_LABELS: Record<PeerLifecycle, string> = {
  active: "Active",
  stuck: "Stuck",
  gone: "Gone",
};

export interface WorkspacePeersPanelProps {
  session: Session;
  /** Injectable for tests; defaults to Date.now(). */
  now?: number;
}

/**
 * WorkspacePeersPanel lists other active sessions sharing the current session's workspace
 * (same repo, any branch/worktree), live-updated via the existing WatchSessions Redux store —
 * no extra polling or RPC needed since Session already carries workspace_key and goal.updatedAt.
 * Renders nothing when the session has no workspace key or no peers.
 */
export function WorkspacePeersPanel({ session, now }: WorkspacePeersPanelProps) {
  const allSessions = useAppSelector(selectAllSessions);
  const nowMs = now ?? Date.now();

  const peers = useMemo(() => {
    if (!session.workspaceKey) return [];
    return allSessions.filter(
      (s) => s.id !== session.id && s.workspaceKey === session.workspaceKey
    );
  }, [allSessions, session.workspaceKey, session.id]);

  if (!session.workspaceKey || peers.length === 0) return null;

  return (
    <div className={panelContainer} data-testid="workspace-peers-panel">
      <div className={heading}>Other Sessions in This Workspace</div>
      <ul className={peerList} role="list">
        {peers.map((peer) => {
          const lifecycle = peerLifecycle(peer, nowMs);
          return (
            <li key={peer.id} className={peerItem} data-testid="workspace-peer-item">
              <div className={peerTitleRow}>
                <span className={peerTitle}>{peer.title}</span>
                <span
                  className={`${lifecycleChipBase} ${lifecycleChipVariants[lifecycle]}`}
                  aria-label={`Peer status: ${LIFECYCLE_LABELS[lifecycle]}`}
                >
                  {LIFECYCLE_LABELS[lifecycle]}
                </span>
              </div>
              <span className={peerMeta}>{peer.branch || peer.path}</span>
              {peer.goal?.goalText && (
                <span className={peerGoal}>{peer.goal.goalText}</span>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
