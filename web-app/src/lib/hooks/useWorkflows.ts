"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import {
  SessionService,
  WorkflowProto,
  ListWorkflowsRequestSchema,
  CreateWorkflowRequestSchema,
  UpdateWorkflowRequestSchema,
  DeleteWorkflowRequestSchema,
  ArchiveWorkflowSessionsRequestSchema,
  DeleteWorkflowFailedSessionsRequestSchema,
} from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import type { Timestamp } from "@bufbuild/protobuf/wkt";
import { getConnectTransport } from "@/lib/api/transport";

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

/**
 * React hook for managing workflow definitions.
 *
 * Fetches all workflows via listWorkflows and exposes create/update/delete actions.
 */
export function useWorkflows(): UseWorkflowsReturn {
  const [workflows, setWorkflows] = useState<WorkflowProto[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  useEffect(() => {
    clientRef.current = createClient(SessionService, getConnectTransport());
  }, []);

  const fetchWorkflows = useCallback(async () => {
    if (!clientRef.current) return;
    setLoading(true);
    setError(null);
    try {
      const req = create(ListWorkflowsRequestSchema, {});
      const resp = await clientRef.current.listWorkflows(req);
      setWorkflows(resp.workflows ?? []);
    } catch (err) {
      const e = err instanceof Error ? err : new Error("Failed to fetch workflows");
      setError(e);
      console.error("Failed to fetch workflows:", e);
    } finally {
      setLoading(false);
    }
  }, []);

  const refresh = useCallback(async () => {
    await fetchWorkflows();
  }, [fetchWorkflows]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const createWorkflow = useCallback(
    async (data: WorkflowFormData) => {
      if (!clientRef.current) return;
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
      await clientRef.current.createWorkflow(req);
      await refresh();
    },
    [refresh]
  );

  const updateWorkflow = useCallback(
    async (id: string, data: Partial<WorkflowFormData>) => {
      if (!clientRef.current) return;
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
      await clientRef.current.updateWorkflow(req);
      await refresh();
    },
    [refresh]
  );

  const deleteWorkflow = useCallback(
    async (id: string) => {
      if (!clientRef.current) return;
      // Snapshot before optimistic update for rollback.
      const prev = workflows;
      setWorkflows((w) => w.filter((wf) => wf.id !== id));
      try {
        const req = create(DeleteWorkflowRequestSchema, { id });
        await clientRef.current.deleteWorkflow(req);
      } catch (err) {
        setWorkflows(prev); // rollback on failure
        throw err;
      }
    },
    [workflows]
  );

  const archiveWorkflowSessions = useCallback(
    async (workflowId: string): Promise<number> => {
      if (!clientRef.current) return 0;
      const req = create(ArchiveWorkflowSessionsRequestSchema, { workflowId });
      const resp = await clientRef.current.archiveWorkflowSessions(req);
      await refresh();
      return resp.archivedCount;
    },
    [refresh]
  );

  const deleteWorkflowFailedSessions = useCallback(
    async (workflowId: string): Promise<number> => {
      if (!clientRef.current) return 0;
      const req = create(DeleteWorkflowFailedSessionsRequestSchema, { workflowId });
      const resp = await clientRef.current.deleteWorkflowFailedSessions(req);
      await refresh();
      return resp.deletedCount;
    },
    [refresh]
  );

  return { workflows, loading, error, createWorkflow, updateWorkflow, deleteWorkflow, archiveWorkflowSessions, deleteWorkflowFailedSessions, refresh };
}
