import { useState } from "react";
import { ConnectError, Code } from "@connectrpc/connect";

export type DiagnoseActionState = "idle" | "pending" | "error" | "already-diagnosing";

export interface UseDiagnoseActionResult {
  state: DiagnoseActionState;
  error: string | null;
  diagnose: (itemId: string) => Promise<void>;
}

/**
 * Shared Diagnose-button state machine for `StuckItemDetail` and
 * `BacklogItemDetail` (design/ux.md Surfaces 1-3, 14), mirroring
 * `onApprovePlan`'s idle/pending/error contract with one addition: a
 * `Code.FailedPrecondition` rejection means the backend declined a
 * duplicate/racing dispatch for this item (`DiagnoseBacklogItem` returns
 * this, not a response field, when a dispatch is already in flight) — the
 * gate working correctly, not a failure — so it settles into
 * `"already-diagnosing"` (Surface 3's busy display, extended) instead of the
 * generic error path, and does not revert to idle the way a real failure
 * does.
 */
export function useDiagnoseAction(
  onDiagnose?: (itemId: string) => Promise<void>
): UseDiagnoseActionResult {
  const [state, setState] = useState<DiagnoseActionState>("idle");
  const [error, setError] = useState<string | null>(null);

  async function diagnose(itemId: string) {
    if (!onDiagnose) return;
    setState("pending");
    setError(null);
    try {
      await onDiagnose(itemId);
      setState("idle");
    } catch (err) {
      if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
        setState("already-diagnosing");
        return;
      }
      setState("error");
      setError(err instanceof Error ? err.message : "Failed to diagnose — try again.");
    }
  }

  return { state, error, diagnose };
}
