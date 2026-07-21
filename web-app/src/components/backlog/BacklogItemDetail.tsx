"use client";
// +feature: backlog:item-detail

import { useState, useEffect, useCallback, useRef } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import type { BacklogItem, AcCriterion, BacklogItemInput, LinkedSession, PipelineMode } from "@/lib/hooks/useBacklogService";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { useAnalytics } from "@/lib/analytics";
import { getStatusLabel } from "@/lib/backlog/status";
import { useVcsStatus } from "@/lib/hooks/useVcsStatus";
import { useBacklogItemShipStatus } from "@/lib/hooks/useBacklogItemShipStatus";
import { getApiBaseUrl } from "@/lib/config";
import { VcsWidget } from "@/components/shared/VcsWidget";
import { fromSessionVcs, fromShipStatus } from "@/lib/vcs/adapters";
import { BacklogItemForm } from "./BacklogItemForm";
import { AcCriteriaList } from "./AcCriteriaList";
import { SessionMonitor } from "./SessionMonitor";
import { GateVerdictBox } from "./GateVerdictBox";
import { InlineError } from "./InlineError";
import { TriageLoadingIndicator } from "./TriageLoadingIndicator";
import { TriageReviewPanel } from "./TriageReviewPanel";
import { ReviewChangesModal } from "./ReviewChangesModal";
import { BacklogFileBrowserModal } from "./BacklogFileBrowserModal";
import * as styles from "./BacklogItemDetail.css";
import * as markdownStyles from "./markdownBody.css";

interface BacklogItemDetailProps {
  itemId: string;
  onClose?: () => void;
}

const STATUS_CLASS: Record<string, string> = {
  idea: styles.statusIdea,
  refining: styles.statusRefining,
  ready: styles.statusReady,
  in_progress: styles.statusInProgress,
  review: styles.statusReview,
  done: styles.statusDone,
  archived: styles.statusArchived,
};

const getStatusClass = (s: string): string => STATUS_CLASS[s] ?? styles.statusArchived;

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

/** Renders a button's label, swapping in a spinner + "Running…" while `pending`. */
function ActionButtonLabel({ pending, label }: { pending: boolean; label: string }) {
  if (!pending) return <>{label}</>;
  return (
    <>
      <span className={styles.buttonSpinner} aria-hidden="true" />
      Running…
    </>
  );
}

function formatDate(iso?: string): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/**
 * Epic 3.4 "what ran" surface — resolves an ItemSession's frozen
 * pipelineModeSnapshot against the currently-fetched mode list, purely for
 * display (looking up the human-readable name). The underlying stored
 * value is never re-resolved live. Case priority (see plan.md Story 3.4.1):
 *   1. Snapshot slug not found in the current mode list → "unrecognized"
 *      (checked first — there's no live mode to compare a hash against).
 *   2. Snapshot slug found, but its content hash has since changed →
 *      "resolved" with drifted: true.
 *   3. Snapshot slug found and unchanged (or snapshot hash is empty,
 *      meaning default mode / a pre-feature session) → "resolved" with
 *      drifted: false. Empty pipelineModeSnapshot always short-circuits to
 *      the "default" case before any lookup is attempted.
 */
type PipelineModeDisplay =
  | { kind: "resolved"; name: string; drifted: boolean }
  | { kind: "unrecognized"; slug: string };

function resolvePipelineModeDisplay(
  session: LinkedSession,
  modes: PipelineMode[]
): PipelineModeDisplay {
  const snapshot = session.pipelineModeSnapshot ?? "";
  if (snapshot === "") {
    return { kind: "resolved", name: "default", drifted: false };
  }

  const match = modes.find((m) => m.slug === snapshot);
  if (!match) {
    return { kind: "unrecognized", slug: snapshot };
  }

  const snapshotHash = session.pipelineModeSnapshotHash ?? "";
  const drifted = snapshotHash !== "" && snapshotHash !== match.contentHash;
  return { kind: "resolved", name: match.name, drifted };
}

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
  const latestWorkSession = [...(item?.linkedSessions ?? [])].reverse().find((s) => s.role === "work");
  // Surfaces the "most recent work session" heuristic's ambiguity when more than
  // one work session is currently active — the heuristic above is unchanged, this
  // only makes it visible via VcsWidgetHeader's "N active sessions" indicator.
  const activeWorkSessionCount = (item?.linkedSessions ?? []).filter((s) => s.role === "work" && !s.endedAt).length;
  const { data: vcsStatus } = useVcsStatus(latestWorkSession?.sessionId ?? "", getApiBaseUrl());
  // Fallback for once the live session's worktree has been cleaned up (the normal
  // state for a done item) — vcsStatus above comes back null in that case since
  // useVcsStatus needs a live in-memory Instance to query.
  const { data: shipStatus } = useBacklogItemShipStatus(!vcsStatus && item ? item.id : "");

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
  // Suspend polling while the edit form is open so a background refresh can't clobber unsaved edits.
  useEffect(() => {
    const shouldPoll = (item?.triageStatus === "running" || (item?.status === "review" && (!item?.gateVerdict || item.gateVerdict === "PENDING")) || item?.status === "pr_pending") && !editMode;
    if (!shouldPoll) return;
    const interval = setInterval(() => { void load(); }, 5_000);
    return () => clearInterval(interval);
  }, [item?.triageStatus, item?.status, item?.gateVerdict, editMode, load]); // item.status covers pr_pending polling

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
      try {
        switch (action) {
          case "mark_ready":
            await transitionStatus(item.id, "ready");
            break;
          case "trigger_triage":
            await triggerTriage(item.id);
            break;
          case "spawn_session":
            await spawnSessionFromItem(item.id);
            break;
          case "spawn_session_autonomous":
            await spawnSessionFromItem(item.id, { autonomous: true });
            break;
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
        showActionToast(ACTION_SUCCESS_MESSAGES[action] ?? "Done.", "success", toastKey);
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

  const canSpawnSession =
    item.status === "ready" &&
    (item.skipPlanning || item.planApproved);
  // Autonomous mode does its own planning — no plan-approval gate needed.
  const canRunAutonomously = item.status === "ready";

  // Self-service "Ship PR" action: only makes sense for an item sitting in
  // review with no PR yet — the exact gap this closes (see
  // docs/tasks/backlog-feature-improvement.md, 2026-07-18 update). All AC
  // criteria must be complete before shipping; a gate verdict of PASS is
  // encouraged (via the button's title) but not required — same
  // human-override philosophy as the existing "Override → Done" action.
  const acAllComplete =
    item.acCriteria.length > 0 && item.acCriteria.every((c) => c.status === "done");
  const canShipPR = item.status === "review" && !item.prUrl;

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
                className={`${styles.statusBadge} ${getStatusClass(item.status)}`}
                aria-label={`Status: ${getStatusLabel(item.status)}`}
              >
                {getStatusLabel(item.status)}
              </span>
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
        {item.triageResult && item.status !== "idea" && (
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Planning</h3>
            <p className={styles.planSummary}>{item.triageResult.summary}</p>
            {item.triageResult.tasks && item.triageResult.tasks.length > 0 && (
              <div className={styles.planTaskList}>
                {item.triageResult.tasks.map((t, i) => (
                  <div key={i} className={styles.planTask}>
                    <span className={styles.planTaskText}>{t.text}</span>
                    <span className={styles.planTaskMeta}>
                      {t.estimate && <span className={styles.planTaskBadge}>{t.estimate}</span>}
                      {t.category && <span className={styles.planTaskBadge}>{t.category}</span>}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {/* Triage failed banner */}
        {item.triageStatus === "failed" && item.status === "idea" && (
          <div className={styles.section}>
            <InlineError type="permanent" onRetry={handleRetriggerTriage} />
          </div>
        )}

        {/* Gate Verdict + review context */}
        {item.status === "review" && (() => {
          const workSession = [...item.linkedSessions].reverse().find((s) => s.role === "work");
          const activeReviewSession = [...item.linkedSessions].reverse().find((s) => s.role === "review" && !s.endedAt && !s.sessionId.startsWith("headless-") && !s.sessionId.startsWith("review-blocked-"));
          return (
            <>
              <div className={styles.section}>
                <h3 className={styles.sectionTitle}>Reviewing</h3>
                <div className={styles.reviewContextBox}>
                  <div className={styles.reviewContextInfo}>
                    {workSession ? (
                      <>
                        <span className={styles.reviewContextLabel}>Work session</span>
                        <a
                          className={styles.reviewContextSessionId}
                          href={`/?session=${workSession.sessionId}`}
                          title="Open in terminal"
                        >
                          {workSession.sessionId}
                        </a>
                        {workSession.endedAt && (
                          <span className={styles.reviewContextDate}>
                            Completed {new Date(workSession.endedAt).toLocaleString()}
                          </span>
                        )}
                      </>
                    ) : (
                      <span className={styles.reviewContextLabel}>No work session found</span>
                    )}
                    {activeReviewSession && (
                      <>
                        <span className={styles.reviewContextLabel}>Review session</span>
                        <a
                          className={styles.reviewContextSessionId}
                          href={`/?session=${activeReviewSession.sessionId}`}
                          title="Open review session in terminal"
                        >
                          {activeReviewSession.sessionId}
                        </a>
                      </>
                    )}
                  </div>
                  {workSession && (
                    <button
                      className={styles.viewChangesButton}
                      onClick={() => setShowChangesModal(true)}
                      data-testid="backlog-review-view-changes"
                    >
                      View Changes ↗
                    </button>
                  )}
                </div>
              </div>

              <div className={styles.section}>
                <GateVerdictBox
                  verdict={item.gateVerdict ?? "PENDING"}
                  summary={item.gateVerdictSummary || "Review in progress"}
                  criteria={item.gateCriteria}
                  elapsedSeconds={undefined}
                  onApprove={handleGateApprove}
                  onReopen={handleGateReopen}
                  onOverride={handleGateOverride}
                  onSkipGate={handleGateSkip}
                  onReReview={() => triggerReReview(item.id).then(() => load())}
                  actionPending={actionLoading !== null}
                />
              </div>

            </>
          );
        })()}

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

        {/* PR Pending */}
        {item.status === "pr_pending" && (
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Pull Request</h3>
            <div className={styles.reviewContextBox}>
              <div className={styles.reviewContextInfo}>
                {item.prUrl ? (
                  <>
                    <span className={styles.reviewContextLabel}>
                      PR #{item.prNumber} — waiting for merge
                    </span>
                    <a
                      className={styles.reviewContextSessionId}
                      href={item.prUrl}
                      target="_blank"
                      rel="noreferrer"
                      title="Open pull request on GitHub"
                    >
                      {item.prUrl}
                    </a>
                  </>
                ) : (
                  <span className={styles.reviewContextLabel}>
                    PR pending — no URL recorded yet
                  </span>
                )}
              </div>
              <button
                className={styles.actionButton}
                onClick={() => handleAction("mark_done")}
                disabled={actionLoading !== null}
                aria-busy={actionLoading === "mark_done"}
                title="Mark done manually (if PR already merged)"
                data-testid="backlog-action-mark-done"
              >
                <ActionButtonLabel pending={actionLoading === "mark_done"} label="Mark Done" />
              </button>
            </div>
          </div>
        )}

        {/* Description */}
        <div className={styles.section}>
          <h3 className={styles.sectionTitle}>Description</h3>
          {item.description ? (
            <div className={markdownStyles.markdownBody} data-testid="backlog-description-rendered">
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{item.description}</ReactMarkdown>
            </div>
          ) : (
            <p className={styles.emptyText}>No description.</p>
          )}
        </div>

        {/* Acceptance Criteria */}
        <div className={styles.section}>
          <h3 className={styles.sectionTitle}>
            Acceptance Criteria ({item.acCriteria.filter((c) => c.status === "done").length}/{item.acCriteria.length})
          </h3>
          <AcCriteriaList criteria={item.acCriteria} />
        </div>

        {/* Actions */}
        <div className={styles.section}>
          <h3 className={styles.sectionTitle}>Actions</h3>
          <div className={styles.actionsPanel} role="group" aria-label="Item actions">
            {item.status === "idea" && (
              <>
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("mark_ready")}
                  disabled={actionLoading !== null || item.acCriteria.length === 0}
                  aria-disabled={item.acCriteria.length === 0}
                  aria-busy={actionLoading === "mark_ready"}
                  title={item.acCriteria.length === 0 ? "Add at least one AC criterion first" : undefined}
                  data-testid="backlog-action-mark-ready"
                >
                  <ActionButtonLabel pending={actionLoading === "mark_ready"} label="Mark Ready" />
                </button>
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("trigger_triage")}
                  disabled={actionLoading !== null || !item.repoPath}
                  aria-disabled={!item.repoPath}
                  aria-busy={actionLoading === "trigger_triage"}
                  title={!item.repoPath ? "Set repository path first" : undefined}
                  data-testid="backlog-action-trigger-triage"
                >
                  <ActionButtonLabel pending={actionLoading === "trigger_triage"} label="Trigger Triage" />
                </button>
              </>
            )}

            {item.status === "ready" && (
              <>
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("trigger_triage")}
                  disabled={actionLoading !== null || !item.repoPath}
                  aria-disabled={!item.repoPath}
                  aria-busy={actionLoading === "trigger_triage"}
                  title={!item.repoPath ? "Set repository path first" : undefined}
                  data-testid="backlog-action-trigger-triage"
                >
                  <ActionButtonLabel pending={actionLoading === "trigger_triage"} label="Trigger Triage" />
                </button>
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("spawn_session")}
                  disabled={actionLoading !== null || !canSpawnSession}
                  aria-disabled={!canSpawnSession}
                  aria-busy={actionLoading === "spawn_session"}
                  title={
                    !canSpawnSession
                      ? "Approve the plan or enable skip_planning to spawn a session"
                      : undefined
                  }
                  data-testid="backlog-action-spawn-session"
                >
                  <ActionButtonLabel pending={actionLoading === "spawn_session"} label="Spawn Session" />
                </button>
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("spawn_session_autonomous")}
                  disabled={actionLoading !== null || !canRunAutonomously}
                  aria-disabled={!canRunAutonomously}
                  aria-busy={actionLoading === "spawn_session_autonomous"}
                  title={
                    !canRunAutonomously
                      ? "Item must be in Ready status to run autonomously"
                      : "Run the agent without human approval for tool calls"
                  }
                  data-testid="backlog-action-run-autonomously"
                >
                  <ActionButtonLabel pending={actionLoading === "spawn_session_autonomous"} label="Run Autonomously" />
                </button>
                {item.planArtifactsPath && (
                  <button
                    className={styles.actionButton}
                    onClick={() => handleAction("approve_plan")}
                    disabled={actionLoading !== null}
                    aria-busy={actionLoading === "approve_plan"}
                    data-testid="backlog-action-approve-plan"
                  >
                    <ActionButtonLabel pending={actionLoading === "approve_plan"} label="Approve Plan" />
                  </button>
                )}
              </>
            )}

            {item.status === "in_progress" && item.linkedSessions.length > 0 && (
              <>
                <a
                  className={styles.actionButton}
                  href={`/?session=${([...item.linkedSessions].reverse().find(s => s.role === "work") ?? item.linkedSessions[item.linkedSessions.length - 1]).sessionId}`}
                  data-testid="backlog-action-view-session"
                >
                  View Session
                </a>
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("restart_session")}
                  disabled={actionLoading !== null}
                  aria-busy={actionLoading === "restart_session"}
                  title="Stop the current session and re-spawn it in a fresh git worktree"
                  data-testid="backlog-action-restart-session"
                >
                  <ActionButtonLabel pending={actionLoading === "restart_session"} label="Restart" />
                </button>
              </>
            )}

            {item.status === "review" && (
              <>
                {canShipPR && (
                  <button
                    className={styles.actionButton}
                    onClick={() => handleAction("ship_pr")}
                    disabled={actionLoading !== null || !acAllComplete}
                    aria-disabled={!acAllComplete}
                    aria-busy={actionLoading === "ship_pr"}
                    title={
                      !acAllComplete
                        ? "All acceptance criteria must be complete before shipping a PR."
                        : "Ask the agent to push the branch and open a pull request for this item."
                    }
                    data-testid="backlog-action-ship-pr"
                  >
                    <ActionButtonLabel pending={actionLoading === "ship_pr"} label="🚀 Ship PR" />
                  </button>
                )}
                <button
                  className={`${styles.actionButton} ${styles.actionButtonDanger}`}
                  onClick={() => handleAction("override_done")}
                  disabled={actionLoading !== null}
                  aria-busy={actionLoading === "override_done"}
                  data-testid="backlog-action-override-done"
                >
                  <ActionButtonLabel pending={actionLoading === "override_done"} label="Override → Done" />
                </button>
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("re_review")}
                  disabled={actionLoading !== null}
                  aria-busy={actionLoading === "re_review"}
                  data-testid="backlog-action-re-review"
                >
                  <ActionButtonLabel pending={actionLoading === "re_review"} label="Re-review" />
                </button>
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("manual_review")}
                  disabled={actionLoading !== null}
                  data-testid="backlog-action-manual-review"
                >
                  Submit Review
                </button>
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("restart_session")}
                  disabled={actionLoading !== null}
                  aria-busy={actionLoading === "restart_session"}
                  title="Stop the review session and restart work from scratch in a fresh git worktree"
                  data-testid="backlog-action-restart-session"
                >
                  <ActionButtonLabel pending={actionLoading === "restart_session"} label="Restart" />
                </button>
              </>
            )}

            {showManualReview && item.status === "review" && (
              <div className={styles.manualReviewForm} data-testid="manual-review-form">
                <h4 className={styles.manualReviewTitle}>Submit Review</h4>
                <div className={styles.manualReviewRow}>
                  <label className={styles.manualReviewLabel}>Verdict</label>
                  <select
                    className={styles.manualReviewSelect}
                    value={manualReviewOutcome}
                    onChange={(e) => setManualReviewOutcome(e.target.value)}
                    data-testid="manual-review-outcome"
                  >
                    <option value="PASS">PASS — meets all criteria</option>
                    <option value="FAIL">FAIL — does not meet criteria</option>
                    <option value="PARTIAL">PARTIAL — partially meets criteria</option>
                    <option value="UNVERIFIABLE">UNVERIFIABLE — cannot verify</option>
                  </select>
                </div>
                <div className={styles.manualReviewRow}>
                  <label className={styles.manualReviewLabel}>Summary</label>
                  <textarea
                    className={styles.manualReviewTextarea}
                    placeholder="Describe your findings…"
                    value={manualReviewSummary}
                    onChange={(e) => setManualReviewSummary(e.target.value)}
                    rows={4}
                    data-testid="manual-review-summary"
                  />
                </div>
                <div className={styles.manualReviewActions}>
                  <button
                    className={styles.actionButton}
                    disabled={!manualReviewSummary.trim() || actionLoading !== null}
                    aria-busy={actionLoading === "manual_review_submit"}
                    onClick={async () => {
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
                    }}
                    data-testid="manual-review-submit"
                  >
                    <ActionButtonLabel pending={actionLoading === "manual_review_submit"} label="Submit" />
                  </button>
                  <button
                    className={styles.actionButtonSecondary}
                    onClick={() => { setShowManualReview(false); setManualReviewSummary(""); }}
                    data-testid="manual-review-cancel"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            )}

            {item.status === "done" && (
              <>
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("archive")}
                  disabled={actionLoading !== null}
                  aria-busy={actionLoading === "archive"}
                  data-testid="backlog-action-archive"
                >
                  <ActionButtonLabel pending={actionLoading === "archive"} label="Archive" />
                </button>
                <button
                  className={styles.actionButton}
                  onClick={() => handleAction("reopen")}
                  disabled={actionLoading !== null}
                  aria-busy={actionLoading === "reopen"}
                  data-testid="backlog-action-reopen"
                >
                  <ActionButtonLabel pending={actionLoading === "reopen"} label="Re-open to Review" />
                </button>
              </>
            )}

            {/* Backward transitions — visible whenever there's an earlier stage to return to */}
            {["refining", "ready", "in_progress", "review", "pr_pending", "done"].includes(item.status) && (
              <>
                <button
                  className={`${styles.actionButton} ${styles.actionButtonSecondary}`}
                  onClick={() => handleAction("send_back_idea")}
                  disabled={actionLoading !== null}
                  aria-busy={actionLoading === "send_back_idea"}
                  title="Reset to Idea and clear plan approval so triage can re-run"
                  data-testid="backlog-action-send-back-idea"
                >
                  <ActionButtonLabel pending={actionLoading === "send_back_idea"} label="↩ Return to Triage" />
                </button>
                {["in_progress", "review", "pr_pending", "done"].includes(item.status) && (
                  <button
                    className={`${styles.actionButton} ${styles.actionButtonSecondary}`}
                    onClick={() => handleAction("send_back_ready")}
                    disabled={actionLoading !== null}
                    aria-busy={actionLoading === "send_back_ready"}
                    title="Move back to Ready to re-spawn without full re-triage"
                    data-testid="backlog-action-send-back-ready"
                  >
                    <ActionButtonLabel pending={actionLoading === "send_back_ready"} label="↩ Back to Ready" />
                  </button>
                )}
              </>
            )}

            <button
              className={styles.actionButtonDanger}
              onClick={() => handleAction("delete")}
              disabled={actionLoading !== null}
              aria-busy={actionLoading === "delete"}
              data-testid="backlog-action-delete"
            >
              <ActionButtonLabel pending={actionLoading === "delete"} label="Delete" />
            </button>
          </div>
        </div>

        {/* Plan Artifacts Path */}
        {item.planArtifactsPath && (
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Plan Artifacts</h3>
            <code className={styles.artifactsPath}>{item.planArtifactsPath}</code>
          </div>
        )}

        {/* Version Control — live VCS state for the most recent work session, falling
            back to the durable ship-status check once the live worktree is gone (the
            normal state for a done item — see useBacklogItemShipStatus). The
            fallback-by-data-presence rule (vcsStatus wins when both resolve non-null)
            is preserved exactly from the pre-VcsWidget implementation. */}
        {(() => {
          const widgetData = vcsStatus ? fromSessionVcs(vcsStatus) : shipStatus ? fromShipStatus(shipStatus) : null;
          return (
            widgetData && (
              <div className={styles.section}>
                <h3 className={styles.sectionTitle}>Version Control</h3>
                <VcsWidget
                  data={widgetData}
                  mode="full"
                  onViewDiff={() => setShowChangesModal(true)}
                  activeSessionCount={activeWorkSessionCount}
                  worktreePath={latestWorkSession?.worktreePath}
                  onBrowseFiles={() => setShowFileBrowser(true)}
                />
              </div>
            )
          );
        })()}

        {/* Linked Sessions */}
        {item.linkedSessions.length > 0 && (
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Sessions ({item.linkedSessions.length})</h3>
            <div className={styles.sessionList} role="list" aria-label="Linked sessions">
              {item.linkedSessions.map((s) => {
                // A session without endedAt that isn't in the active phase for this
                // item's current status is a stale/orphaned record — label it ended.
                const statusToRole: Record<string, string> = {
                  idea: "triage",
                  in_progress: "work",
                  review: "review",
                };
                const isOrphan = !s.endedAt && s.role !== statusToRole[item.status];
                const pipelineDisplay = resolvePipelineModeDisplay(s, pipelineModes);
                return (
                  <div
                    key={s.entityId ?? s.sessionId}
                    className={styles.sessionRow}
                    role="listitem"
                  >
                    <div className={styles.sessionRowMain}>
                    {s.role === "triage" || s.sessionId.startsWith("headless-") || s.sessionId.startsWith("review-blocked-") ? (
                      <span className={styles.sessionLink}>
                        <span className={styles.sessionId} title={s.sessionId}>
                          {s.sessionId.startsWith("headless-review-") ? "headless review" : s.sessionId.startsWith("review-blocked-") ? "review blocked" : s.sessionId}
                        </span>
                        <span className={styles.sessionRole}>{s.role}</span>
                        {s.worktreeBranch && (
                          <span className={styles.branchBadge} title="Git branch for this work session">{s.worktreeBranch}</span>
                        )}
                        {s.startedAt && (
                          <span className={styles.sessionDate}>{formatDate(s.startedAt)}</span>
                        )}
                        {s.estimatedCostUsd > 0 && (
                          <span className={styles.sessionCost} title="Estimated session cost">${s.estimatedCostUsd.toFixed(4)}</span>
                        )}
                        {isOrphan && (
                          <span className={styles.sessionEndedBadge}>ended</span>
                        )}
                      </span>
                    ) : (
                      <a
                        className={styles.sessionLink}
                        href={`/?session=${s.sessionId}`}
                        title="Open in terminal"
                      >
                        <span className={styles.sessionId} title={s.sessionId}>
                          {s.sessionId}
                        </span>
                        <span className={styles.sessionRole}>{s.role}</span>
                        {s.worktreeBranch && (
                          <span className={styles.branchBadge} title="Git branch for this work session">{s.worktreeBranch}</span>
                        )}
                        {s.startedAt && (
                          <span className={styles.sessionDate}>{formatDate(s.startedAt)}</span>
                        )}
                        {s.estimatedCostUsd > 0 && (
                          <span className={styles.sessionCost} title="Estimated session cost">${s.estimatedCostUsd.toFixed(4)}</span>
                        )}
                        {isOrphan && (
                          <span className={styles.sessionEndedBadge}>ended</span>
                        )}
                      </a>
                    )}
                    <button
                      className={styles.sessionDeleteBtn}
                      disabled={deletingSessionId === s.sessionId}
                      aria-label="Delete session"
                      onClick={async (e) => {
                        e.preventDefault();
                        if (!confirm("Delete this session? This cannot be undone.")) return;
                        const toastKey = `${item.id}:delete_session:${s.sessionId}`;
                        setDeletingSessionId(s.sessionId);
                        try {
                          if (s.role === "triage") {
                            await cancelTriage(item.id);
                          } else {
                            track({ name: "backlog_delete_session", category: "user_action", component: "BacklogItemDetail", labels: { role: s.role } });
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
                      }}
                    >
                      {deletingSessionId === s.sessionId ? "…" : "Delete"}
                    </button>
                  </div>
                  <div className={styles.pipelineGroup} role="group" aria-label="Pipeline">
                    <span className={styles.pipelineLabel}>Pipeline:</span>{" "}
                    {pipelineDisplay.kind === "unrecognized" ? (
                      <span className={styles.pipelineValue}>
                        {`custom (unrecognized mode: '${pipelineDisplay.slug}')`}
                      </span>
                    ) : (
                      <>
                        <span className={styles.pipelineValue}>{pipelineDisplay.name}</span>
                        {pipelineDisplay.drifted && (
                          <>
                            {" "}
                            <span className={styles.pipelineDriftBadge}>(content since changed)</span>
                          </>
                        )}
                      </>
                    )}
                  </div>
                  {s.reviewVerdict && (s.reviewVerdict.summary || (s.reviewVerdict.perCriterion?.length ?? 0) > 0) && (
                    <div className={styles.verdictDetail} aria-label="Review verdict detail">
                      {s.reviewVerdict.summary && (
                        <span className={styles.verdictSummary}>
                          <strong>{s.reviewVerdict.overallOutcome}:</strong> {s.reviewVerdict.summary}
                        </span>
                      )}
                      {s.reviewVerdict.perCriterion?.map((c) => (
                        <div key={c.criterionIndex} className={styles.verdictCriterion}>
                          <span>#{c.criterionIndex} {c.outcome}</span>
                          {c.evidence && <span>— {c.evidence}</span>}
                        </div>
                      ))}
                    </div>
                  )}
                  </div>
                );
              })}
            </div>
            {item.totalEstimatedCostUsd > 0 && (
              <p className={styles.sessionTotalCost}>
                Total estimated cost: <strong>${item.totalEstimatedCostUsd.toFixed(4)}</strong>
              </p>
            )}

            {/* Session monitor for the most recent active session.
                A session is only considered active if the item is in the
                matching lifecycle phase — prevents ghost "RUNNING" tiles for
                sessions that died without setting endedAt. */}
            {(() => {
              const statusToRole: Record<string, string> = {
                idea: "triage",
                in_progress: "work",
                review: "review",
              };
              const expectedRole = statusToRole[item.status];
              const active = [...item.linkedSessions]
                .reverse()
                .find((s) => !s.endedAt && s.role === expectedRole && !s.sessionId.startsWith("headless-") && !s.sessionId.startsWith("review-blocked-"));
              if (!active) return null;
              return (
                <SessionMonitor
                  sessionId={active.sessionId}
                  sessionRole={active.role}
                  isRunning={true}
                />
              );
            })()}
          </div>
        )}

        {/* Workflow / Status History */}
        {item.statusEvents.length > 0 && (
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Workflow</h3>
            <div className={styles.workflowTimeline} role="list" aria-label="Status history">
              {item.statusEvents.map((ev) => (
                <div key={ev.id} className={styles.workflowEvent} role="listitem">
                  <div className={styles.workflowEventRow}>
                    <span className={styles.workflowEventFrom}>{ev.fromStatus.replace("_", " ")}</span>
                    <span className={styles.workflowEventArrow}>→</span>
                    <span className={styles.workflowEventTo}>{ev.toStatus.replace("_", " ")}</span>
                    <span className={styles.workflowEventMeta}>
                      {ev.createdAt ? formatDate(ev.createdAt) : ""}
                      {" · "}{ev.triggeredBy}
                    </span>
                  </div>
                  {ev.note && <span className={styles.workflowEventNote}>{ev.note}</span>}
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Progress History — the implementer's report_progress audit trail */}
        {item.progressNotes.length > 0 && (
          <div className={styles.section}>
            <h3 className={styles.sectionTitle}>Progress History</h3>
            <div className={styles.progressNoteList} role="list" aria-label="Implementer progress history">
              {item.progressNotes.map((n) => (
                <div key={n.id} className={styles.progressNoteItem} role="listitem">
                  <div className={styles.progressNoteMeta}>
                    <span>Criterion #{n.criterionIndex}</span>
                    <span>·</span>
                    <span>{n.status}</span>
                    {n.createdAt && (
                      <>
                        <span>·</span>
                        <span>{formatDate(n.createdAt)}</span>
                      </>
                    )}
                  </div>
                  {n.note && <span>{n.note}</span>}
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Notes */}
        <div className={styles.section}>
          <h3 className={styles.sectionTitle}>Notes</h3>
          {editingNotes ? (
            <>
              <textarea
                className={styles.notesTextarea}
                value={notesValue}
                onChange={(e) => setNotesValue(e.target.value)}
                aria-label="Notes"
                data-testid="backlog-notes-textarea"
              />
              <div className={styles.inlineEditActions}>
                <button
                  className={styles.saveNotesButton}
                  onClick={handleSaveNotes}
                  disabled={actionLoading !== null}
                  aria-busy={actionLoading === "save_notes"}
                  data-testid="backlog-notes-save"
                >
                  <ActionButtonLabel pending={actionLoading === "save_notes"} label="Save" />
                </button>
                <button
                  className={styles.cancelNotesButton}
                  onClick={() => {
                    setNotesValue(item.notes ?? "");
                    setEditingNotes(false);
                  }}
                  data-testid="backlog-notes-cancel"
                >
                  Cancel
                </button>
              </div>
            </>
          ) : (
            <p
              className={item.notes ? styles.description : styles.emptyText}
              onClick={() => setEditingNotes(true)}
              role="button"
              tabIndex={0}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") setEditingNotes(true);
              }}
              aria-label="Click to edit notes"
              data-testid="backlog-notes-display"
            >
              {item.notes ?? "Click to add notes…"}
            </p>
          )}
        </div>
      </div>

    </article>
  );
}
