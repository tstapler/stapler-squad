"use client";

import { useCallback } from "react";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";

/**
 * Returns a function that asks the server to rescan a session's transcript.
 * The client is created per call so rendering the Artifacts tab opens no transport.
 * Resolves when the scan is queued (its result arrives as a session update);
 * rejects when the RPC fails or the session has no transcript to scan.
 */
export function useRescanArtifacts(sessionId: string): () => Promise<void> {
  return useCallback(async () => {
    const client = createClient(SessionService, getConnectTransport());
    // A queued rescan must finish even if the tab unmounts.
    // abort-signal-exempt
    const resp = await client.rescanSessionArtifacts({ id: sessionId });
    if (!resp.scanStarted) {
      throw new Error("This session has no linked transcript to scan.");
    }
  }, [sessionId]);
}
