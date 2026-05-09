"use client";

import { useState, useCallback, useRef, useEffect } from "react";
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
import { WorktreeDiffModal } from "./WorktreeDiffModal";
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
  const [showDiffModal, setShowDiffModal] = useState(false);
  const [showSessionPicker, setShowSessionPicker] = useState(false);
  const pickerRef = useRef<HTMLDivElement>(null);

  // Close picker on outside click
  useEffect(() => {
    if (!showSessionPicker) return;
    function handleClick(e: MouseEvent) {
      if (pickerRef.current && !pickerRef.current.contains(e.target as Node)) {
        setShowSessionPicker(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [showSessionPicker]);

  const transport = createConnectTransport({
    baseUrl: getApiBaseUrl(),
    interceptors: [createAuthInterceptor()],
  });
  const client = createClient(UnfinishedWorkService, transport);

  const hasStats = worktree.changedFiles > 0 || worktree.linesAdded > 0 || worktree.linesRemoved > 0;
  const noChanges = !worktree.hasUncommitted && worktree.commitsAhead === 0;

  const handleOpenSession = useCallback(() => {
    if (worktree.sessionIds.length > 1) {
      setShowSessionPicker((v) => !v);
    } else if (worktree.sessionIds.length === 1) {
      router.push(routes.sessionDetail(worktree.sessionIds[0]));
    } else {
      router.push(routes.newSessionFromWorktree(worktree.worktreePath, worktree.branch, worktree.branch));
    }
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
        <div className={styles.sessionBtnWrapper} ref={pickerRef}>
          <button className={styles.btnPrimary} onClick={handleOpenSession} aria-expanded={showSessionPicker}>
            {worktree.sessionIds.length > 1
              ? `Reattach Session (${worktree.sessionIds.length})`
              : worktree.sessionIds.length === 1
              ? "Reattach Session"
              : "Open Session"}
          </button>
          {showSessionPicker && (
            <div className={styles.sessionPicker} role="listbox" aria-label="Select session">
              {worktree.sessionIds.map((id, i) => (
                <button
                  key={id}
                  className={styles.sessionPickerItem}
                  role="option"
                  aria-selected={false}
                  onClick={() => {
                    setShowSessionPicker(false);
                    router.push(routes.sessionDetail(id));
                  }}
                >
                  Session {i + 1}
                  <span className={styles.sessionPickerIdHint}>{id.slice(0, 8)}</span>
                </button>
              ))}
            </div>
          )}
        </div>
        <button className={styles.btn} onClick={() => setShowDiffModal(true)}>
          View Diff
        </button>
        <button className={styles.btn} onClick={() => setShowCommitModal(true)}>
          Commit &amp; Push
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

      {showDiffModal && (
        <WorktreeDiffModal
          repoPath={worktree.repoPath}
          branch={worktree.branch}
          repoName={worktree.repoName || worktree.branch}
          onClose={() => setShowDiffModal(false)}
        />
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
