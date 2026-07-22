"use client";
// +feature: backlog:list-page

import { useState, useEffect, useCallback, useRef, Suspense } from "react";
import { resizeHandle as resizeHandleCss } from "@/styles/pane/resizeHandle.css";
import { useRouter, useSearchParams } from "next/navigation";
import { useAnalytics } from "@/lib/analytics";
import { usePageView } from "@/lib/analytics/usePageView";
import { AppLink } from "@/components/ui/AppLink";
import { BacklogItemDetail } from "@/components/backlog/BacklogItemDetail";
import { BacklogItemForm } from "@/components/backlog/BacklogItemForm";
import { BacklogEmptyState, FilterZeroState, FooterNudge } from "@/components/backlog/BacklogEmptyState";
import { VaguenessPromptModal } from "@/components/backlog/VaguenessPromptModal";
import { BacklogTourModal } from "@/components/backlog/BacklogTourModal";
import { useBacklogTour } from "@/components/backlog/useBacklogTour";
import { GitHubIssuePicker } from "@/components/backlog/GitHubIssuePicker";
import {
  useBacklogService,
  type BacklogItem,
  type BacklogItemStatus,
  type BacklogItemInput,
} from "@/lib/hooks/useBacklogService";
import { getStatusLabel } from "@/lib/backlog/status";
import { compareByRepoPath, groupByRepoPath } from "@/lib/backlog/sortGroup";
import * as styles from "./backlog.css";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type SortColumn = "title" | "status" | "priority" | "updatedAt" | "repoPath";
type GroupBy = "none" | "repoPath";

const ALL_STATUSES: BacklogItemStatus[] = [
  "idea",
  "refining",
  "ready",
  "queued",
  "in_progress",
  "review",
  "pr_pending",
  "done",
  "archived",
];

const STATUS_CSS: Record<string, string> = {
  idea: styles.statusIdea,
  refining: styles.statusRefining,
  ready: styles.statusReady,
  queued: styles.statusQueued,
  in_progress: styles.statusInProgress,
  review: styles.statusReview,
  pr_pending: styles.statusReview,
  done: styles.statusDone,
  archived: styles.statusArchived,
};

const getStatusClass = (s: string): string => STATUS_CSS[s] ?? styles.statusArchived;

const PRIORITY_LABELS: Record<number, string> = {
  1: "P1",
  2: "P2",
  3: "P3",
  4: "P4",
  5: "P5",
};

function formatDateShort(iso?: string): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function StatusFilterChips({
  selected,
  onChange,
}: {
  selected: BacklogItemStatus[];
  onChange: (s: BacklogItemStatus[]) => void;
}) {
  const { track } = useAnalytics();
  const toggle = (status: BacklogItemStatus) => {
    const next = selected.includes(status)
      ? selected.filter((s) => s !== status)
      : [...selected, status];
    onChange(next);
  };

  // Exclude "archived" from default chips (too noisy)
  const displayStatuses = ALL_STATUSES.filter((s) => s !== "archived");

  return (
    <div className={styles.filterChipGroup} role="group" aria-label="Filter by status">
      {displayStatuses.map((status) => {
        const active = selected.includes(status);
        return (
          <button
            key={status}
            type="button"
            className={`${styles.filterChip} ${active ? styles.filterChipActive : ""}`}
            onClick={() => { track({ name: "backlog_filter_status", category: "user_action", component: "BacklogPage", labels: { status, active: String(!selected.includes(status)) } }); toggle(status); }}
            aria-pressed={active}
            data-testid={`backlog-filter-status-${status}`}
          >
            {getStatusLabel(status)}
          </button>
        );
      })}
    </div>
  );
}

function PriorityFilterChips({
  selected,
  onChange,
}: {
  selected: number[];
  onChange: (p: number[]) => void;
}) {
  const { track } = useAnalytics();
  const toggle = (p: number) => {
    const next = selected.includes(p)
      ? selected.filter((x) => x !== p)
      : [...selected, p];
    onChange(next);
  };

  return (
    <div className={styles.filterChipGroup} role="group" aria-label="Filter by priority">
      {[1, 2, 3, 4, 5].map((p) => {
        const active = selected.includes(p);
        return (
          <button
            key={p}
            type="button"
            className={`${styles.filterChip} ${active ? styles.filterChipActive : ""}`}
            onClick={() => { track({ name: "backlog_filter_priority", category: "user_action", component: "BacklogPage", labels: { priority: String(p), active: String(!selected.includes(p)) } }); toggle(p); }}
            aria-pressed={active}
            data-testid={`backlog-filter-priority-${p}`}
          >
            P{p}
          </button>
        );
      })}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

function BacklogPageInner() {
  usePageView();
  const { track } = useAnalytics();
  const { listBacklogItems, createBacklogItem, importGitHubIssue, triggerTriage } = useBacklogService();
  const router = useRouter();
  const searchParams = useSearchParams();

  const selectedItemId = searchParams.get("item");

  const [items, setItems] = useState<BacklogItem[]>([]);
  const [loading, setLoading] = useState(true);

  // Filters
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<BacklogItemStatus[]>([]);
  const [priorityFilter, setPriorityFilter] = useState<number[]>([]);
  // showArchived: archived items are excluded server-side by default (only
  // "done" is shown by default); enabling this re-fetches with
  // includeArchived=true. Mirrors SessionList's "Show Archived" toggle.
  const [showArchived, setShowArchived] = useState(false);

  // Sort
  const [sortCol, setSortCol] = useState<SortColumn>("updatedAt");
  const [sortAsc, setSortAsc] = useState(false);

  // Group by
  const [groupBy, setGroupBy] = useState<GroupBy>("none");

  // Detail pane resize
  const [detailWidth, setDetailWidth] = useState(420);
  const dragRef = useRef({ active: false, startX: 0, startWidth: 0 });

  const handleResizePointerDown = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    (e.currentTarget as HTMLDivElement).setPointerCapture(e.pointerId);
    dragRef.current = { active: true, startX: e.clientX, startWidth: detailWidth };
  }, [detailWidth]);

  const handleResizePointerMove = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!dragRef.current.active) return;
    const delta = dragRef.current.startX - e.clientX;
    setDetailWidth(Math.max(240, Math.min(800, dragRef.current.startWidth + delta)));
  }, []);

  const handleResizePointerUp = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    dragRef.current.active = false;
    (e.currentTarget as HTMLDivElement).releasePointerCapture(e.pointerId);
  }, []);

  // New-item modal
  const [showForm, setShowForm] = useState(false);
  const [formMode, setFormMode] = useState<"manual" | "github">("manual");
  const [githubIssueUrl, setGithubIssueUrl] = useState("");
  const [githubImporting, setGithubImporting] = useState(false);
  const [githubImportError, setGithubImportError] = useState<string | null>(null);

  // First-visit walkthrough
  const { showTour, setTourComplete, hideTour, resetTour } = useBacklogTour();

  // Vagueness prompt modal state
  const [vaguenessItem, setVaguenessItem] = useState<BacklogItem | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const result = await listBacklogItems({
        statuses: statusFilter.length > 0 ? statusFilter : undefined,
        priorities: priorityFilter.length > 0 ? priorityFilter : undefined,
        search: search.trim() || undefined,
        includeTerminal: true, // show done items by default; user can filter them out
        includeArchived: showArchived, // archived items are hidden by default — see the "Show Archived" toggle
      });
      setItems(result);
    } finally {
      setLoading(false);
    }
  }, [listBacklogItems, statusFilter, priorityFilter, search, showArchived]);

  useEffect(() => {
    void load();
  }, [load]);

  // Sort items client-side
  const sortedItems = [...items].sort((a, b) => {
    let cmp = 0;
    if (sortCol === "title") {
      cmp = a.title.localeCompare(b.title);
    } else if (sortCol === "status") {
      cmp = ALL_STATUSES.indexOf(a.status) - ALL_STATUSES.indexOf(b.status);
    } else if (sortCol === "priority") {
      cmp = a.priority - b.priority;
    } else if (sortCol === "updatedAt") {
      cmp = (a.updatedAt ?? "").localeCompare(b.updatedAt ?? "");
    } else if (sortCol === "repoPath") {
      cmp = compareByRepoPath(a, b);
    }
    return sortAsc ? cmp : -cmp;
  });

  // Grouping is applied after sorting so items within each group keep the
  // currently selected sort order (mirrors SessionList's sort-then-group).
  const groups = groupBy === "repoPath" ? groupByRepoPath(sortedItems) : null;

  const handleSortClick = (col: SortColumn) => {
    if (sortCol === col) {
      setSortAsc((prev) => !prev);
    } else {
      setSortCol(col);
      setSortAsc(false);
    }
  };

  const handleRowClick = (itemId: string) => {
    const params = new URLSearchParams(searchParams.toString());
    params.set("item", itemId);
    router.push(`/backlog?${params.toString()}`);
  };

  const handleDetailClose = () => {
    const params = new URLSearchParams(searchParams.toString());
    params.delete("item");
    const qs = params.toString();
    router.push(qs ? `/backlog?${qs}` : "/backlog");
  };

  const handleCreateItem = useCallback(
    async (data: BacklogItemInput) => {
      const result = await createBacklogItem(data);
      setShowForm(false);
      await load();
      // Show vagueness prompt if item was created with skip_triage=true
      if (result && data.skipTriage) {
        setVaguenessItem(result.item);
        // Navigate to the new item
        const params = new URLSearchParams(searchParams.toString());
        params.set("item", result.item.id);
        router.push(`/backlog?${params.toString()}`);
      } else if (result) {
        const params = new URLSearchParams(searchParams.toString());
        params.set("item", result.item.id);
        router.push(`/backlog?${params.toString()}`);
      }
    },
    [createBacklogItem, load, router, searchParams]
  );

  const handleImportGitHubIssue = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!githubIssueUrl.trim()) return;
      setGithubImportError(null);
      setGithubImporting(true);
      try {
        const result = await importGitHubIssue(githubIssueUrl.trim());
        if (result) {
          setShowForm(false);
          setGithubIssueUrl("");
          await load();
          const params = new URLSearchParams(searchParams.toString());
          params.set("item", result.item.id);
          router.push(`/backlog?${params.toString()}`);
        } else {
          setGithubImportError("Import failed. Check the URL and try again.");
        }
      } catch (err) {
        setGithubImportError(err instanceof Error ? err.message : "Import failed.");
      } finally {
        setGithubImporting(false);
      }
    },
    [githubIssueUrl, importGitHubIssue, load, router, searchParams]
  );

  const handlePickerSelect = useCallback(
    async (owner: string, repo: string, issue: { number: number; title: string; url: string }) => {
      const url = issue.url || `https://github.com/${owner}/${repo}/issues/${issue.number}`;
      setShowForm(false);
      const result = await importGitHubIssue(url.trim());
      if (result) {
        await load();
        const params = new URLSearchParams(searchParams.toString());
        params.set("item", result.item.id);
        router.push(`/backlog?${params.toString()}`);
      }
    },
    [importGitHubIssue, load, router, searchParams]
  );

  const sortIndicator = (col: SortColumn) => {
    if (sortCol !== col) return null;
    return sortAsc ? " ↑" : " ↓";
  };

  const renderItemRow = (item: BacklogItem) => {
    const acDone = item.acCriteria.filter((c) => c.status === "done").length;
    const isActive = selectedItemId === item.id;
    return (
      <tr
        key={item.id}
        className={`${styles.tableRow} ${isActive ? styles.tableRowActive : ""}`}
        tabIndex={0}
        role="row"
        aria-selected={isActive}
        data-testid="backlog-table-row"
        data-item-id={item.id}
        onClick={() => handleRowClick(item.id)}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            handleRowClick(item.id);
          }
        }}
      >
        <td className={`${styles.tableCell} ${styles.titleCell}`}>
          {item.title}
        </td>
        <td className={styles.tableCell}>
          <span
            className={`${styles.statusBadge} ${getStatusClass(item.status)}`}
            aria-label={`Status: ${getStatusLabel(item.status)}`}
            data-testid={item.status === "queued" ? "backlog-status-queued" : undefined}
          >
            {getStatusLabel(item.status)}
          </span>
        </td>
        <td className={styles.tableCell}>
          <span
            className={styles.priorityBadge}
            data-testid="priority-badge"
            aria-label={`Priority: ${PRIORITY_LABELS[item.priority] ?? "Unknown"}`}
          >
            {PRIORITY_LABELS[item.priority] ?? "P?"}
          </span>
        </td>
        <td className={`${styles.tableCell} ${styles.acProgressCell}`}>
          {item.acCriteria.length > 0
            ? `${acDone}/${item.acCriteria.length}`
            : "—"}
        </td>
        <td className={styles.tableCell} style={{ whiteSpace: "nowrap" }}>
          {formatDateShort(item.updatedAt)}
        </td>
        <td className={`${styles.tableCell} ${styles.repoPathCell}`} data-testid="backlog-repo-path-cell">
          {item.repoPath || "—"}
        </td>
      </tr>
    );
  };

  return (
    <div className={styles.pageWrapper} data-testid="backlog-page">
      {/* Header */}
      <div className={styles.pageHeader}>
        <h1 className={styles.pageTitle}>Backlog</h1>
        <div className={styles.headerActions}>
          <button
            className={styles.helpButton}
            onClick={() => { track({ name: "backlog_open_tour", category: "user_action", component: "BacklogPage" }); resetTour(); }}
            aria-label="How this page works"
            data-testid="backlog-tour-button"
          >
            ?
          </button>
          <button
            className={styles.newItemButton}
            onClick={() => { track({ name: "backlog_new_item", category: "user_action", component: "BacklogPage" }); setFormMode("manual"); setShowForm(true); }}
            aria-label="Create new backlog item"
            data-testid="backlog-new-item-button"
          >
            + New Item
          </button>
        </div>
      </div>

      {/* Tab Bar */}
      <nav className={styles.tabBar} aria-label="Backlog views">
        <button
          type="button"
          className={`${styles.tab} ${styles.tabActive}`}
          aria-current="page"
          data-testid="backlog-tab-list"
        >
          List
        </button>
        <AppLink
          href="/backlog/board"
          className={styles.tab}
          data-testid="backlog-tab-board"
        >
          Board
        </AppLink>
      </nav>

      {/* Filter Bar */}
      <div className={styles.filterBar} role="search" aria-label="Filter backlog items">
        <input
          type="search"
          className={styles.searchInput}
          placeholder="Search by title…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          aria-label="Search backlog items"
          data-testid="backlog-search-input"
        />
        <StatusFilterChips selected={statusFilter} onChange={setStatusFilter} />
        <PriorityFilterChips selected={priorityFilter} onChange={setPriorityFilter} />
        {/* Archived items are excluded from the default view server-side;
            enabling this re-fetches with includeArchived=true. */}
        <label className={styles.showArchivedLabel}>
          <input
            type="checkbox"
            checked={showArchived}
            onChange={(e) => setShowArchived(e.target.checked)}
            aria-label="Show archived items"
            data-testid="backlog-show-archived-toggle"
          />
          Show Archived
        </label>
        <label className={styles.groupByLabel}>
          Group by:{" "}
          <select
            className={styles.groupBySelect}
            value={groupBy}
            onChange={(e) => setGroupBy(e.target.value as GroupBy)}
            aria-label="Group by"
            data-testid="backlog-group-by-select"
          >
            <option value="none">None</option>
            <option value="repoPath">Repository</option>
          </select>
        </label>
      </div>

      {/* Content */}
      <div className={styles.contentArea}>
        <div className={styles.listPane}>
          {loading ? (
            <div role="status" aria-label="Loading backlog items" style={{ padding: "32px", textAlign: "center", color: "inherit", opacity: 0.6 }}>
              Loading…
            </div>
          ) : sortedItems.length === 0 && items.length === 0 ? (
            <BacklogEmptyState onCreateItem={() => { setFormMode("manual"); setShowForm(true); }} />
          ) : sortedItems.length === 0 ? (
            <FilterZeroState onClearFilters={() => { setStatusFilter([]); setPriorityFilter([]); setSearch(""); }} />
          ) : (
            <table className={styles.table} aria-label="Backlog items">
              <thead className={styles.tableHead}>
                <tr>
                  <th
                    scope="col"
                    className={styles.tableHeaderCell}
                    onClick={() => handleSortClick("title")}
                    style={{ cursor: "pointer" }}
                    aria-sort={sortCol === "title" ? (sortAsc ? "ascending" : "descending") : "none"}
                  >
                    Title{sortIndicator("title")}
                  </th>
                  <th
                    scope="col"
                    className={styles.tableHeaderCell}
                    onClick={() => handleSortClick("status")}
                    style={{ cursor: "pointer" }}
                    aria-sort={sortCol === "status" ? (sortAsc ? "ascending" : "descending") : "none"}
                  >
                    Status{sortIndicator("status")}
                  </th>
                  <th
                    scope="col"
                    className={styles.tableHeaderCell}
                    onClick={() => handleSortClick("priority")}
                    style={{ cursor: "pointer" }}
                    aria-sort={sortCol === "priority" ? (sortAsc ? "ascending" : "descending") : "none"}
                  >
                    Priority{sortIndicator("priority")}
                  </th>
                  <th scope="col" className={styles.tableHeaderCell}>
                    AC
                  </th>
                  <th
                    scope="col"
                    className={styles.tableHeaderCell}
                    onClick={() => handleSortClick("updatedAt")}
                    style={{ cursor: "pointer" }}
                    aria-sort={sortCol === "updatedAt" ? (sortAsc ? "ascending" : "descending") : "none"}
                  >
                    Updated{sortIndicator("updatedAt")}
                  </th>
                  <th
                    scope="col"
                    className={styles.tableHeaderCell}
                    onClick={() => handleSortClick("repoPath")}
                    style={{ cursor: "pointer" }}
                    aria-sort={sortCol === "repoPath" ? (sortAsc ? "ascending" : "descending") : "none"}
                    data-testid="backlog-col-repo-path"
                  >
                    Repository{sortIndicator("repoPath")}
                  </th>
                </tr>
              </thead>
              {groups ? (
                groups.map((group) => (
                  <tbody key={group.groupKey}>
                    <tr>
                      <td
                        colSpan={6}
                        className={styles.groupHeaderCell}
                        data-testid="backlog-group-header"
                      >
                        {group.displayName} ({group.items.length})
                      </td>
                    </tr>
                    {group.items.map((item) => renderItemRow(item))}
                  </tbody>
                ))
              ) : (
                <tbody>{sortedItems.map((item) => renderItemRow(item))}</tbody>
              )}
            </table>
          )}
          {items.length > 0 && !items.some((i) => i.status === "in_progress") && <FooterNudge />}
        </div>

        {/* Detail pane */}
        {selectedItemId && (
          <>
            <div
              className={resizeHandleCss({ direction: "vertical" })}
              style={{ touchAction: "none" }}
              aria-label="Resize detail panel"
              onPointerDown={handleResizePointerDown}
              onPointerMove={handleResizePointerMove}
              onPointerUp={handleResizePointerUp}
              onPointerCancel={handleResizePointerUp}
            />
            <aside
              className={styles.detailPane}
              style={{ width: detailWidth }}
              aria-label="Item detail"
            >
              <BacklogItemDetail
                itemId={selectedItemId}
                onClose={handleDetailClose}
              />
            </aside>
          </>
        )}
      </div>

      {/* New Item Modal */}
      {showForm && (
        <div
          className={styles.modalOverlay}
          role="dialog"
          aria-modal="true"
          aria-label="Create new backlog item"
          onClick={(e) => {
            if (e.target === e.currentTarget) { setShowForm(false); setGithubIssueUrl(""); setGithubImportError(null); }
          }}
          data-testid="backlog-form-modal"
        >
          <div className={styles.modalBox} onClick={(e) => e.stopPropagation()}>
            <h2 className={styles.modalTitle}>New Backlog Item</h2>
            {/* Mode toggle */}
            <div style={{ display: "flex", gap: "8px", marginBottom: "16px" }}>
              <button
                type="button"
                onClick={() => setFormMode("manual")}
                style={{
                  padding: "6px 16px",
                  borderRadius: "6px",
                  border: "1px solid",
                  cursor: "pointer",
                  fontSize: "13px",
                  fontWeight: formMode === "manual" ? 600 : 400,
                  background: formMode === "manual" ? "var(--primary)" : "transparent",
                  color: formMode === "manual" ? "var(--primary-text)" : "var(--text-secondary)",
                  borderColor: formMode === "manual" ? "var(--primary)" : "var(--border-color)",
                }}
              >
                Manual
              </button>
              <button
                type="button"
                onClick={() => setFormMode("github")}
                style={{
                  padding: "6px 16px",
                  borderRadius: "6px",
                  border: "1px solid",
                  cursor: "pointer",
                  fontSize: "13px",
                  fontWeight: formMode === "github" ? 600 : 400,
                  background: formMode === "github" ? "var(--primary)" : "transparent",
                  color: formMode === "github" ? "var(--primary-text)" : "var(--text-secondary)",
                  borderColor: formMode === "github" ? "var(--primary)" : "var(--border-color)",
                }}
              >
                Import from GitHub Issue
              </button>
            </div>

            {formMode === "manual" ? (
              <BacklogItemForm
                onSubmit={handleCreateItem}
                onCancel={() => setShowForm(false)}
              />
            ) : (
              <GitHubIssuePicker
                onSelect={handlePickerSelect}
                onCancel={() => { setShowForm(false); setGithubIssueUrl(""); setGithubImportError(null); }}
              />
            )}

            {false && (
              <form onSubmit={handleImportGitHubIssue} style={{ display: "flex", flexDirection: "column", gap: "16px" }}>
                <div style={{ display: "flex", flexDirection: "column", gap: "6px" }}>
                  <input
                    type="url"
                    value={githubIssueUrl}
                    onChange={(e) => setGithubIssueUrl(e.target.value)}
                    placeholder="https://github.com/owner/repo/issues/123"
                    required
                  />
                </div>
                {githubImportError && (
                  <p style={{ fontSize: "12px", color: "var(--error)", margin: 0 }}>{githubImportError}</p>
                )}
                <div style={{ display: "flex", gap: "8px", justifyContent: "flex-end" }}>
                  <button
                    type="submit"
                    disabled={githubImporting || !githubIssueUrl.trim()}
                  >
                    {githubImporting ? "Importing…" : "Import Issue"}
                  </button>
                </div>
              </form>
            )}
          </div>
        </div>
      )}

      {/* First-visit walkthrough */}
      <BacklogTourModal
        isOpen={showTour}
        onComplete={(persist) => (persist ? setTourComplete() : hideTour())}
      />

      {/* Vagueness Prompt Modal */}
      {vaguenessItem && (
        <VaguenessPromptModal
          itemTitle={vaguenessItem.title}
          onRefine={() => {
            setVaguenessItem(null);
            setFormMode("manual");
            setShowForm(true);
          }}
          onProceed={() => {
            const item = vaguenessItem;
            setVaguenessItem(null);
            void triggerTriage(item.id);
          }}
        />
      )}
    </div>
  );
}

export default function BacklogPage() {
  return (
    <Suspense>
      <BacklogPageInner />
    </Suspense>
  );
}
