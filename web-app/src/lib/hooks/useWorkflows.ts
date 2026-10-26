"use client";

import { useEffect, useCallback, useSyncExternalStore } from "react";
import { createClient } from "@connectrpc/connect";
import {
  SessionService,
  ListWorkflowsRequestSchema,
  CreateWorkflowRequestSchema,
  UpdateWorkflowRequestSchema,
  DeleteWorkflowRequestSchema,
  ArchiveWorkflowSessionsRequestSchema,
  DeleteWorkflowFailedSessionsRequestSchema,
} from "@/gen/session/v1/session_pb";
import type { WorkflowProto } from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import type { Timestamp } from "@bufbuild/protobuf/wkt";
import { getConnectTransport } from "@/lib/api/transport";
import { useWatchWorkflows } from "@/lib/hooks/useWatchWorkflows";

export interface WorkflowFormData {
  slug: string;
  name: string;
  description?: string;
  command: string;
  targetDirectory: string;
  inputTemplate?: string;
  sessionType?: string;
  model?: string;
  agentType?: string;
  cronExpression?: string;
  cronEnabled: boolean;
  /**
   * enabled is the generic per-trigger-type "is this trigger active" gate, read by
   * both webhook handlers and written by TriggersPanel.tsx's toggle — distinct from
   * cronEnabled, which is the literal cron-schedule flag only.
   */
  enabled?: boolean;
  keepSessions?: number;
  archiveAfterHours?: number;
  /**
   * Trigger fields (webhook-triggers Phase 7). triggerType discriminates the
   * activation mechanism: "cron" | "github_push" | "webhook" | "manual".
   */
  triggerType?: string;
  githubRepo?: string;
  githubBranch?: string;
  webhookSlug?: string;
  eventFilter?: string;
  labelFilter?: string;
  promptTemplate?: string;
  /**
   * Write-only plaintext webhook/HMAC shared secret (webhook-triggers Phase 7 backend
   * follow-up). "" or undefined means "no change" on update, or "no secret configured"
   * on create — see CreateWorkflowRequest.webhook_secret's proto doc comment. Never
   * populated when reading a WorkflowProto back (it has no such field).
   */
  webhookSecret?: string;
  /**
   * Optimistic-concurrency CAS precondition (webhook-triggers verify follow-ups AC9):
   * when set on an update, the write is rejected (CodeAborted) unless the row's
   * current updatedAt still matches this value — closes the two-tabs-both-save race
   * a blind Get-then-write previously had. Omitted means no precondition, matching
   * pre-CAS behavior. Never meaningful on create.
   */
  expectedUpdatedAt?: Timestamp;
}

interface UseWorkflowsReturn {
  workflows: WorkflowProto[];
  loading: boolean;
  error: Error | null;
  createWorkflow: (data: WorkflowFormData) => Promise<void>;
  updateWorkflow: (id: string, data: Partial<WorkflowFormData>) => Promise<void>;
  deleteWorkflow: (id: string) => Promise<void>;
  archiveWorkflowSessions: (workflowId: string) => Promise<number>;
  deleteWorkflowFailedSessions: (workflowId: string) => Promise<number>;
  refresh: () => Promise<void>;
}

interface WorkflowsSnapshot {
  workflows: WorkflowProto[];
  loading: boolean;
  error: Error | null;
}

// Module-level store: every useWorkflows() consumer (Omnibar, each SessionDetailView,
// panels) reads one list, so mounting N views costs one ListWorkflows instead of N.
let snapshot: WorkflowsSnapshot = { workflows: [], loading: false, error: null };
let loaded = false;
let inflight: Promise<void> | null = null;
const listeners = new Set<() => void>();

function patchSnapshot(patch: Partial<WorkflowsSnapshot>): void {
  snapshot = { ...snapshot, ...patch };
  listeners.forEach((l) => l());
}

function updateWorkflows(fn: (prev: WorkflowProto[]) => WorkflowProto[]): void {
  patchSnapshot({ workflows: fn(snapshot.workflows) });
}

const getClient = () => createClient(SessionService, getConnectTransport());

/** One shared fetch: joins an in-flight request, and skips entirely once loaded unless `force`. */
function fetchWorkflowsShared(force = false): Promise<void> {
  if (inflight) return inflight;
  if (loaded && !force) return Promise.resolve();
  patchSnapshot({ loading: true, error: null });
  const run = (async () => {
    try {
      // shared store fetch has no single owning component to cancel it
      // abort-signal-exempt
      const resp = await getClient().listWorkflows(create(ListWorkflowsRequestSchema, {}));
      loaded = true;
      patchSnapshot({ workflows: resp.workflows ?? [] });
    } catch (err) {
      const e = err instanceof Error ? err : new Error("Failed to fetch workflows");
      patchSnapshot({ error: e });
      console.error("Failed to fetch workflows:", e);
    } finally {
      inflight = null;
      patchSnapshot({ loading: false });
    }
  })();
  inflight = run;
  return run;
}

// Staleness backstop: WatchWorkflows (useWatchWorkflows.ts) can silently
// stall -- e.g. a proxy idle-timeout that never surfaces a stream error or
// close -- in which case its own reconnect-on-error logic never fires and
// a long-lived tab keeps serving an ever-stale list with no indication.
// A low-frequency re-fetch, plus one on tab refocus, catches that case
// without connection-state tracking; the workflow list is small enough
// that a full ListWorkflows call is cheap. Installed once while any consumer is mounted.
const BACKSTOP_MS = 90_000;
let uninstallBackstop: (() => void) | null = null;

function installBackstop(): () => void {
  const interval = setInterval(() => void fetchWorkflowsShared(true), BACKSTOP_MS);
  const onVisibility = () => {
    if (document.visibilityState === "visible") void fetchWorkflowsShared(true);
  };
  document.addEventListener("visibilitychange", onVisibility);
  return () => {
    clearInterval(interval);
    document.removeEventListener("visibilitychange", onVisibility);
  };
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  uninstallBackstop ??= installBackstop();
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0) {
      uninstallBackstop?.();
      uninstallBackstop = null;
    }
  };
}

const getSnapshot = () => snapshot;

/** Test-only: reset the shared store. */
export function resetWorkflowsStoreForTests(): void {
  snapshot = { workflows: [], loading: false, error: null };
  loaded = false;
  inflight = null;
  uninstallBackstop?.();
  uninstallBackstop = null;
  listeners.clear();
}

interface UseWorkflowsOptions {
  /**
   * Subscribe to WatchWorkflows for live updates (default true). Read-only consumers
   * that sit beside an always-mounted watcher (the Omnibar) pass false.
   */
  watch?: boolean;
}

/**
 * React hook for managing workflow definitions.
 *
 * Reads the shared workflow store (one ListWorkflows for all consumers) and exposes
 * create/update/delete actions.
 */
export function useWorkflows({ watch = true }: UseWorkflowsOptions = {}): UseWorkflowsReturn {
  const { workflows, loading, error } = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);

  const refresh = useCallback(() => fetchWorkflowsShared(true), []);

  useEffect(() => {
    void fetchWorkflowsShared();
  }, []);

  // Live updates from any client (MCP tool, another tab, a cron fire) via
  // WatchWorkflows -- keeps `workflows` current without requiring a manual
  // refresh. See useWatchWorkflows.ts's header for why this stays a plain
  // upsert/remove against local state rather than a Redux-backed store.
  const onWorkflowCreatedOrUpdated = useCallback((workflow: WorkflowProto) => {
    updateWorkflows((prev) => {
      const idx = prev.findIndex((w) => w.id === workflow.id);
      if (idx === -1) return [...prev, workflow];
      const next = prev.slice();
      next[idx] = workflow;
      return next;
    });
  }, []);

  const onWorkflowDeleted = useCallback((id: string) => {
    updateWorkflows((prev) => prev.filter((w) => w.id !== id));
  }, []);

  useWatchWorkflows(
    {
      onCreatedOrUpdated: onWorkflowCreatedOrUpdated,
      onDeleted: onWorkflowDeleted,
    },
    { enabled: watch }
  );

  const createWorkflow = useCallback(
    async (data: WorkflowFormData) => {
      const req = create(CreateWorkflowRequestSchema, {
        slug: data.slug,
        name: data.name,
        description: data.description ?? "",
        command: data.command,
        targetDirectory: data.targetDirectory,
        inputTemplate: data.inputTemplate ?? "",
        sessionType: data.sessionType ?? "directory",
        model: data.model ?? "",
        agentType: data.agentType ?? "",
        cronExpression: data.cronExpression ?? "",
        cronEnabled: data.cronEnabled,
        ...(data.enabled !== undefined && { enabled: data.enabled }),
        ...(data.keepSessions !== undefined && { keepSessions: data.keepSessions }),
        ...(data.archiveAfterHours !== undefined && { archiveAfterHours: data.archiveAfterHours }),
        triggerType: data.triggerType ?? "",
        githubRepo: data.githubRepo ?? "",
        githubBranch: data.githubBranch ?? "",
        webhookSlug: data.webhookSlug ?? "",
        eventFilter: data.eventFilter ?? "",
        labelFilter: data.labelFilter ?? "",
        promptTemplate: data.promptTemplate ?? "",
        webhookSecret: data.webhookSecret ?? "",
      });
      // user-initiated mutation
      // abort-signal-exempt
      await getClient().createWorkflow(req);
      await refresh();
    },
    [refresh]
  );

  const updateWorkflow = useCallback(
    async (id: string, data: Partial<WorkflowFormData>) => {
      const req = create(UpdateWorkflowRequestSchema, {
        id,
        ...(data.name !== undefined && { name: data.name }),
        ...(data.description !== undefined && { description: data.description }),
        ...(data.command !== undefined && { command: data.command }),
        ...(data.targetDirectory !== undefined && { targetDirectory: data.targetDirectory }),
        ...(data.inputTemplate !== undefined && { inputTemplate: data.inputTemplate }),
        ...(data.sessionType !== undefined && { sessionType: data.sessionType }),
        ...(data.model !== undefined && { model: data.model }),
        ...(data.agentType !== undefined && { agentType: data.agentType }),
        ...(data.cronExpression !== undefined && { cronExpression: data.cronExpression }),
        ...(data.cronEnabled !== undefined && { cronEnabled: data.cronEnabled }),
        ...(data.enabled !== undefined && { enabled: data.enabled }),
        ...(data.keepSessions !== undefined && { keepSessions: data.keepSessions }),
        ...(data.archiveAfterHours !== undefined && { archiveAfterHours: data.archiveAfterHours }),
        ...(data.triggerType !== undefined && { triggerType: data.triggerType }),
        ...(data.githubRepo !== undefined && { githubRepo: data.githubRepo }),
        ...(data.githubBranch !== undefined && { githubBranch: data.githubBranch }),
        ...(data.webhookSlug !== undefined && { webhookSlug: data.webhookSlug }),
        ...(data.eventFilter !== undefined && { eventFilter: data.eventFilter }),
        ...(data.labelFilter !== undefined && { labelFilter: data.labelFilter }),
        ...(data.promptTemplate !== undefined && { promptTemplate: data.promptTemplate }),
        ...(data.expectedUpdatedAt !== undefined && { expectedUpdatedAt: data.expectedUpdatedAt }),
        // Not `optional` on the wire (see WorkflowFormData.webhookSecret) — "" is
        // always a safe no-op ("leave unchanged"), so always include it rather than
        // conditionally spreading like the optional fields above.
        webhookSecret: data.webhookSecret ?? "",
      });
      // user-initiated mutation
      // abort-signal-exempt
      await getClient().updateWorkflow(req);
      await refresh();
    },
    [refresh]
  );

  const deleteWorkflow = useCallback(
    async (id: string) => {
      // Snapshot before optimistic update for rollback.
      const prev = workflows;
      updateWorkflows((w) => w.filter((wf) => wf.id !== id));
      try {
        const req = create(DeleteWorkflowRequestSchema, { id });
        // user-initiated mutation
        // abort-signal-exempt
        await getClient().deleteWorkflow(req);
      } catch (err) {
        patchSnapshot({ workflows: prev }); // rollback on failure
        throw err;
      }
    },
    [workflows]
  );

  const archiveWorkflowSessions = useCallback(
    async (workflowId: string): Promise<number> => {
      const req = create(ArchiveWorkflowSessionsRequestSchema, { workflowId });
      // user-initiated mutation
      // abort-signal-exempt
      const resp = await getClient().archiveWorkflowSessions(req);
      await refresh();
      return resp.archivedCount;
    },
    [refresh]
  );

  const deleteWorkflowFailedSessions = useCallback(
    async (workflowId: string): Promise<number> => {
      const req = create(DeleteWorkflowFailedSessionsRequestSchema, { workflowId });
      // user-initiated mutation
      // abort-signal-exempt
      const resp = await getClient().deleteWorkflowFailedSessions(req);
      await refresh();
      return resp.deletedCount;
    },
    [refresh]
  );

  return { workflows, loading, error, createWorkflow, updateWorkflow, deleteWorkflow, archiveWorkflowSessions, deleteWorkflowFailedSessions, refresh };
}
