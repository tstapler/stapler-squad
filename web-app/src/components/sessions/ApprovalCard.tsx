"use client";

import { useEffect, useState } from "react";
import { PendingApprovalProto } from "@/gen/session/v1/types_pb";
import styles from "./ApprovalCard.module.css";

interface ApprovalCardProps {
  approval: PendingApprovalProto;
  onApprove: () => void;
  onDeny: () => void;
}

/**
 * Displays a single pending tool-use approval request.
 *
 * Shows the tool name, relevant input preview, working directory,
 * and a live countdown timer. Provides Approve (green) and Deny (red) buttons.
 */
export function ApprovalCard({ approval, onApprove, onDeny }: ApprovalCardProps) {
  const [secondsLeft, setSecondsLeft] = useState(approval.secondsRemaining);

  // Decrement countdown every second
  useEffect(() => {
    setSecondsLeft(approval.secondsRemaining);

    const interval = setInterval(() => {
      setSecondsLeft((prev) => Math.max(0, prev - 1));
    }, 1000);

    return () => clearInterval(interval);
  }, [approval.secondsRemaining]);

  // Determine which tool input field to preview
  const getToolInputPreview = (): { label: string; value: string } | null => {
    const input = approval.toolInput;
    if (!input) return null;

    if (input["command"]) {
      return { label: "Command", value: input["command"] };
    }
    if (input["file_path"]) {
      return { label: "File", value: input["file_path"] };
    }
    if (input["description"]) {
      return { label: "Description", value: input["description"] };
    }

    // Fallback: show first key-value pair if any
    const keys = Object.keys(input);
    if (keys.length > 0) {
      const firstKey = keys[0];
      return { label: firstKey, value: input[firstKey] };
    }

    return null;
  };

  const inputPreview = getToolInputPreview();

  // Countdown styling based on urgency
  const getCountdownClass = (): string => {
    if (secondsLeft <= 10) return styles.countdownUrgent;
    if (secondsLeft <= 30) return styles.countdownWarning;
    return styles.countdownNormal;
  };

  const formatCountdown = (seconds: number): string => {
    if (seconds <= 0) return "Expired";
    const mins = Math.floor(seconds / 60);
    const secs = seconds % 60;
    if (mins > 0) {
      return `${mins}m ${secs.toString().padStart(2, "0")}s`;
    }
    return `${secs}s`;
  };

  return (
    <div className={styles.card} data-testid={`approval-card-${approval.id}`}>
      <div className={styles.header}>
        <div className={styles.toolName}>
          <span className={styles.toolIcon} aria-hidden="true">&#x1F527;</span>
          {approval.toolName}
        </div>
        <span
          className={`${styles.countdown} ${getCountdownClass()}`}
          title={`Expires in ${formatCountdown(secondsLeft)}`}
        >
          {formatCountdown(secondsLeft)}
        </span>
      </div>

      <div className={styles.body}>
        {approval.sessionId && (
          <div className={styles.detail}>
            <span className={styles.detailLabel}>Session:</span>
            <span className={styles.detailValue} title={approval.sessionId}>
              {approval.sessionId}
            </span>
          </div>
        )}

        {inputPreview && (
          <div className={styles.toolInputPreview} title={inputPreview.value}>
            {inputPreview.value}
          </div>
        )}

        {approval.cwd && (
          <div className={styles.detail}>
            <span className={styles.detailLabel}>Directory:</span>
            <span className={styles.detailValue} title={approval.cwd}>
              {approval.cwd}
            </span>
          </div>
        )}
      </div>

      <div className={styles.actions}>
        <button
          className={styles.approveButton}
          onClick={onApprove}
          disabled={secondsLeft <= 0}
          title="Allow this tool use"
          aria-label={`Approve ${approval.toolName}`}
        >
          Approve
        </button>
        <button
          className={styles.denyButton}
          onClick={onDeny}
          disabled={secondsLeft <= 0}
          title="Deny this tool use"
          aria-label={`Deny ${approval.toolName}`}
        >
          Deny
        </button>
      </div>
    </div>
  );
}
