"use client";

import { useAnalytics } from "@/lib/analytics";
import { getStatusLabel } from "@/lib/backlog/status";
import { ALL_STATUSES } from "@/lib/hooks/useBacklogFilters";
import type { BacklogItemStatus } from "@/lib/hooks/useBacklogService";
import * as styles from "./BacklogFilterBar.css";

export type BacklogGroupBy = "none" | "repoPath";

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
            onClick={() => { track({ name: "backlog_filter_status", category: "user_action", component: "BacklogFilterBar", labels: { status, active: String(!selected.includes(status)) } }); toggle(status); }}
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
            onClick={() => { track({ name: "backlog_filter_priority", category: "user_action", component: "BacklogFilterBar", labels: { priority: String(p), active: String(!selected.includes(p)) } }); toggle(p); }}
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

export interface BacklogFilterBarProps {
  search: string;
  onSearchChange: (v: string) => void;
  statusFilter: BacklogItemStatus[];
  onStatusFilterChange: (v: BacklogItemStatus[]) => void;
  priorityFilter: number[];
  onPriorityFilterChange: (v: number[]) => void;
  showArchived: boolean;
  onShowArchivedChange: (v: boolean) => void;
  onResetView: () => void;
  /** Sort/group-by controls are list-view only (see AC 6) — the board hides them. */
  showSortGroupControls: boolean;
  groupBy?: BacklogGroupBy;
  onGroupByChange?: (v: BacklogGroupBy) => void;
  /**
   * The board view excludes archived items unconditionally (stageOf() always
   * returns null for them), so the "Show Archived" checkbox does nothing
   * there — default true (list view keeps it), board passes false.
   */
  showArchivedControl?: boolean;
}

export function BacklogFilterBar({
  search,
  onSearchChange,
  statusFilter,
  onStatusFilterChange,
  priorityFilter,
  onPriorityFilterChange,
  showArchived,
  onShowArchivedChange,
  onResetView,
  showSortGroupControls,
  groupBy,
  onGroupByChange,
  showArchivedControl = true,
}: BacklogFilterBarProps) {
  return (
    <div className={styles.filterBar} role="search" aria-label="Filter backlog items">
      <input
        type="search"
        className={styles.searchInput}
        placeholder="Search by title…"
        value={search}
        onChange={(e) => onSearchChange(e.target.value)}
        aria-label="Search backlog items"
        data-testid="backlog-search-input"
      />
      <StatusFilterChips selected={statusFilter} onChange={onStatusFilterChange} />
      <PriorityFilterChips selected={priorityFilter} onChange={onPriorityFilterChange} />
      {/* Archived items are excluded from the default view client-side
          (Epic 5.1); enabling this re-includes them from the live store. */}
      {showArchivedControl && (
        <label className={styles.showArchivedLabel}>
          <input
            type="checkbox"
            checked={showArchived}
            onChange={(e) => onShowArchivedChange(e.target.checked)}
            aria-label="Show archived items"
            data-testid="backlog-show-archived-toggle"
          />
          Show Archived
        </label>
      )}
      {showSortGroupControls && groupBy !== undefined && onGroupByChange && (
        <label className={styles.groupByLabel}>
          Group by:{" "}
          <select
            className={styles.groupBySelect}
            value={groupBy}
            onChange={(e) => onGroupByChange(e.target.value as BacklogGroupBy)}
            aria-label="Group by"
            data-testid="backlog-group-by-select"
          >
            <option value="none">None</option>
            <option value="repoPath">Repository</option>
          </select>
        </label>
      )}
      <button
        type="button"
        className={styles.resetViewButton}
        onClick={onResetView}
        aria-label="Reset view"
        data-testid="backlog-reset-view-button"
      >
        Reset view
      </button>
    </div>
  );
}
