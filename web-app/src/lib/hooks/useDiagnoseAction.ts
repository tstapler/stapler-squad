"use client";

import { useCallback, useMemo } from "react";
import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import {
  DiagnosticService,
  DispatchDiagnoseRequestSchema,
  type DispatchDiagnoseResponse,
} from "@/gen/session/v1/diagnose_pb";
import { StuckReason } from "@/gen/session/v1/backlog_pb";
import { create } from "@bufbuild/protobuf";

export interface UseDiagnoseActionReturn {
  /**
   * Dispatches a Diagnose & Nudge session for itemId. reason (the specific
   * StuckReason StuckItemDetail is showing) is optional — omit it (or pass
   * StuckReason.UNSPECIFIED, e.g. from BacklogItemDetail on a non-stuck
   * item) to get a diagnose-only dispatch with no nudge action space.
   * Deliberately NOT try/catch-swallowed (mirrors triggerRemediationNow in
   * useStuckBacklogItems.ts) — callers need the specific error to show the
   * operator why dispatch failed, not just a generic failure.
   */
  dispatchDiagnose: (
    itemId: string,
    reason?: StuckReason
  ) => Promise<DispatchDiagnoseResponse>;
}

/**
 * Dispatches the "Diagnose & Nudge" backlog action (backlog item 68964304).
 * A separate hook from useStuckBacklogItems (rather than adding another
 * method there) since this dispatch is meaningful from BacklogItemDetail.tsx
 * too, on items that aren't flagged stuck at all — bundling it into the
 * stuck-items-only hook would force every consumer through that hook's
 * polling/context machinery for a single one-shot action call.
 */
export function useDiagnoseAction(): UseDiagnoseActionReturn {
  const transport = useMemo(() => getConnectTransport(), []);
  const client = useMemo(() => createClient(DiagnosticService, transport), [transport]);

  const dispatchDiagnose = useCallback(
    async (itemId: string, reason: StuckReason = StuckReason.UNSPECIFIED) => {
      const req = create(DispatchDiagnoseRequestSchema, {
        itemId,
        stuckReason: reason,
      });
      return client.dispatchDiagnose(req);
    },
    [client]
  );

  return { dispatchDiagnose };
}
