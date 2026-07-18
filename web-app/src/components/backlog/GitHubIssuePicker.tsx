// +feature: backlog:github-issue-picker
"use client";

import { useCallback, useRef, useEffect, useState, Fragment } from "react";
import { useBacklogService, type GitHubIssue } from "@/lib/hooks/useBacklogService";
import { useGitHubIssuePicker } from "@/lib/hooks/useGitHubIssuePicker";
import * as styles from "./GitHubIssuePicker.css";

// ─── Helpers ─────────────────────────────────────────────────────────────────

function highlightMatch(text: string, query: string): React.ReactNode {
  const q = query.trim().toLowerCase();
  if (!q) return text;
  const idx = text.toLowerCase().indexOf(q);
  if (idx === -1) return text;
  return (
    <>
      {text.slice(0, idx)}
      <mark className={styles.matchHighlight}>{text.slice(idx, idx + q.length)}</mark>
      {text.slice(idx + q.length)}
    </>
  );
}

function relativeTime(iso?: string): string {
  if (!iso) return "";
  const diff = Date.now() - new Date(iso).getTime();
  const s = Math.floor(diff / 1000);
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  if (d < 30) return `${d}d ago`;
  const mo = Math.floor(d / 30);
  if (mo < 12) return `${mo}mo ago`;
  return `${Math.floor(mo / 12)}y ago`;
}

// ─── Props ────────────────────────────────────────────────────────────────────

interface GitHubIssuePickerProps {
  onSelect: (owner: string, repo: string, issue: GitHubIssue) => void;
  onCancel: () => void;
}

// ─── Component ────────────────────────────────────────────────────────────────

export function GitHubIssuePicker({ onSelect, onCancel }: GitHubIssuePickerProps) {
  const { searchGitHubRepos, listGitHubIssues } = useBacklogService();

  const picker = useGitHubIssuePicker({ searchGitHubRepos, listGitHubIssues, onSelect });

  const searchRef = useRef<HTMLInputElement>(null);

  // Focus input when phase changes.
  useEffect(() => {
    searchRef.current?.focus();
  }, [picker.phase]);

  // Two-level Escape: issue → repo → onCancel.
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.preventDefault();
      if (picker.phase === "issue") {
        picker.goBack();
      } else {
        onCancel();
      }
    },
    [picker, onCancel]
  );

  if (picker.authError) {
    return (
      <div className={styles.container}>
        <div className={styles.authErrorBox}>
          No GitHub token configured. Set <code>GITHUB_TOKEN</code> or <code>GH_TOKEN</code> to
          enable the GitHub issue picker.
        </div>
        <div style={{ display: "flex", justifyContent: "flex-end" }}>
          <button type="button" onClick={onCancel} className={styles.backButton}>
            Close
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className={styles.container} onKeyDown={handleKeyDown}>
      {picker.phase === "repo" ? (
        <RepoPhase picker={picker} searchRef={searchRef} />
      ) : (
        <IssuePhase picker={picker} searchRef={searchRef} />
      )}
      <div style={{ display: "flex", justifyContent: "flex-end" }}>
        <button type="button" onClick={onCancel} className={styles.backButton}>
          Cancel
        </button>
      </div>
    </div>
  );
}

// ─── Repo phase ───────────────────────────────────────────────────────────────

function RepoPhase({
  picker,
  searchRef,
}: {
  picker: ReturnType<typeof useGitHubIssuePicker>;
  searchRef: React.RefObject<HTMLInputElement | null>;
}) {
  const [selectedIndex, setSelectedIndex] = useState(-1);
  const listRef = useRef<HTMLDivElement>(null);
  const repos = picker.repos;

  // Reset selection when the repo list changes (query update or initial load).
  useEffect(() => {
    setSelectedIndex(-1);
  }, [repos]);

  // Scroll keyboard-selected item into view.
  useEffect(() => {
    if (selectedIndex < 0 || !listRef.current) return;
    const items = listRef.current.querySelectorAll<HTMLElement>('[role="option"]');
    items[selectedIndex]?.scrollIntoView({ block: "nearest" });
  }, [selectedIndex]);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (repos.length === 0) return;
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setSelectedIndex((i) => Math.min(i + 1, repos.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSelectedIndex((i) => Math.max(i - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      const idx = selectedIndex >= 0 ? selectedIndex : 0;
      const repo = repos[idx];
      if (repo) picker.selectRepo(repo);
    }
  };

  return (
    <>
      <div className={styles.phaseHeader}>
        <span style={{ fontSize: "13px", fontWeight: 600 }}>Select a repository</span>
      </div>
      <input
        ref={searchRef}
        className={styles.searchInput}
        type="text"
        placeholder="Search repos…"
        value={picker.repoQuery}
        onChange={(e) => picker.setRepoQuery(e.target.value)}
        onKeyDown={handleKeyDown}
        aria-label="Search GitHub repositories"
        aria-autocomplete="list"
        aria-controls="repo-list"
        autoComplete="off"
        autoFocus
      />
      <div id="repo-list" ref={listRef} role="listbox" aria-label="GitHub repositories" className={styles.listContainer}>
        {picker.reposLoading ? (
          <div className={styles.loadingText}>Loading…</div>
        ) : repos.length === 0 ? (
          <div className={styles.emptyState}>
            {picker.repoQuery ? "No repos found." : "No repos available."}
          </div>
        ) : (
          repos.map((repo, idx) => (
            <Fragment key={`${repo.owner}/${repo.repo}`}>
              {idx === picker.repoHistoryCount && picker.repoHistoryCount > 0 && (
                <div className={styles.historyDivider} role="separator" />
              )}
              <div
                role="option"
                aria-selected={idx === selectedIndex}
                className={
                  idx === selectedIndex
                    ? `${styles.listItem} ${styles.listItemSelected}`
                    : styles.listItem
                }
                onMouseDown={(e) => {
                  e.preventDefault();
                  picker.selectRepo(repo);
                }}
                onMouseEnter={() => setSelectedIndex(idx)}
              >
                {idx < picker.repoHistoryCount && (
                  <span className={styles.historyIcon}>🕒</span>
                )}
                <span className={styles.listItemName}>
                  {highlightMatch(`${repo.owner}/${repo.repo}`, picker.repoQuery)}
                </span>
                {repo.isLocal && <span className={styles.localBadge}>local</span>}
                {repo.description && (
                  <span className={styles.listItemMeta}>
                    {highlightMatch(repo.description, picker.repoQuery)}
                  </span>
                )}
              </div>
            </Fragment>
          ))
        )}
      </div>
    </>
  );
}

// ─── Issue phase ──────────────────────────────────────────────────────────────

function IssuePhase({
  picker,
  searchRef,
}: {
  picker: ReturnType<typeof useGitHubIssuePicker>;
  searchRef: React.RefObject<HTMLInputElement | null>;
}) {
  const states: Array<"open" | "closed" | "all"> = ["open", "closed", "all"];

  return (
    <>
      <div className={styles.phaseHeader}>
        <button
          type="button"
          className={styles.backButton}
          onClick={picker.goBack}
          aria-label="Back to repository selection"
        >
          ← Back
        </button>
        <span className={styles.repoChip} aria-label="Selected repository">
          {picker.selectedRepo?.owner}/{picker.selectedRepo?.repo}
        </span>
      </div>

      <div className={styles.filterBar}>
        <input
          ref={searchRef}
          className={styles.searchInput}
          type="search"
          placeholder="Search issues…"
          value={picker.issueSearch}
          onChange={(e) => picker.setIssueSearch(e.target.value)}
          aria-label="Search issues"
          autoComplete="off"
          style={{ flex: 1 }}
          autoFocus
        />
        <div className={styles.stateToggle} role="group" aria-label="Issue state filter">
          {states.map((s) => (
            <button
              key={s}
              type="button"
              className={picker.issueState === s ? styles.stateButton.active : styles.stateButton.inactive}
              onMouseDown={(e) => {
                e.preventDefault();
                picker.setIssueState(s);
              }}
            >
              {s}
            </button>
          ))}
        </div>
      </div>

      <div role="listbox" aria-label="GitHub issues" className={styles.listContainer}>
        {picker.issuesLoading ? (
          <div className={styles.loadingText}>Loading…</div>
        ) : picker.issues.length === 0 ? (
          <div className={styles.emptyState}>
            {picker.issueSearch ? "No issues match your search." : "No issues found."}
          </div>
        ) : (
          picker.issues.map((issue) => (
            <IssueRow key={issue.number} issue={issue} onSelect={picker.selectIssue} />
          ))
        )}
      </div>
    </>
  );
}

// ─── Issue row ────────────────────────────────────────────────────────────────

function IssueRow({ issue, onSelect }: { issue: GitHubIssue; onSelect: (i: GitHubIssue) => void }) {
  const age = relativeTime(issue.updatedAt || issue.createdAt);
  return (
    <div
      role="option"
      aria-selected={false}
      className={styles.listItem}
      onMouseDown={(e) => {
        e.preventDefault();
        onSelect(issue);
      }}
    >
      <span className={issue.isPR ? styles.prTypeBadge : styles.issueTypeBadge}>
        {issue.isPR ? "PR" : "#"}
      </span>
      <span className={styles.issueNumber}>{issue.number}</span>
      <span className={styles.listItemName}>{issue.title}</span>
      {issue.labels.slice(0, 2).map((label) => (
        <span key={label} className={styles.labelBadge} title={label}>
          {label}
        </span>
      ))}
      {age && <span className={styles.relativeDate}>{age}</span>}
    </div>
  );
}
