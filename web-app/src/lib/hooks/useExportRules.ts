"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService, ExportRulesRequestSchema } from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import { getConnectTransport } from "@/lib/api/transport";

export interface UseExportRulesReturn {
  exportRules: (ruleIds?: string[]) => Promise<void>;
  loading: boolean;
  error: Error | null;
}

/**
 * Hook that calls ExportRules and triggers a browser file download.
 * - Creates a Blob from the returned YAML content.
 * - Triggers an <a download="rules.yaml"> click to start the download.
 * - Revokes the object URL after the click to release memory.
 */
export function useExportRules(): UseExportRulesReturn {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  // Initialize client inside useEffect to avoid SSR issues (Concern 3 from validation.md).
  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  useEffect(() => {
    clientRef.current = createClient(SessionService, getConnectTransport());
  }, []);

  const exportRules = useCallback(async (ruleIds?: string[]) => {
    if (!clientRef.current) return;
    setLoading(true);
    setError(null);
    try {
      const req = create(ExportRulesRequestSchema, { ruleIds: ruleIds ?? [] });
      const resp = await clientRef.current.exportRules(req);
      const blob = new Blob([resp.yamlContent], { type: "text/yaml" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = "rules.yaml";
      a.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      setError(err instanceof Error ? err : new Error("Export failed"));
    } finally {
      setLoading(false);
    }
  }, []);

  return { exportRules, loading, error };
}
