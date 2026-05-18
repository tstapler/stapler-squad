"use client";
// +feature: backlog:list-page

import { useState, useEffect, useCallback, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useAnalytics } from "@/lib/analytics";
import { usePageView } from "@/lib/analytics/usePageView";
import { AppLink } from "@/components/ui/AppLink";
import { BacklogItemDetail } from "@/components/backlog/BacklogItemDetail";
import { BacklogItemForm } from "@/components/backlog/BacklogItemForm";
import { BacklogEmptyState, FilterZeroState, FooterNudge } from "@/components/backlog/BacklogEmptyState";
import {
  useBacklogService,
  type BacklogItem,
  type BacklogItemStatus,
  type BacklogItemInput,
} from "@/lib/hooks/useBacklogService";
import * as styles from "./backlog.css";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type SortColumn = "title" | "status" | "priority" | "updatedAt";

const ALL_STATUSES: BacklogItemStatus[] = [
  "idea",
  "ready",
  "in_progress",
  "review",
  "done",
  "archived",
];

const STATUS_LABELS: Record<BacklogItemStatus, string> = {
  idea: "Idea",
  ready: "Ready",
  in_progress: "In Progress",
  review: "Review",
  done: "Done",
  archived: "Archived",
};

const STATUS_CSS: Record<BacklogItemStatus, string> = {
  idea: styles.statusIdea,
  ready: styles.statusReady,
  in_progress: styles.statusInProgress,
  review: styles.statusReview,
  done: styles.statusDone,
  archived: styles.statusArchived,
};

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
            {STATUS_LABELS[status]}
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
  const service = useBacklogService();
  const router = useRouter();
  const searchParams = useSearchParams();

  const selectedItemId = searchParams.get("item");

  const [items, setItems] = useState<BacklogItem[]>([]);
  const [loading, setLoading] = useState(true);

  // Filters
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<BacklogItemStatus[]>([]);
  const [priorityFilter, setPriorityFilter] = useState<number[]>([]);

  // Sort
  const [sortCol, setSortCol] = useState<SortColumn>("updatedAt");
  const [sortAsc, setSortAsc] = useState(false);

  // New-item modal
  const [showForm, setShowForm] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const result = await service.listBacklogItems({
        statuses: statusFilter.length > 0 ? statusFilter : undefined,
        priorities: priorityFilter.length > 0 ? priorityFilter : undefined,
        search: search.trim() || undefined,
      });
      setItems(result);
    } finally {
      setLoading(false);
    }
  }, [service, statusFilter, priorityFilter, search]);

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
    }
    return sortAsc ? cmp : -cmp;
  });

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
      await service.createBacklogItem(data);
      setShowForm(false);
      await load();
    },
    [service, load]
  );

  const sortIndicator = (col: SortColumn) => {
    if (sortCol !== col) return null;
    return sortAsc ? " ↑" : " ↓";
  };

  return (
    <div className={styles.pageWrapper} data-testid="backlog-page">
      {/* Header */}
      <div className={styles.pageHeader}>
        <h1 className={styles.pageTitle}>Backlog</h1>
        <div className={styles.headerActions}>
          <button
            className={styles.newItemButton}
            onClick={() => { track({ name: "backlog_new_item", category: "user_action", component: "BacklogPage" }); setShowForm(true); }}
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
      </div>

      {/* Content */}
      <div className={styles.contentArea}>
        <div className={styles.listPane}>
          {loading ? (
            <div role="status" aria-label="Loading backlog items" style={{ padding: "32px", textAlign: "center", color: "inherit", opacity: 0.6 }}>
              Loading…
            </div>
          ) : sortedItems.length === 0 && items.length === 0 ? (
            <BacklogEmptyState onCreateItem={handleCreateItem} />
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
                </tr>
              </thead>
              <tbody>
                {sortedItems.map((item) => {
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
                          className={`${styles.statusBadge} ${STATUS_CSS[item.status]}`}
                          aria-label={`Status: ${STATUS_LABELS[item.status]}`}
                        >
                          {STATUS_LABELS[item.status]}
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
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
          {items.length > 0 && !items.some((i) => i.status === "in_progress") && <FooterNudge />}
        </div>

        {/* Detail pane */}
        {selectedItemId && (
          <aside className={styles.detailPane} aria-label="Item detail">
            <BacklogItemDetail
              itemId={selectedItemId}
              onClose={handleDetailClose}
            />
          </aside>
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
            if (e.target === e.currentTarget) setShowForm(false);
          }}
          data-testid="backlog-form-modal"
        >
          <div className={styles.modalBox} onClick={(e) => e.stopPropagation()}>
            <h2 className={styles.modalTitle}>New Backlog Item</h2>
            <BacklogItemForm
              onSubmit={handleCreateItem}
              onCancel={() => setShowForm(false)}
            />
          </div>
        </div>
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
