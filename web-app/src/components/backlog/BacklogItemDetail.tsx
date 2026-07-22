"use client";
// +feature: backlog:item-detail

import { useState, useEffect, useCallback, useRef } from "react";
import type { BacklogItem, AcCriterion, BacklogItemInput, LinkedSession, PipelineMode } from "@/lib/hooks/useBacklogService";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { useAnalytics } from "@/lib/analytics";
import { useCurrentWorkSession } from "@/lib/backlog/currentWorkSession";
import { classifySessionKind } from "@/lib/backlog/sessionKind";
import { resolvePipelineModeDisplay } from "@/lib/backlog/pipelineModeDisplay";
import { formatDate } from "@/lib/backlog/formatDate";
import { useVcsStatus } from "@/lib/hooks/useVcsStatus";
import { useBacklogItemShipStatus } from "@/lib/hooks/useBacklogItemShipStatus";
import { getApiBaseUrl } from "@/lib/config";
import { fromSessionVcs, fromShipStatus } from "@/lib/vcs/adapters";
import { useSectionExpandState } from "@/lib/hooks/useSectionExpandState";
import { CollapsibleGroup } from "@/components/ui/Collapsible";
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
import { PullRequestSection } from "./detail/PullRequestSection";
import { DescriptionSection } from "./detail/DescriptionSection";
import { ActionsSection } from "./detail/ActionsSection";
import { PlanArtifactsSection } from "./detail/PlanArtifactsSection";
import { VersionControlSection } from "./detail/VersionControlSection";
import { SessionsSection } from "./detail/SessionsSection";
import { WorkflowHistorySection } from "./detail/WorkflowHistorySection";
import { ProgressHistorySection } from "./detail/ProgressHistorySection";
import { NotesSection } from "./detail/NotesSection";
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
  mark_done: "Marked done.",
  override_done: "Overridden to done.",
  re_review: "Re-review triggered.",
  ship_pr: "PR created.",
  archive: "Archived.",
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
    overrideVerdict,
    triggerReReview,
    triggerShipPR,
    submitManualReview,
    archiveBacklogItem,
    deleteBacklogItem,
    updateBacklogItem,
    listPipelineModes,
    lastError,
  } = useBacklogService();
  const { deleteSession } = useSessionService();
  const { showActionToast } = useNotifications();
  const [item, setItem] = useState<BacklogItem | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  /** The action key currently in flight (e.g. "mark_ready"), or null when idle. */
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [editMode, setEditMode] = useState(false);
  const [deletingSessionId, setDeletingSessionId] = useState<string | null>(null);

  // Epic 3.4 "what ran" surface: the currently-fetched mode list, used only
  // to resolve a session's frozen pipelineModeSnapshot slug to a
  // human-readable name (and to detect content drift). Fetch failure
  // degrades every session's "Pipeline" group to the unrecognized-mode
  // fallback rather than blocking the rest of the item detail view.
  const [pipelineModes, setPipelineModes] = useState<PipelineMode[]>([]);

  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
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
  const [pullRequestExpanded, setPullRequestExpanded] = useSectionExpandState(itemId, "pull-request", true);
  const [planArtifactsExpanded, setPlanArtifactsExpanded] = useSectionExpandState(itemId, "plan-artifacts", false);
  const [versionControlExpanded, setVersionControlExpanded] = useSectionExpandState(itemId, "version-control", false);
  const [sessionsExpanded, setSessionsExpanded] = useSectionExpandState(itemId, "sessions", true);
  const [workflowExpanded, setWorkflowExpanded] = useSectionExpandState(itemId, "workflow", false);
  const [progressHistoryExpanded, setProgressHistoryExpanded] = useSectionExpandState(itemId, "progress-history", false);
  const [notesExpanded, setNotesExpanded] = useSectionExpandState(itemId, "notes", false);
  const [descriptionExpanded, setDescriptionExpanded] = useSectionExpandState(itemId, "description", false);

  // Story 3.1.5: applies each status-dependent section's real default
  // exactly once, the first time `item` becomes available after this
  // instance's initial load — never again afterward (guarded by the ref),
  // so a later poll-driven status change can never retroactively re-open a
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
    ["pull-request", pullRequestExpanded, setPullRequestExpanded],
    ["description", descriptionExpanded, setDescriptionExpanded],
    ["plan-artifacts", planArtifactsExpanded, setPlanArtifactsExpanded],
    ["version-control", versionControlExpanded, setVersionControlExpanded],
    ["sessions", sessionsExpanded, setSessionsExpanded],
    ["workflow", workflowExpanded, setWorkflowExpanded],
    ["progress-history", progressHistoryExpanded, setProgressHistoryExpanded],
    ["notes", notesExpanded, setNotesExpanded],
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
    setLoading(true);
    setError(null);
    try {
      const result = await getBacklogItem(itemId);
      if (!mountedRef.current) return;
      if (!result) {
        setError("Item not found.");
      } else {
        setItem(result);
        setNotesValue(result.notes ?? "");
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

  // Poll for updated item data while triage is running or while in review (waiting for gate verdict).
  // Suspend polling while the edit form is open, the manual-review form is
  // open (Story 3.1.3, Task 3.1.3c — a poll firing mid-draft would clobber
  // unsaved manual-review text the same way it would clobber an open edit
  // form), or any action is in flight (Task 3.1.3c, pre-mortem P1 #4 — a
  // poll-triggered re-render while an Approve/Override request with real
  // backend side effects is in flight could unmount its containing section,
  // risking a double-submit).
  useEffect(() => {
    const shouldPoll = (item?.triageStatus === "running" || (item?.status === "review" && (!item?.gateVerdict || item.gateVerdict === "PENDING")) || item?.status === "pr_pending") && !editMode && !showManualReview && actionLoading === null;
    if (!shouldPoll) return;
    const interval = setInterval(() => { void load(); }, 5_000);
    return () => clearInterval(interval);
  }, [item?.triageStatus, item?.status, item?.gateVerdict, editMode, showManualReview, actionLoading, load]); // item.status covers pr_pending polling

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
            await archiveBacklogItem(item.id);
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
    [item, transitionStatus, triggerTriage, spawnSessionFromItem, approvePlan, overrideVerdict, triggerReReview, triggerShipPR, archiveBacklogItem, deleteBacklogItem, onClose, load, showActionToast]
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
      await triggerTriage(item.id);
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
  }, [item, triggerTriage, load, showActionToast]);

  const handleRefineTriage = useCallback(
    async (feedback: string) => {
      if (!item) return;
      await triggerTriage(item.id, feedback);
      await load();
    },
    [item, triggerTriage, load]
  );

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
      const transitioned = await transitionStatus(item.id, "ready", "idea");
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
  // refreshes (the triage poll re-runs load() every 5s and toggles `loading`) must
  // NOT unmount the detail view / edit form, or in-progress edits like unsaved
  // acceptance criteria get discarded when the form remounts. See stapler-squad#146.
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
        <div className={styles.scrollArea}>
          <BacklogItemForm
            initialValues={item}
            onSubmit={handleUpdateItem}
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
          </div>
          <div className={styles.headerActions}>
            <button
              className={styles.editButton}
              onClick={() => setEditMode(true)}
              aria-label="Edit item"
              data-testid="backlog-detail-edit"
            >
              Edit
            </button>
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
        <LifecycleSummary item={item} pipelineDisplay={pipelineDisplay} />
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
                onSkip={() => { void load(); }}
                onRefine={handleRefineTriage}
              />
            </div>
          )}

        {/* Planning record — read-only triage result for items past idea status */}
        <PlanningSection item={item} />

        {/* Triage failed banner */}
        {item.triageStatus === "failed" && item.status === "idea" && (
          <div className={styles.section}>
            <InlineError type="permanent" onRetry={handleRetriggerTriage} />
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
            />
          )}

          {item.status === "pr_pending" && (
            <PullRequestSection
              item={item}
              actionLoading={actionLoading}
              onMarkDone={() => handleAction("mark_done")}
            />
          )}

          <DescriptionSection item={item} />

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
        </CollapsibleGroup>
      </div>

    </article>
  );
}
