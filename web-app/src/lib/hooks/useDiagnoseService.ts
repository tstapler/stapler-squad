"use client";

import { useCallback } from "react";
import { createClient } from "@connectrpc/connect";
import { DiagnoseService } from "@/gen/session/v1/diagnose_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { useAbortableRequest } from "@/lib/hooks/useAbortableRequest";

export interface UseDiagnoseServiceResult {
  /**
   * Requests a diagnose dispatch for a backlog item (DiagnoseBacklogItem
   * RPC) -- the `(itemId) => Promise<void>` contract shared by
   * StuckItemDetail/BacklogItemDetail's `onDiagnose` prop and
   * useDiagnoseAction. Rejects (throws the real ConnectError, e.g.
   * Code.FailedPrecondition for an in-flight dispatch) rather than
   * swallowing failures -- see useDiagnoseAction's doc comment for how that
   * rejection is turned into UI state.
   */
  diagnose: (itemId: string) => Promise<void>;
}

/**
 * Thin RPC wrapper shared by every real onDiagnose call site
 * (StuckItemsSection, the backlog list/board pages) -- mirrors
 * useDiagnoseDispatches.ts's own `createClient(DiagnoseService,
 * getConnectTransport())` construction rather than useBacklogService's
 * heavier clientRef setup, since this hook wraps a single unary RPC.
 */
export function useDiagnoseService(): UseDiagnoseServiceResult {
  const startRequest = useAbortableRequest();

  const diagnose = useCallback(
    async (itemId: string): Promise<void> => {
      const client = createClient(DiagnoseService, getConnectTransport());
      const signal = startRequest();
      await client.diagnoseBacklogItem({ itemId }, { signal });
    },
    [startRequest]
  );

  return { diagnose };
}
