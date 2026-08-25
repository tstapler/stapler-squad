"use client";
// +feature: review-queue-pr-creation
// +feature: review-queue-severity-sort-filter

import { useState, useEffect, useMemo, useRef, useCallback } from "react";
import { createPortal } from "react-dom";
import { create } from "@bufbuild/protobuf";
import { useReviewQueueContext } from "@/lib/contexts/ReviewQueueContext";
import { useApprovalsContext } from "@/lib/contexts/ApprovalsContext";
import { useSessionServiceContext } from "@/lib/contexts/SessionServiceContext";
import { useReviewQueueNavigation } from "@/lib/hooks/useReviewQueueNavigation";
import { useGenerateRule } from "@/lib/hooks/useGenerateRule";
import { useFilterState } from "@/lib/hooks/useFilterState";
import { GroupingStrategy, GroupingStrategyLabels, groupSessions } from "@/lib/grouping/strategies";
import { parseGitHubRef } from "@/lib/github/urlParser";
import { ReviewQueueBadge } from "./ReviewQueueBadge";
import { SuggestedRuleCard } from "./SuggestedRuleCard";
import { CreatePullRequestModal } from "./CreatePullRequestModal";
import { Priority, AttentionReason, ReviewItem, WorkingState, SuggestionSource, Session, SessionSchema } from "@/gen/session/v1/types_pb";
import { deriveWorkingState } from "@/lib/utils/deriveWorkingState";
import type { EscalationCategory } from "@/lib/sessions/escalationCategory";
import { riskLevelRank } from "@/lib/sessions/riskLevel";
import { SeverityBadge, getRiskLevelInfo } from "./SeverityBadge";
import {
  panel,
  header,
  titleRow,
  title,
  count,
  refreshButton,
  stats,
  stat,
  filters,
  filterGroup,
  filterLabel,
  filterButtons,
  filterButton,
  filterButtonActive,
  filterButtonExcluded,
  items as itemsClass,
  item,
  itemClickable,
  currentItem,
  itemActions,
  itemHeader,
  itemTitle,
  itemBody,
  itemContext,
  escalationReasonText,
  commandPreview,
  expiredBadge,
  itemPattern,
  sessionDetails,
  detailRow,
  detailLabel,
  detailValue,
  tags,
  tag,
  itemFooter,
  itemAge,
  diffStats,
  diffAdded,
  diffRemoved,
  loading as loadingClass,
  empty as emptyClass,
  error as errorClass,
  emptySubtext,
  completionState,
  completionIcon,
  retryButton,
  visuallyHidden,
  oldestCallout,
  newItemsBanner,
  filterToggleRow,
  filterToggle,
  filterToggleActive,
  filterClear,
  autoAdvanceToggle,
  savedIndicator,
  modalOverlay,
  ruleModalContent,
  divergedBadge,
  searchInput,
  sortRow,
  sortSelect,
  groupSection,
  groupHeading,
} from "./ReviewQueuePanel.css";
import { Button } from "@/components/ui";

interface ReviewQueuePanelProps {
  onSessionClick?: (sessionId: string) => void;
  onSkipSession?: (sessionId: string) => Promise<void>;
  autoRefresh?: boolean;
  refreshInterval?: number;
  onItemsChange?: (items: ReviewItem[]) => void; // Callback to expose queue items for navigation
  onAcknowledged?: (sessionId: string) => void; // Notifies parent when a session is acknowledged (for auto-advance)
  autoAdvance?: boolean;
  onAutoAdvanceChange?: (value: boolean) => void;
}

/**
 * ReviewQueuePanel displays all sessions that need user attention.
 *
 * Shows items sorted by priority with filtering capabilities.
 * Uses hybrid push/poll strategy for real-time updates:
 * - WebSocket push notifications for immediate session status changes
 * - 30-second fallback polling to catch any missed events
 *
 * @example
 * ```tsx
 * <ReviewQueuePanel
 *   onSessionClick={(id) => navigateToSession(id)}
 *   autoRefresh={true}
 *   refreshInterval={5000}
 * />
 * ```
 */

type SortField = "default" | "severity" | "priority" | "age" | "diffSize" | "name";

// URL query param keys, persisted/restored via useFilterState for shareable/bookmarkable filter state.
// The `*Exclude` keys hold the exclude side of each dimension's include/exclude/neutral cycle.
const FILTER_URL_KEYS = [
  "priority",
  "priorityExclude",
  "reason",
  "reasonExclude",
  "severity",
  "severityExclude",
  "program",
  "programExclude",
  "category",
  "categoryExclude",
  "tag",
  "tagExclude",
  "pr",
  "diverged",
  "q",
  "sort",
  "dir",
  "group",
] as const;

// Grouping strategies that map onto fields ReviewItem actually carries (no project/workflow/session-type data).
const REVIEW_GROUPING_STRATEGIES = [
  GroupingStrategy.None,
  GroupingStrategy.Category,
  GroupingStrategy.Tag,
  GroupingStrategy.Branch,
  GroupingStrategy.Program,
  GroupingStrategy.Status,
];

const SORT_FIELDS: SortField[] = ["severity", "priority", "age", "diffSize", "name"];

// The baseline default sort (AC3: severity-first). Referenced everywhere "severity is the
// no-filter-applied sort" matters (initial state fallback, URL suppression, clear-all reset,
// active-filter-count exemption) so a future change to the default only needs one edit.
const DEFAULT_SORT_FIELD: SortField = "severity";

// Sentinel Set/URL member for "risk_level metadata key absent" — kept distinct from the
// literal empty string since parseStrSet/joinSet drop empty strings on the comma-joined
// URL round trip. Never confused with a real RiskLevel value.
const UNRECORDED_SEVERITY = "unrecorded";

// The 4 known RiskLevel values plus UNRECORDED_SEVERITY, in default-sort-order — drives the
// severity filter chip set. Unrecorded gets its own chip (design/ux.md Surface 3) rather than
// being omitted, since an unrecorded-severity item must still be reachable via the filter UI.
const SEVERITY_FILTER_VALUES = ["critical", "high", "medium", "low", UNRECORDED_SEVERITY] as const;

// The known, filterable RiskLevel values (SEVERITY_FILTER_VALUES minus the sentinel) —
// derived once so severityFilterKey and SEVERITY_FILTER_VALUES can't drift apart.
const KNOWN_RISK_LEVELS = new Set<string>(SEVERITY_FILTER_VALUES.filter((v) => v !== UNRECORDED_SEVERITY));

// Buckets any value outside the known RiskLevel set (absent, "", or an unrecognized future
// value) into UNRECORDED_SEVERITY, so every item is always reachable through one of the
// SEVERITY_FILTER_VALUES chips — a future/unrecognized risk_level can't become its own
// unfilterable key (PR #411 review finding).
function severityFilterKey(riskLevel: string | undefined): string {
  return riskLevel && KNOWN_RISK_LEVELS.has(riskLevel) ? riskLevel : UNRECORDED_SEVERITY;
}

// Category -> emoji prefix for the escalation reason line (WCAG 1.4.1 — not color-only).
// No "secret-scan" entry: that category never reaches a ReviewItem (requirements.md
// out-of-scope note). An unrecognized/missing category falls through to no emoji via `?? ""`.
const ESCALATION_REASON_EMOJI: Partial<Record<EscalationCategory, string>> = {
  "no-match": "❓",
  "explicit-rule": "🛑",
  "domain-age": "🌐",
  "unclassifiable": "⚙️",
  "unexpected": "⚠️",
};

// Which EscalationCategory values (PR #315) disqualify the Create Rule button — a Record,
// not a Set, so TypeScript forces every member of the union to be listed here; adding a 6th
// category to EscalationCategory (web-app/src/lib/sessions/escalationCategory.ts, which mirrors
// pkg/classifier/escalation.go) without updating this map is a compile error, not a silent
// fail-open drift. An orphaned pre-deploy approval (escalation_reason_category key entirely
// absent because it predates this field) and any wholly unrecognized string are both treated as
// eligible (fail-open by design) — see backlog 5fb93d9d.
const CREATE_RULE_INELIGIBLE_CATEGORIES: Record<EscalationCategory, boolean> = {
  "no-match": false,
  "explicit-rule": true,
  "domain-age": true,
  "secret-scan": true,
  "unclassifiable": true,
  "unexpected": true,
};

function isKnownEscalationCategory(value: string): value is EscalationCategory {
  return Object.prototype.hasOwnProperty.call(CREATE_RULE_INELIGIBLE_CATEGORIES, value);
}

export function isCreateRuleEligibleCategory(category: string | undefined): boolean {
  if (category === undefined || !isKnownEscalationCategory(category)) {
    return true;
  }
  return !CREATE_RULE_INELIGIBLE_CATEGORIES[category];
}

function joinSet(set: Set<string> | Set<number>): string | undefined {
  return set.size > 0 ? [...set].join(",") : undefined;
}

function parseNumSet(v: string | undefined): Set<number> {
  return new Set(
    v
      ? v
          .split(",")
          .filter(Boolean)
          .map(Number)
          .filter((n) => Number.isFinite(n))
      : []
  );
}

function parseStrSet(v: string | undefined): Set<string> {
  return new Set(v ? v.split(",").filter(Boolean) : []);
}

// Resolves the initial sortField from the URL. "default" (natural queue order) is a valid,
// explicitly-selectable SortField that SORT_FIELDS deliberately excludes (it's the pre-AC3
// baseline this feature replaced) — checked first so an old bookmarked `?sort=default` URL
// still round-trips. Any other missing/unrecognized value falls back to DEFAULT_SORT_FIELD.
function resolveInitialSortField(urlSort: string | undefined): SortField {
  if (urlSort === "default") return "default";
  if (urlSort && (SORT_FIELDS as string[]).includes(urlSort)) return urlSort as SortField;
  return DEFAULT_SORT_FIELD;
}

// Minimal Session shape for groupSessions() — only the fields grouping strategies read.
function reviewItemToSession(item: ReviewItem): Session {
  return create(SessionSchema, {
    id: item.sessionId,
    title: item.sessionName,
    path: item.path,
    branch: item.branch,
    status: item.status,
    program: item.program,
    tags: item.tags,
    category: item.category,
  });
}

function toggleInSet<T>(set: Set<T>, value: T): Set<T> {
  const next = new Set(set);
  if (next.has(value)) {
    next.delete(value);
  } else {
    next.add(value);
  }
  return next;
}

// Cycles a single value through neutral -> include -> exclude -> neutral across a paired
// include/exclude Set for one filter dimension. Each Set stays mutually exclusive for any
// given value (a value is never in both at once).
function cycleFilterValue<T>(
  include: Set<T>,
  exclude: Set<T>,
  value: T
): { include: Set<T>; exclude: Set<T> } {
  const nextInclude = new Set(include);
  const nextExclude = new Set(exclude);
  if (include.has(value)) {
    nextInclude.delete(value);
    nextExclude.add(value);
  } else if (exclude.has(value)) {
    nextExclude.delete(value);
  } else {
    nextInclude.add(value);
  }
  return { include: nextInclude, exclude: nextExclude };
}

// Counts distinct non-empty values of `pick(item)` (string or string[]) across items, sorted by frequency desc.
function countByField(items: ReviewItem[], pick: (item: ReviewItem) => string | string[]): [string, number][] {
  const counts = new Map<string, number>();
  for (const item of items) {
    const values = pick(item);
    for (const v of Array.isArray(values) ? values : [values]) {
      if (!v) continue;
      counts.set(v, (counts.get(v) ?? 0) + 1);
    }
  }
  return [...counts.entries()].sort((a, b) => b[1] - a[1]);
}

export function ReviewQueuePanel({
  onSessionClick,
  onSkipSession,
  autoRefresh = true,
  refreshInterval = 5000,
  onItemsChange,
  onAcknowledged,
  autoAdvance,
  onAutoAdvanceChange,
}: ReviewQueuePanelProps) {
  // Epic 2.4: Create PR modal state — holds the sessionId whose modal is open (null = closed),
  // mirroring SessionActionsOverflow.tsx's isCreatePrOpen/createPrTriggerRef pattern (Epic 2.3)
  // so both entry points open the identical shared CreatePullRequestModal (ux.md Surface 1 & 2).
  const [isCreatePrOpen, setIsCreatePrOpen] = useState<string | null>(null);
  const createPrTriggerRef = useRef<HTMLElement | null>(null);
  const { draftPullRequest, createPullRequest } = useSessionServiceContext();

  // Epic 4: Create Rule modal state
  // activeRuleItemId tracks which item's "Create Rule" modal is currently open.
  const [activeRuleItemId, setActiveRuleItemId] = useState<string | null>(null);
  const [ruleSaved, setRuleSaved] = useState(false);
  const { suggestions, loading: ruleLoading, error: ruleError, generate: generateRule, clear: clearRule } = useGenerateRule();
  // Filter/sort/group state is persisted to URL query params (shareable/bookmarkable),
  // seeded once from the URL on mount; local state is the source of truth thereafter.
  const { filterState: urlFilters, setFilter: setUrlFilter, clearFilters: clearUrlFilters } = useFilterState(FILTER_URL_KEYS);

  // Combinable multi-select filters — each dimension is a Set; empty Set = "no filter applied".
  // Each also has a paired `*Exclude` Set (neutral -> include -> exclude -> neutral cycle,
  // see cycleFilterValue) so a value can be explicitly hidden rather than only included.
  const [priorityFilter, setPriorityFilter] = useState<Set<Priority>>(() => parseNumSet(urlFilters.priority) as Set<Priority>);
  const [priorityExcludeFilter, setPriorityExcludeFilter] = useState<Set<Priority>>(() => parseNumSet(urlFilters.priorityExclude) as Set<Priority>);
  const [reasonFilter, setReasonFilter] = useState<Set<AttentionReason>>(() => parseNumSet(urlFilters.reason) as Set<AttentionReason>);
  const [reasonExcludeFilter, setReasonExcludeFilter] = useState<Set<AttentionReason>>(() => parseNumSet(urlFilters.reasonExclude) as Set<AttentionReason>);
  const [severityFilter, setSeverityFilter] = useState<Set<string>>(() => parseStrSet(urlFilters.severity));
  const [severityExcludeFilter, setSeverityExcludeFilter] = useState<Set<string>>(() => parseStrSet(urlFilters.severityExclude));
  const [programFilter, setProgramFilter] = useState<Set<string>>(() => parseStrSet(urlFilters.program));
  const [programExcludeFilter, setProgramExcludeFilter] = useState<Set<string>>(() => parseStrSet(urlFilters.programExclude));
  const [categoryFilter, setCategoryFilter] = useState<Set<string>>(() => parseStrSet(urlFilters.category));
  const [categoryExcludeFilter, setCategoryExcludeFilter] = useState<Set<string>>(() => parseStrSet(urlFilters.categoryExclude));
  const [tagFilter, setTagFilter] = useState<Set<string>>(() => parseStrSet(urlFilters.tag));
  const [tagExcludeFilter, setTagExcludeFilter] = useState<Set<string>>(() => parseStrSet(urlFilters.tagExclude));
  const [prFilter, setPrFilter] = useState<"all" | "has-pr" | "no-pr">(() =>
    urlFilters.pr === "has-pr" || urlFilters.pr === "no-pr" ? urlFilters.pr : "all"
  );
  const [divergedOnly, setDivergedOnly] = useState(() => urlFilters.diverged === "1");
  const [searchText, setSearchText] = useState(() => urlFilters.q ?? "");
  // Default sort is "severity" (highest risk first, AC3) — "default" (natural
  // server/queue order) remains a selectable option but is no longer the fallback when no
  // sort param is present in the URL.
  const [sortField, setSortField] = useState<SortField>(() => resolveInitialSortField(urlFilters.sort));
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">(() => (urlFilters.dir === "desc" ? "desc" : "asc"));
  const [groupingStrategy, setGroupingStrategy] = useState<GroupingStrategy>(() =>
    urlFilters.group && REVIEW_GROUPING_STRATEGIES.includes(urlFilters.group as GroupingStrategy)
      ? (urlFilters.group as GroupingStrategy)
      : GroupingStrategy.None
  );
  const [isFiltersOpen, setIsFiltersOpen] = useState(false);
  // Track whether queue ever had items so we can show "all done" vs generic empty state
  const [hadItems, setHadItems] = useState(false);

  // Live region announcement text for screen readers
  const [liveAnnouncement, setLiveAnnouncement] = useState('');
  // Prevent announcement on initial mount
  const hasMountedRef = useRef(false);
  // "Saved" flash for auto-advance toggle
  const [autoAdvanceSaved, setAutoAdvanceSaved] = useState(false);

  const {
    items: allItems,
    totalItems,
    loading,
    error,
    byPriority,
    byReason,
    averageAgeSeconds,
    oldestAgeSeconds,
    refresh,
    acknowledgeSession,
  } = useReviewQueueContext();

  // ─── Snapshot-on-enter pattern ────────────────────────────────────────────
  // Captures the session IDs present when the user enters the queue.
  // New items arriving while reviewing appear in a banner rather than being
  // injected mid-list, preventing queue jumps during triage (Twitter-style).
  const [reviewingIdsSnapshot, setReviewingIdsSnapshot] = useState<Set<string> | null>(null);

  // Initialize snapshot when the queue first loads with items
  useEffect(() => {
    if (reviewingIdsSnapshot === null && allItems.length > 0) {
      setReviewingIdsSnapshot(new Set(allItems.map((item) => item.sessionId)));
    }
  }, [allItems, reviewingIdsSnapshot]);

  // Remove acknowledged/resolved items from snapshot (forward-only — no re-injection)
  useEffect(() => {
    if (reviewingIdsSnapshot === null) return;
    const liveIds = new Set(allItems.map((item) => item.sessionId));
    const pruned = new Set([...reviewingIdsSnapshot].filter((id) => liveIds.has(id)));
    if (pruned.size !== reviewingIdsSnapshot.size) {
      setReviewingIdsSnapshot(pruned);
    }
  }, [allItems, reviewingIdsSnapshot]);

  const refreshSnapshot = useCallback(() => {
    setReviewingIdsSnapshot(new Set(allItems.map((item) => item.sessionId)));
    refresh();
  }, [allItems, refresh]);
  // ─────────────────────────────────────────────────────────────────────────

  // Separate working sessions from waiting sessions for count display.
  // Items with INPUT_REQUIRED or APPROVAL_PENDING reason need user action — exclude from "working" count.
  const workingCount = useMemo(
    () =>
      allItems.filter((item) => {
        const ws = deriveWorkingState(item);
        return (
          (ws === WorkingState.ACTIVE || ws === WorkingState.PROCESSING) &&
          item.reason !== AttentionReason.INPUT_REQUIRED &&
          item.reason !== AttentionReason.APPROVAL_PENDING
        );
      }).length,
    [allItems]
  );
  const stuckCount = useMemo(
    () =>
      allItems.filter(
        (item) => deriveWorkingState(item) === WorkingState.WAITING
      ).length,
    [allItems]
  );

  // Apply client-side filtering to all live items, excluding actively-working sessions
  // so the queue only shows sessions that need user attention.
  // Exception: INPUT_REQUIRED and APPROVAL_PENDING items always show — they need immediate action
  // even though deriveWorkingState maps their subStatus to WorkingState.PROCESSING.
  const allFilteredItems = useMemo(() => {
    let filtered = allItems.filter((item) => {
      const ws = deriveWorkingState(item);
      if (ws === WorkingState.ACTIVE || ws === WorkingState.PROCESSING) {
        return (
          item.reason === AttentionReason.INPUT_REQUIRED ||
          item.reason === AttentionReason.APPROVAL_PENDING
        );
      }
      return true;
    });
    if (priorityFilter.size > 0) {
      filtered = filtered.filter((item) => priorityFilter.has(item.priority));
    }
    if (priorityExcludeFilter.size > 0) {
      filtered = filtered.filter((item) => !priorityExcludeFilter.has(item.priority));
    }
    if (reasonFilter.size > 0) {
      filtered = filtered.filter((item) => reasonFilter.has(item.reason));
    }
    if (reasonExcludeFilter.size > 0) {
      filtered = filtered.filter((item) => !reasonExcludeFilter.has(item.reason));
    }
    if (severityFilter.size > 0) {
      filtered = filtered.filter((item) => severityFilter.has(severityFilterKey(item.metadata?.["risk_level"])));
    }
    if (severityExcludeFilter.size > 0) {
      filtered = filtered.filter((item) => !severityExcludeFilter.has(severityFilterKey(item.metadata?.["risk_level"])));
    }
    if (programFilter.size > 0) {
      filtered = filtered.filter((item) => programFilter.has(item.program));
    }
    if (programExcludeFilter.size > 0) {
      filtered = filtered.filter((item) => !programExcludeFilter.has(item.program));
    }
    if (categoryFilter.size > 0) {
      filtered = filtered.filter((item) => categoryFilter.has(item.category));
    }
    if (categoryExcludeFilter.size > 0) {
      filtered = filtered.filter((item) => !categoryExcludeFilter.has(item.category));
    }
    if (tagFilter.size > 0) {
      filtered = filtered.filter((item) => item.tags.some((t) => tagFilter.has(t)));
    }
    if (tagExcludeFilter.size > 0) {
      filtered = filtered.filter((item) => !item.tags.some((t) => tagExcludeFilter.has(t)));
    }
    if (prFilter === "has-pr") {
      filtered = filtered.filter((item) => !!item.githubPrUrl);
    } else if (prFilter === "no-pr") {
      filtered = filtered.filter((item) => !item.githubPrUrl);
    }
    if (divergedOnly) {
      filtered = filtered.filter((item) => item.branchDivergedFromBase);
    }
    const search = searchText.trim().toLowerCase();
    if (search) {
      filtered = filtered.filter((item) =>
        [item.sessionName, item.context, item.patternName, item.branch, item.program]
          .some((field) => field?.toLowerCase().includes(search))
      );
    }

    if (sortField !== "default") {
      const dir = sortDirection === "asc" ? 1 : -1;
      filtered = [...filtered].sort((a, b) => {
        switch (sortField) {
          // ponytail: no snapshot-order-freeze needed here (unlike this file's general
          // reviewingIdsSnapshot pattern) — risk_level is captured once at approval
          // creation and never re-derived (server/services/approval_store.go), so an
          // already-visible item's severity rank cannot change mid-review. Membership in
          // `items` is still frozen to the snapshot as usual; only a genuinely new item
          // (excluded until the snapshot is manually refreshed) could shift severity order.
          case "severity":
            return (riskLevelRank(b.metadata?.["risk_level"] ?? "") - riskLevelRank(a.metadata?.["risk_level"] ?? "")) * dir;
          case "priority":
            return (a.priority - b.priority) * dir;
          case "age":
            return (Number(a.lastActivity?.seconds ?? 0) - Number(b.lastActivity?.seconds ?? 0)) * dir;
          case "diffSize":
            return (
              ((a.diffStats?.added ?? 0) + (a.diffStats?.removed ?? 0)) -
              ((b.diffStats?.added ?? 0) + (b.diffStats?.removed ?? 0))
            ) * dir;
          case "name":
            return a.sessionName.localeCompare(b.sessionName) * dir;
          default:
            return 0;
        }
      });
    }

    return filtered;
  }, [
    allItems,
    priorityFilter,
    priorityExcludeFilter,
    reasonFilter,
    reasonExcludeFilter,
    severityFilter,
    severityExcludeFilter,
    programFilter,
    programExcludeFilter,
    categoryFilter,
    categoryExcludeFilter,
    tagFilter,
    tagExcludeFilter,
    prFilter,
    divergedOnly,
    searchText,
    sortField,
    sortDirection,
  ]);

  // Distinct values available for the Program/Category/Tag multi-select filters,
  // with counts computed from the full (unfiltered) queue.
  // Per-severity-value counts (including the UNRECORDED_SEVERITY bucket) for the filter
  // chip counts — computed client-side from the unfiltered queue, mirroring availablePrograms.
  const bySeverity = useMemo(() => {
    const counts = new Map<string, number>();
    for (const item of allItems) {
      const key = severityFilterKey(item.metadata?.["risk_level"]);
      counts.set(key, (counts.get(key) ?? 0) + 1);
    }
    return counts;
  }, [allItems]);

  const availablePrograms = useMemo(() => countByField(allItems, (i) => i.program), [allItems]);
  const availableCategories = useMemo(() => countByField(allItems, (i) => i.category), [allItems]);
  const availableTags = useMemo(() => countByField(allItems, (i) => i.tags), [allItems]);

  // Items that are in the snapshot (stable ordered list for the main queue)
  const items = useMemo(() => {
    if (reviewingIdsSnapshot === null) return allFilteredItems;
    return allFilteredItems.filter((item) => reviewingIdsSnapshot.has(item.sessionId));
  }, [allFilteredItems, reviewingIdsSnapshot]);

  // New items not yet in snapshot — shown in the refresh banner
  const newItemsCount = useMemo(() => {
    if (reviewingIdsSnapshot === null) return 0;
    return allFilteredItems.filter((item) => !reviewingIdsSnapshot.has(item.sessionId)).length;
  }, [allFilteredItems, reviewingIdsSnapshot]);

  // Position of each item within the flat `items` array — used so grouped rendering can still
  // highlight the keyboard-nav "current item" at its real index.
  const indexById = useMemo(() => new Map(items.map((it, i) => [it.sessionId, i])), [items]);

  // ReviewItem -> Session conversion cache for grouping, keyed on the stable unfiltered
  // `allItems` array (only changes on queue refresh) rather than the post-filter/sort `items`
  // array (changes on every keystroke/filter toggle). Avoids rebuilding protobuf Session
  // messages for the whole queue on every search keystroke when grouping is enabled.
  const sessionByItemId = useMemo(() => {
    const map = new Map<string, Session>();
    for (const item of allItems) {
      map.set(item.sessionId, reviewItemToSession(item));
    }
    return map;
  }, [allItems]);

  // Resolves the ReviewItem behind isCreatePrOpen into the full-enough Session object
  // CreatePullRequestModal requires — reuses the same reviewItemToSession bridge the
  // grouping strategies already rely on above, rather than building a second lookup.
  const activePrSession = isCreatePrOpen ? sessionByItemId.get(isCreatePrOpen) : undefined;

  // Reuses groupSessions() (the same grouping engine SessionList uses) by bridging each
  // ReviewItem to a minimal Session — avoids building a parallel grouping implementation.
  const groupedItems = useMemo(() => {
    if (groupingStrategy === GroupingStrategy.None) return null;
    const sessions = items.map((it) => sessionByItemId.get(it.sessionId) ?? reviewItemToSession(it));
    const groups = groupSessions(sessions, groupingStrategy);
    const bySessionId = new Map(items.map((it) => [it.sessionId, it]));
    return groups
      .map((g) => ({
        groupKey: g.groupKey,
        displayName: g.displayName,
        items: g.sessions.map((s) => bySessionId.get(s.id)).filter((it): it is ReviewItem => !!it),
      }))
      .filter((g) => g.items.length > 0);
  }, [items, groupingStrategy, sessionByItemId]);

  // Approval actions for APPROVAL_PENDING items
  const { approve: approveRequest, deny: denyRequest } = useApprovalsContext();

  // Keyboard navigation
  const { currentIndex, goToNext, goToPrevious } = useReviewQueueNavigation({
    items,
    onNavigate: (item, index) => {
      // Navigate to the selected session
      onSessionClick?.(item.sessionId);
    },
    enableKeyboardShortcuts: true,
  });

  // Notify parent component when queue items change (for navigation)
  useEffect(() => {
    if (onItemsChange) {
      onItemsChange(items);
    }
  }, [items, onItemsChange]);

  // Track if queue ever had items (for "all done" vs generic empty state)
  useEffect(() => {
    if (items.length > 0) {
      setHadItems(true);
    }
  }, [items.length]);

  // Update live announcement for screen readers when queue changes
  useEffect(() => {
    if (loading) return;
    if (!hasMountedRef.current) {
      hasMountedRef.current = true;
      return; // Skip announcement on initial mount
    }
    if (items.length === 0 && hadItems) {
      setLiveAnnouncement('Queue cleared. All items reviewed.');
    } else if (items.length > 0) {
      setLiveAnnouncement(`${items.length} ${items.length === 1 ? 'item' : 'items'} need attention.`);
    }
  }, [items.length, hadItems, loading]);

  // Format duration in seconds (e.g., averageAgeSeconds, oldestAgeSeconds)
  const formatDuration = (durationSeconds: bigint): string => {
    const duration = Number(durationSeconds);
    if (duration < 0 || duration > 31_536_000) return "Unknown"; // Cap at 1 year; guards clock skew / unit mismatch
    if (duration < 60) return `${duration}s`;
    if (duration < 3600) return `${Math.floor(duration / 60)}m`;
    if (duration < 86400) return `${Math.floor(duration / 3600)}h`;
    return `${Math.floor(duration / 86400)}d`;
  };

  // Format timestamp (seconds since epoch) as "time ago"
  const formatTimestamp = (timestampSeconds: bigint): string => {
    const timestamp = Number(timestampSeconds);
    if (timestamp === 0) return "never";

    const now = Math.floor(Date.now() / 1000);
    const age = now - timestamp;

    if (age < 0) return "in the future"; // Clock skew protection
    if (age < 60) return `${age}s`;
    if (age < 3600) return `${Math.floor(age / 60)}m`;
    if (age < 86400) return `${Math.floor(age / 3600)}h`;
    return `${Math.floor(age / 86400)}d`;
  };

  const getPriorityLabel = (priority: Priority): string => {
    switch (priority) {
      case Priority.URGENT:
        return "Urgent";
      case Priority.HIGH:
        return "High";
      case Priority.MEDIUM:
        return "Medium";
      case Priority.LOW:
        return "Low";
      default:
        return "All";
    }
  };

  const getReasonLabel = (reason: AttentionReason): string => {
    switch (reason) {
      case AttentionReason.APPROVAL_PENDING:
        return "Approval";
      case AttentionReason.INPUT_REQUIRED:
        return "Input";
      case AttentionReason.ERROR_STATE:
        return "Error";
      case AttentionReason.IDLE_TIMEOUT:
      case AttentionReason.IDLE:
        return "Idle";
      case AttentionReason.TASK_COMPLETE:
        return "Complete";
      case AttentionReason.STALE:
        return "Stale";
      case AttentionReason.WAITING_FOR_USER:
        return "Waiting";
      case AttentionReason.TESTS_FAILING:
        return "Tests Failing";
      default:
        return "All";
    }
  };

  // Six filter dimensions (priority/reason/severity/program/category/tag) each need an
  // identical include/exclude/neutral cycle handler: cycle the value via cycleFilterValue,
  // write both resulting Sets to state, and persist both to the URL. Factored into one
  // generic builder — called once per dimension below — instead of six near-identical
  // hand-written handlers (interface-pollution-checklist.md smell #5 doesn't apply here:
  // this generic has 6 real call sites, not 1).
  function makeFilterCycleHandler<T extends string | number>(
    include: Set<T>,
    exclude: Set<T>,
    setInclude: (s: Set<T>) => void,
    setExclude: (s: Set<T>) => void,
    includeKey: (typeof FILTER_URL_KEYS)[number],
    excludeKey: (typeof FILTER_URL_KEYS)[number]
  ): (value: T) => void {
    return (value: T) => {
      const next = cycleFilterValue(include, exclude, value);
      setInclude(next.include);
      setExclude(next.exclude);
      setUrlFilter(includeKey, joinSet(next.include as Set<string> | Set<number>));
      setUrlFilter(excludeKey, joinSet(next.exclude as Set<string> | Set<number>));
    };
  }

  const handleFilterByPriority = makeFilterCycleHandler(
    priorityFilter, priorityExcludeFilter, setPriorityFilter, setPriorityExcludeFilter, "priority", "priorityExclude"
  );

  const handleFilterByReason = makeFilterCycleHandler(
    reasonFilter, reasonExcludeFilter, setReasonFilter, setReasonExcludeFilter, "reason", "reasonExclude"
  );

  const handleFilterBySeverity = makeFilterCycleHandler(
    severityFilter, severityExcludeFilter, setSeverityFilter, setSeverityExcludeFilter, "severity", "severityExclude"
  );

  const handleFilterByProgram = makeFilterCycleHandler(
    programFilter, programExcludeFilter, setProgramFilter, setProgramExcludeFilter, "program", "programExclude"
  );

  const handleFilterByCategory = makeFilterCycleHandler(
    categoryFilter, categoryExcludeFilter, setCategoryFilter, setCategoryExcludeFilter, "category", "categoryExclude"
  );

  const handleFilterByTag = makeFilterCycleHandler(
    tagFilter, tagExcludeFilter, setTagFilter, setTagExcludeFilter, "tag", "tagExclude"
  );

  const handlePrFilterChange = (value: "all" | "has-pr" | "no-pr") => {
    setPrFilter(value);
    setUrlFilter("pr", value === "all" ? undefined : value);
  };

  const handleDivergedOnlyChange = (value: boolean) => {
    setDivergedOnly(value);
    setUrlFilter("diverged", value ? "1" : undefined);
  };

  // Local `searchText` state updates immediately so filtering stays responsive on every
  // keystroke, but the URL write (which triggers router.replace()) is debounced to avoid
  // a history/navigation update per character typed.
  const searchDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleSearchTextChange = (value: string) => {
    setSearchText(value);
    if (searchDebounceRef.current) {
      clearTimeout(searchDebounceRef.current);
    }
    searchDebounceRef.current = setTimeout(() => {
      setUrlFilter("q", value || undefined);
    }, 300);
  };

  useEffect(() => {
    return () => {
      if (searchDebounceRef.current) {
        clearTimeout(searchDebounceRef.current);
      }
    };
  }, []);

  const handleSortFieldChange = (value: SortField) => {
    setSortField(value);
    // DEFAULT_SORT_FIELD is omitted from the URL like "default" used to be, so a plain/
    // no-param URL still resolves to severity sort via resolveInitialSortField's fallback.
    setUrlFilter("sort", value === DEFAULT_SORT_FIELD ? undefined : value);
  };

  const handleSortDirectionChange = (value: "asc" | "desc") => {
    setSortDirection(value);
    setUrlFilter("dir", value === "asc" ? undefined : value);
  };

  const handleGroupingStrategyChange = (value: GroupingStrategy) => {
    setGroupingStrategy(value);
    setUrlFilter("group", value === GroupingStrategy.None ? undefined : value);
  };

  const clearAllFilters = useCallback(() => {
    if (searchDebounceRef.current) {
      clearTimeout(searchDebounceRef.current);
      searchDebounceRef.current = null;
    }
    setPriorityFilter(new Set());
    setPriorityExcludeFilter(new Set());
    setReasonFilter(new Set());
    setReasonExcludeFilter(new Set());
    setSeverityFilter(new Set());
    setSeverityExcludeFilter(new Set());
    setProgramFilter(new Set());
    setProgramExcludeFilter(new Set());
    setCategoryFilter(new Set());
    setCategoryExcludeFilter(new Set());
    setTagFilter(new Set());
    setTagExcludeFilter(new Set());
    setPrFilter("all");
    setDivergedOnly(false);
    setSearchText("");
    setSortField(DEFAULT_SORT_FIELD);
    setSortDirection("asc");
    setGroupingStrategy(GroupingStrategy.None);
    clearUrlFilters();
  }, [clearUrlFilters]);

  const summaryCount = useMemo(() => {
    const parts: string[] = [];
    const reasonFormatters: [AttentionReason, (n: number) => string][] = [
      [AttentionReason.APPROVAL_PENDING, (n) => `${n} approval${n !== 1 ? "s" : ""}`],
      [AttentionReason.INPUT_REQUIRED, (n) => `${n} input${n !== 1 ? "s" : ""} needed`],
      [AttentionReason.ERROR_STATE, (n) => `${n} error${n !== 1 ? "s" : ""}`],
      [AttentionReason.IDLE_TIMEOUT, (n) => `${n} timed out`],
      [AttentionReason.IDLE, (n) => `${n} idle`],
      [AttentionReason.STALE, (n) => `${n} stale`],
      [AttentionReason.TASK_COMPLETE, (n) => `${n} complete`],
      [AttentionReason.WAITING_FOR_USER, (n) => `${n} waiting`],
      [AttentionReason.TESTS_FAILING, (n) => `${n} test${n !== 1 ? "s" : ""} failing`],
    ];
    for (const [reason, format] of reasonFormatters) {
      const n = byReason.get(reason) ?? 0;
      if (n > 0) parts.push(format(n));
    }
    return parts.join(", ");
  }, [byReason]);

  const activeFilterCount =
    priorityFilter.size +
    priorityExcludeFilter.size +
    reasonFilter.size +
    reasonExcludeFilter.size +
    severityFilter.size +
    severityExcludeFilter.size +
    programFilter.size +
    programExcludeFilter.size +
    categoryFilter.size +
    categoryExcludeFilter.size +
    tagFilter.size +
    tagExcludeFilter.size +
    (prFilter !== "all" ? 1 : 0) +
    (divergedOnly ? 1 : 0) +
    (searchText.trim() ? 1 : 0) +
    // Only count as an active filter when the user has deviated from DEFAULT_SORT_FIELD
    // (mirrors "default"'s old exemption before this feature).
    (sortField !== DEFAULT_SORT_FIELD ? 1 : 0) +
    (groupingStrategy !== GroupingStrategy.None ? 1 : 0);

  const activeFilterLabel = activeFilterCount > 0 ? `Filter (${activeFilterCount})` : "Filter";

  const hasActiveFilter = activeFilterCount > 0;

  const renderQueueItem = (queueItem: ReviewItem, index: number) => (
    <div
      key={queueItem.sessionId}
      className={item}
      data-testid={index === currentIndex ? "current-item" : "review-item"}
      data-session-id={queueItem.sessionId}
    >
      <div
        className={`${itemClickable} ${index === currentIndex ? currentItem : ""}`}
        onClick={() => onSessionClick?.(queueItem.sessionId)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            onSessionClick?.(queueItem.sessionId);
          }
        }}
        role="button"
        tabIndex={0}
        data-testid={`review-item-${queueItem.sessionId}`}
        data-current={index === currentIndex ? "true" : undefined}
        aria-describedby={
          queueItem.metadata?.["pending_approval_id"]
            ? `escalation-reason-${queueItem.sessionId}`
            : undefined
        }
      >
        <div className={itemHeader}>
          <h3 className={itemTitle}>{queueItem.sessionName}</h3>
          <ReviewQueueBadge
            priority={queueItem.priority}
            reason={queueItem.reason}
            compact={true}
          />
        </div>
        <div className={itemBody}>
          <ReviewQueueBadge
            priority={queueItem.priority}
            reason={queueItem.reason}
            compact={false}
          />
          {queueItem.context && !queueItem.metadata?.["pending_approval_id"] && (
            <p className={itemContext}>{queueItem.context}</p>
          )}
          {queueItem.patternName && (
            <span className={itemPattern}>
              Pattern: {queueItem.patternName}
            </span>
          )}
          {queueItem.metadata?.["pending_approval_id"] && (
            <>
              <SeverityBadge riskLevel={queueItem.metadata["risk_level"] ?? ""} compact />
              <p
                className={`${escalationReasonText}`}
                id={`escalation-reason-${queueItem.sessionId}`}
                data-testid={`escalation-reason-${queueItem.sessionId}`}
              >
                {queueItem.metadata["escalation_reason"]
                  ? `${ESCALATION_REASON_EMOJI[queueItem.metadata["escalation_reason_category"] as EscalationCategory] ?? ""} ${queueItem.metadata["escalation_reason"]}`.trim()
                  : "Reason not recorded — this request predates escalation-reason tracking."}
              </p>
              {(queueItem.metadata["tool_input_command"] || queueItem.metadata["tool_input_file"]) && (
                <pre className={commandPreview}>
                  {queueItem.metadata["tool_input_command"] || queueItem.metadata["tool_input_file"]}
                </pre>
              )}
              {queueItem.metadata["cwd"] && (
                <div className={detailRow}>
                  <span className={detailLabel}>Directory:</span>
                  <span className={detailValue}>{queueItem.metadata["cwd"]}</span>
                </div>
              )}
              {queueItem.metadata["orphaned"] === "true" && (
                <span className={expiredBadge}>Expired</span>
              )}
            </>
          )}
          {/* Session details */}
          <div className={sessionDetails}>
            <div className={detailRow}>
              <span className={detailLabel}>Program:</span>
              <span className={detailValue}>{queueItem.program}</span>
            </div>
            <div className={detailRow}>
              <span className={detailLabel}>Branch:</span>
              <span className={detailValue}>{queueItem.branch}</span>
            </div>
            <div className={detailRow}>
              <span className={detailLabel}>Path:</span>
              <span className={detailValue} title={queueItem.path}>{queueItem.path}</span>
            </div>
            {queueItem.tags && queueItem.tags.length > 0 && (
              <div className={detailRow}>
                <span className={detailLabel}>Tags:</span>
                <div className={tags}>
                  {queueItem.tags.map((t, idx) => (
                    <span key={idx} className={tag}>{t}</span>
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>
        <div className={itemFooter}>
          <span className={itemAge}>
            Last Activity: {formatTimestamp(queueItem.lastActivity?.seconds ?? BigInt(0))}{" "}
            ago
          </span>
          {queueItem.diffStats && (queueItem.diffStats.added > 0 || queueItem.diffStats.removed > 0) && (
            <span className={diffStats}>
              <span className={diffAdded}>+{queueItem.diffStats.added}</span>
              <span className={diffRemoved}>-{queueItem.diffStats.removed}</span>
            </span>
          )}
        </div>
      </div>
      <div className={itemActions} style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
        {queueItem.metadata?.["pending_approval_id"] && (
          <>
            <Button
              intent="primary"
              size="lg"
              onClick={(e) => {
                e.stopPropagation();
                approveRequest(queueItem.metadata!["pending_approval_id"]).finally(() => {
                  acknowledgeSession(queueItem.sessionId);
                  onAcknowledged?.(queueItem.sessionId);
                });
              }}
              title="Approve this tool-use request"
              aria-label="Approve"
              data-testid={`approve-${queueItem.sessionId}`}
            >
              ✓ Approve
            </Button>
            <Button
              intent="danger"
              size="lg"
              onClick={(e) => {
                e.stopPropagation();
                denyRequest(queueItem.metadata!["pending_approval_id"]).finally(() => {
                  acknowledgeSession(queueItem.sessionId);
                  onAcknowledged?.(queueItem.sessionId);
                });
              }}
              title="Deny this tool-use request"
              aria-label="Deny"
              data-testid={`deny-${queueItem.sessionId}`}
            >
              ✗ Deny
            </Button>
            {queueItem.metadata?.["tool_input_command"] &&
              isCreateRuleEligibleCategory(queueItem.metadata?.["escalation_reason_category"]) && (
              <Button
                intent="secondary"
                size="md"
                onClick={(e) => {
                  e.stopPropagation();
                  setRuleSaved(false);
                  setActiveRuleItemId(queueItem.sessionId);
                  void generateRule({
                    source: SuggestionSource.COMMAND_SAMPLE,
                    commandSample: queueItem.metadata!["tool_input_command"],
                    toolNameFilter: queueItem.metadata?.["tool_name"] ?? "",
                  });
                }}
                title="Generate an auto-approval rule from this command"
                aria-label="Create Rule"
                data-testid={`create-rule-${queueItem.sessionId}`}
              >
                ✦ Create Rule
              </Button>
            )}
          </>
        )}
        {/* Skip button: only shown for non-approval items.
            Approval items already have explicit ✓ Approve / ✗ Deny buttons above. */}
        {!queueItem.metadata?.["pending_approval_id"] && (
          <Button
            intent="ghost"
            size="md"
            onClick={(e) => {
              e.stopPropagation();
              if (onSkipSession) {
                onSkipSession(queueItem.sessionId);
              } else {
                acknowledgeSession(queueItem.sessionId);
              }
              onAcknowledged?.(queueItem.sessionId);
            }}
            title="Acknowledge session (remove from queue)"
            aria-label="Acknowledge session"
            data-testid={`acknowledge-${queueItem.sessionId}`}
          >
            ⏭ Skip
          </Button>
        )}
        {/* Epic 2.4: Create PR trigger — same 3-state machine as SessionActionsOverflow.tsx's
            (ux.md Surface 1 & 2): State A (enabled, opens CreatePullRequestModal), State B
            (disabled, no commits ahead), State C (existing PR — link, never reopens the modal). */}
        {queueItem.reason === AttentionReason.TASK_COMPLETE && (() => {
          const hasCommitsAhead = queueItem.hasCommitsAhead;
          const prNumber = queueItem.githubPrUrl ? parseGitHubRef(queueItem.githubPrUrl)?.prNumber : undefined;
          return (
            <div style={{ display: "flex", alignItems: "center", gap: "6px" }}>
              {queueItem.branchDivergedFromBase && (
                <span className={divergedBadge}>
                  ⚠ Diverged from main
                </span>
              )}
              {queueItem.githubPrUrl ? (
                <a
                  href={queueItem.githubPrUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  onClick={(e) => e.stopPropagation()}
                  aria-label={prNumber ? `PR #${prNumber}: ${queueItem.sessionName}` : `View PR: ${queueItem.sessionName}`}
                  data-testid="github-pr-link"
                >
                  {prNumber ? `✅ View PR #${prNumber}` : "✅ View PR"}
                </a>
              ) : (
                <Button
                  intent="primary"
                  size="md"
                  disabled={!hasCommitsAhead}
                  onClick={(e) => {
                    e.stopPropagation();
                    createPrTriggerRef.current = e.currentTarget;
                    setIsCreatePrOpen(queueItem.sessionId);
                  }}
                  title={hasCommitsAhead ? "Create a pull request for this session" : "No commits ahead of main yet"}
                  aria-label="Create PR"
                  data-testid={`create-pr-trigger-${queueItem.sessionId}`}
                >
                  🔀 Create PR
                </Button>
              )}
            </div>
          );
        })()}
      </div>
    </div>
  );

  if (error) {
    return (
      <div className={errorClass}>
        <p>Failed to load review queue: {error.message}</p>
        <Button onClick={refresh} intent="secondary" size="md">
          Retry
        </Button>
      </div>
    );
  }

  return (
    <div className={panel} data-testid="review-queue">
      {/* Signals the initial fetch has resolved, independent of whether the
          queue ended up empty — tests and other consumers use this to know
          when it's safe to make assertions about queue contents. */}
      {!loading && <div data-testid="review-queue-loaded" aria-hidden="true" />}
      {/* Screen reader live region for queue count changes */}
      <div aria-live="polite" aria-atomic="true" className={visuallyHidden}>
        {liveAnnouncement}
      </div>
      <div className={header}>
        <div className={titleRow}>
          <h2 className={title}>
            Review Queue{" "}
            <span className={count} data-testid="review-queue-badge">
              {totalItems}
            </span>
          </h2>
          {onAutoAdvanceChange !== undefined && (
            <label className={autoAdvanceToggle}>
              <input
                type="checkbox"
                checked={autoAdvance ?? true}
                onChange={(e) => {
                  onAutoAdvanceChange(e.target.checked);
                  setAutoAdvanceSaved(true);
                  setTimeout(() => setAutoAdvanceSaved(false), 2000);
                }}
              />
              Auto-advance
              {autoAdvanceSaved && <span className={savedIndicator}>Saved</span>}
            </label>
          )}
          <button
            onClick={refreshSnapshot}
            className={refreshButton}
            disabled={loading}
            aria-label="Refresh review queue"
          >
            {loading ? "⟳" : "↻"}
          </button>
        </div>

        {totalItems > 0 && (
          <div className={stats} data-testid="queue-statistics">
            <span className={stat} data-testid="total-items">
              {summaryCount || `${totalItems} ${totalItems === 1 ? "item" : "items"}`}
            </span>
            {(workingCount > 0 || stuckCount > 0) && (
              <span className={stat} data-testid="working-state-counts">
                {items.filter(i => deriveWorkingState(i) !== WorkingState.WAITING).length} waiting
                {workingCount > 0 && ` · ${workingCount} working`}
                {stuckCount > 0 && ` · ${stuckCount} stuck`}
              </span>
            )}
          </div>
        )}

        {/* Heads-up callout when oldest item is over 5 minutes old */}
        {oldestAgeSeconds > BigInt(300) && (
          <div className={oldestCallout} role="status">
            Oldest item: {formatDuration(oldestAgeSeconds)}
          </div>
        )}

        {/* New-items banner: shows when items arrive after snapshot was taken */}
        {newItemsCount > 0 && (
          <button
            className={newItemsBanner}
            onClick={refreshSnapshot}
            aria-label={`${newItemsCount} new item${newItemsCount !== 1 ? "s" : ""} added. Click to refresh the list.`}
          >
            {newItemsCount} new item{newItemsCount !== 1 ? "s" : ""} added — click to refresh
          </button>
        )}
      </div>

      {allItems.length > 0 && (
        <div className={filterToggleRow}>
          <button
            className={`${filterToggle} ${hasActiveFilter ? filterToggleActive : ""}`}
            onClick={() => setIsFiltersOpen((o) => !o)}
            aria-expanded={isFiltersOpen}
            aria-controls="review-queue-filters"
          >
            {activeFilterLabel} {isFiltersOpen ? "▲" : "▼"}
          </button>
          {hasActiveFilter && (
            <button
              className={filterClear}
              onClick={clearAllFilters}
              aria-label="Clear active filter"
            >
              ✕ Clear
            </button>
          )}
        </div>
      )}

      {isFiltersOpen && (
        <div id="review-queue-filters" className={filters}>
          <div className={filterGroup}>
            <label className={filterLabel} htmlFor="review-queue-search">Search:</label>
            <input
              id="review-queue-search"
              type="text"
              className={searchInput}
              placeholder="Search name, context, branch, program…"
              value={searchText}
              onChange={(e) => handleSearchTextChange(e.target.value)}
              data-testid="review-queue-search"
            />
          </div>

          <div className={filterGroup}>
            <label className={filterLabel}>Priority (any):</label>
            <div className={filterButtons}>
              {[Priority.URGENT, Priority.HIGH, Priority.MEDIUM, Priority.LOW].map(
                (priority) => {
                  const priorityCount = byPriority.get(priority) ?? 0;
                  const isExcluded = priorityExcludeFilter.has(priority);
                  return (
                    <button
                      key={priority}
                      className={`${filterButton} ${priorityFilter.has(priority) ? filterButtonActive : isExcluded ? filterButtonExcluded : ""}`}
                      onClick={() => handleFilterByPriority(priority)}
                      disabled={priorityCount === 0}
                      aria-pressed={priorityFilter.has(priority)}
                      title={isExcluded ? "Excluded — click to clear" : "Click to include, click again to exclude"}
                    >
                      {isExcluded ? "🚫 " : ""}
                      {getPriorityLabel(priority)} ({priorityCount})
                    </button>
                  );
                }
              )}
            </div>
          </div>

          <div className={filterGroup}>
            <label className={filterLabel}>Reason (any):</label>
            <div className={filterButtons}>
              {[
                AttentionReason.APPROVAL_PENDING,
                AttentionReason.INPUT_REQUIRED,
                AttentionReason.WAITING_FOR_USER,
                AttentionReason.ERROR_STATE,
                AttentionReason.TESTS_FAILING,
                AttentionReason.IDLE_TIMEOUT,
                AttentionReason.IDLE,
                AttentionReason.STALE,
                AttentionReason.TASK_COMPLETE,
              ].map((reason) => {
                const reasonCount = byReason.get(reason) ?? 0;
                // Hide TESTS_FAILING when count is 0 (detection may be disabled)
                if (reason === AttentionReason.TESTS_FAILING && reasonCount === 0) return null;
                const isExcluded = reasonExcludeFilter.has(reason);
                return (
                  <button
                    key={reason}
                    className={`${filterButton} ${reasonFilter.has(reason) ? filterButtonActive : isExcluded ? filterButtonExcluded : ""}`}
                    onClick={() => handleFilterByReason(reason)}
                    disabled={reasonCount === 0}
                    aria-pressed={reasonFilter.has(reason)}
                    title={isExcluded ? "Excluded — click to clear" : "Click to include, click again to exclude"}
                  >
                    {isExcluded ? "🚫 " : ""}
                    {getReasonLabel(reason)} ({reasonCount})
                  </button>
                );
              })}
            </div>
          </div>

          <div className={filterGroup}>
            <label className={filterLabel}>Severity (any):</label>
            <div className={filterButtons}>
              {SEVERITY_FILTER_VALUES.map((severity) => {
                const severityCount = bySeverity.get(severity) ?? 0;
                const label = severity === UNRECORDED_SEVERITY ? "Not recorded" : getRiskLevelInfo(severity).label;
                const isExcluded = severityExcludeFilter.has(severity);
                return (
                  <button
                    key={severity}
                    className={`${filterButton} ${severityFilter.has(severity) ? filterButtonActive : isExcluded ? filterButtonExcluded : ""}`}
                    onClick={() => handleFilterBySeverity(severity)}
                    disabled={severityCount === 0}
                    aria-pressed={severityFilter.has(severity)}
                    title={isExcluded ? "Excluded — click to clear" : "Click to include, click again to exclude"}
                  >
                    {isExcluded ? "🚫 " : ""}
                    {label} ({severityCount})
                  </button>
                );
              })}
            </div>
          </div>

          {availablePrograms.length > 0 && (
            <div className={filterGroup}>
              <label className={filterLabel}>Program (any):</label>
              <div className={filterButtons}>
                {availablePrograms.map(([program, n]) => {
                  const isExcluded = programExcludeFilter.has(program);
                  return (
                    <button
                      key={program}
                      className={`${filterButton} ${programFilter.has(program) ? filterButtonActive : isExcluded ? filterButtonExcluded : ""}`}
                      onClick={() => handleFilterByProgram(program)}
                      aria-pressed={programFilter.has(program)}
                      title={isExcluded ? "Excluded — click to clear" : "Click to include, click again to exclude"}
                    >
                      {isExcluded ? "🚫 " : ""}
                      {program} ({n})
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          {availableCategories.length > 0 && (
            <div className={filterGroup}>
              <label className={filterLabel}>Category (any):</label>
              <div className={filterButtons}>
                {availableCategories.map(([category, n]) => {
                  const isExcluded = categoryExcludeFilter.has(category);
                  return (
                    <button
                      key={category}
                      className={`${filterButton} ${categoryFilter.has(category) ? filterButtonActive : isExcluded ? filterButtonExcluded : ""}`}
                      onClick={() => handleFilterByCategory(category)}
                      aria-pressed={categoryFilter.has(category)}
                      title={isExcluded ? "Excluded — click to clear" : "Click to include, click again to exclude"}
                    >
                      {isExcluded ? "🚫 " : ""}
                      {category} ({n})
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          {availableTags.length > 0 && (
            <div className={filterGroup}>
              <label className={filterLabel}>Tags (any):</label>
              <div className={filterButtons}>
                {availableTags.map(([t, n]) => {
                  const isExcluded = tagExcludeFilter.has(t);
                  return (
                    <button
                      key={t}
                      className={`${filterButton} ${tagFilter.has(t) ? filterButtonActive : isExcluded ? filterButtonExcluded : ""}`}
                      onClick={() => handleFilterByTag(t)}
                      aria-pressed={tagFilter.has(t)}
                      title={isExcluded ? "Excluded — click to clear" : "Click to include, click again to exclude"}
                    >
                      {isExcluded ? "🚫 " : ""}
                      {t} ({n})
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          <div className={filterGroup}>
            <label className={filterLabel}>Pull Request:</label>
            <div className={filterButtons}>
              {(["all", "has-pr", "no-pr"] as const).map((v) => (
                <button
                  key={v}
                  className={`${filterButton} ${prFilter === v ? filterButtonActive : ""}`}
                  onClick={() => handlePrFilterChange(v)}
                  aria-pressed={prFilter === v}
                >
                  {v === "all" ? "All" : v === "has-pr" ? "Has PR" : "No PR"}
                </button>
              ))}
              <button
                className={`${filterButton} ${divergedOnly ? filterButtonActive : ""}`}
                onClick={() => handleDivergedOnlyChange(!divergedOnly)}
                aria-pressed={divergedOnly}
              >
                Diverged from base
              </button>
            </div>
          </div>

          <div className={filterGroup}>
            <label className={filterLabel} htmlFor="review-queue-sort">Sort by:</label>
            <div className={sortRow}>
              <select
                id="review-queue-sort"
                className={sortSelect}
                value={sortField}
                onChange={(e) => handleSortFieldChange(e.target.value as SortField)}
              >
                <option value="severity">Severity</option>
                <option value="default">Queue order</option>
                <option value="priority">Priority</option>
                <option value="age">Last activity</option>
                <option value="diffSize">Diff size</option>
                <option value="name">Name</option>
              </select>
              {sortField !== "default" && (
                <button
                  className={filterButton}
                  onClick={() => handleSortDirectionChange(sortDirection === "asc" ? "desc" : "asc")}
                  aria-label={`Sort direction: ${sortDirection === "asc" ? "ascending" : "descending"}`}
                >
                  {sortDirection === "asc" ? "↑ Asc" : "↓ Desc"}
                </button>
              )}
            </div>
          </div>

          <div className={filterGroup}>
            <label className={filterLabel} htmlFor="review-queue-group">Group by:</label>
            <select
              id="review-queue-group"
              className={sortSelect}
              value={groupingStrategy}
              onChange={(e) => handleGroupingStrategyChange(e.target.value as GroupingStrategy)}
            >
              {REVIEW_GROUPING_STRATEGIES.map((strategy) => (
                <option key={strategy} value={strategy}>
                  {GroupingStrategyLabels[strategy]}
                </option>
              ))}
            </select>
          </div>
        </div>
      )}

      <div className={itemsClass}>
        {loading && items.length === 0 ? (
          <div className={loadingClass}>Loading review queue...</div>
        ) : items.length === 0 ? (
          hasActiveFilter ? (
            <div className={emptyClass}>
              <p>No items match the current filter.</p>
              <p className={emptySubtext}>
                {totalItems} {totalItems === 1 ? "item" : "items"} in queue
              </p>
              <Button
                intent="secondary"
                size="md"
                onClick={clearAllFilters}
              >
                Clear filter
              </Button>
            </div>
          ) : hadItems ? (
            <div className={`${emptyClass} ${completionState}`}>
              <p className={completionIcon}>[✓]</p>
              <p>All done! 0 items remaining.</p>
              <p className={emptySubtext}>
                Queue cleared.
              </p>
            </div>
          ) : (
            <div className={emptyClass}>
              <p>No sessions need attention!</p>
              <p className={emptySubtext}>
                All sessions are running smoothly.
              </p>
            </div>
          )
        ) : (
          <>
            {groupedItems ? (
              groupedItems.map((group) => (
                <div key={group.groupKey} className={groupSection} data-testid={`review-group-${group.groupKey}`}>
                  <h4 className={groupHeading}>
                    {group.displayName} ({group.items.length})
                  </h4>
                  {group.items.map((queueItem) =>
                    renderQueueItem(queueItem, indexById.get(queueItem.sessionId) ?? -1)
                  )}
                </div>
              ))
            ) : (
              items.map((queueItem, index) => renderQueueItem(queueItem, index))
            )}
          </>
        )}
      </div>

      {/* Epic 2.4: shared Create Pull Request modal — same component + behavior as
          SessionActionsOverflow.tsx's entry point (ux.md Surface 1 & 2, AC7). */}
      {activePrSession && (
        <CreatePullRequestModal
          session={activePrSession}
          isOpen={!!isCreatePrOpen}
          onClose={() => setIsCreatePrOpen(null)}
          draftPullRequest={draftPullRequest}
          createPullRequest={createPullRequest}
          triggerRef={createPrTriggerRef}
        />
      )}

      {/* Epic 4: Create Rule modal */}
      {activeRuleItemId && createPortal(
        <div
          className={modalOverlay}
          onClick={() => {
            setActiveRuleItemId(null);
            clearRule();
          }}
        >
          <div
            className={ruleModalContent}
            onClick={(e) => e.stopPropagation()}
            role="dialog"
            aria-modal="true"
            aria-label="Create Auto-Approval Rule"
            data-testid="create-rule-modal"
          >
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <h3 style={{ margin: 0, fontSize: "1.125rem", fontWeight: 600, color: "var(--text-primary)" }}>
                Create Auto-Approval Rule
              </h3>
              <button
                style={{
                  background: "none",
                  border: "none",
                  cursor: "pointer",
                  color: "var(--text-secondary)",
                  fontSize: "1.25rem",
                  padding: "0.25rem",
                  lineHeight: 1,
                }}
                onClick={() => {
                  setActiveRuleItemId(null);
                  clearRule();
                }}
                aria-label="Close"
                data-testid="create-rule-modal-close"
              >
                ✕
              </button>
            </div>

            {ruleSaved && (
              <p
                role="status"
                style={{ margin: 0, fontSize: "0.875rem", color: "var(--success)" }}
                data-testid="rule-saved-indicator"
              >
                ✓ Rule saved
              </p>
            )}

            {ruleLoading && (
              <p
                style={{ margin: 0, fontSize: "0.875rem", color: "var(--text-secondary)", fontStyle: "italic" }}
                data-testid="create-rule-loading"
              >
                ⏳ Generating suggestion…
              </p>
            )}

            {ruleError && !ruleLoading && (
              <p role="alert" style={{ margin: 0, fontSize: "0.875rem", color: "var(--error)" }}>
                ✗ {ruleError.message}
              </p>
            )}

            {!ruleLoading && suggestions.length > 0 && (
              <SuggestedRuleCard
                suggestion={suggestions[0]}
                onAccept={() => {
                  setRuleSaved(true);
                  setActiveRuleItemId(null);
                  clearRule();
                }}
                onDiscard={() => {
                  setActiveRuleItemId(null);
                  clearRule();
                }}
              />
            )}

            {!ruleLoading && suggestions.length === 0 && !ruleError && (
              <p style={{ margin: 0, fontSize: "0.875rem", color: "var(--text-secondary)", fontStyle: "italic" }}>
                Waiting for suggestion…
              </p>
            )}
          </div>
        </div>,
        document.body
      )}
    </div>
  );
}
