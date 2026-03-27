"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_connect";
import { ApprovalRuleProto, AutoDecision } from "@/gen/session/v1/types_pb";
import {
  ListApprovalRulesRequest,
  UpsertApprovalRuleRequest,
  DeleteApprovalRuleRequest,
} from "@/gen/session/v1/session_pb";
import { getApiBaseUrl } from "@/lib/config";

interface UseApprovalRulesOptions {
  sourceFilter?: string; // "user" | "seed" | "claude-settings"
}

interface UseApprovalRulesReturn {
  rules: ApprovalRuleProto[];
  loading: boolean;
  error: Error | null;
  upsertRule: (rule: Partial<ApprovalRuleProto> & { id: string }) => Promise<void>;
  deleteRule: (id: string) => Promise<void>;
  refresh: () => Promise<void>;
}

/**
 * React hook for managing auto-approval rules.
 *
 * Loads all rules via `listApprovalRules` and exposes upsert/delete actions
 * that call `upsertApprovalRule` / `deleteApprovalRule` on the ConnectRPC SessionService.
 *
 * Only "user" source rules can be edited or deleted; seed and claude-settings
 * rules are read-only.
 */
export function useApprovalRules(
  options: UseApprovalRulesOptions = {}
): UseApprovalRulesReturn {
  const { sourceFilter } = options;

  const [rules, setRules] = useState<ApprovalRuleProto[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const clientRef = useRef<ReturnType<typeof createPromiseClient<typeof SessionService>> | null>(null);

  useEffect(() => {
    const transport = createConnectTransport({ baseUrl: getApiBaseUrl() });
    clientRef.current = createPromiseClient(SessionService, transport);
  }, []);

  const fetchRules = useCallback(async () => {
    if (!clientRef.current) return;
    setLoading(true);
    setError(null);
    try {
      const req = new ListApprovalRulesRequest();
      if (sourceFilter) {
        req.sourceFilter = sourceFilter;
      }
      const resp = await clientRef.current.listApprovalRules(req);
      setRules(resp.rules ?? []);
    } catch (err) {
      const e = err instanceof Error ? err : new Error("Failed to fetch approval rules");
      setError(e);
      console.error("Failed to fetch approval rules:", e);
    } finally {
      setLoading(false);
    }
  }, [sourceFilter]);

  const refresh = useCallback(async () => {
    await fetchRules();
  }, [fetchRules]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const upsertRule = useCallback(
    async (ruleData: Partial<ApprovalRuleProto> & { id: string }) => {
      if (!clientRef.current) return;
      const rule = new ApprovalRuleProto({
        id: ruleData.id,
        name: ruleData.name ?? "",
        toolName: ruleData.toolName ?? "",
        toolPattern: ruleData.toolPattern ?? "",
        commandPattern: ruleData.commandPattern ?? "",
        filePattern: ruleData.filePattern ?? "",
        decision: ruleData.decision ?? AutoDecision.ESCALATE,
        riskLevel: ruleData.riskLevel ?? "",
        reason: ruleData.reason ?? "",
        alternative: ruleData.alternative ?? "",
        priority: ruleData.priority ?? 10,
        enabled: ruleData.enabled ?? true,
        source: "user",
      });
      const req = new UpsertApprovalRuleRequest({ rule });
      await clientRef.current.upsertApprovalRule(req);
      await refresh();
    },
    [refresh]
  );

  const deleteRule = useCallback(
    async (id: string) => {
      if (!clientRef.current) return;
      const req = new DeleteApprovalRuleRequest({ id });
      await clientRef.current.deleteApprovalRule(req);
      // Optimistic update
      setRules((prev) => prev.filter((r) => r.id !== id));
    },
    []
  );

  return { rules, loading, error, upsertRule, deleteRule, refresh };
}
