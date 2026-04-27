"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { UnfinishedWorktree } from "@/gen/session/v1/types_pb";
import { UnfinishedWorkService } from "@/gen/session/v1/unfinished_pb";
import {
  GetWorktreeAISummaryRequestSchema,
} from "@/gen/session/v1/unfinished_pb";
import { create } from "@bufbuild/protobuf";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { routes } from "@/lib/routes";
import { CommitPushModal } from "./CommitPushModal";
import * as styles from "./UnfinishedItemDetail.css";

interface UnfinishedItemDetailProps {
  worktree: UnfinishedWorktree;
}

/**
 * Expanded accordion detail panel for an UnfinishedItem.
 * Shows diff stats, commit messages, and action buttons.
 */
export function UnfinishedItemDetail({ worktree }: UnfinishedItemDetailProps) {
  const router = useRouter();
  const [aiSummary, setAiSummary] = useState<string | null>(null);
  const [aiLoading, setAiLoading] = useState(false);
  const [aiError, setAiError] = useState(false);
  const [showCommitModal, setShowCommitModal] = useState(false);

  const transport = createConnectTransport({
    baseUrl: getApiBaseUrl(),
    interceptors: [createAuthInterceptor()],
  });
  const client = createClient(UnfinishedWorkService, transport);

  const hasStats = worktree.changedFiles > 0 || worktree.linesAdded > 0 || worktree.linesRemoved > 0;
  const noChanges = !worktree.hasUncommitted && worktree.commitsAhead === 0;

  const handleOpenSession = useCallback(() => {
    if (worktree.sessionId) {
      router.push(routes.sessionDetail(worktree.sessionId));
    } else {
      // Navigate to home with new session params for this worktree
      router.push(
        `/?new=true&worktree=${encodeURIComponent(worktree.worktreePath)}&branch=${encodeURIComponent(worktree.branch)}`
      );
    }
  }, [router, worktree]);

  const handleViewFiles = useCallback(() => {
    // Navigate to home with file browser opened at the worktree path
    router.push(`/?files=${encodeURIComponent(worktree.worktreePath)}`);
  }, [router, worktree]);

  const handleSummarize = useCallback(async () => {
    if (aiSummary !== null) return; // already cached
    setAiLoading(true);
    setAiError(false);
    try {
      const req = create(GetWorktreeAISummaryRequestSchema, {
        repoPath: worktree.repoPath,
        branch: worktree.branch,
      });
      const res = await client.getWorktreeAISummary(req);
      setAiSummary(res.summary);
    } catch {
      setAiError(true);
    } finally {
      setAiLoading(false);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [worktree.repoPath, worktree.branch, aiSummary]);

  return (
    <div className={styles.detail}>
      {/* Diff stats */}
      {noChanges ? (
        <p className={styles.noChanges}>No uncommitted changes</p>
      ) : (
        <div className={styles.statsRow}>
          {hasStats && (
            <>
              <span className={styles.statItem}>
                {worktree.changedFiles} file{worktree.changedFiles !== 1 ? "s" : ""} changed
              </span>
              {worktree.linesAdded > 0 && (
                <span className={`${styles.statItem} ${styles.added}`}>
                  +{worktree.linesAdded}
                </span>
              )}
              {worktree.linesRemoved > 0 && (
                <span className={`${styles.statItem} ${styles.removed}`}>
                  -{worktree.linesRemoved}
                </span>
              )}
            </>
          )}
        </div>
      )}

      {/* Ahead commit messages */}
      {worktree.aheadCommitMessages.length > 0 && (
        <ul className={styles.commitList} aria-label="Commits ahead of default branch">
          {worktree.aheadCommitMessages.map((msg, i) => (
            <li key={i} className={styles.commitItem} title={msg}>
              {msg}
            </li>
          ))}
        </ul>
      )}

      {/* Action buttons */}
      <div className={styles.actionRow}>
        <button className={styles.btnPrimary} onClick={handleOpenSession}>
          {worktree.sessionId ? "Reattach Session" : "Open Session"}
        </button>
        <button className={styles.btn} onClick={() => setShowCommitModal(true)}>
          Commit &amp; Push
        </button>
        <button className={styles.btn} onClick={handleViewFiles}>
          View Files
        </button>
        <button
          className={styles.btn}
          onClick={handleSummarize}
          disabled={aiLoading}
          aria-label="Generate AI summary of changes"
        >
          {aiLoading ? "Summarizing…" : "Summarize"}
        </button>
      </div>

      {/* AI summary area */}
      {aiLoading && (
        <div className={styles.summaryBox}>
          <span className={styles.spinner} aria-label="Generating summary" /> Generating summary…
        </div>
      )}
      {aiError && (
        <div className={`${styles.summaryBox} ${styles.summaryError}`}>
          Summary unavailable — try again.
        </div>
      )}
      {aiSummary !== null && !aiLoading && !aiError && (
        <div className={styles.summaryBox}>{aiSummary}</div>
      )}

      {/* Commit & Push modal */}
      {showCommitModal && (
        <CommitPushModal
          repoPath={worktree.repoPath}
          branch={worktree.branch}
          onClose={() => setShowCommitModal(false)}
        />
      )}
    </div>
  );
}
