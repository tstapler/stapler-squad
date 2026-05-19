"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService, GenerateSuggestedRuleRequestSchema } from "@/gen/session/v1/session_pb";
import { SuggestedRuleProto, SuggestionSource } from "@/gen/session/v1/types_pb";
import { create } from "@bufbuild/protobuf";
import { getConnectTransport } from "@/lib/api/transport";

/** Caller-supplied parameters for a GenerateSuggestedRule RPC call. All fields optional. */
export interface GenerateRuleRequest {
  source?: SuggestionSource;
  windowDays?: number;
  commandSample?: string;
  analyticsItemId?: string;
  toolNameFilter?: string;
  programNameFilter?: string;
}

export interface UseGenerateRuleResult {
  /** Empty until loaded; up to 5 items per analytics-gaps call. */
  suggestions: SuggestedRuleProto[];
  loading: boolean;
  error: Error | null;
  generate: (req: GenerateRuleRequest) => Promise<void>;
  /** Abort any in-flight generate call. Does not set error. */
  cancel: () => void;
  /** Reset suggestions and error back to initial state. */
  clear: () => void;
}

/**
 * React hook encapsulating all state for the GenerateSuggestedRule RPC.
 *
 * Used by all four AI rule-generation surfaces (rules panel, review queue,
 * analytics gaps, command-sample form). Callers may also call clear() or
 * cancel() directly.
 */
export function useGenerateRule(): UseGenerateRuleResult {
  const [suggestions, setSuggestions] = useState<SuggestedRuleProto[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const abortRef = useRef<AbortController | null>(null);
  const clientRef = useRef<ReturnType<typeof createClient<typeof SessionService>> | null>(null);

  // Abort any in-flight request on unmount to prevent state updates on unmounted components.
  useEffect(() => {
    return () => {
      abortRef.current?.abort();
    };
  }, []);

  // Lazily initialise the client (mirrors useApprovalRules pattern).
  const getClient = useCallback(() => {
    if (!clientRef.current) {
      clientRef.current = createClient(SessionService, getConnectTransport());
    }
    return clientRef.current;
  }, []);

  // Track whether the most recent abort was a user-initiated cancel (vs. timeout).
  const userCancelledRef = useRef(false);

  const generate = useCallback(
    async (req: GenerateRuleRequest) => {
      // Cancel any previous in-flight request.
      userCancelledRef.current = false;
      abortRef.current?.abort();
      abortRef.current = new AbortController();

      // Reset previous state at the start of each call.
      setSuggestions([]);
      setError(null);
      setLoading(true);

      try {
        const client = getClient();
        const request = create(GenerateSuggestedRuleRequestSchema, req as Record<string, unknown>);
        const resp = await client.generateSuggestedRule(request, {
          signal: abortRef.current.signal,
          timeoutMs: 60_000,
        });
        setSuggestions(resp.suggestions ?? []);
      } catch (err) {
        const e = err instanceof Error ? err : new Error("GenerateSuggestedRule failed");
        if (e.name === "AbortError") {
          if (userCancelledRef.current) {
            // Manual cancel via cancel() — silently swallow. "Rule generation was cancelled."
          } else {
            // Timeout-triggered abort — surface a friendly message.
            setError(new Error("Rule generation timed out. Please try again."));
          }
        } else {
          setError(e);
        }
      } finally {
        setLoading(false);
      }
    },
    [getClient]
  );

  const cancel = useCallback(() => {
    userCancelledRef.current = true;
    abortRef.current?.abort();
    setLoading(false);
  }, []);

  const clear = useCallback(() => {
    setSuggestions([]);
    setError(null);
  }, []);

  return { suggestions, loading, error, generate, cancel, clear };
}
