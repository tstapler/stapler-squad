"use client";

import { useCallback, useRef, useEffect, useState, useMemo } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import type { Timestamp } from "@bufbuild/protobuf/wkt";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import {
  BacklogService,
  BacklogItem as BacklogItemProto,
  AcCriterion as AcCriterionProto,
  ItemSession as ItemSessionProto,
  TriageTask as TriageTaskProto,
  BacklogStatusEvent as BacklogStatusEventProto,
  BacklogProgressNote as BacklogProgressNoteProto,
  PipelineMode as PipelineModeProto,
} from "@/gen/session/v1/backlog_pb";

// ---------------------------------------------------------------------------
// Domain types exposed to UI (mapped from proto, but without Message<> noise)
// ---------------------------------------------------------------------------

export type KnownBacklogStatus = "idea" | "refining" | "ready" | "queued" | "in_progress" | "review" | "pr_pending" | "done" | "archived";
// (string & {}) preserves autocomplete for KnownBacklogStatus values while still
// accepting unknown statuses returned by newer server versions.
export type BacklogItemStatus = KnownBacklogStatus | (string & {});

export type AcCriterionStatus = "pending" | "in_progress" | "done";

export interface AcCriterion {
  index: number;
  text: string;
  status: AcCriterionStatus;
}

export interface TriageSuggestion {
  text: string;
  rationale: string; // "question" for R7-lite clarifying questions
}

export interface TriageTask {
  text: string;
  estimate: string;
  category: string;
}

export interface TriageResult {
  summary: string;
  suggestions: TriageSuggestion[];
  clarifyingQuestions: string[];
  tasks?: TriageTask[];
  /** 1 for the initial triage run, incrementing for each feedback-driven refine. */
  iteration?: number;
  /** Feedback text that produced this iteration, empty for the initial run. */
  feedback?: string;
}

export interface LinkedSession {
  /** Entity UUID of the ItemSession record — use for overrideVerdict calls. */
  entityId: string;
  /** Tmux session UUID — use for linking to the session terminal. */
  sessionId: string;
  role: string;
  startedAt?: string;
  endedAt?: string;
  /** Number of commits made since this session was spawned; 0 if none yet. */
  commitCountSinceSpawn?: number;
  /** Timestamp of the session's most recent commit, if it has made one. */
  lastCommitAt?: string;
  /** Full text (possibly multi-line) of the session's most recent commit message. */
  lastCommitMessage?: string;
  /** Timestamp of the session's most recent file modification, if any. */
  lastFileTouchAt?: string;
  reviewVerdict?: {
    overallOutcome?: "PASS" | "PARTIAL" | "FAIL" | "PENDING" | "UNVERIFIABLE";
    summary?: string;
    perCriterion?: Array<{ criterionIndex: number; outcome: string; evidence: string }>;
  };
  triageResult?: TriageResult;
  estimatedCostUsd: number;
  /** Git branch name for the session's worktree, if one exists. */
  worktreeBranch?: string;
  /** Absolute path to the session's git worktree, if one exists. */
  worktreePath?: string;
  /**
   * Pipeline mode slug this session actually ran under, frozen at session
   * start — "" means the built-in default mode (or a pre-Epic-1.6 session
   * predating this field). Never re-resolved against the live mode list;
   * see BacklogItemDetail.tsx's "what ran" surface (Epic 3.4).
   */
  pipelineModeSnapshot?: string;
  /**
   * SHA-256 content hash of the pipeline mode at the moment this session
   * ran — "" when pipelineModeSnapshot is "" (default mode / pre-feature
   * session, no meaningful drift comparison). Compared against the mode's
   * current PipelineMode.contentHash to detect content drift.
   */
  pipelineModeSnapshotHash?: string;
  /**
   * Set alongside endedAt for a headless (triage/review) call: a coarse
   * failure bucket ("shutdown", "timeout", "process_error", "claude_not_found",
   * "other"), or "" for a successful end. See classifyHeadlessCallError
   * (server/services/backlog_service_triage.go) for the bucketing logic.
   */
  endReason?: string;
  /**
   * Absolute (server-local) path to a durable capture of the raw LLM output
   * for a headless triage/review call that errored or failed to parse — see
   * session.WriteHeadlessFailureCapture. "" when the call succeeded and
   * parsed cleanly, or nothing was captured.
   */
  failureCapturePath?: string;
}

export interface BacklogItem {
  id: string;
  title: string;
  description?: string;
  status: BacklogItemStatus;
  /** 1 = highest priority, 5 = lowest */
  priority: number;
  repoPath?: string;
  skipPlanning: boolean;
  skipReviewGate: boolean;
  /** When true, a work session is spawned automatically once the item reaches ready — no manual "Spawn Session" click required. */
  autoSpawnSession: boolean;
  /** When true, a PR is created automatically (same one-shot prompt as the manual Review Queue "Create PR" button) once a work session reaches TASK_COMPLETE — no manual click required. */
  autoCreatePR: boolean;
  planApproved: boolean;
  planArtifactsPath?: string;
  /**
   * Free-text reason from the most recent RejectPlan call. Cleared on
   * ApprovePlan, on the next TriggerTriage completion (fresh or
   * feedback-driven), and on any backward transition to idea/refining.
   * Undefined/"" means "no outstanding rejection" — see
   * derivePlanReviewStatus (web-app/src/lib/backlog/planReviewStatus.ts)
   * and project_plans/plan-approval-ux/decisions/ADR-001.
   */
  planRejectionReason?: string;
  /** Timestamp of the most recent RejectPlan call, paired with planRejectionReason above. */
  planRejectedAt?: string;
  acCriteria: AcCriterion[];
  linkedSessions: LinkedSession[];
  notes?: string;
  createdAt?: string;
  updatedAt?: string;
  /**
   * The raw, undecoded protobuf Timestamp backing `updatedAt` — kept
   * alongside the display-oriented ISO string because `updatedAt` is
   * lossy (a JS Date is millisecond-precision; the server's real
   * updated_at column is nanosecond-precision Go time.Time, and ent's CAS
   * check is exact equality). Passing this straight back as
   * transitionStatus's expectedUpdatedAt is a lossless passthrough — the
   * exact bytes the server sent, never touched by a Date conversion — so
   * it round-trips correctly where `updatedAt` cannot.
   */
  updatedAtRaw?: Timestamp;
  /** Gate verdict from the most recent item session (if in review status) */
  gateVerdict?: "PASS" | "PARTIAL" | "FAIL" | "PENDING" | "UNVERIFIABLE";
  gateVerdictSummary?: string;
  gateCriteria?: Array<{ label: string; passed: boolean }>;
  /** Triage progress indicator: when item is in "idea" status being triaged */
  triageStatus?: "running" | "completed" | "failed";
  /** Triage result from the most recent triage session (populated when triageStatus === "completed") */
  triageResult?: TriageResult;
  /** Status transition history for this item (audit log) */
  statusEvents: StatusEvent[];
  /** Implementer's report_progress audit trail (audit log) */
  progressNotes: ProgressNote[];
  /** Sum of estimated USD cost across all linked sessions */
  totalEstimatedCostUsd: number;
  /** GitHub PR URL when item is in pr_pending status */
  prUrl?: string;
  /** GitHub PR number when item is in pr_pending status */
  prNumber?: number;
  /** Source tracker's identifier for this item (e.g. a GitHub issue number), populated for imported items only. */
  externalId?: string;
  /** Deep link to the source tracker's issue for this item (e.g. a GitHub issue's html_url), populated for imported items only. */
  externalUrl?: string;
  /** Labels mirrored from the source tracker (e.g. GitHub issue labels), populated for imported items only. */
  labels?: string[];
  /**
   * Server-authoritative set of statuses a manual override may transition
   * this item to (session.WorkflowEngine.AllowedTransitions(status)). The
   * manual-override UI must render this verbatim, not re-derive the
   * transition graph client-side. Optional (defaults to []) so existing
   * test fixtures that predate this field don't all need updating.
   */
  allowedTransitions?: string[];
  /** Pipeline mode slug driving this item's triage/work/review, or "" for the built-in default. */
  pipelineMode?: string;
  /**
   * Coarse classification (bugfix/feature/chore/refactor) used only as a
   * frontend-defaulting hint at creation time — see
   * web-app/src/lib/backlog/categoryDefaults.ts. Undefined/"" means
   * uncategorized.
   */
  category?: string;
  /**
   * Per-item override for the auto-rework cap. Undefined = use the global
   * default (Settings → Defaults). 0 = unlimited retries for this item. >0 =
   * this item's own cap, replacing the global value.
   */
  reworkCapOverride?: number;
  /**
   * Live-update generation counter (Epic 6.1, backlog-event-driven-updates).
   * Populated only by `useWatchBacklogItems` — incremented once per genuine
   * live (non-snapshot) `BacklogItemEvent` for this item, so
   * `BacklogItemCard` can flash on a real change without ever flashing on
   * the initial snapshot or a reconnect/poll resync. Undefined for items
   * obtained via a one-shot RPC call (`listBacklogItems`, `getBacklogItem`,
   * etc.) rather than the watch stream.
   */
  liveVersion?: number;
}

/**
 * PipelineMode is a named, slug-addressed, user-creatable definition of which
 * slash-commands and prompt content a backlog item's pipeline uses. Mapped
 * from session.v1.PipelineMode (see backlog_pb.ts).
 */
export interface PipelineMode {
  id: string;
  slug: string;
  name: string;
  description: string;
  enabled: boolean;
  statusCommandTemplate: string;
  doneCommandTemplate: string;
  failCommandTemplate: string;
  reviewCommandTemplate: string;
  shipCommandTemplate: string;
  helpCommandTemplate: string;
  triagePromptTemplate: string;
  reviewPromptTemplate: string;
  initialPromptTemplate: string;
  /** SHA-256 (hex, truncated to 16 chars) over the 9 content-template fields, computed server-side. */
  contentHash: string;
}

/**
 * Payload for CreatePipelineMode. All 9 content-template fields are optional —
 * an omitted field is sent as "" (no partial-update semantics on create,
 * unlike PipelineModeUpdateInput below).
 */
export interface PipelineModeInput {
  slug: string;
  name: string;
  description?: string;
  enabled?: boolean;
  statusCommandTemplate?: string;
  doneCommandTemplate?: string;
  failCommandTemplate?: string;
  reviewCommandTemplate?: string;
  shipCommandTemplate?: string;
  helpCommandTemplate?: string;
  triagePromptTemplate?: string;
  reviewPromptTemplate?: string;
  initialPromptTemplate?: string;
}

/**
 * Payload for UpdatePipelineMode. Slug is immutable after creation (per
 * plan.md's Story 3.3.2) so it is intentionally absent here — only
 * name/description/enabled/content-template fields may be changed. Every
 * field is a true partial-update: an omitted key leaves the stored value
 * untouched (mirrors BacklogItemInput's Partial<> update convention).
 */
export type PipelineModeUpdateInput = Partial<Omit<PipelineModeInput, "slug">>;

export interface StatusEvent {
  id: string;
  fromStatus: string;
  toStatus: string;
  triggeredBy: string;
  createdAt?: string;
  /** Human-readable reason for this transition, e.g. "auto-reopened after FAIL verdict". */
  note?: string;
}

/** A single report_progress call — the implementer's append-only decision history. */
export interface ProgressNote {
  id: string;
  criterionIndex: number;
  note: string;
  status: string;
  createdAt?: string;
}

export interface BacklogItemInput {
  title: string;
  description?: string;
  repoPath?: string;
  priority?: number;
  skipPlanning?: boolean;
  skipReviewGate?: boolean;
  autoSpawnSession?: boolean;
  autoCreatePR?: boolean;
  acCriteria?: AcCriterion[];
  notes?: string;
  skipTriage?: boolean;
  /** Pipeline mode slug, or "" for the built-in default. */
  pipelineMode?: string;
  /** Coarse classification (bugfix/feature/chore/refactor), or "" for uncategorized. See BacklogItem.category. */
  category?: string;
  /** Per-item rework-cap override. 0 = unlimited for this item, >0 = this item's own cap. See BacklogItem.reworkCapOverride. */
  reworkCapOverride?: number;
  /**
   * Manually associate an existing PR with this item (the "escape hatch" for
   * a PR that shipped via an out-of-band worktree). Must be set together
   * with prNumber, and only takes effect while the item is in "review"
   * status — see UpdateBacklogItem's server-side validation.
   */
  prUrl?: string;
  prNumber?: number;
}

export interface ListBacklogItemsFilter {
  statuses?: BacklogItemStatus[];
  priorities?: number[];
  includeTerminal?: boolean;
  /**
   * When true, includes items with status "archived" in the default
   * (no explicit `statuses`) result set. Independent of includeTerminal —
   * defaults to false so archived items stay hidden unless the caller opts
   * in (see the backlog page's "Show Archived" toggle).
   */
  includeArchived?: boolean;
  search?: string;
}

// ---------------------------------------------------------------------------
// Proto ↔ domain mapping helpers
// ---------------------------------------------------------------------------

function mapAcCriterion(c: AcCriterionProto): AcCriterion {
  return {
    index: c.index,
    text: c.text,
    status: (c.status || "pending") as AcCriterionStatus,
  };
}

function mapItemSession(s: ItemSessionProto): LinkedSession {
  const session: LinkedSession = {
    entityId: s.id,
    sessionId: s.sessionUuid,
    role: s.sessionRole,
    startedAt: s.startedAt ? new Date(Number(s.startedAt.seconds) * 1000).toISOString() : undefined,
    endedAt: s.endedAt ? new Date(Number(s.endedAt.seconds) * 1000).toISOString() : undefined,
    commitCountSinceSpawn: s.commitCountSinceSpawn ?? 0,
    lastCommitAt: s.lastCommitAt ? timestampDate(s.lastCommitAt).toISOString() : undefined,
    lastCommitMessage: s.lastCommitMessage || undefined,
    lastFileTouchAt: s.lastFileTouchAt ? timestampDate(s.lastFileTouchAt).toISOString() : undefined,
    estimatedCostUsd: s.estimatedCostUsd ?? 0,
    worktreeBranch: s.worktreeBranch || undefined,
    worktreePath: s.worktreePath || undefined,
    pipelineModeSnapshot: s.pipelineModeSnapshot ?? "",
    pipelineModeSnapshotHash: s.pipelineModeSnapshotHash ?? "",
    endReason: s.endReason || undefined,
    failureCapturePath: s.failureCapturePath || undefined,
  };

  // Map review verdict if present
  if (s.reviewVerdict) {
    const rv = s.reviewVerdict;
    const knownOutcomes = new Set(["PASS", "FAIL", "PARTIAL", "UNVERIFIABLE"]);
    session.reviewVerdict = {
      overallOutcome: knownOutcomes.has(rv.overallOutcome)
        ? (rv.overallOutcome as "PASS" | "PARTIAL" | "FAIL" | "PENDING" | "UNVERIFIABLE")
        : rv.overallOutcome
          ? "PARTIAL"
          : "PENDING",
      summary: rv.summary,
      perCriterion: (rv.perCriterion ?? []).map((c) => ({
        criterionIndex: c.criterionIndex,
        outcome: c.outcome,
        evidence: c.evidence,
      })),
    };
  }

  // Map triage result if present
  if (s.triageResult) {
    const tr = s.triageResult;
    session.triageResult = {
      summary: tr.summary,
      suggestions: (tr.suggestions ?? []).map((sg) => ({
        text: sg.text,
        rationale: sg.rationale,
      })),
      clarifyingQuestions: tr.clarifyingQuestions ?? [],
      tasks: (tr.tasks ?? []).map((t: TriageTaskProto) => ({
        text: t.text,
        estimate: t.estimate,
        category: t.category,
      })),
      iteration: tr.iteration,
      feedback: tr.feedback,
    };
  }

  return session;
}

function mapStatusEvent(e: BacklogStatusEventProto): StatusEvent {
  return {
    id: e.id,
    fromStatus: e.fromStatus,
    toStatus: e.toStatus,
    triggeredBy: e.triggeredBy,
    createdAt: e.createdAt ? new Date(Number(e.createdAt.seconds) * 1000).toISOString() : undefined,
    note: e.note,
  };
}

function mapProgressNote(n: BacklogProgressNoteProto): ProgressNote {
  return {
    id: n.id,
    criterionIndex: n.criterionIndex,
    note: n.note,
    status: n.status,
    createdAt: n.createdAt ? new Date(Number(n.createdAt.seconds) * 1000).toISOString() : undefined,
  };
}

function mapPipelineMode(p: PipelineModeProto): PipelineMode {
  return {
    id: p.id,
    slug: p.slug,
    name: p.name,
    description: p.description,
    enabled: p.enabled,
    statusCommandTemplate: p.statusCommandTemplate,
    doneCommandTemplate: p.doneCommandTemplate,
    failCommandTemplate: p.failCommandTemplate,
    reviewCommandTemplate: p.reviewCommandTemplate,
    shipCommandTemplate: p.shipCommandTemplate,
    helpCommandTemplate: p.helpCommandTemplate,
    triagePromptTemplate: p.triagePromptTemplate,
    reviewPromptTemplate: p.reviewPromptTemplate,
    initialPromptTemplate: p.initialPromptTemplate,
    contentHash: p.contentHash,
  };
}

// Exported so other real-time consumers (useWatchBacklogItems) can convert
// the raw proto BacklogItem their stream/store deals in to this file's mapped
// domain BacklogItem — the shape BacklogItemCard/BacklogBoard/BacklogItemDetail
// actually render (acCriteria, gateVerdict, triageStatus, ISO date strings,
// etc., none of which exist on the raw proto message). Also exported for
// direct unit testing of triageStatus derivation — see
// useBacklogService.test.ts.
export function mapBacklogItem(p: BacklogItemProto): BacklogItem {
  const linkedSessions = (p.itemSessions ?? []).map(mapItemSession);

  // Extract gate verdict from the most recent session (for review status)
  let gateVerdict: "PASS" | "PARTIAL" | "FAIL" | "PENDING" | "UNVERIFIABLE" | undefined;
  let gateVerdictSummary: string | undefined;
  let gateCriteria: Array<{ label: string; passed: boolean }> | undefined;

  // Use the most recent review session's verdict — not the most recent session of any
  // role, which could be a work session (no verdict) after a reopen-for-revision cycle.
  const mostRecentReviewSession = linkedSessions.filter((s) => s.role === "review").at(-1);
  if (mostRecentReviewSession?.reviewVerdict?.overallOutcome) {
    gateVerdict = mostRecentReviewSession.reviewVerdict.overallOutcome;
    gateVerdictSummary = mostRecentReviewSession.reviewVerdict.summary;

    if (mostRecentReviewSession.reviewVerdict.perCriterion?.length) {
      gateCriteria = mostRecentReviewSession.reviewVerdict.perCriterion.map((c) => ({
        label: c.evidence ? `${c.outcome}: ${c.evidence}` : `Criterion ${c.criterionIndex}: ${c.outcome}`,
        passed: c.outcome === "PASS" || c.outcome === "pass",
      }));
    }
  }

  // Derive triageStatus from linked sessions.
  // P12 fix: only mark "completed" if the session ended AND has a non-empty summary.
  // Orphan detection: a triage session without endedAt is only "running" while the item
  // is in "idea" status. If the item has advanced (ready, in_progress, etc.) the session
  // died without cleanly recording its end — treat it as "failed" so the UI doesn't show
  // a loading indicator for a session that no longer exists.
  const itemStatus = (p.status || "idea") as BacklogItemStatus;
  let triageStatus: BacklogItem["triageStatus"];
  const triageSession = linkedSessions.filter((s) => s.role === "triage").at(-1);
  if (triageSession) {
    if (triageSession.endedAt) {
      triageStatus = triageSession.triageResult?.summary ? "completed" : "failed";
    } else if (itemStatus === "idea") {
      triageStatus = "running";
    } else {
      triageStatus = "failed";
    }
  }

  const triageResult = triageSession?.triageResult;

  return {
    id: p.id,
    title: p.title,
    description: p.description || undefined,
    status: (p.status || "idea") as BacklogItemStatus,
    priority: p.priority || 3,
    repoPath: p.repoPath || undefined,
    skipPlanning: p.skipPlanning,
    skipReviewGate: p.skipReviewGate,
    autoSpawnSession: p.autoSpawnSession,
    autoCreatePR: p.autoCreatePr,
    planApproved: p.planApproved,
    planArtifactsPath: p.planArtifactsPath || undefined,
    planRejectionReason: p.planRejectionReason || undefined,
    // timestampDate, not a hand-rolled `Number(seconds) * 1000` — see the
    // createdAt/updatedAt comment above for why.
    planRejectedAt: p.planRejectedAt ? timestampDate(p.planRejectedAt).toISOString() : undefined,
    acCriteria: (p.acceptanceCriteria ?? []).map(mapAcCriterion),
    linkedSessions,
    notes: p.notes || undefined,
    // timestampDate (not a hand-rolled `Number(seconds) * 1000`) — the
    // previous conversion silently dropped the sub-second `nanos` field
    // entirely, truncating to the whole second. Still only millisecond
    // precision (a JS Date's ceiling) for display purposes — updatedAtRaw
    // below is the lossless form used for exact-equality CAS checks.
    createdAt: p.createdAt ? timestampDate(p.createdAt).toISOString() : undefined,
    updatedAt: p.updatedAt ? timestampDate(p.updatedAt).toISOString() : undefined,
    updatedAtRaw: p.updatedAt,
    gateVerdict,
    gateVerdictSummary,
    gateCriteria,
    triageStatus,
    triageResult,
    statusEvents: (p.statusEvents ?? []).map(mapStatusEvent),
    progressNotes: (p.progressNotes ?? []).map(mapProgressNote),
    totalEstimatedCostUsd: p.totalEstimatedCostUsd ?? 0,
    prUrl: p.prUrl || undefined,
    prNumber: p.prNumber || undefined,
    pipelineMode: p.pipelineMode || undefined,
    category: p.category || undefined,
    reworkCapOverride: p.reworkCapOverride,
    externalId: p.externalId || undefined,
    externalUrl: p.externalUrl || undefined,
    labels: p.labels ?? [],
    allowedTransitions: p.allowedTransitions ?? [],
  };
}

function toProtoAcCriteria(criteria: AcCriterion[]): AcCriterionProto[] {
  return criteria.map((c) => ({
    $typeName: "session.v1.AcCriterion" as const,
    index: c.index,
    text: c.text,
    status: c.status,
  }));
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// GitHub picker domain types
// ---------------------------------------------------------------------------

export interface GitHubRepo {
  owner: string;
  repo: string;
  isLocal: boolean;
  localPath: string;
  description: string;
}

export interface GitHubIssue {
  number: number;
  title: string;
  body?: string;
  author?: string;
  state: string;
  url: string;
  labels: string[];
  createdAt?: string;
  updatedAt?: string;
  isPR: boolean;
}

export class GitHubAuthError extends Error {
  constructor() {
    super("No GitHub token configured");
    this.name = "GitHubAuthError";
  }
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

interface UseBacklogServiceReturn {
  listBacklogItems: (filter?: ListBacklogItemsFilter) => Promise<BacklogItem[]>;
  getBacklogItem: (id: string) => Promise<BacklogItem | null>;
  createBacklogItem: (data: BacklogItemInput) => Promise<{ item: BacklogItem; triageTriggered: boolean } | null>;
  /** One turn of chat-based backlog creation/refinement. Empty existingItemId creates a new item (delegates to createBacklogItem); a set existingItemId delegates to TriggerTriage's feedback-driven refine path. */
  createBacklogItemFromChat: (message: string, existingItemId?: string) => Promise<{ item: BacklogItem; triageTriggered: boolean } | null>;
  importGitHubIssue: (issueUrl: string, options?: { repoPath?: string; skipPlanning?: boolean }) => Promise<{ item: BacklogItem; triageTriggered: boolean } | null>;
  searchGitHubRepos: (query: string, limit?: number) => Promise<GitHubRepo[]>;
  listGitHubIssues: (owner: string, repo: string, options?: { state?: string; search?: string; limit?: number }) => Promise<GitHubIssue[]>;
  updateBacklogItem: (id: string, data: Partial<BacklogItemInput>) => Promise<BacklogItem | null>;
  archiveBacklogItem: (id: string) => Promise<boolean>;
  unarchiveBacklogItem: (id: string) => Promise<boolean>;
  deleteBacklogItem: (id: string) => Promise<boolean>;
  transitionStatus: (
    id: string,
    toStatus: BacklogItemStatus,
    options?: {
      /** CAS precondition: reject if the item's current status isn't this. */
      expectedStatus?: BacklogItemStatus;
      /**
       * CAS precondition: reject if the item's updated_at isn't this.
       * Pass the item's own `updatedAtRaw` (the undecoded protobuf
       * Timestamp), not `updatedAt` (a display-oriented ISO string) — the
       * latter is millisecond-precision and can never exactly match the
       * server's nanosecond-precision column, so a write built from it
       * would spuriously fail CAS on virtually every call.
       */
      expectedUpdatedAt?: Timestamp;
      /**
       * Non-empty means this is a manual operator override (bypasses
       * TransitionGuard's business-rule gates, e.g. review->done without a
       * PASS verdict) rather than a routine automated transition — required
       * by the manual-override UI, threaded to the server so the audit trail
       * and success notification carry the operator's stated reason.
       */
      overrideReason?: string;
    }
  ) => Promise<BacklogItem | null>;
  spawnSessionFromItem: (id: string, options?: { autonomous?: boolean; force?: boolean }) => Promise<{ sessionUuid: string; queued: boolean } | null>;
  triggerTriage: (id: string, feedback?: string) => Promise<{ itemSessionId: string } | null>;
  cancelTriage: (id: string) => Promise<boolean>;
  approvePlan: (id: string) => Promise<BacklogItem | null>;
  /**
   * Persists a rejection reason only — does not itself trigger regeneration.
   * See project_plans/plan-approval-ux/decisions/ADR-002: the frontend
   * closes the "feedback should be actionable" gap with a separate, explicit
   * "Regenerate Plan with This Feedback" button that calls triggerTriage.
   */
  rejectPlan: (id: string, reason: string) => Promise<BacklogItem | null>;
  overrideVerdict: (id: string, overrideReason: string, toStatus?: string) => Promise<boolean>;
  triggerReReview: (id: string) => Promise<boolean>;
  /** Self-service "Ship PR" action — runs the one-shot PR-creation prompt for an item in review with no PR yet. */
  triggerShipPR: (id: string) => Promise<{ prUrl: string } | null>;
  submitManualReview: (id: string, overallOutcome: string, summary: string) => Promise<BacklogItem | null>;
  /**
   * Fetches all pipeline modes (enabled AND disabled — callers that only want
   * selectable modes, e.g. the item-form selector, must filter on `enabled`
   * themselves). Rethrows on failure so callers can distinguish "no modes
   * defined yet" from "the fetch failed".
   */
  listPipelineModes: () => Promise<PipelineMode[]>;
  /** Fetches a single pipeline mode by slug. Rethrows (incl. CodeNotFound) so callers can distinguish "not found" from other failures. */
  getPipelineMode: (slug: string) => Promise<PipelineMode | null>;
  /**
   * Creates a new pipeline mode. Rethrows on failure (in particular
   * `ConnectError` with `Code.InvalidArgument` from the backend's Story 2.3.1
   * content validation) so callers can display the error inline instead of a
   * generic failure state.
   */
  createPipelineMode: (data: PipelineModeInput) => Promise<PipelineMode>;
  /** Partial-updates an existing pipeline mode. Rethrows on failure — see createPipelineMode. */
  updatePipelineMode: (id: string, data: PipelineModeUpdateInput) => Promise<PipelineMode>;
  /** Deletes a pipeline mode by id. Rethrows on failure. */
  deletePipelineMode: (id: string) => Promise<boolean>;
  /** Last error from createBacklogItem, updateBacklogItem, transitionStatus, or spawnSessionFromItem. */
  lastError: Error | null;
  /** Clears the lastError state. */
  clearError: () => void;
}

export function useBacklogService(): UseBacklogServiceReturn {
  const clientRef = useRef<ReturnType<typeof createClient<typeof BacklogService>> | null>(null);
  const [lastError, setLastError] = useState<Error | null>(null);

  const clearError = useCallback(() => setLastError(null), []);

  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
      interceptors: [createAuthInterceptor()],
    });
    clientRef.current = createClient(BacklogService, transport);
  }, []);

  const listBacklogItems = useCallback(
    async (filter?: ListBacklogItemsFilter): Promise<BacklogItem[]> => {
      if (!clientRef.current) return [];
      try {
        const resp = await clientRef.current.listBacklogItems({
          status: filter?.statuses ?? [],
          priority: filter?.priorities ?? [],
          includeTerminal: filter?.includeTerminal ?? false,
          includeArchived: filter?.includeArchived ?? false,
          sortBy: "",
        });
        const items = (resp.items ?? []).map(mapBacklogItem);
        if (filter?.search) {
          const q = filter.search.toLowerCase();
          return items.filter(
            (item) =>
              item.title.toLowerCase().includes(q) ||
              item.description?.toLowerCase().includes(q)
          );
        }
        return items;
      } catch (err) {
        console.error("[useBacklogService] listBacklogItems:", err);
        return [];
      }
    },
    []
  );

  const getBacklogItem = useCallback(async (id: string): Promise<BacklogItem | null> => {
    if (!clientRef.current) return null;
    try {
      const resp = await clientRef.current.getBacklogItem({ itemId: id });
      return resp.item ? mapBacklogItem(resp.item) : null;
    } catch (err) {
      console.error("[useBacklogService] getBacklogItem:", err);
      return null;
    }
  }, []);

  const createBacklogItem = useCallback(
    async (data: BacklogItemInput): Promise<{ item: BacklogItem; triageTriggered: boolean } | null> => {
      if (!clientRef.current) return null;
      try {
        setLastError(null);
        const resp = await clientRef.current.createBacklogItem({
          title: data.title,
          description: data.description ?? "",
          repoPath: data.repoPath ?? "",
          priority: data.priority ?? 3,
          skipPlanning: data.skipPlanning ?? false,
          skipReviewGate: data.skipReviewGate ?? false,
          autoSpawnSession: data.autoSpawnSession ?? false,
          autoCreatePr: data.autoCreatePR ?? false,
          acceptanceCriteria: toProtoAcCriteria(data.acCriteria ?? []),
          notes: data.notes ?? "",
          skipTriage: data.skipTriage ?? false,
          pipelineMode: data.pipelineMode ?? "",
          category: data.category ?? "",
        });
        return resp.item
          ? { item: mapBacklogItem(resp.item), triageTriggered: resp.triageTriggered }
          : null;
      } catch (err) {
        console.error("[useBacklogService] createBacklogItem:", err);
        setLastError(err instanceof Error ? err : new Error(String(err)));
        return null;
      }
    },
    []
  );

  const createBacklogItemFromChat = useCallback(
    async (message: string, existingItemId?: string): Promise<{ item: BacklogItem; triageTriggered: boolean } | null> => {
      if (!clientRef.current) return null;
      try {
        setLastError(null);
        const resp = await clientRef.current.createBacklogItemFromChat({
          message,
          existingItemId: existingItemId ?? "",
        });
        return resp.item
          ? { item: mapBacklogItem(resp.item), triageTriggered: resp.triageTriggered }
          : null;
      } catch (err) {
        console.error("[useBacklogService] createBacklogItemFromChat:", err);
        setLastError(err instanceof Error ? err : new Error(String(err)));
        return null;
      }
    },
    []
  );

  const updateBacklogItem = useCallback(
    async (id: string, data: Partial<BacklogItemInput>): Promise<BacklogItem | null> => {
      if (!clientRef.current) return null;
      try {
        setLastError(null);
        const resp = await clientRef.current.updateBacklogItem({
          itemId: id,
          title: data.title,
          description: data.description,
          repoPath: data.repoPath,
          priority: data.priority,
          skipPlanning: data.skipPlanning,
          skipReviewGate: data.skipReviewGate,
          autoSpawnSession: data.autoSpawnSession,
          autoCreatePr: data.autoCreatePR,
          acceptanceCriteria: data.acCriteria ? toProtoAcCriteria(data.acCriteria) : undefined,
          notes: data.notes,
          pipelineMode: data.pipelineMode,
          category: data.category,
          reworkCapOverride: data.reworkCapOverride,
          prUrl: data.prUrl,
          prNumber: data.prNumber,
        });
        return resp.item ? mapBacklogItem(resp.item) : null;
      } catch (err) {
        console.error("[useBacklogService] updateBacklogItem:", err);
        setLastError(err instanceof Error ? err : new Error(String(err)));
        return null;
      }
    },
    []
  );

  const archiveBacklogItem = useCallback(async (id: string): Promise<boolean> => {
    if (!clientRef.current) return false;
    try {
      await clientRef.current.archiveBacklogItem({ itemId: id });
      return true;
    } catch (err) {
      console.error("[useBacklogService] archiveBacklogItem:", err);
      setLastError(err instanceof Error ? err : new Error(String(err)));
      throw err;
    }
  }, []);

  const unarchiveBacklogItem = useCallback(async (id: string): Promise<boolean> => {
    if (!clientRef.current) return false;
    try {
      await clientRef.current.unarchiveBacklogItem({ itemId: id });
      return true;
    } catch (err) {
      console.error("[useBacklogService] unarchiveBacklogItem:", err);
      setLastError(err instanceof Error ? err : new Error(String(err)));
      throw err;
    }
  }, []);

  const deleteBacklogItem = useCallback(async (id: string): Promise<boolean> => {
    if (!clientRef.current) return false;
    try {
      await clientRef.current.deleteBacklogItem({ itemId: id });
      return true;
    } catch (err) {
      console.error("[useBacklogService] deleteBacklogItem:", err);
      setLastError(err instanceof Error ? err : new Error(String(err)));
      throw err;
    }
  }, []);

  const transitionStatus = useCallback(
    async (
      id: string,
      toStatus: BacklogItemStatus,
      options?: {
        expectedStatus?: BacklogItemStatus;
        expectedUpdatedAt?: Timestamp;
        overrideReason?: string;
      }
    ): Promise<BacklogItem | null> => {
      if (!clientRef.current) return null;
      try {
        setLastError(null);
        const resp = await clientRef.current.transitionBacklogItemStatus({
          itemId: id,
          targetStatus: toStatus,
          expectedStatus: options?.expectedStatus ?? "",
          // Passed through verbatim — no Date round-trip. See the
          // expectedUpdatedAt doc comment above for why that matters.
          expectedUpdatedAt: options?.expectedUpdatedAt,
          overrideReason: options?.overrideReason ?? "",
        });
        return resp.item ? mapBacklogItem(resp.item) : null;
      } catch (err) {
        console.error("[useBacklogService] transitionStatus:", err);
        setLastError(err instanceof Error ? err : new Error(String(err)));
        throw err;
      }
    },
    []
  );

  const spawnSessionFromItem = useCallback(
    async (id: string, options?: { autonomous?: boolean; force?: boolean }): Promise<{ sessionUuid: string; queued: boolean } | null> => {
      if (!clientRef.current) return null;
      try {
        setLastError(null);
        const resp = await clientRef.current.spawnSessionFromItem({
          itemId: id,
          autonomous: options?.autonomous ?? false,
          force: options?.force ?? false,
        });
        return { sessionUuid: resp.sessionUuid, queued: resp.queued };
      } catch (err) {
        console.error("[useBacklogService] spawnSessionFromItem:", err);
        setLastError(err instanceof Error ? err : new Error(String(err)));
        throw err;
      }
    },
    []
  );

  const triggerTriage = useCallback(
    async (id: string, feedback?: string): Promise<{ itemSessionId: string } | null> => {
      if (!clientRef.current) return null;
      try {
        const resp = await clientRef.current.triggerTriage({ itemId: id, feedback: feedback ?? "" });
        return { itemSessionId: resp.itemSession?.id ?? "" };
      } catch (err) {
        console.error("[useBacklogService] triggerTriage:", err);
        setLastError(err instanceof Error ? err : new Error(String(err)));
        throw err;
      }
    },
    []
  );

  const cancelTriage = useCallback(async (id: string): Promise<boolean> => {
    if (!clientRef.current) return false;
    try {
      const resp = await clientRef.current.cancelTriage({ itemId: id });
      return resp.cancelled;
    } catch (err) {
      console.error("[useBacklogService] cancelTriage:", err);
      setLastError(err instanceof Error ? err : new Error(String(err)));
      throw err;
    }
  }, []);

  const approvePlan = useCallback(async (id: string): Promise<BacklogItem | null> => {
    if (!clientRef.current) return null;
    try {
      const resp = await clientRef.current.approvePlan({ itemId: id });
      return resp.item ? mapBacklogItem(resp.item) : null;
    } catch (err) {
      console.error("[useBacklogService] approvePlan:", err);
      setLastError(err instanceof Error ? err : new Error(String(err)));
      throw err;
    }
  }, []);

  const rejectPlan = useCallback(async (id: string, reason: string): Promise<BacklogItem | null> => {
    if (!clientRef.current) return null;
    try {
      const resp = await clientRef.current.rejectPlan({ itemId: id, reason });
      return resp.item ? mapBacklogItem(resp.item) : null;
    } catch (err) {
      console.error("[useBacklogService] rejectPlan:", err);
      setLastError(err instanceof Error ? err : new Error(String(err)));
      throw err;
    }
  }, []);

  const overrideVerdict = useCallback(
    async (id: string, overrideReason: string, toStatus?: string): Promise<boolean> => {
      if (!clientRef.current) return false;
      try {
        await clientRef.current.overrideVerdict({
          itemSessionId: id,
          overrideReason,
          toStatus: toStatus ?? "done",
        });
        return true;
      } catch (err) {
        console.error("[useBacklogService] overrideVerdict:", err);
        setLastError(err instanceof Error ? err : new Error(String(err)));
        throw err;
      }
    },
    []
  );

  const triggerReReview = useCallback(async (id: string): Promise<boolean> => {
    if (!clientRef.current) return false;
    try {
      await clientRef.current.triggerReReview({ itemId: id });
      return true;
    } catch (err) {
      console.error("[useBacklogService] triggerReReview:", err);
      setLastError(err instanceof Error ? err : new Error(String(err)));
      throw err;
    }
  }, []);

  /**
   * Runs the same one-shot PR-creation prompt the opt-in AutoCreatePR policy
   * uses automatically, for an item in review with no PR yet — the
   * self-service "Ship PR" action on the item detail page. Can take a while
   * (the underlying RunOneShot call may run for several minutes), matching
   * ReviewQueuePanel's existing manual "Create PR" flow. Rethrows on failure
   * so the caller can show the specific error (e.g. "work session not
   * running") rather than a generic failure state.
   */
  const triggerShipPR = useCallback(async (id: string): Promise<{ prUrl: string } | null> => {
    if (!clientRef.current) return null;
    try {
      const resp = await clientRef.current.triggerShipPR({ itemId: id });
      return { prUrl: resp.prUrl };
    } catch (err) {
      console.error("[useBacklogService] triggerShipPR:", err);
      setLastError(err instanceof Error ? err : new Error(String(err)));
      throw err;
    }
  }, []);

  const submitManualReview = useCallback(
    async (id: string, overallOutcome: string, summary: string): Promise<BacklogItem | null> => {
      if (!clientRef.current) return null;
      try {
        setLastError(null);
        const resp = await clientRef.current.submitManualReview({ itemId: id, overallOutcome, summary });
        return resp.item ? mapBacklogItem(resp.item) : null;
      } catch (err) {
        console.error("[useBacklogService] submitManualReview:", err);
        setLastError(err instanceof Error ? err : new Error(String(err)));
        throw err;
      }
    },
    []
  );

  const listPipelineModes = useCallback(async (): Promise<PipelineMode[]> => {
    if (!clientRef.current) return [];
    try {
      const resp = await clientRef.current.listPipelineModes({});
      return (resp.items ?? []).map(mapPipelineMode);
    } catch (err) {
      console.error("[useBacklogService] listPipelineModes:", err);
      throw err;
    }
  }, []);

  const getPipelineMode = useCallback(async (slug: string): Promise<PipelineMode | null> => {
    if (!clientRef.current) return null;
    try {
      const resp = await clientRef.current.getPipelineMode({ slug });
      return resp.item ? mapPipelineMode(resp.item) : null;
    } catch (err) {
      console.error("[useBacklogService] getPipelineMode:", err);
      throw err;
    }
  }, []);

  const createPipelineMode = useCallback(async (data: PipelineModeInput): Promise<PipelineMode> => {
    if (!clientRef.current) throw new Error("Backlog service not connected");
    try {
      const resp = await clientRef.current.createPipelineMode({
        slug: data.slug,
        name: data.name,
        description: data.description ?? "",
        enabled: data.enabled ?? false,
        statusCommandTemplate: data.statusCommandTemplate ?? "",
        doneCommandTemplate: data.doneCommandTemplate ?? "",
        failCommandTemplate: data.failCommandTemplate ?? "",
        reviewCommandTemplate: data.reviewCommandTemplate ?? "",
        shipCommandTemplate: data.shipCommandTemplate ?? "",
        helpCommandTemplate: data.helpCommandTemplate ?? "",
        triagePromptTemplate: data.triagePromptTemplate ?? "",
        reviewPromptTemplate: data.reviewPromptTemplate ?? "",
        initialPromptTemplate: data.initialPromptTemplate ?? "",
      });
      if (!resp.item) throw new Error("createPipelineMode: server returned no item");
      return mapPipelineMode(resp.item);
    } catch (err) {
      console.error("[useBacklogService] createPipelineMode:", err);
      throw err;
    }
  }, []);

  const updatePipelineMode = useCallback(
    async (id: string, data: PipelineModeUpdateInput): Promise<PipelineMode> => {
      if (!clientRef.current) throw new Error("Backlog service not connected");
      try {
        const resp = await clientRef.current.updatePipelineMode({
          id,
          name: data.name,
          description: data.description,
          enabled: data.enabled,
          statusCommandTemplate: data.statusCommandTemplate,
          doneCommandTemplate: data.doneCommandTemplate,
          failCommandTemplate: data.failCommandTemplate,
          reviewCommandTemplate: data.reviewCommandTemplate,
          shipCommandTemplate: data.shipCommandTemplate,
          helpCommandTemplate: data.helpCommandTemplate,
          triagePromptTemplate: data.triagePromptTemplate,
          reviewPromptTemplate: data.reviewPromptTemplate,
          initialPromptTemplate: data.initialPromptTemplate,
        });
        if (!resp.item) throw new Error("updatePipelineMode: server returned no item");
        return mapPipelineMode(resp.item);
      } catch (err) {
        console.error("[useBacklogService] updatePipelineMode:", err);
        throw err;
      }
    },
    []
  );

  const deletePipelineMode = useCallback(async (id: string): Promise<boolean> => {
    if (!clientRef.current) return false;
    try {
      await clientRef.current.deletePipelineMode({ id });
      return true;
    } catch (err) {
      console.error("[useBacklogService] deletePipelineMode:", err);
      throw err;
    }
  }, []);

  const importGitHubIssue = useCallback(
    async (
      issueUrl: string,
      options?: { repoPath?: string; skipPlanning?: boolean }
    ): Promise<{ item: BacklogItem; triageTriggered: boolean } | null> => {
      if (!clientRef.current) return null;
      try {
        setLastError(null);
        const resp = await clientRef.current.importGitHubIssue({
          issueUrl,
          repoPath: options?.repoPath ?? "",
          skipPlanning: options?.skipPlanning ?? false,
        });
        return resp.item
          ? { item: mapBacklogItem(resp.item), triageTriggered: resp.triageTriggered }
          : null;
      } catch (err) {
        console.error("[useBacklogService] importGitHubIssue:", err);
        setLastError(err instanceof Error ? err : new Error(String(err)));
        return null;
      }
    },
    []
  );

  const searchGitHubRepos = useCallback(
    async (query: string, limit?: number): Promise<GitHubRepo[]> => {
      if (!clientRef.current) return [];
      try {
        const resp = await clientRef.current.searchGitHubRepos({ query, limit: limit ?? 30 });
        return resp.repos.map((r) => ({
          owner: r.owner,
          repo: r.repo,
          isLocal: r.isLocal,
          localPath: r.localPath,
          description: r.description,
        }));
      } catch (err) {
        if (err instanceof Error && err.message.toLowerCase().includes("token")) {
          throw new GitHubAuthError();
        }
        throw err;
      }
    },
    []
  );

  const listGitHubIssues = useCallback(
    async (
      owner: string,
      repo: string,
      options?: { state?: string; search?: string; limit?: number }
    ): Promise<GitHubIssue[]> => {
      if (!clientRef.current) return [];
      try {
        const resp = await clientRef.current.listGitHubIssues({
          owner,
          repo,
          state: options?.state ?? "open",
          search: options?.search ?? "",
          limit: options?.limit ?? 30,
        });
        return resp.issues.map((i) => ({
          number: i.number,
          title: i.title,
          body: i.body,
          author: i.author,
          state: i.state,
          url: i.url,
          labels: i.labels,
          createdAt: i.createdAt ? new Date(Number(i.createdAt.seconds) * 1000).toISOString() : undefined,
          updatedAt: i.updatedAt ? new Date(Number(i.updatedAt.seconds) * 1000).toISOString() : undefined,
          isPR: i.isPr ?? false,
        }));
      } catch (err) {
        if (err instanceof Error && err.message.toLowerCase().includes("token")) {
          throw new GitHubAuthError();
        }
        throw err;
      }
    },
    []
  );

  // Stable object reference: all methods are useCallback(fn,[]) — only lastError changes.
  // Without useMemo, every render creates a new object, making callers' useCallback deps
  // fire on every render and causing infinite reload loops.
  return useMemo(
    () => ({
      listBacklogItems,
      getBacklogItem,
      createBacklogItem,
      createBacklogItemFromChat,
      importGitHubIssue,
      searchGitHubRepos,
      listGitHubIssues,
      updateBacklogItem,
      archiveBacklogItem,
      unarchiveBacklogItem,
      deleteBacklogItem,
      transitionStatus,
      spawnSessionFromItem,
      triggerTriage,
      cancelTriage,
      approvePlan,
      rejectPlan,
      overrideVerdict,
      triggerReReview,
      triggerShipPR,
      submitManualReview,
      listPipelineModes,
      getPipelineMode,
      createPipelineMode,
      updatePipelineMode,
      deletePipelineMode,
      lastError,
      clearError,
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [lastError]
  );
}

// ---------------------------------------------------------------------------
// Backlog session index — maps tmux session UUIDs to backlog item metadata
// ---------------------------------------------------------------------------

export interface BacklogIndexEntry {
  itemId: string;
  itemTitle: string;
  itemStatus: string;
  sessionRole: string;
}

export interface UseBacklogSessionIndexReturn {
  index: Map<string, BacklogIndexEntry>;
  loading: boolean;
}

/**
 * Fetches the full session→backlog index once on mount.
 * Returns a stable Map keyed by tmux session UUID.
 */
export function useBacklogSessionIndex(): UseBacklogSessionIndexReturn {
  const [index, setIndex] = useState<Map<string, BacklogIndexEntry>>(new Map());
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
      interceptors: [createAuthInterceptor()],
    });
    const client = createClient(BacklogService, transport);

    let cancelled = false;
    client
      .getSessionBacklogIndex({})
      .then((resp) => {
        if (cancelled) return;
        const map = new Map<string, BacklogIndexEntry>();
        for (const e of resp.entries ?? []) {
          if (e.sessionUuid) {
            map.set(e.sessionUuid, {
              itemId: e.itemId,
              itemTitle: e.itemTitle,
              itemStatus: e.itemStatus,
              sessionRole: e.sessionRole,
            });
          }
        }
        setIndex(map);
      })
      .catch((err) => {
        console.error("[useBacklogSessionIndex] failed:", err);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  return { index, loading };
}
