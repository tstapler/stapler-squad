"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import { BacklogService, type GateStatus as GateStatusProto } from "@/gen/session/v1/backlog_pb";
import type { GateChecklistItem, GateKind } from "@/components/backlog/GateChecklist";
import { getErrorMessage } from "@/lib/utils/connectError";

/** One non-`archived` candidate transition with at least one configured gate. */
export interface GateBlockingCandidate {
  toStatus: string;
  gates: GateChecklistItem[];
}

export interface UseGateChecklistReturn {
  candidates: GateBlockingCandidate[];
  isLoading: boolean;
  error: string | null;
  refetch: () => void;
}

function mapGateStatus(g: GateStatusProto): GateChecklistItem {
  return {
    gateId: g.gateId,
    kind: g.kind as GateKind,
    satisfied: g.satisfied,
    description: g.description || undefined,
    actionHint: g.actionHint || undefined,
    configError: g.configError || undefined,
  };
}

/**
 * ADR-005 Decision point 2: resolves "which transition(s) is the item-detail
 * gate checklist about" as every non-`archived` entry in `allowedTransitions`
 * that has at least one configured gate — not a single "next" transition,
 * since the transition graph has no unambiguous one. `archived` is excluded
 * unconditionally: it's this codebase's universal operator escape hatch
 * (OverrideVerdict bypasses gate checks the same way), so a "what's blocking
 * X -> Archived?" row would contradict that.
 *
 * Calls GetPendingGates(itemId, to) once per remaining candidate and keeps
 * only the candidates whose response is non-empty — a target with zero gates
 * renders nothing, which is what collapses the common built-in case (most
 * edges have no configured gates) to "section does not render" without a
 * special case.
 */
async function fetchCandidates(
  client: ReturnType<typeof createClient<typeof BacklogService>>,
  itemId: string,
  targets: string[]
): Promise<GateBlockingCandidate[]> {
  const results = await Promise.all(
    targets.map(async (to): Promise<GateBlockingCandidate> => {
      const resp = await client.getPendingGates({ itemId, toStatus: to });
      return { toStatus: to, gates: (resp.gates ?? []).map(mapGateStatus) };
    })
  );
  return results.filter((c) => c.gates.length > 0);
}

interface GateFetchSetters {
  setCandidates: (c: GateBlockingCandidate[]) => void;
  setIsLoading: (b: boolean) => void;
  setError: (e: string | null) => void;
}

/** Runs one fetch-and-apply cycle for the effect below; returns its cleanup (cancel) function. */
function runGateFetch(
  itemId: string,
  targets: string[],
  clientRef: { current: ReturnType<typeof createClient<typeof BacklogService>> | null },
  { setCandidates, setIsLoading, setError }: GateFetchSetters
): () => void {
  if (!itemId || targets.length === 0) {
    setCandidates([]);
    setIsLoading(false);
    setError(null);
    return () => {};
  }

  let cancelled = false;
  if (!clientRef.current) {
    clientRef.current = createClient(BacklogService, getConnectTransport());
  }

  setIsLoading(true);
  setError(null);

  fetchCandidates(clientRef.current, itemId, targets)
    .then((results) => {
      if (!cancelled) setCandidates(results);
    })
    .catch((err) => {
      if (cancelled) return;
      console.error("[useGateChecklist] getPendingGates:", err);
      setError(getErrorMessage(err, "Couldn't load pending gates"));
    })
    .finally(() => {
      if (!cancelled) setIsLoading(false);
    });

  return () => {
    cancelled = true;
  };
}

export function useGateChecklist(itemId: string, allowedTransitions: string[]): UseGateChecklistReturn {
  const [candidates, setCandidates] = useState<GateBlockingCandidate[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reloadToken, setReloadToken] = useState(0);
  const clientRef = useRef<ReturnType<typeof createClient<typeof BacklogService>> | null>(null);

  const targets = allowedTransitions.filter((t) => t !== "archived");
  // Stable string key so this effect doesn't refire just because the caller
  // passed a fresh array literal this render — same idiom as
  // useWatchBacklogItems.ts's statusFilterKey/categoryFilterKey.
  const targetsKey = targets.join(",");

  useEffect(() => {
    return runGateFetch(itemId, targets, clientRef, { setCandidates, setIsLoading, setError });
    // targets/targetsKey intentionally paired: targetsKey is the real dep,
    // targets itself is recomputed fresh every render from allowedTransitions.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [itemId, targetsKey, reloadToken]);

  const refetch = useCallback(() => setReloadToken((t) => t + 1), []);

  return { candidates, isLoading, error, refetch };
}

export interface UseGateApprovalReturn {
  /**
   * Calls RecordGateApproval(itemId, gateId, approved) — ADR-006 Part A.
   * `approved: true` is permanently one-shot (a second call once approved is
   * always rejected); `approved: false` is reversible (a later call with
   * `approved: true` overwrites it). Rejects on RPC failure — caller
   * (GateChecklist) shows a row-scoped error.
   */
  recordApproval: (itemId: string, gateId: string, approved: boolean) => Promise<void>;
}

/**
 * Thin client wrapper for BacklogService.RecordGateApproval (Epic 2.4.1,
 * ADR-006 Part A), following useBacklogStagesAdmin's self-contained-client
 * convention rather than folding into useBacklogService.ts.
 */
export function useGateApproval(): UseGateApprovalReturn {
  const clientRef = useRef<ReturnType<typeof createClient<typeof BacklogService>> | null>(null);

  const recordApproval = useCallback(async (itemId: string, gateId: string, approved: boolean): Promise<void> => {
    if (!clientRef.current) {
      clientRef.current = createClient(BacklogService, getConnectTransport());
    }
    try {
      await clientRef.current.recordGateApproval({ itemId, gateId, satisfiedBy: "", approved });
    } catch (err) {
      console.error("[useGateApproval] recordGateApproval:", err);
      throw new Error(getErrorMessage(err, approved ? "Couldn't record approval" : "Couldn't record rejection"));
    }
  }, []);

  return { recordApproval };
}
