import { usePersistedViewState, type PersistedFieldsConfig } from "@/lib/hooks/usePersistedViewState";
import type { BacklogItem, BacklogItemStatus } from "@/lib/hooks/useBacklogService";

// ---------------------------------------------------------------------------
// Shared backlog filter state — used by both the list view (page.tsx) and the
// board view (BacklogBoard.tsx) so that filtering a status/priority chip in
// one view is reflected in the other. Sort and group-by remain list-only
// (see .backlog-context.md AC 6) and are NOT part of this shared state.
// ---------------------------------------------------------------------------

export const ALL_STATUSES: BacklogItemStatus[] = [
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

export interface BacklogFilterState {
  search: string;
  statusFilter: BacklogItemStatus[];
  priorityFilter: number[];
  showArchived: boolean;
}

// Keys and validators must exactly match the fields previously inlined in
// page.tsx's BACKLOG_VIEW_FIELDS so both views read/write the same
// localStorage entries (AC 3).
export const BACKLOG_FILTER_FIELDS: PersistedFieldsConfig<BacklogFilterState> = {
  search: { key: "stapler-squad-backlog-search", defaultValue: "" },
  statusFilter: {
    key: "stapler-squad-backlog-status-filter",
    defaultValue: [],
    isValid: (v) => Array.isArray(v) && v.every((s) => ALL_STATUSES.includes(s as BacklogItemStatus)),
  },
  priorityFilter: {
    key: "stapler-squad-backlog-priority-filter",
    defaultValue: [],
    isValid: (v) => Array.isArray(v) && v.every((n) => typeof n === "number"),
  },
  showArchived: {
    key: "stapler-squad-backlog-show-archived",
    defaultValue: false,
    isValid: (v) => typeof v === "boolean",
  },
};

export function useBacklogFilters() {
  return usePersistedViewState<BacklogFilterState>(BACKLOG_FILTER_FIELDS);
}

export function filterBacklogItems(items: BacklogItem[], filters: BacklogFilterState): BacklogItem[] {
  const q = filters.search.trim().toLowerCase();
  return items.filter((item) => {
    if (!filters.showArchived && item.status === "archived") return false;
    if (filters.statusFilter.length > 0 && !filters.statusFilter.includes(item.status)) return false;
    if (filters.priorityFilter.length > 0 && !filters.priorityFilter.includes(item.priority)) return false;
    if (q && !item.title.toLowerCase().includes(q) && !(item.description ?? "").toLowerCase().includes(q)) {
      return false;
    }
    return true;
  });
}
