// +feature: unfinished-backlog-queue
"use client";

import { useState, useCallback, useEffect, useId, useRef } from "react";
import { createPortal } from "react-dom";
import Link from "next/link";
import { useBacklogService, type BacklogItem, type GitHubIssue } from "@/lib/hooks/useBacklogService";
import { GitHubIssuePicker } from "@/components/backlog/GitHubIssuePicker";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import * as styles from "./BacklogQueueSection.css";

const QUEUED_STATUSES = ["idea", "refining", "ready"] as const;

function statusLabel(status: string): string {
  switch (status) {
    case "idea":
      return "Idea";
    case "refining":
      return "Refining";
    case "ready":
      return "Ready";
    default:
      return status;
  }
}

interface QueueCardProps {
  item: BacklogItem;
}

function QueueCard({ item }: QueueCardProps) {
  return (
    <Link
      href={`/backlog?item=${encodeURIComponent(item.id)}`}
      className={styles.card}
      data-testid="backlog-queue-card"
    >
      <span className={styles.cardTitle}>{item.title}</span>
      <span className={styles.priorityChip}>P{item.priority}</span>
      <span className={styles.statusChip}>{statusLabel(item.status)}</span>
    </Link>
  );
}

/**
 * "Up Next" section on the Unfinished page: surfaces queued backlog items
 * (idea/refining/ready) and lets the user import a GitHub issue as a new
 * task, mirroring the collapsible-section pattern used by GitHubPRsSection.
 */
export function BacklogQueueSection() {
  const { listBacklogItems, importGitHubIssue } = useBacklogService();
  const [items, setItems] = useState<BacklogItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isOpen, setIsOpen] = useState(true);
  const [showImport, setShowImport] = useState(false);
  const headingId = useId();
  const importDialogRef = useRef<HTMLDivElement>(null);
  // No Escape-to-close: pre-existing behavior, unchanged by this fix (which
  // scopes to wiring the Tab trap, not adding new dismiss affordances) — use
  // the dialog's own Cancel button to close it.
  useFocusTrap(importDialogRef, showImport);

  // ponytail: request-id guard, not AbortController — listBacklogItems has no signal param
  const loadRequestIdRef = useRef(0);
  const load = useCallback(async () => {
    const requestId = ++loadRequestIdRef.current;
    setLoading(true);
    setError(null);
    try {
      const result = await listBacklogItems({ statuses: [...QUEUED_STATUSES] });
      if (loadRequestIdRef.current === requestId) setItems(result);
    } catch {
      if (loadRequestIdRef.current === requestId) setError("Failed to load queued backlog items.");
    } finally {
      if (loadRequestIdRef.current === requestId) setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const toggleOpen = useCallback(() => setIsOpen((v) => !v), []);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      toggleOpen();
    }
  };

  // ponytail: generic failure message, same pattern as app/backlog/page.tsx's
  // handlePickerSelect — the hook's `lastError` state updates asynchronously,
  // so reading it synchronously right after this await would race with it.
  const handlePickerSelect = useCallback(
    async (owner: string, repo: string, issues: GitHubIssue[]) => {
      setShowImport(false);
      let successCount = 0;
      for (const issue of issues) {
        const url = issue.url || `https://github.com/${owner}/${repo}/issues/${issue.number}`;
        const result = await importGitHubIssue(url);
        if (result) successCount++;
      }
      if (successCount > 0) await load();
      if (successCount < issues.length) {
        const failed = issues.length - successCount;
        setError(
          issues.length === 1
            ? "Failed to import GitHub issue. Pull requests can't be imported as backlog items."
            : `Imported ${successCount} of ${issues.length} issues — ${failed} failed. Pull requests can't be imported as backlog items.`
        );
      }
    },
    [importGitHubIssue, load]
  );

  return (
    <section className={styles.section} aria-label="Up Next">
      <div
        role="button"
        tabIndex={0}
        className={styles.sectionHeader}
        onClick={toggleOpen}
        onKeyDown={handleKeyDown}
        aria-expanded={isOpen}
        aria-controls="backlog-queue-list"
      >
        <span
          className={`${styles.chevron} ${isOpen ? styles.chevronExpanded : ""}`}
          aria-hidden="true"
        >
          ▶
        </span>
        <span className={styles.sectionTitle}>Up Next</span>
        <span className={styles.badge}>{items.length}</span>
        <button
          type="button"
          className={styles.importButton}
          onClick={(e) => {
            e.stopPropagation();
            setShowImport(true);
          }}
          onKeyDown={(e) => e.stopPropagation()}
          data-testid="import-github-issue-button"
        >
          + Import GitHub Issue
        </button>
      </div>

      {isOpen && (
        <div id="backlog-queue-list">
          {error ? (
            <div className={styles.errorBox}>{error}</div>
          ) : loading ? (
            <div className={styles.empty}>Loading…</div>
          ) : items.length === 0 ? (
            <div className={styles.empty}>
              No queued backlog items. Import a GitHub issue to get started.
            </div>
          ) : (
            <div className={styles.list}>
              {items.map((item) => (
                <QueueCard key={item.id} item={item} />
              ))}
            </div>
          )}
        </div>
      )}

      {showImport &&
        typeof document !== "undefined" &&
        createPortal(
          <div className={styles.overlay} data-testid="backlog-queue-import-modal">
            <div ref={importDialogRef} role="dialog" aria-modal="true" aria-labelledby={headingId} className={styles.dialog}>
              <h2 id={headingId} className={styles.dialogHeading}>
                Import GitHub Issue
              </h2>
              <GitHubIssuePicker onSelect={handlePickerSelect} onCancel={() => setShowImport(false)} />
            </div>
          </div>,
          document.body
        )}
    </section>
  );
}
