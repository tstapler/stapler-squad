"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { GetProgramAnalyticsResponse } from "@/gen/session/v1/session_pb";
import { GetProgramAnalyticsRequestSchema } from "@/gen/session/v1/session_pb";
import { create } from "@bufbuild/protobuf";
import { getConnectTransport } from "@/lib/api/transport";

interface UseProgramAnalyticsResult {
  data: GetProgramAnalyticsResponse | null;
  isLoading: boolean;
  error: Error | null;
  refresh: () => void;
}

export function useProgramAnalytics(
  program: string | null,
  windowDays: number
): UseProgramAnalyticsResult {
  const clientRef = useRef<ReturnType<
    typeof createClient<typeof SessionService>
  > | null>(null);

  const [data, setData] = useState<GetProgramAnalyticsResponse | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    clientRef.current = createClient(SessionService, getConnectTransport());
  }, []);

  const fetch = useCallback(() => {
    if (!program) {
      setData(null);
      setIsLoading(false);
      return () => {};
    }
    if (!clientRef.current) {
      return () => {};
    }
    const controller = new AbortController();
    setIsLoading(true);
    setError(null);

    const req = create(GetProgramAnalyticsRequestSchema, {
      program,
      windowDays,
    });

    clientRef.current
      .getProgramAnalytics(req, { signal: controller.signal })
      .then((resp) => {
        setData(resp);
        setIsLoading(false);
      })
      .catch((err: unknown) => {
        if (err instanceof Error && err.name === "AbortError") return;
        setData(null);
        setError(err instanceof Error ? err : new Error(String(err)));
        setIsLoading(false);
      });
    return () => controller.abort();
  }, [program, windowDays]);

  useEffect(() => {
    const cleanup = fetch();
    return cleanup;
  }, [fetch]);

  return { data, isLoading, error, refresh: fetch };
}
