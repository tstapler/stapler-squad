"use client";
// +feature: backlog:item-detail

import { useState, useEffect, useCallback, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import type { BacklogItem, AcCriterion, BacklogItemInput, LinkedSession, PipelineMode } from "@/lib/hooks/useBacklogService";
import { useBacklogService, mapBacklogItem } from "@/lib/hooks/useBacklogService";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { useAnalytics } from "@/lib/analytics";
import { useCurrentWorkSession } from "@/lib/backlog/currentWorkSession";
import { useStuckBacklogItems } from "@/lib/hooks/useStuckBacklogItems";
import { classifySessionKind } from "@/lib/backlog/sessionKind";
import { resolvePipelineModeDisplay } from "@/lib/backlog/pipelineModeDisplay";
import { formatDate } from "@/lib/backlog/formatDate";
import { useVcsStatus } from "@/lib/hooks/useVcsStatus";
import { useBacklogItemShipStatus } from "@/lib/hooks/useBacklogItemShipStatus";
import { useWatchBacklogItems } from "@/lib/hooks/useWatchBacklogItems";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { BacklogService } from "@/gen/session/v1/backlog_pb";
import { useAppSelector } from "@/lib/store";
import { store } from "@/lib/store/store";
import { selectBacklogItemById } from "@/lib/store/backlogItemsSlice";
import { selectSessionsError } from "@/lib/store/sessionsSlice";
import { fromSessionVcs, fromShipStatus } from "@/lib/vcs/adapters";
import { useSectionExpandState } from "@/lib/hooks/useSectionExpandState";
import { copyToClipboard } from "@/lib/clipboard";
import { CollapsibleGroup } from "@/components/ui/Collapsible";
import { InlineNotice } from "@/components/common/InlineNotice";
import { ConnectionIndicator } from "./ConnectionIndicator";
import { BacklogItemForm } from "./BacklogItemForm";
import { AcCriteriaList } from "./AcCriteriaList";
import { InlineError } from "./InlineError";
import { TriageLoadingIndicator } from "./TriageLoadingIndicator";
import { TriageReviewPanel } from "./TriageReviewPanel";
import { ReviewChangesModal } from "./ReviewChangesModal";
import { BacklogFileBrowserModal } from "./BacklogFileBrowserModal";
import { LifecycleSummary } from "./detail/LifecycleSummary";
import { PlanningSection } from "./detail/PlanningSection";
import { ReviewingSection } from "./detail/ReviewingSection";
import { LastReviewResultSection } from "./detail/LastReviewResultSection";
import { PullRequestSection } from "./detail/PullRequestSection";
import { SourceSection } from "./detail/SourceSection";
import { DescriptionSection } from "./detail/DescriptionSection";
import { ActionsSection } from "./detail/ActionsSection";
import { PlanVerdictBox } from "./PlanVerdictBox";
import { derivePlanReviewStatus } from "@/lib/backlog/planReviewStatus";
import { PlanArtifactsSection } from "./detail/PlanArtifactsSection";
import { VersionControlSection } from "./detail/VersionControlSection";
import { SessionsSection } from "./detail/SessionsSection";
import { WorkflowHistorySection } from "./detail/WorkflowHistorySection";
import { ProgressHistorySection } from "./detail/ProgressHistorySection";
import { NotesSection } from "./detail/NotesSection";
import { ManualOverrideSection } from "./detail/ManualOverrideSection";
import * as styles from "./BacklogItemDetail.css";

interface BacklogItemDetailProps {
  itemId: string;
  onClose?: () => void;
}

const PRIORITY_LABELS: Record<number, string> = { 1: "P1", 2: "P2", 3: "P3", 4: "P4", 5: "P5" };

const ACTION_SUCCESS_MESSAGES: Record<string, string> = {
  mark_ready: "Marked ready.",
  trigger_triage: "Triage started.",
  spawn_session: "Session started.",
  spawn_session_autonomous: "Autonomous session started.",
  restart_session: "Session restarted.",
  approve_plan: "Plan approved.",
  retry_triage: "Triage re-triggered.",
  mark_done: "Marked done.",
  override_done: "Overridden to done.",
  re_review: "Re-review triggered.",
  ship_pr: "PR created.",
  archive: "Archived.",
  unarchive: "Unarchived — back in the idea column. Needs a fresh session.",
  reopen: "Reopened for review.",
  send_back_idea: "Sent back to triage.",
  send_back_refining: "Sent back to refining.",
  send_back_ready: "Sent back to ready.",
};

export function BacklogItemDetail({ itemId, onClose }: BacklogItemDetailProps) {
  const { track } = useAnalytics();
  const {
    getBacklogItem,
    transitionStatus,
    triggerTriage,
    cancelTriage,
    spawnSessionFromItem,
    approvePlan,
    rejectPlan,
    overrideVerdict,
    triggerReReview,
    triggerShipPR,
    submitManualReview,
    archiveBacklogItem,
    unarchiveBacklogItem,
    deleteBacklogItem,
    updateBacklogItem,
    listPipelineModes,
    lastError,
  } = useBacklogService();
  const { deleteSession, updateSession } = useSessionService();
  const { showActionToast } = useNotifications();
  const [item, setItem] = useState<BacklogItem | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  /** The action key currently in flight (e.g. "mark_ready"), or null when idle. */
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [editMode, setEditMode] = useState(false);
  const [deletingSessionId, setDeletingSessionId] = useState<string | null>(null);
  const [steeringSessionId, setSteeringSessionId] = useState<string | null>(null);
  const [copiedField, setCopiedField] = useState<"id" | "link" | null>(null);

  // Epic 3.4 "what ran" surface: the currently-fetched mode list, used only
  // to resolve a session's frozen pipelineModeSnapshot slug to a
  // human-readable name (and to detect content drift). Fetch failure
  // degrades every session's "Pipeline" group to the unrecognized-mode
  // fallback rather than blocking the rest of the item detail view.
  const [pipelineModes, setPipelineModes] = useState<PipelineMode[]>([]);

  // Mirrors `item` on every render (assignment during render, not in an
  // effect, so it's always current by the time an in-flight load() resolves)
  // — lets load() compare a freshly-fetched result against whatever is
  // currently displayed without needing `item` in its own dependency array.
  const itemRef = useRef<BacklogItem | null>(null);
  itemRef.current = item;

  // Monotonic guard against out-of-order load() resolution (pitfall #4):
  // each call captures its own sequence number, and only the response
  // matching the most-recently-issued call is applied.
  const loadSeqRef = useRef(0);

  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  // copyTimerRef cancels a still-pending prior confirmation timeout so
  // clicking Copy ID then Copy Link within the 1.5s window doesn't let the
  // first timer clear the second button's confirmation state early.
  const copyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    return () => {
      if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
    };
  }, []);

  const handleCopy = useCallback((field: "id" | "link", value: string) => {
    void copyToClipboard(value).then((ok) => {
      if (!ok || !mountedRef.current) return;
      if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
      setCopiedField(field);
      copyTimerRef.current = setTimeout(() => {
        if (mountedRef.current) setCopiedField(null);
        copyTimerRef.current = null;
      }, 1500);
    });
  }, []);

  // Review changes modal
  const [showChangesModal, setShowChangesModal] = useState(false);

  // File browser modal
  const [showFileBrowser, setShowFileBrowser] = useState(false);

  // Manual review form
  const [showManualReview, setShowManualReview] = useState(false);
  const [manualReviewOutcome, setManualReviewOutcome] = useState("PASS");
  const [manualReviewSummary, setManualReviewSummary] = useState("");

  // Notes inline editing
  const [editingNotes, setEditingNotes] = useState(false);
  const [notesValue, setNotesValue] = useState("");

  // Triage progress tracking
  const [triageElapsedSeconds, setTriageElapsedSeconds] = useState(0);

  // Called once here (not inside LifecycleSummary) so this component owns the
  // single fetch/poll — mirrors board/page.tsx calling it once at the page
  // level and passing the resolved StuckBacklogItem down as a prop, rather
  // than LifecycleSummary standing up its own transport/client and 60s poll
  // on every remount (this component remounts via `key={selectedItemId}` on
  // every backlog item click — see stapler-squad PR #208 review).
  const { items: stuckItems } = useStuckBacklogItems();
  const stuckItem = item ? stuckItems.find((i) => i.itemId === item.id) : undefined;

  // Version control state for the most recent work session's worktree.
  const latestWorkSession = useCurrentWorkSession(item);
  // Surfaces the "most recent work session" heuristic's ambiguity when more than
  // one work session is currently active — the heuristic above is unchanged, this
  // only makes it visible via VcsWidgetHeader's "N active sessions" indicator.
  const activeWorkSessionCount = (item?.linkedSessions ?? []).filter((s) => s.role === "work" && !s.endedAt).length;
  const { data: vcsStatus } = useVcsStatus(latestWorkSession?.sessionId ?? "", getApiBaseUrl());
  // Fallback for once the live session's worktree has been cleaned up (the normal
  // state for a done item) — vcsStatus above comes back null in that case since
  // useVcsStatus needs a live in-memory Instance to query.
  const { data: shipStatus } = useBacklogItemShipStatus(!vcsStatus && item ? item.id : "");
  const vcsWidgetData = vcsStatus ? fromSessionVcs(vcsStatus) : shipStatus ? fromShipStatus(shipStatus) : null;

  // D6 fix: the current work session's resolved pipeline mode, surfaced as
  // a compact badge in LifecycleSummary (Task 3.1.4g) — same value the
  // Sessions section resolves per-row, just also glanceable at the top.
  const pipelineDisplay = latestWorkSession
    ? resolvePipelineModeDisplay(latestWorkSession, pipelineModes)
    : undefined;

  // Epic 5.3 (Story 5.3.1, backlog-event-driven-updates): live updates
  // replace the old 5s poll entirely. Subscribed unfiltered (no status/
  // category filter) so this panel keeps showing the item's current state
  // even if a list/board filter elsewhere would have excluded it (design/
  // ux.md §3). The hook's own `items` return value is unused here — it
  // exists only to keep the shared store hydrated/connected; this panel
  // reads the single item it cares about straight off the store below so
  // unrelated item updates elsewhere never cause this component to re-run
  // (selectBacklogItemById only changes reference when THIS item's store
  // entry changes, unlike the hook's fully-remapped `items` array).
  // Task 6.2.1c: connectionState is captured here to mount the
  // ConnectionIndicator in this panel's header (ux.md §3 wireframe, UX AC #20).
  const { connectionState } = useWatchBacklogItems();
  const liveRawItem = useAppSelector((state) => selectBacklogItemById(state, itemId));

  // Buffered-update state (Story 5.3.2): while editMode is true, an incoming
  // live update is NOT applied to the visible item/form — it's stashed here
  // and offered via an InlineNotice "Reload" action instead, so it can't
  // silently clobber an in-progress edit.
  const [bufferedItem, setBufferedItem] = useState<BacklogItem | null>(null);
  // A Save was attempted while a buffered update was pending — show the
  // warn-before-overwrite confirmation instead of calling the save RPC.
  const [saveConfirmPending, setSaveConfirmPending] = useState(false);
  const [pendingSaveData, setPendingSaveData] = useState<BacklogItemInput | null>(null);

  // Terminal-state (Task 5.3.1c): set when an ArchivedEvent/RemovedEvent
  // arrives for this item from the separate raw watch below.
  const [terminalState, setTerminalState] = useState<"archived" | "removed" | null>(null);

  useEffect(() => {
    if (!liveRawItem) return;
    const mapped = mapBacklogItem(liveRawItem);
    if (editMode) {
      setBufferedItem(mapped);
      return;
    }
    // Guard against a stale live-store entry stomping more-current state
    // (pitfall #2's other half, alongside load()'s matching guard below):
    // backlogItemsSlice's own upsertItem guard only protects the 2nd+ write
    // for a given item id — the *first* live event this component observes
    // for an item (e.g. a "created" event delivered, after a connection
    // delay, later than a separately-issued load()/GetBacklogItem fetch
    // already completed) has no `existing` entry to compare against there,
    // so it's accepted into the store unconditionally and would otherwise
    // unconditionally overwrite this component's already-fresher `item`.
    const current = itemRef.current;
    const currentMs = current?.updatedAt ? new Date(current.updatedAt).getTime() : 0;
    const mappedMs = mapped.updatedAt ? new Date(mapped.updatedAt).getTime() : 0;
    if (current && mappedMs < currentMs) return;
    setItem(mapped);
    setNotesValue(mapped.notes ?? "");
    // editMode is read from the closure at the time this effect actually
    // fires (i.e. whenever liveRawItem changes) — it deliberately does NOT
    // belong in the dependency array. Depending on it would re-run this
    // effect on every editMode toggle even when no new live item arrived,
    // which would re-apply a possibly-stale `liveRawItem` right as edit mode
    // closes and momentarily stomp a just-completed save.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [liveRawItem]);

  // Once editMode flips back to false (Cancel, or a completed Save), apply
  // any update that was buffered while editing — design/ux.md §6's "or exits
  // edit mode" path.
  const prevEditModeRef = useRef(editMode);
  useEffect(() => {
    if (prevEditModeRef.current && !editMode && bufferedItem) {
      setItem(bufferedItem);
      setNotesValue(bufferedItem.notes ?? "");
      setBufferedItem(null);
    }
    prevEditModeRef.current = editMode;
  }, [editMode, bufferedItem]);

  // Task 5.3.1c: a separate, item-scoped raw event subscription dedicated to
  // detecting BacklogItemArchivedEvent/BacklogItemRemovedEvent for this item.
  // useWatchBacklogItems.ts intentionally does NOT dispatch itemArchived into
  // backlogItemsSlice (see that hook's file header, note 1) — item_archived
  // carries no full BacklogItem payload, so there is nothing meaningful to
  // upsert, and the design defers this to component-level handling. There is
  // also no server-side item-id filter on WatchBacklogItemsRequest, so this
  // watches unfiltered and matches events against `itemId` client-side. This
  // is a lightweight, single-purpose stream — unlike useWatchBacklogItems.ts
  // it does not implement exponential-backoff reconnect/afterSeq replay; a
  // dropped connection simply stops detecting further archive/removal for
  // this item until the component remounts.
  useEffect(() => {
    setTerminalState(null);
    const abortController = new AbortController();

    const watchTerminal = async () => {
      try {
        const transport = createConnectTransport({
          baseUrl: getApiBaseUrl(),
          interceptors: [createAuthInterceptor()],
        });
        const client = createClient(BacklogService, transport);
        const stream = client.watchBacklogItems(
          { statusFilter: [], categoryFilter: [], afterSeq: 0n },
          { signal: abortController.signal }
        );
        for await (const event of stream) {
          if (event.event.case === "itemArchived" && event.event.value.itemId === itemId) {
            setTerminalState("archived");
          } else if (event.event.case === "itemRemoved" && event.event.value.itemId === itemId) {
            setTerminalState("removed");
          }
        }
      } catch (err) {
        if (err instanceof Error && err.name === "AbortError") return;
        if (abortController.signal.aborted) return;
        console.error("[BacklogItemDetail] terminal-state watch stream error:", err);
      }
    };

    void watchTerminal();
    return () => {
      abortController.abort();
    };
  }, [itemId]);

  // Task 3.1.4i: every sibling CollapsibleSection (Reviewing/Pull Request
  // from Story 3.1.2, Description from 3.1.3, the rest from 3.1.4) shares
  // one CollapsibleGroup so their headers get Home/End/Arrow roving-
  // tabindex nav. Persistence (Story 1.1.1's contract) is threaded through
  // as a controlled `value`/`onValueChange` on the group, backed by
  // useSectionExpandState per section key — the same
  // `backlog-detail-section-${itemId}-${sectionKey}` localStorage
  // convention used everywhere else.
  //
  // `item` is still null on this component's very first render (load()
  // resolves asynchronously), so a status-derived default computed here
  // would always evaluate against a not-yet-loaded item and get locked in
  // as false by useSectionExpandState's one-time useState initializer —
  // every status-dependent section would default to false regardless of
  // the actual loaded status. Sections with a static default (not
  // status-dependent) are unaffected and get it directly.
  const [reviewingExpanded, setReviewingExpanded] = useSectionExpandState(itemId, "reviewing", false);
  const [lastReviewResultExpanded, setLastReviewResultExpanded] = useSectionExpandState(itemId, "last-review-result", false);
  const [pullRequestExpanded, setPullRequestExpanded] = useSectionExpandState(itemId, "pull-request", true);
  const [planArtifactsExpanded, setPlanArtifactsExpanded] = useSectionExpandState(itemId, "plan-artifacts", false);
  const [versionControlExpanded, setVersionControlExpanded] = useSectionExpandState(itemId, "version-control", false);
  const [sessionsExpanded, setSessionsExpanded] = useSectionExpandState(itemId, "sessions", true);
  const [workflowExpanded, setWorkflowExpanded] = useSectionExpandState(itemId, "workflow", false);
  const [progressHistoryExpanded, setProgressHistoryExpanded] = useSectionExpandState(itemId, "progress-history", false);
  const [notesExpanded, setNotesExpanded] = useSectionExpandState(itemId, "notes", false);
  const [descriptionExpanded, setDescriptionExpanded] = useSectionExpandState(itemId, "description", true);
  const [sourceExpanded, setSourceExpanded] = useSectionExpandState(itemId, "source", false);
  const [manualOverrideExpanded, setManualOverrideExpanded] = useSectionExpandState(itemId, "manual-override", false);

  // Story 3.1.5: applies each status-dependent section's real default
  // exactly once, the first time `item` becomes available after this
  // instance's initial load — never again afterward (guarded by the ref),
  // so a later live update's status change can never retroactively re-open a
  // section the user has since collapsed. Combined with Story 3.1.1's
  // `key={itemId}` remount at the call site, this ref naturally resets per
  // item with no extra plumbing. Only opens a section when the user has no
  // existing persisted preference for it (a direct localStorage read,
  // bypassing useSectionExpandState's own already-applied fallback) — it
  // must never clobber a preference from a previous visit to this same
  // item.
  const initialExpandAppliedRef = useRef(false);
  useEffect(() => {
    if (initialExpandAppliedRef.current || !item) return;
    initialExpandAppliedRef.current = true;

    const hasStoredPreference = (sectionKey: string): boolean => {
      try {
        return localStorage.getItem(`backlog-detail-section-${itemId}-${sectionKey}`) !== null;
      } catch {
        return false;
      }
    };

    if (item.status === "review" && !hasStoredPreference("reviewing")) {
      setReviewingExpanded(true);
    }
    if (item.status !== "review" && item.gateVerdict && !hasStoredPreference("last-review-result")) {
      setLastReviewResultExpanded(true);
    }
    if (
      ["in_progress", "review", "pr_pending"].includes(item.status) &&
      !hasStoredPreference("version-control")
    ) {
      setVersionControlExpanded(true);
    }
    if (item.notes && !hasStoredPreference("notes")) {
      setNotesExpanded(true);
    }
    // Deliberately omits setExpanded functions from deps — they're stable
    // (useCallback in useSectionExpandState) and including them would
    // invite a lint-driven "fix" that reintroduces a re-run per render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [item, itemId]);

  const sectionExpandEntries: Array<[string, boolean, (next: boolean) => void]> = [
    ["reviewing", reviewingExpanded, setReviewingExpanded],
    ["last-review-result", lastReviewResultExpanded, setLastReviewResultExpanded],
    ["pull-request", pullRequestExpanded, setPullRequestExpanded],
    ["description", descriptionExpanded, setDescriptionExpanded],
    ["plan-artifacts", planArtifactsExpanded, setPlanArtifactsExpanded],
    ["version-control", versionControlExpanded, setVersionControlExpanded],
    ["sessions", sessionsExpanded, setSessionsExpanded],
    ["workflow", workflowExpanded, setWorkflowExpanded],
    ["progress-history", progressHistoryExpanded, setProgressHistoryExpanded],
    ["notes", notesExpanded, setNotesExpanded],
    ["source", sourceExpanded, setSourceExpanded],
    ["manual-override", manualOverrideExpanded, setManualOverrideExpanded],
  ];
  const openSectionKeys = sectionExpandEntries.filter(([, expanded]) => expanded).map(([key]) => key);
  const handleGroupValueChange = (next: string[]) => {
    const nextSet = new Set(next);
    for (const [key, expanded, setExpanded] of sectionExpandEntries) {
      const shouldBeExpanded = nextSet.has(key);
      if (shouldBeExpanded !== expanded) setExpanded(shouldBeExpanded);
    }
  };

  const load = useCallback(async () => {
    const seq = ++loadSeqRef.current;
    setLoading(true);
    setError(null);
    try {
      const result = await getBacklogItem(itemId);
      // Drop this response if it's not the most recently issued load() call
      // (pitfall #4 — resolution order isn't guaranteed to match call order)
      // or the component has since unmounted.
      if (!mountedRef.current || seq !== loadSeqRef.current) return;
      if (!result) {
        setError("Item not found.");
      } else {
        // Guard against stomping a fresher live-watch/optimistic update with
        // an older fetched result (pitfall #2) — `>=` is intentional: a
        // same-millisecond result from the user's own just-triggered
        // mutation must still apply, only a strictly *older* one is dropped.
        const current = itemRef.current;
        const currentMs = current?.updatedAt ? new Date(current.updatedAt).getTime() : 0;
        const resultMs = result.updatedAt ? new Date(result.updatedAt).getTime() : 0;
        if (!current || resultMs >= currentMs) {
          setItem(result);
          setNotesValue(result.notes ?? "");
        }
      }
    } catch (e) {
      if (mountedRef.current) setError(e instanceof Error ? e.message : "Failed to load item.");
    } finally {
      if (mountedRef.current) setLoading(false);
    }
  }, [itemId, getBacklogItem]);

  useEffect(() => {
    void load();
  }, [load]);

  // Extracted verbatim from the inline session-delete onClick handler
  // (Story 3.1.4, Task 3.1.4c) so SessionsSection stays a props-down/
  // callbacks-up presentational component.
  const handleDeleteSession = useCallback(
    async (s: LinkedSession) => {
      if (!item) return;
      if (!confirm("Delete this session? This cannot be undone.")) return;
      const toastKey = `${item.id}:delete_session:${s.sessionId}`;
      setDeletingSessionId(s.sessionId);
      try {
        if (s.role === "triage") {
          await cancelTriage(item.id);
        } else {
          track({
            name: "backlog_delete_session",
            category: "user_action",
            component: "BacklogItemDetail",
            labels: { role: s.role },
          });
          await deleteSession(s.sessionId, true);
        }
        showActionToast("Session deleted.", "success", toastKey);
        await load();
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Failed to delete session.";
        if (mountedRef.current) setError(msg);
        showActionToast(msg, "error", toastKey);
      } finally {
        if (mountedRef.current) setDeletingSessionId(null);
      }
    },
    [item, cancelTriage, track, deleteSession, showActionToast, load]
  );

  // Epic 2.2 (Story 2.2.2, ADR-002): steers a live work/review session via
  // the same widened UpdateSession RPC/hook the general session list's own
  // Steer dialog uses (AC7). Mirrors handleDeleteSession's shape.
  // updateSession() itself swallows RPC errors and returns null rather than
  // throwing (it dispatches the real failure message to the shared
  // session-service redux error slice instead — see useSessionService.ts's
  // updateSession) — so failure is re-thrown here, letting SessionsSection's
  // inline composer (which awaits onSteerSession) catch it, keep itself
  // open, and surface the error instead of closing optimistically. Read the
  // real message via store.getState() rather than a memoized selector value
  // (e.g. useAppSelector) — updateSession's dispatch happens synchronously
  // before it resolves, but a selector captured in this callback's closure
  // would still reflect the pre-call render and lag one render behind.
  const handleSteerSession = useCallback(
    async (s: LinkedSession, message: string) => {
      const toastKey = `${s.sessionId}:steer`;
      setSteeringSessionId(s.sessionId);
      try {
        const result = await updateSession(s.sessionId, { steerMessage: message });
        if (!result) {
          throw new Error(selectSessionsError(store.getState()) ?? "Failed to steer session.");
        }
        showActionToast("Steering message sent.", "success", toastKey);
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Failed to steer session.";
        showActionToast(msg, "error", toastKey);
        throw err instanceof Error ? err : new Error(msg);
      } finally {
        setSteeringSessionId(null);
      }
    },
    [updateSession, showActionToast]
  );

  // Epic 3.4: fetch the current mode list once, for resolving each linked
  // session's frozen pipelineModeSnapshot to a name/drift state. Not
  // filtered by `enabled` — a since-disabled (but not deleted) mode is
  // still a "found" match, not "unrecognized".
  useEffect(() => {
    let cancelled = false;
    listPipelineModes()
      .then((modes) => {
        if (!cancelled) setPipelineModes(modes);
      })
      .catch((e) => {
        console.warn("[BacklogItemDetail] listPipelineModes failed", e);
      });
    return () => {
      cancelled = true;
    };
  }, [listPipelineModes]);

  // Track triage progress: increment elapsed time while triageStatus === "running"
  useEffect(() => {
    if (item?.triageStatus !== "running") {
      return;
    }

    const interval = setInterval(() => {
      setTriageElapsedSeconds((prev) => prev + 1);
    }, 1000);

    return () => clearInterval(interval);
  }, [item?.triageStatus]);

  // Reset triage timer when status changes away from triage
  useEffect(() => {
    if (item?.triageStatus !== "running") {
      setTriageElapsedSeconds(0);
    }
  }, [item?.triageStatus]);

  // retriggerTriageCore is shared by the standalone "Retry" button (InlineError,
  // shown for the triage-failed banner) and the "retry_triage" action dispatched
  // from ActionsSection's approve-plan-replacement affordance for a queued/ready
  // item that's gated on plan approval with no usable triage result (see
  // itemActions.ts — docs/tasks/backlog-feature-improvement.md's 2026-08-03
  // entry, item be676dab). TriggerTriage only ever accepts idea/ready, so a
  // queued item needs the same reset-to-idea step the manual "Return to Triage"
  // action performs first — mirrors AutoRespawnTriage's server-side handling of
  // the identical generalized case (server/services/backlog_service_triage.go).
  const retriggerTriageCore = useCallback(
    async (targetItemId: string, currentStatus: string) => {
      if (currentStatus !== "idea" && currentStatus !== "ready") {
        await transitionStatus(targetItemId, "idea");
      }
      await triggerTriage(targetItemId);
    },
    [transitionStatus, triggerTriage]
  );

  const handleAction = useCallback(
    async (action: string) => {
      if (!item) return;
      setActionLoading(action);
      const toastKey = `${item.id}:${action}`;
      let successMessage = ACTION_SUCCESS_MESSAGES[action] ?? "Done.";
      try {
        switch (action) {
          case "mark_ready":
            await transitionStatus(item.id, "ready");
            break;
          case "trigger_triage":
            await triggerTriage(item.id);
            break;
          case "spawn_session": {
            const resp = await spawnSessionFromItem(item.id);
            if (resp?.queued) successMessage = "At capacity — item queued, will start automatically.";
            break;
          }
          case "spawn_session_autonomous": {
            const resp = await spawnSessionFromItem(item.id, { autonomous: true });
            if (resp?.queued) successMessage = "At capacity — item queued, will start automatically.";
            break;
          }
          case "restart_session":
            await spawnSessionFromItem(item.id, { force: true });
            break;
          case "approve_plan":
            await approvePlan(item.id);
            break;
          case "retry_triage":
            await retriggerTriageCore(item.id, item.status);
            break;
          case "mark_done":
            await transitionStatus(item.id, "done");
            break;
          case "override_done": {
            const reviewSession = item.linkedSessions.filter((s) => s.role === "review").at(-1);
            if (reviewSession) {
              await overrideVerdict(reviewSession.entityId, "Manual override to done", "done");
            } else {
              if (mountedRef.current) setError("No review session found — cannot override verdict.");
              return;
            }
            break;
          }
          case "re_review":
            await triggerReReview(item.id);
            break;
          case "ship_pr":
            await triggerShipPR(item.id);
            break;
          case "manual_review":
            setShowManualReview(true);
            return;
          case "archive":
            if (
              !confirm(
                "Archive this item? It will be hidden from the default view. Its git worktree (if any) will be deleted from disk and cannot be recreated by unarchiving.",
              )
            )
              return;
            await archiveBacklogItem(item.id);
            break;
          case "unarchive":
            await unarchiveBacklogItem(item.id);
            break;
          case "delete":
            if (!confirm("Permanently delete this item and all its history? This cannot be undone.")) return;
            await deleteBacklogItem(item.id);
            onClose?.();
            return;
          case "reopen":
            await transitionStatus(item.id, "review");
            break;
          case "send_back_idea":
            await transitionStatus(item.id, "idea");
            break;
          case "send_back_refining":
            await transitionStatus(item.id, "refining");
            break;
          case "send_back_ready":
            await transitionStatus(item.id, "ready");
            break;
          default:
            return;
        }
        showActionToast(successMessage, "success", toastKey);
        await load();
      } catch (e) {
        const msg = e instanceof Error ? e.message : "Action failed.";
        if (mountedRef.current) setError(msg);
        showActionToast(msg, "error", toastKey);
      } finally {
        if (mountedRef.current) setActionLoading(null);
      }
    },
    [item, transitionStatus, triggerTriage, retriggerTriageCore, spawnSessionFromItem, approvePlan, overrideVerdict, triggerReReview, triggerShipPR, archiveBacklogItem, unarchiveBacklogItem, deleteBacklogItem, onClose, load, showActionToast]
  );

  // Extracted verbatim from the inline manual-review-submit onClick handler
  // (Story 3.1.3, Task 3.1.3b) so ActionsSection stays a props-down/
  // callbacks-up presentational component.
  const handleManualReviewSubmit = useCallback(async () => {
    if (!item) return;
    const toastKey = `${item.id}:manual_review_submit`;
    setActionLoading("manual_review_submit");
    try {
      await submitManualReview(item.id, manualReviewOutcome, manualReviewSummary.trim());
      showActionToast("Review submitted.", "success", toastKey);
      setShowManualReview(false);
      setManualReviewSummary("");
      setManualReviewOutcome("PASS");
      await load();
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Submit failed.";
      if (mountedRef.current) setError(msg);
      showActionToast(msg, "error", toastKey);
    } finally {
      if (mountedRef.current) setActionLoading(null);
    }
  }, [item, manualReviewOutcome, manualReviewSummary, submitManualReview, showActionToast, load]);

  // The backend writes skipPlanning/skipReviewGate/autoSpawnSession/autoCreatePR
  // unconditionally on every UpdateBacklogItem call (they're plain proto bools, not
  // optional — no "unset" wire representation), so any partial update that omits them
  // silently resets them to false. Every partial updateBacklogItem call below must
  // spread these current values.
  const currentFlags = useCallback(
    () => ({
      skipPlanning: item?.skipPlanning ?? false,
      skipReviewGate: item?.skipReviewGate ?? false,
      autoSpawnSession: item?.autoSpawnSession ?? false,
      autoCreatePR: item?.autoCreatePR ?? false,
    }),
    [item]
  );

  const handleSaveNotes = useCallback(async () => {
    if (!item) return;
    setActionLoading("save_notes");
    try {
      const updated = await updateBacklogItem(item.id, { ...currentFlags(), notes: notesValue });
      if (!mountedRef.current) return;
      if (updated) setItem(updated);
      setEditingNotes(false);
    } finally {
      if (mountedRef.current) setActionLoading(null);
    }
  }, [item, notesValue, updateBacklogItem, currentFlags]);

  const handleUpdateItem = useCallback(
    async (data: BacklogItemInput) => {
      if (!item) return;
      const updated = await updateBacklogItem(item.id, data);
      if (updated) {
        setItem(updated);
        setNotesValue(updated.notes ?? "");
      }
      setEditMode(false);
    },
    [item, updateBacklogItem]
  );

  // Task 5.3.2c: the form's Save button submits through this wrapper instead
  // of calling handleUpdateItem directly. If a live update landed while
  // editing (bufferedItem is set), the save is intercepted — the real save
  // RPC is not called until the user explicitly picks "Save Anyway" or
  // "Reload" on the warn-before-overwrite banner below.
  const handleFormSubmit = useCallback(
    async (data: BacklogItemInput) => {
      if (bufferedItem) {
        setPendingSaveData(data);
        setSaveConfirmPending(true);
        return;
      }
      await handleUpdateItem(data);
    },
    [bufferedItem, handleUpdateItem]
  );

  /** "Save Anyway" on the warn-before-overwrite banner: proceeds with the original save, discarding the buffered server-side change. */
  const handleSaveAnyway = useCallback(async () => {
    if (!pendingSaveData) return;
    const data = pendingSaveData;
    setPendingSaveData(null);
    setSaveConfirmPending(false);
    setBufferedItem(null);
    await handleUpdateItem(data);
  }, [pendingSaveData, handleUpdateItem]);

  /** Plain "Reload" on the buffered-update banner (Task 5.3.2b) — applies the buffered update but stays in edit mode with the refreshed values. */
  const handleReloadBuffered = useCallback(() => {
    if (!bufferedItem) return;
    setItem(bufferedItem);
    setNotesValue(bufferedItem.notes ?? "");
    setBufferedItem(null);
  }, [bufferedItem]);

  /** "Reload" on the warn-before-overwrite banner (Task 5.3.2c) — discards the in-progress edit, applies the buffered update, and returns to view mode without saving. */
  const handleConfirmReload = useCallback(() => {
    if (bufferedItem) {
      setItem(bufferedItem);
      setNotesValue(bufferedItem.notes ?? "");
    }
    setBufferedItem(null);
    setPendingSaveData(null);
    setSaveConfirmPending(false);
    setEditMode(false);
  }, [bufferedItem]);

  const handleCancelTriage = useCallback(async () => {
    if (!item) return;
    const toastKey = `${item.id}:cancel_triage`;
    setActionLoading("cancel_triage");
    try {
      await cancelTriage(item.id);
      showActionToast("Triage cancelled.", "success", toastKey);
      await load();
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Cancel failed.";
      if (mountedRef.current) setError(msg);
      showActionToast(msg, "error", toastKey);
    } finally {
      if (mountedRef.current) setActionLoading(null);
    }
  }, [item, cancelTriage, load, showActionToast]);

  const handleRetriggerTriage = useCallback(async () => {
    if (!item) return;
    const toastKey = `${item.id}:retrigger_triage`;
    setActionLoading("retrigger_triage");
    try {
      await retriggerTriageCore(item.id, item.status);
      showActionToast("Triage re-triggered.", "success", toastKey);
      await load();
    } catch (e) {
      console.error("[BacklogItemDetail] retrigger triage failed", e);
      const msg = e instanceof Error ? e.message : "Triage re-trigger failed.";
      if (mountedRef.current) setError(msg);
      showActionToast(msg, "error", toastKey);
    } finally {
      if (mountedRef.current) setActionLoading(null);
    }
  }, [item, retriggerTriageCore, load, showActionToast]);

  const handleRefineTriage = useCallback(
    async (feedback: string) => {
      if (!item) return;
      await triggerTriage(item.id, feedback);
      await load();
    },
    [item, triggerTriage, load]
  );

  // Story 4.3.1: RejectPlan only persists state (ADR-002) — it does not
  // itself trigger regeneration. handleRegeneratePlanWithFeedback is the
  // separate, explicit follow-up action PlanVerdictBox renders once an item
  // is in the changes_requested state.
  const handleRejectPlan = useCallback(
    async (reason: string) => {
      if (!item) return;
      const toastKey = `${item.id}:reject_plan`;
      setActionLoading("reject_plan");
      try {
        await rejectPlan(item.id, reason);
        showActionToast("Revisions requested.", "success", toastKey);
        await load();
      } catch (e) {
        showActionToast(e instanceof Error ? e.message : "Reject failed.", "error", toastKey);
        throw e;
      } finally {
        if (mountedRef.current) setActionLoading(null);
      }
    },
    [item, rejectPlan, load, showActionToast]
  );

  const handleRegeneratePlanWithFeedback = useCallback(async () => {
    if (!item?.planRejectionReason) return;
    await triggerTriage(item.id, item.planRejectionReason);
    await load();
  }, [item, triggerTriage, load]);

  const handleApplyTriageSuggestions = useCallback(
    async (preApplyCriteria: AcCriterion[]) => {
      if (!item) return;
      // Step 1: Update AC with suggestions
      const acSuggestions = item.triageResult?.suggestions.filter((s) => s.rationale !== "question") ?? [];
      const newAcCriteria: AcCriterion[] = acSuggestions.map((s, i) => ({
        index: i,
        text: s.text,
        status: "pending" as const,
      }));
      const updated = await updateBacklogItem(item.id, { ...currentFlags(), acCriteria: newAcCriteria });
      if (!updated) {
        throw new Error("Failed to apply suggestions — item may have been modified by another process. Reload and try again.");
      }
      // Step 2: Transition to ready
      const transitioned = await transitionStatus(item.id, "ready", { expectedStatus: "idea" });
      if (!transitioned) {
        throw new Error("Failed to mark item ready — please try again.");
      }
      await load();
      // Store pre-apply criteria for undo (returned via throw on error, used by panel)
      void preApplyCriteria; // captured in panel for undo
    },
    [item, updateBacklogItem, transitionStatus, load, currentFlags]
  );

  const handleUndoTriageSuggestions = useCallback(
    async (preApplyCriteria: AcCriterion[]) => {
      if (!item) return;
      // Called via `void onUndoApply(...)` from TriageReviewPanel — nothing upstream
      // awaits this, so failures must be caught and surfaced here, not thrown.
      try {
        // Revert AC to the pre-apply snapshot
        const updated = await updateBacklogItem(item.id, { ...currentFlags(), acCriteria: preApplyCriteria });
        if (!updated) {
          throw new Error("Failed to undo — item may have been modified. Reload and try again.");
        }
        // Revert status back to idea
        await transitionStatus(item.id, "idea");
        showActionToast("Undo applied.", "success", `${item.id}:undo_triage_apply`);
        await load();
      } catch (e) {
        const msg = e instanceof Error ? e.message : "Undo failed.";
        if (mountedRef.current) setError(msg);
        showActionToast(msg, "error", `${item.id}:undo_triage_apply`);
      }
    },
    [item, updateBacklogItem, transitionStatus, load, currentFlags, showActionToast]
  );

  // The four handlers below are called from GateVerdictBox, which already wraps each
  // call in its own try/catch and shows a local InlineError on failure. We add a toast
  // here too (rethrowing so GateVerdictBox's own handling still runs) since the inline
  // banner there is easy to miss.
  const handleGateApprove = useCallback(async () => {
    if (!item) return;
    const toastKey = `${item.id}:gate_approve`;
    setActionLoading("gate_approve");
    try {
      const ok = await transitionStatus(item.id, "done");
      if (!ok) {
        const msg = lastError?.message ?? "Failed to approve — please try again.";
        if (mountedRef.current) setError(msg);
        showActionToast(msg, "error", toastKey);
        return;
      }
      showActionToast("Approved.", "success", toastKey);
      await load();
    } catch (e) {
      showActionToast(e instanceof Error ? e.message : "Approve failed.", "error", toastKey);
      throw e;
    } finally {
      if (mountedRef.current) setActionLoading(null);
    }
  }, [item, transitionStatus, lastError, load, showActionToast]);

  const handleGateReopen = useCallback(async (feedback: string) => {
    if (!item) return;
    const toastKey = `${item.id}:gate_reopen`;
    setActionLoading("gate_reopen");
    try {
      // Append feedback to notes so the next work session sees it in its prompt.
      if (feedback.trim()) {
        const timestamp = new Date().toISOString().slice(0, 16).replace("T", " ");
        const note = `\n\n[Revision feedback ${timestamp}]\n${feedback.trim()}`;
        await updateBacklogItem(item.id, { ...currentFlags(), notes: (item.notes ?? "") + note });
      }
      await transitionStatus(item.id, "in_progress");
      // Spawn a new work session immediately — the backend now accepts in_progress.
      await spawnSessionFromItem(item.id);
      showActionToast("Reopened — new session started.", "success", toastKey);
      await load();
    } catch (e) {
      showActionToast(e instanceof Error ? e.message : "Reopen failed.", "error", toastKey);
      throw e;
    } finally {
      if (mountedRef.current) setActionLoading(null);
    }
  }, [item, transitionStatus, spawnSessionFromItem, updateBacklogItem, load, currentFlags, showActionToast]);

  const handleGateOverride = useCallback(
    async (reason: string) => {
      if (!item) return;
      const toastKey = `${item.id}:gate_override`;
      setActionLoading("gate_override");
      try {
        const reviewSession = item.linkedSessions.filter((s) => s.role === "review").at(-1);
        if (!reviewSession) {
          const msg = "No review session found — cannot override verdict.";
          if (mountedRef.current) setError(msg);
          showActionToast(msg, "error", toastKey);
          return;
        }
        await overrideVerdict(reviewSession.entityId, reason, "done");
        showActionToast("Overridden to done.", "success", toastKey);
        await load();
      } catch (e) {
        const msg = e instanceof Error ? e.message : "Override failed.";
        if (mountedRef.current) setError(msg);
        showActionToast(msg, "error", toastKey);
        throw e;
      } finally {
        if (mountedRef.current) setActionLoading(null);
      }
    },
    [item, overrideVerdict, load, showActionToast]
  );

  // Manual overrides (ManualOverrideSection) — the operator escape hatch for
  // an item whose automation has gotten wedged. Unlike the four handlers
  // above (called from GateVerdictBox, status-conditional on being in
  // review), these are always reachable regardless of status.
  const handleManualStatusOverride = useCallback(
    async (toStatus: string, reason: string) => {
      if (!item) return;
      const toastKey = `${item.id}:manual_override_status`;
      setActionLoading("manual_override_status");
      try {
        // expectedUpdatedAt uses item.updatedAtRaw (the undecoded protobuf
        // Timestamp), not item.updatedAt (a display-oriented ISO string) —
        // an earlier version of this code built the precondition from the
        // ISO string via `new Date(...)` + timestampFromDate, which is only
        // millisecond-precision and can never exactly match the server's
        // nanosecond-precision updated_at column (ent's CAS check is exact
        // equality), so every write spuriously failed. Passing the raw
        // Timestamp through verbatim avoids the round-trip entirely.
        const ok = await transitionStatus(item.id, toStatus, {
          expectedStatus: item.status,
          expectedUpdatedAt: item.updatedAtRaw,
          overrideReason: reason,
        });
        if (!ok) {
          const msg = lastError?.message ?? "Someone else changed this item — reload and try again.";
          showActionToast(msg, "error", toastKey);
          throw new Error(msg);
        }
        showActionToast(`Status manually overridden to ${toStatus}.`, "success", toastKey);
        await load();
      } catch (e) {
        showActionToast(e instanceof Error ? e.message : "Status override failed.", "error", toastKey);
        throw e;
      } finally {
        if (mountedRef.current) setActionLoading(null);
      }
    },
    [item, transitionStatus, lastError, load, showActionToast]
  );

  const handleManualAssociatePR = useCallback(
    async (prUrl: string, prNumber: number) => {
      if (!item) return;
      const toastKey = `${item.id}:manual_associate_pr`;
      setActionLoading("manual_associate_pr");
      try {
        const updated = await updateBacklogItem(item.id, { ...currentFlags(), prUrl, prNumber });
        if (!updated) {
          const msg = lastError?.message ?? "Failed to link PR — please try again.";
          showActionToast(msg, "error", toastKey);
          throw new Error(msg);
        }
        showActionToast(`PR #${prNumber} linked — item moved to pr_pending.`, "success", toastKey);
        await load();
      } catch (e) {
        showActionToast(e instanceof Error ? e.message : "PR association failed.", "error", toastKey);
        throw e;
      } finally {
        if (mountedRef.current) setActionLoading(null);
      }
    },
    [item, updateBacklogItem, lastError, load, currentFlags, showActionToast]
  );

  const handleGateSkip = useCallback(async () => {
    if (!item) return;
    const toastKey = `${item.id}:gate_skip`;
    setActionLoading("gate_skip");
    try {
      const reviewSession = item.linkedSessions.filter((s) => s.role === "review").at(-1);
      if (reviewSession) {
        await overrideVerdict(reviewSession.entityId, "Gate skipped by user", "done");
      } else {
        // No review session yet — direct transition (item.skipReviewGate path)
        const ok = await transitionStatus(item.id, "done");
        if (!ok) {
          const msg = lastError?.message ?? "Failed to skip gate — please try again.";
          if (mountedRef.current) setError(msg);
          showActionToast(msg, "error", toastKey);
          return;
        }
      }
      showActionToast("Gate skipped — marked done.", "success", toastKey);
      await load();
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Skip gate failed.";
      if (mountedRef.current) setError(msg);
      showActionToast(msg, "error", toastKey);
      throw e;
    } finally {
      if (mountedRef.current) setActionLoading(null);
    }
  }, [item, overrideVerdict, transitionStatus, lastError, load, showActionToast]);

  // Only show the full-screen loader on the INITIAL load (no item yet). Background
  // refreshes must NOT unmount the detail view / edit form, or in-progress edits
  // like unsaved acceptance criteria get discarded when the form remounts.
  // See stapler-squad#146.
  if (loading && !item) {
    return (
      <article className={styles.container} data-testid="backlog-item-detail">
        {onClose && (
          <div className={styles.header}>
            <div className={styles.headerRow}>
              <span />
              <div className={styles.headerActions}>
                <button className={styles.closeButton} onClick={onClose} aria-label="Close">×</button>
              </div>
            </div>
          </div>
        )}
        <div className={styles.loadingState} role="status" aria-label="Loading backlog item">
          Loading…
        </div>
      </article>
    );
  }

  if (!item) {
    return (
      <article className={styles.container} data-testid="backlog-item-detail">
        {onClose && (
          <div className={styles.header}>
            <div className={styles.headerRow}>
              <span />
              <div className={styles.headerActions}>
                <button className={styles.closeButton} onClick={onClose} aria-label="Close">×</button>
              </div>
            </div>
          </div>
        )}
        <div className={styles.errorState} role="alert">
          {error ?? "Item not found."}
        </div>
      </article>
    );
  }

  if (editMode) {
    return (
      <article
        className={styles.container}
        data-testid="backlog-item-detail"
        aria-label={`Edit backlog item: ${item.title}`}
      >
        <div className={styles.header}>
          <div className={styles.headerRow}>
            <div className={styles.titleGroup}>
              <h2 className={styles.itemTitle}>{item.title}</h2>
            </div>
            <div className={styles.headerActions}>
              <button
                className={styles.closeButton}
                onClick={() => setEditMode(false)}
                aria-label="Cancel editing"
                data-testid="backlog-detail-cancel-edit"
              >
                ×
              </button>
            </div>
          </div>
        </div>
        {/* Epic 6.4: pinned outside scrollArea so the notice can't scroll out
            of view while editing a long form (ux.md §6 copy/placement pass —
            non-blocking, informational styling, matches InlineError's
            informational variant rather than its assertive/error one). */}
        {saveConfirmPending ? (
          <div className={styles.bannerBar}>
            <InlineNotice
              message="Saving will overwrite a change made elsewhere — Reload first?"
              actions={[
                { label: "Save Anyway", onClick: () => void handleSaveAnyway(), variant: "primary" },
                { label: "Reload", onClick: handleConfirmReload },
              ]}
              data-testid="backlog-detail-save-conflict-notice"
            />
          </div>
        ) : bufferedItem ? (
          <div className={styles.bannerBar}>
            <InlineNotice
              message="This item changed elsewhere."
              actions={[{ label: "Reload", onClick: handleReloadBuffered }]}
              onDismiss={() => setBufferedItem(null)}
              data-testid="backlog-detail-buffered-update-notice"
            />
          </div>
        ) : null}
        <div className={styles.scrollArea}>
          <BacklogItemForm
            key={`${item.id}:${item.updatedAt ?? ""}`}
            initialValues={item}
            onSubmit={handleFormSubmit}
            onCancel={() => setEditMode(false)}
          />
        </div>
      </article>
    );
  }

  return (
    <article
      className={styles.container}
      data-testid="backlog-item-detail"
      aria-label={`Backlog item: ${item.title}`}
    >
      {/* Sticky header — always visible regardless of scroll */}
      <div className={styles.header}>
        <div className={styles.headerRow}>
          <div className={styles.titleGroup}>
            <h2 className={styles.itemTitle}>{item.title}</h2>
            <div className={styles.metaRow}>
              <span
                className={styles.priorityBadge}
                aria-label={`Priority: ${PRIORITY_LABELS[item.priority] ?? "Unknown"}`}
              >
                {PRIORITY_LABELS[item.priority] ?? "P?"}
              </span>
              {item.createdAt && (
                <span className={styles.dateMeta}>
                  Created {formatDate(item.createdAt)}
                </span>
              )}
              {item.updatedAt && (
                <span className={styles.dateMeta}>
                  · Updated {formatDate(item.updatedAt)}
                </span>
              )}
            </div>
            <div className={styles.idRow}>
              <span className={styles.idText} data-testid="backlog-item-id">{item.id}</span>
              <button
                type="button"
                className={styles.copyButton}
                onClick={() => handleCopy("id", item.id)}
                aria-label="Copy item ID"
                data-testid="copy-item-id-button"
              >
                {copiedField === "id" ? "✓ Copied" : "Copy ID"}
              </button>
              <button
                type="button"
                className={styles.copyButton}
                onClick={() => handleCopy("link", `${window.location.origin}/backlog?item=${item.id}`)}
                aria-label="Copy shareable link"
                data-testid="copy-item-link-button"
              >
                {copiedField === "link" ? "✓ Copied" : "Copy Link"}
              </button>
            </div>
          </div>
          <div className={styles.headerActions}>
            <ConnectionIndicator connectionState={connectionState} />
            {!terminalState && (
              <button
                className={styles.editButton}
                onClick={() => setEditMode(true)}
                aria-label="Edit item"
                data-testid="backlog-detail-edit"
              >
                Edit
              </button>
            )}
            {onClose && (
              <button
                className={styles.closeButton}
                onClick={onClose}
                aria-label="Close item detail"
                data-testid="backlog-detail-close"
              >
                ×
              </button>
            )}
          </div>
        </div>
        {/* Always-visible lifecycle summary — the single authoritative
            status display, replacing the old standalone status badge (D1). */}
        <LifecycleSummary item={item} pipelineDisplay={pipelineDisplay} stuckItem={stuckItem} />
      </div>

      <div className={styles.scrollArea}>
        {/* Inline action error banner */}
        {error && (
          <div className={styles.errorBanner} role="alert">
            <span>{error}</span>
            <button
              className={styles.errorBannerDismiss}
              onClick={() => setError(null)}
              aria-label="Dismiss error"
            >
              ×
            </button>
          </div>
        )}

        {/* Triage Progress Indicator */}
        {(item.status === "idea" || item.status === "ready") && item.triageStatus === "running" && (
          <div className={styles.section}>
            <TriageLoadingIndicator
              elapsedSeconds={triageElapsedSeconds}
              context="item"
              onCancel={actionLoading !== null ? () => {} : handleCancelTriage}
              compact={false}
            />
          </div>
        )}

        {/* Triage Review Panel — only shown while still in idea status */}
        {item.triageStatus === "completed" &&
          item.status === "idea" &&
          item.triageResult && (
            <div className={styles.section} aria-live="polite">
              <TriageReviewPanel
                item={item}
                triageResult={item.triageResult}
                onApply={handleApplyTriageSuggestions}
                onUndoApply={handleUndoTriageSuggestions}
                // Dismissing the panel is a purely client-local state change
                // (localStorage + local React state, no server mutation) —
                // nothing to refresh (pitfall #3). A load() here was a
                // needless, unguarded race source that could stomp
                // more-current live-store state with a redundant read.
                onSkip={() => {}}
                onRefine={handleRefineTriage}
                onAnswerQuestion={handleRefineTriage}
              />
            </div>
          )}

        {/* Planning record — read-only triage result for items past idea status */}
        <PlanningSection item={item} />

        {/* Triage failed banner — generalized 2026-08-03
            (docs/tasks/backlog-feature-improvement.md, item be676dab) from
            idea-only to also cover a queued item gated on plan approval: a
            triage session can end with no usable result AFTER the item has
            already advanced past idea (e.g. queued via the WIP cap), and
            before this fix that state rendered nothing at all — no summary,
            no retry affordance, just the stuck badge. Not extended to
            "ready": a ready item's CURRENT plan is reliably in place (see
            itemActions.ts's doc comment), so a failed *later* refine attempt
            there shouldn't imply the existing plan is invalid. */}
        {item.triageStatus === "failed" &&
          !item.skipPlanning &&
          !item.planApproved &&
          (item.status === "idea" || item.status === "queued") && (
            <div className={styles.section}>
              <InlineError
                type="permanent"
                customMessage={
                  item.status === "queued"
                    ? "This item's most recent triage session ended without producing a usable plan. Retry triage to generate one."
                    : undefined
                }
                // InlineError itself has no disabled/loading state (unmodified,
                // pre-existing component) — guard the same way
                // TriageLoadingIndicator's onCancel does above, swapping in a
                // no-op while a retry is already in flight. Without this, a
                // double-click fires two concurrent retriggerTriageCore calls,
                // each doing transitionStatus(id,"idea") + triggerTriage(id) —
                // worse for a queued item (two competing status resets) than the
                // pre-existing idea-only exposure of this same button.
                onRetry={actionLoading !== null ? () => {} : handleRetriggerTriage}
              />
            </div>
          )}

        {/* Diff modal — reused by the review-flow "View Changes" button above and
            the Version Control section's "View Diff" button below; works for any
            status since GetBacklogItemDiff resolves the shipped range from durable
            git history, not a live session. */}
        {showChangesModal && (
          <ReviewChangesModal
            itemId={item.id}
            sessionId={latestWorkSession?.sessionId}
            sessionTitle={item.title}
            onClose={() => setShowChangesModal(false)}
          />
        )}

        {showFileBrowser && latestWorkSession && (
          <BacklogFileBrowserModal
            sessionId={latestWorkSession.sessionId}
            sessionTitle={item.title}
            onClose={() => setShowFileBrowser(false)}
          />
        )}

        {/* Acceptance Criteria */}
        <div className={styles.section}>
          <h3 className={styles.sectionTitle}>
            Acceptance Criteria ({item.acCriteria.filter((c) => c.status === "done").length}/{item.acCriteria.length})
          </h3>
          <AcCriteriaList criteria={item.acCriteria} />
        </div>

        {/* Plan Review (Story 4.3.1): rendered immediately before Actions,
            NOT after PlanArtifactsSection's plan-artifacts display as
            plan.md originally described — Story 3.1.3's later refactor
            (see the CollapsibleGroup comment below) already pulled
            ActionsSection out ahead of the collapsible group so it stays
            reachable without expanding anything; PlanVerdictBox follows the
            same always-visible convention and sits right above it, so
            Approve (in ActionsSection) and Request Changes (here) are both
            reachable without navigating away (AC3). */}
        {(item.status === "ready" ||
          item.status === "queued" ||
          derivePlanReviewStatus(item) !== "no_plan") && (
          <PlanVerdictBox
            status={derivePlanReviewStatus(item)}
            rejectionReason={item.planRejectionReason}
            readOnly={terminalState !== null}
            actionPending={actionLoading === "reject_plan"}
            onReject={handleRejectPlan}
            onRegenerateWithFeedback={handleRegeneratePlanWithFeedback}
          />
        )}

        {/* Actions */}
        <ActionsSection
          item={item}
          actionLoading={actionLoading}
          latestWorkSession={latestWorkSession}
          showManualReview={showManualReview}
          manualReviewOutcome={manualReviewOutcome}
          manualReviewSummary={manualReviewSummary}
          onAction={handleAction}
          onManualReviewOutcomeChange={setManualReviewOutcome}
          onManualReviewSummaryChange={setManualReviewSummary}
          onManualReviewSubmit={() => { void handleManualReviewSubmit(); }}
          onManualReviewCancel={() => { setShowManualReview(false); setManualReviewSummary(""); }}
          terminalState={terminalState}
        />

        {/* Secondary sections — sibling CollapsibleSections sharing one
            CollapsibleGroup so their headers get Home/End/Arrow roving-
            tabindex keyboard nav across all of them (Task 3.1.4i; the
            concrete justification ADR-027 gives for Radix Accordion over
            @radix-ui/react-collapsible). Expand state is persisted per
            item/section via useSectionExpandState and threaded through the
            group as a controlled value (Story 1.1.1's persistence
            contract). PlanningSection/ActionsSection are primary,
            always-visible content and intentionally sit outside the
            group — since Actions needs to stay reachable without
            expanding anything, it is positioned before the group rather
            than in its original position between Description and Plan
            Artifacts, so every Collapsible-wrapped section (including
            Reviewing/Pull Request, extracted in Story 3.1.2) can share one
            contiguous Radix Root. */}
        <CollapsibleGroup value={openSectionKeys} onValueChange={handleGroupValueChange}>
          {item.status === "review" && (
            <ReviewingSection
              item={item}
              workSession={latestWorkSession}
              actionLoading={actionLoading}
              defaultExpanded={reviewingExpanded}
              onViewChanges={() => setShowChangesModal(true)}
              onGateApprove={handleGateApprove}
              onGateReopen={handleGateReopen}
              onGateOverride={handleGateOverride}
              onGateSkip={handleGateSkip}
              onReReview={() => triggerReReview(item.id).then(() => load())}
              readOnly={terminalState !== null}
            />
          )}

          {item.status !== "review" && item.gateVerdict && (
            <LastReviewResultSection item={item} defaultExpanded={lastReviewResultExpanded} />
          )}

          {item.status === "pr_pending" && (
            <PullRequestSection
              item={item}
              actionLoading={actionLoading}
              onMarkDone={() => handleAction("mark_done")}
              readOnly={terminalState !== null}
            />
          )}

          {item.externalUrl && (
            <SourceSection
              externalUrl={item.externalUrl}
              externalId={item.externalId}
              labels={item.labels ?? []}
              defaultExpanded={sourceExpanded}
            />
          )}

          <DescriptionSection item={item} defaultExpanded={descriptionExpanded} />

          {item.planArtifactsPath && (
            <PlanArtifactsSection item={item} defaultExpanded={planArtifactsExpanded} />
          )}

          <VersionControlSection
            item={item}
            widgetData={vcsWidgetData}
            activeSessionCount={activeWorkSessionCount}
            worktreePath={latestWorkSession?.worktreePath}
            defaultExpanded={versionControlExpanded}
            onViewDiff={() => setShowChangesModal(true)}
            onBrowseFiles={() => setShowFileBrowser(true)}
          />

          <SessionsSection
            item={item}
            pipelineModes={pipelineModes}
            latestWorkSession={latestWorkSession}
            deletingSessionId={deletingSessionId}
            defaultExpanded={sessionsExpanded}
            onDeleteSession={handleDeleteSession}
            onSteerSession={handleSteerSession}
            steeringSessionId={steeringSessionId}
          />

          <WorkflowHistorySection item={item} defaultExpanded={workflowExpanded} />

          <ProgressHistorySection item={item} defaultExpanded={progressHistoryExpanded} />

          <NotesSection
            item={item}
            actionLoading={actionLoading}
            editingNotes={editingNotes}
            notesValue={notesValue}
            defaultExpanded={notesExpanded}
            onNotesValueChange={setNotesValue}
            onStartEditing={() => setEditingNotes(true)}
            onSave={handleSaveNotes}
            onCancel={() => {
              setNotesValue(item.notes ?? "");
              setEditingNotes(false);
            }}
          />

          <ManualOverrideSection
            item={item}
            defaultExpanded={manualOverrideExpanded}
            readOnly={terminalState !== null}
            onOverrideStatus={handleManualStatusOverride}
            onAssociatePR={handleManualAssociatePR}
          />
        </CollapsibleGroup>
      </div>

    </article>
  );
}
