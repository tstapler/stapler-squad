"use client";

import { Session, SessionStatus } from "@/gen/session/v1/types_pb";
import styles from "./SessionCard.module.css";

interface SessionCardProps {
  session: Session;
  onClick?: () => void;
  onDelete?: () => void;
  onPause?: () => void;
  onResume?: () => void;
}

export function SessionCard({
  session,
  onClick,
  onDelete,
  onPause,
  onResume,
}: SessionCardProps) {
  const getStatusColor = (status: SessionStatus): string => {
    switch (status) {
      case SessionStatus.RUNNING:
        return styles.statusRunning;
      case SessionStatus.READY:
        return styles.statusReady;
      case SessionStatus.PAUSED:
        return styles.statusPaused;
      case SessionStatus.LOADING:
        return styles.statusLoading;
      case SessionStatus.NEEDS_APPROVAL:
        return styles.statusNeedsApproval;
      default:
        return styles.statusUnknown;
    }
  };

  const getStatusText = (status: SessionStatus): string => {
    switch (status) {
      case SessionStatus.RUNNING:
        return "Running";
      case SessionStatus.READY:
        return "Ready";
      case SessionStatus.PAUSED:
        return "Paused";
      case SessionStatus.LOADING:
        return "Loading";
      case SessionStatus.NEEDS_APPROVAL:
        return "Needs Approval";
      default:
        return "Unknown";
    }
  };

  const formatDate = (timestamp?: { seconds: bigint; nanos: number }): string => {
    if (!timestamp) return "N/A";
    const date = new Date(Number(timestamp.seconds) * 1000);
    return date.toLocaleString();
  };

  const isPaused = session.status === SessionStatus.PAUSED;

  return (
    <div className={styles.card} onClick={onClick}>
      <div className={styles.header}>
        <div className={styles.titleRow}>
          <h3 className={styles.title}>{session.title}</h3>
          <span className={`${styles.status} ${getStatusColor(session.status)}`}>
            {getStatusText(session.status)}
          </span>
        </div>
        {session.category && (
          <span className={styles.category}>{session.category}</span>
        )}
      </div>

      <div className={styles.body}>
        <div className={styles.info}>
          <div className={styles.infoRow}>
            <span className={styles.label}>Program:</span>
            <span className={styles.value}>{session.program}</span>
          </div>
          <div className={styles.infoRow}>
            <span className={styles.label}>Branch:</span>
            <span className={styles.value}>{session.branch}</span>
          </div>
          <div className={styles.infoRow}>
            <span className={styles.label}>Path:</span>
            <span className={styles.value} title={session.path}>
              {session.path}
            </span>
          </div>
          {session.workingDir && (
            <div className={styles.infoRow}>
              <span className={styles.label}>Working Dir:</span>
              <span className={styles.value}>{session.workingDir}</span>
            </div>
          )}
        </div>

        {session.diffStats && (
          <div className={styles.diffStats}>
            <span className={styles.diffAdded}>+{session.diffStats.added}</span>
            <span className={styles.diffRemoved}>-{session.diffStats.removed}</span>
          </div>
        )}
      </div>

      <div className={styles.footer}>
        <div className={styles.timestamps}>
          <span className={styles.timestamp}>
            Created: {formatDate(session.createdAt)}
          </span>
          <span className={styles.timestamp}>
            Updated: {formatDate(session.updatedAt)}
          </span>
        </div>

        <div className={styles.actions}>
          {isPaused ? (
            <button
              className={styles.actionButton}
              onClick={(e) => {
                e.stopPropagation();
                onResume?.();
              }}
            >
              Resume
            </button>
          ) : (
            <button
              className={styles.actionButton}
              onClick={(e) => {
                e.stopPropagation();
                onPause?.();
              }}
            >
              Pause
            </button>
          )}
          <button
            className={`${styles.actionButton} ${styles.deleteButton}`}
            onClick={(e) => {
              e.stopPropagation();
              onDelete?.();
            }}
          >
            Delete
          </button>
        </div>
      </div>
    </div>
  );
}
