"use client";

import { useEffect, useState, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import {
  ValidateRulesRequestSchema,
  type ParsedRuleResult,
} from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import { getConnectTransport } from "@/lib/api/transport";

export interface UseValidateRulesReturn {
  results: ParsedRuleResult[];
  loading: boolean;
  validCount: number;
  errorCount: number;
  error: Error | null;
}

/**
 * Debounced hook that calls ValidateRules on YAML input.
 * - Debounce delay is configurable (default 400ms).
 * - Clears results when yamlContent is empty.
 * - Cancels in-flight requests via AbortController when input changes or component unmounts.
 */
export function useValidateRules(
  yamlContent: string,
  debounceMs = 400
): UseValidateRulesReturn {
  const [results, setResults] = useState<ParsedRuleResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [validCount, setValidCount] = useState(0);
  const [errorCount, setErrorCount] = useState(0);
  const [error, setError] = useState<Error | null>(null);

  // Initialize client inside useEffect to avoid SSR issues.
  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    clientRef.current = createClient(SessionService, getConnectTransport());
  }, []);

  useEffect(() => {
    if (!yamlContent.trim()) {
      setResults([]);
      setValidCount(0);
      setErrorCount(0);
      setError(null);
      setLoading(false);
      return;
    }

    const timer = setTimeout(async () => {
      if (!clientRef.current) return;
      // Cancel any in-flight request before starting a new one.
      abortRef.current?.abort();
      abortRef.current = new AbortController();

      setLoading(true);
      setError(null);
      try {
        const req = create(ValidateRulesRequestSchema, { yamlContent });
        const resp = await clientRef.current.validateRules(req, {
          signal: abortRef.current.signal,
        });
        setResults(resp.results ?? []);
        setValidCount(resp.validCount);
        setErrorCount(resp.errorCount);
      } catch (err) {
        if ((err as Error).name === "AbortError") return;
        setError(err instanceof Error ? err : new Error("Validation failed"));
        setResults([]);
        setValidCount(0);
        setErrorCount(0);
      } finally {
        setLoading(false);
      }
    }, debounceMs);

    return () => {
      clearTimeout(timer);
      // Cancel in-flight request on unmount or when dependencies change.
      abortRef.current?.abort();
    };
  }, [yamlContent, debounceMs]);

  return { results, loading, validCount, errorCount, error };
}
