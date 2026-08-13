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
  keepSessions?: number;
  archiveAfterHours?: number;
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
        ...(data.keepSessions !== undefined && { keepSessions: data.keepSessions }),
        ...(data.archiveAfterHours !== undefined && { archiveAfterHours: data.archiveAfterHours }),
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
        ...(data.keepSessions !== undefined && { keepSessions: data.keepSessions }),
        ...(data.archiveAfterHours !== undefined && { archiveAfterHours: data.archiveAfterHours }),
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
