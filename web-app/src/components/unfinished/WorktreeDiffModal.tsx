"use client";

import { useState, useEffect, useCallback } from "react";
import { createPortal } from "react-dom";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { UnfinishedWorkService } from "@/gen/session/v1/unfinished_pb";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { DiffRenderer } from "@/components/shared/DiffRenderer";
import * as styles from "./WorktreeDiffModal.css";

interface WorktreeDiffModalProps {
  repoPath: string;
  branch: string;
  repoName: string;
  onClose: () => void;
}

export function WorktreeDiffModal({ repoPath, branch, repoName, onClose }: WorktreeDiffModalProps) {
  const [content, setContent] = useState("");
  const [added, setAdded] = useState(0);
  const [removed, setRemoved] = useState(0);
  const [loading, setLoading] = useState(true);
  const [fetchError, setFetchError] = useState<string | null>(null);

  const fetchDiff = useCallback(async () => {
    setLoading(true);
    setFetchError(null);
    try {
      const client = createClient(
        UnfinishedWorkService,
        createConnectTransport({ baseUrl: getApiBaseUrl(), interceptors: [createAuthInterceptor()] }),
      );
      const res = await client.getWorktreeDiff({ repoPath, branch });
      if (res.error) {
        setFetchError(res.error);
      } else if (res.diffStats) {
        setContent(res.diffStats.content);
        setAdded(res.diffStats.added);
        setRemoved(res.diffStats.removed);
      } else {
        setContent("");
      }
    } catch (err) {
      setFetchError(err instanceof Error ? err.message : "Failed to load diff");
    } finally {
      setLoading(false);
    }
  }, [repoPath, branch]);

  useEffect(() => { fetchDiff(); }, [fetchDiff]);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [onClose]);

  const modalContent = (
    <div
      className={styles.overlay}
      onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
      role="dialog"
      aria-modal="true"
      aria-label={`Diff for ${repoName} / ${branch}`}
    >
      <div className={styles.modal}>
        <div className={styles.header}>
          <div style={{ overflow: "hidden" }}>
            <p className={styles.title}>{repoName} — {branch}</p>
            <p className={styles.subtitle}>{repoPath}</p>
          </div>
          <button className={styles.closeButton} onClick={onClose} aria-label="Close diff">✕</button>
        </div>

        <div className={styles.body}>
          {fetchError ? (
            <div style={{ padding: "2rem", color: "var(--error)", textAlign: "center" }}>{fetchError}</div>
          ) : (
            <DiffRenderer
              content={content}
              added={added}
              removed={removed}
              loading={loading}
              onRefresh={fetchDiff}
            />
          )}
        </div>
      </div>
    </div>
  );

  return createPortal(modalContent, document.body);
}
