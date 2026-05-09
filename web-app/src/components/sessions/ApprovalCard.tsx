"use client";

import { useEffect, useState, useCallback } from "react";
import type { PlainApproval } from "@/lib/api/approvalsApi";
import {
  card,
  cardExpired,
  header,
  toolName,
  toolIcon,
  countdown,
  countdownNormal,
  countdownWarning,
  countdownUrgent,
  body,
  detail,
  detailLabel,
  detailValue,
  toolInputPreview,
  detailsToggle,
  detailsButton,
  fullDetails,
  actions,
  approveButton,
  denyButton,
  dismissButton,
} from "./ApprovalCard.css";

interface ApprovalCardProps {
  approval: PlainApproval;
  onApprove: () => void;
  onDeny: () => void;
  sessionTitle?: string; // Human-readable session name; falls back to approval.sessionId
}

/**
 * Displays a single pending tool-use approval request.
 *
 * Shows the tool name, relevant input preview, working directory,
 * and a live countdown timer. Provides Approve (green) and Deny (red) buttons.
 */
export function ApprovalCard({ approval, onApprove, onDeny, sessionTitle }: ApprovalCardProps) {
  const [secondsLeft, setSecondsLeft] = useState(approval.secondsRemaining);
  const [showDetails, setShowDetails] = useState(false);
  const toggleDetails = useCallback(() => setShowDetails((v) => !v), []);

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
    if (secondsLeft <= 10) return countdownUrgent;
    if (secondsLeft <= 30) return countdownWarning;
    return countdownNormal;
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

  const isExpired = secondsLeft <= 0;

  return (
    <div className={`${card} ${isExpired ? cardExpired : ""}`} data-testid={`approval-card-${approval.id}`}>
      <div className={header}>
        <div className={toolName}>
          <span className={toolIcon} aria-hidden="true">&#x1F527;</span>
          {approval.toolName}
        </div>
        <span
          className={`${countdown} ${getCountdownClass()}`}
          title={`Expires in ${formatCountdown(secondsLeft)}`}
        >
          {formatCountdown(secondsLeft)}
        </span>
      </div>

      <div className={body}>
        {(sessionTitle || approval.sessionId) && (
          <div className={detail}>
            <span className={detailLabel}>Session:</span>
            <span className={detailValue} title={approval.sessionId}>
              {sessionTitle || approval.sessionId}
            </span>
          </div>
        )}

        {inputPreview && (
          <div className={toolInputPreview} title={inputPreview.value}>
            {inputPreview.value}
          </div>
        )}

        {approval.cwd && (
          <div className={detail}>
            <span className={detailLabel}>Directory:</span>
            <span className={detailValue} title={approval.cwd}>
              {approval.cwd}
            </span>
          </div>
        )}
      </div>

      {approval.toolInput && Object.keys(approval.toolInput).length > 0 && (
        <div className={detailsToggle}>
          <button className={detailsButton} onClick={toggleDetails}>
            {showDetails ? "Hide details ▲" : "Show full details ▼"}
          </button>
          {showDetails && (
            <pre className={fullDetails}>
              {Object.entries(approval.toolInput)
                .map(([k, v]) => `${k}: ${v}`)
                .join("\n")}
            </pre>
          )}
        </div>
      )}

      <div className={actions}>
        {isExpired ? (
          <button
            className={dismissButton}
            onClick={onDeny}
            title="Remove this expired approval"
            aria-label="Dismiss expired approval"
          >
            Dismiss
          </button>
        ) : (
          <>
            <button
              className={approveButton}
              onClick={onApprove}
              title="Allow this tool use"
              aria-label={`Approve ${approval.toolName}`}
            >
              Approve
            </button>
            <button
              className={denyButton}
              onClick={onDeny}
              title="Deny this tool use"
              aria-label={`Deny ${approval.toolName}`}
            >
              Deny
            </button>
          </>
        )}
      </div>
    </div>
  );
}
