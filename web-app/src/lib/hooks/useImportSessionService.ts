"use client";

import { useEffect, useRef, useCallback } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { ImportService } from "@/gen/session/v1/import_pb";
import type {
  ExternalSessionCandidateRef,
  CorrelationResultProto,
  PIDIdentity,
  PreviewImportExternalSessionResponse,
  CommitImportExternalSessionResponse,
  ConfirmKillExternalSessionResponse,
  CancelPendingKillResponse,
} from "@/gen/session/v1/import_pb";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import { createRpcTimingInterceptor } from "@/lib/telemetry/rpcTiming";
import { useAnalytics } from "@/lib/contexts/AnalyticsContext";

interface UseImportSessionServiceOptions {
  baseUrl?: string;
}

interface UseImportSessionServiceReturn {
  previewImport: (
    candidate: ExternalSessionCandidateRef
  ) => Promise<PreviewImportExternalSessionResponse | null>;
  commitImport: (params: {
    candidate: ExternalSessionCandidateRef;
    expectedCorrelation: CorrelationResultProto;
    disambiguationChoice?: string;
    pidIdentity?: PIDIdentity;
  }) => Promise<CommitImportExternalSessionResponse | null>;
  confirmKill: (params: {
    instanceId: string;
    pidIdentity?: PIDIdentity;
  }) => Promise<ConfirmKillExternalSessionResponse | null>;
  cancelPendingKill: (params: {
    instanceId: string;
    pidIdentity?: PIDIdentity;
  }) => Promise<CancelPendingKillResponse | null>;
}

// ImportService is unary-only (no Watch* streaming RPCs), so a plain HTTP
// ConnectRPC transport is sufficient -- unlike useSessionService, this hook
// does not need createWatchTransport's WebSocket upgrade path.
export function useImportSessionService(
  options: UseImportSessionServiceOptions = {}
): UseImportSessionServiceReturn {
  const { baseUrl = getApiBaseUrl() } = options;
  const analytics = useAnalytics();
  const clientRef = useRef<ReturnType<typeof createClient<typeof ImportService>> | null>(null);

  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl,
      interceptors: [createAuthInterceptor(), createRpcTimingInterceptor(analytics)],
    });
    clientRef.current = createClient(ImportService, transport);
    // analytics identity is stable (memoized in AnalyticsContextProvider); baseUrl is the only real dep
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [baseUrl]);

  const previewImport = useCallback(
    async (
      candidate: ExternalSessionCandidateRef
    ): Promise<PreviewImportExternalSessionResponse | null> => {
      if (!clientRef.current) return null;
      return await clientRef.current.previewImportExternalSession({ candidate });
    },
    []
  );

  const commitImport = useCallback(
    async ({
      candidate,
      expectedCorrelation,
      disambiguationChoice,
      pidIdentity,
    }: {
      candidate: ExternalSessionCandidateRef;
      expectedCorrelation: CorrelationResultProto;
      disambiguationChoice?: string;
      pidIdentity?: PIDIdentity;
    }): Promise<CommitImportExternalSessionResponse | null> => {
      if (!clientRef.current) return null;
      return await clientRef.current.commitImportExternalSession({
        candidate,
        expectedCorrelation,
        disambiguationChoice: disambiguationChoice ?? "",
        pidIdentity,
      });
    },
    []
  );

  const confirmKill = useCallback(
    async ({
      instanceId,
      pidIdentity,
    }: {
      instanceId: string;
      pidIdentity?: PIDIdentity;
    }): Promise<ConfirmKillExternalSessionResponse | null> => {
      if (!clientRef.current) return null;
      return await clientRef.current.confirmKillExternalSession({
        instanceId,
        pidIdentity,
      });
    },
    []
  );

  const cancelPendingKill = useCallback(
    async ({
      instanceId,
      pidIdentity,
    }: {
      instanceId: string;
      pidIdentity?: PIDIdentity;
    }): Promise<CancelPendingKillResponse | null> => {
      if (!clientRef.current) return null;
      return await clientRef.current.cancelPendingKill({
        instanceId,
        pidIdentity,
      });
    },
    []
  );

  return { previewImport, commitImport, confirmKill, cancelPendingKill };
}
