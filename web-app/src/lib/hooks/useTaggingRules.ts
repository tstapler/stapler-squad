"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { TaggingRuleProto, TaggingRuleProtoSchema } from "@/gen/session/v1/types_pb";
import {
  ListTaggingRulesRequestSchema,
  UpsertTaggingRuleRequestSchema,
  DeleteTaggingRuleRequestSchema,
} from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import { getConnectTransport } from "@/lib/api/transport";

interface UseTaggingRulesReturn {
  rules: TaggingRuleProto[];
  loading: boolean;
  error: Error | null;
  upsertRule: (rule: Partial<TaggingRuleProto> & { id: string }) => Promise<void>;
  deleteRule: (id: string) => Promise<void>;
  refresh: () => Promise<void>;
}

/** Builds a `TaggingRuleProto` from partial rule data, filling in defaults for unset fields. */
function buildTaggingRuleProto(ruleData: Partial<TaggingRuleProto> & { id: string }): TaggingRuleProto {
  return create(TaggingRuleProtoSchema, {
    id: ruleData.id,
    name: ruleData.name ?? "",
    namePattern: ruleData.namePattern ?? "",
    branchPattern: ruleData.branchPattern ?? "",
    pathPattern: ruleData.pathPattern ?? "",
    programPattern: ruleData.programPattern ?? "",
    requiredTags: ruleData.requiredTags ?? [],
    outputTag: ruleData.outputTag ?? "",
    priority: ruleData.priority ?? 50,
    enabled: ruleData.enabled ?? true,
    source: "user",
  });
}

/**
 * React hook for managing session-tagging rules (the sibling rule system to
 * approval rules — see ApprovalRulesPanel/useApprovalRules for the equivalent
 * pattern). Loads rules via `listTaggingRules` and exposes upsert/delete
 * actions against the ConnectRPC SessionService.
 *
 * Only "user" source rules can be edited/deleted/toggled; seed rules are
 * read-only (mirrors approval rules' seed-rule convention).
 */
export function useTaggingRules(): UseTaggingRulesReturn {
  const [rules, setRules] = useState<TaggingRuleProto[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  useEffect(() => {
    clientRef.current = createClient(SessionService, getConnectTransport());
  }, []);

  const fetchRules = useCallback(async () => {
    if (!clientRef.current) return;
    setLoading(true);
    setError(null);
    try {
      const req = create(ListTaggingRulesRequestSchema, {});
      const resp = await clientRef.current.listTaggingRules(req);
      setRules(resp.rules ?? []);
    } catch (err) {
      const e = err instanceof Error ? err : new Error("Failed to fetch tagging rules");
      setError(e);
      console.error("Failed to fetch tagging rules:", e);
    } finally {
      setLoading(false);
    }
  }, []);

  const refresh = useCallback(async () => {
    await fetchRules();
  }, [fetchRules]);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const upsertRule = useCallback(
    async (ruleData: Partial<TaggingRuleProto> & { id: string }) => {
      if (!clientRef.current) return;
      const rule = buildTaggingRuleProto(ruleData);
      const req = create(UpsertTaggingRuleRequestSchema, { rule });
      await clientRef.current.upsertTaggingRule(req);
      await refresh();
    },
    [refresh]
  );

  const deleteRule = useCallback(
    async (id: string) => {
      if (!clientRef.current) return;
      const req = create(DeleteTaggingRuleRequestSchema, { id });
      await clientRef.current.deleteTaggingRule(req);
      setRules((prev) => prev.filter((r) => r.id !== id));
    },
    []
  );

  return { rules, loading, error, upsertRule, deleteRule, refresh };
}
