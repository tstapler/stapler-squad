"use client";

import { useState, useCallback, useRef, useEffect } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { UnfinishedWorkService } from "@/gen/session/v1/unfinished_pb";
import { QuickCommitPushRequestSchema } from "@/gen/session/v1/unfinished_pb";
import { create } from "@bufbuild/protobuf";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import * as styles from "./CommitPushModal.css";

interface CommitPushModalProps {
  repoPath: string;
  branch: string;
  onClose: () => void;
}

/**
 * Modal for staging all changes, entering a commit message, and pushing.
 */
export function CommitPushModal({ repoPath, branch, onClose }: CommitPushModalProps) {
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  const transport = createConnectTransport({
    baseUrl: getApiBaseUrl(),
    interceptors: [createAuthInterceptor()],
  });
  const client = createClient(UnfinishedWorkService, transport);

  const handleSubmit = useCallback(async () => {
    const trimmed = message.trim();
    if (!trimmed) return;

    setLoading(true);
    setError(null);
    try {
      const req = create(QuickCommitPushRequestSchema, {
        repoPath,
        branch,
        commitMessage: trimmed,
      });
      const res = await client.quickCommitPush(req);
      if (res.success) {
        onClose();
      } else {
        setError(res.errorMessage || "Operation failed");
      }
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Unexpected error");
    } finally {
      setLoading(false);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [message, repoPath, branch, onClose]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") onClose();
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") handleSubmit();
  };

  return (
    <div
      className={styles.overlay}
      onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
      onKeyDown={handleKeyDown}
      role="dialog"
      aria-modal="true"
      aria-labelledby="commit-push-title"
    >
      <div className={styles.modal}>
        <h2 id="commit-push-title" className={styles.title}>
          Commit &amp; Push — {branch}
        </h2>

        <div>
          <label className={styles.label} htmlFor="commit-message">
            Commit message
          </label>
          <textarea
            id="commit-message"
            ref={textareaRef}
            className={styles.textarea}
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            placeholder="Describe your changes…"
            disabled={loading}
          />
        </div>

        {error && <div className={styles.errorMsg}>{error}</div>}

        <div className={styles.buttonRow}>
          <button className={styles.btnCancel} onClick={onClose} disabled={loading}>
            Cancel
          </button>
          <button
            className={styles.btnSubmit}
            onClick={handleSubmit}
            disabled={!message.trim() || loading}
            aria-label="Stage all, commit, and push"
          >
            {loading ? "Pushing…" : "Commit & Push"}
          </button>
        </div>
      </div>
    </div>
  );
}
