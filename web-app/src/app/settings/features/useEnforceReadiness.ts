"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import { BacklogService } from "@/gen/session/v1/backlog_pb";
import {
  differencesText,
  readinessFromResponse,
  type CallerClass,
  type ClassReadinessView,
  type SoakVerdictKind,
} from "@/lib/hooks/useEngineModeStatus";

export type EvidenceState =
  | { phase: "loading" }
  | { phase: "loaded"; readiness: ClassReadinessView }
  | { phase: "failed"; error: string };

export type RecheckState = { phase: "idle" } | { phase: "running" } | { phase: "failed"; error: string };

/** What the operator must act on next once the dialog changed under them. */
export type FocusRequest = "ack" | null;

export const VERDICT_LABEL: Record<SoakVerdictKind, string> = {
  ready: "READY",
  not_ready: "NOT READY",
  needs_ack: "needs your acknowledgement",
  unavailable: "unavailable",
};

/** Lower is better; a move up the ladder is "worse". */
const VERDICT_RANK: Record<SoakVerdictKind, number> = { ready: 0, needs_ack: 1, not_ready: 2, unavailable: 3 };

/** Clean means nothing to review: no differences, READY, no item loses an exit. */
export function isClean(r: ClassReadinessView): boolean {
  return r.divergence24h === 0 && r.verdict === "ready" && r.strandedItemCount === 0;
}

/** Describes how `next` is worse than `prev`, or null when it is not worse. */
export function worseningOf(prev: ClassReadinessView | null, next: ClassReadinessView): string | null {
  if (!prev) return "readiness evidence is now available";
  const changes: string[] = [];
  if (next.divergence24h > prev.divergence24h) {
    changes.push(`shadow differences rose from ${prev.divergence24h} to ${next.divergence24h}`);
  }
  if (VERDICT_RANK[next.verdict] > VERDICT_RANK[prev.verdict]) {
    changes.push(`soak verdict moved from ${VERDICT_LABEL[prev.verdict]} to ${VERDICT_LABEL[next.verdict]}`);
  }
  if (next.strandedItemCount > prev.strandedItemCount) {
    changes.push(`items that would lose an exit rose from ${prev.strandedItemCount} to ${next.strandedItemCount}`);
  }
  if (changes.length === 0 && isClean(prev) && !isClean(next)) changes.push("the evidence is no longer clean");
  return changes.length > 0 ? changes.join("; ") : null;
}

export function evidenceAnnouncement(r: ClassReadinessView, needsAck: boolean): string {
  const lines = `${differencesText(r.divergence24h)} shadow ${r.divergence24h === 1 ? "difference" : "differences"}, soak verdict ${
    VERDICT_LABEL[r.verdict]
  }, ${r.strandedItemCount} ${r.strandedItemCount === 1 ? "item" : "items"} would lose an exit.`;
  return `Readiness evidence: ${lines}${needsAck ? " Acknowledgement required." : ""}`;
}

async function fetchReadiness(cls: CallerClass): Promise<ClassReadinessView> {
  const client = createClient(BacklogService, getConnectTransport());
  const resp = await client.getWorkflowEngineStatus({});
  const readiness = readinessFromResponse(resp, cls);
  if (!readiness) throw new Error(`the status carried no readiness for ${cls}`);
  return readiness;
}

function reasonOf(err: unknown): string {
  return err instanceof Error ? err.message : "request failed";
}

export interface UseEnforceReadiness {
  evidence: EvidenceState;
  /** Evidence loaded and clean. */
  clean: boolean;
  /** The acknowledgement row is shown (evidence not clean, or failed to load). */
  needsAck: boolean;
  acknowledged: boolean;
  setAcknowledged: (v: boolean) => void;
  /** Enforce may be activated: evidence resolved and clean, or acknowledged. */
  canEnforce: boolean;
  recheck: RecheckState;
  /** Set when a re-check found the evidence worse than what was acknowledged. */
  changeNote: string | null;
  focusRequest: FocusRequest;
  /** True while a Retry of a failed evidence load is in flight (the Retry button stays mounted). */
  retrying: boolean;
  /** Resolves true when the retry loaded evidence. */
  retryEvidence: () => Promise<boolean>;
  /**
   * Re-fetches readiness. Resolves true only when the write may proceed:
   * evidence unchanged or not worse than what the operator was shown.
   */
  confirm: () => Promise<boolean>;
}

/**
 * Evidence state machine for the enforce dialog (UX-35, UX-44, UX-45): loading,
 * clean, not clean, failed, re-checking, changed-since-open, re-check-failed.
 * Fetches on mount (the dialog mounts when opened); the caller writes the flag
 * only when `confirm()` resolves true.
 */
export function useEnforceReadiness(cls: CallerClass): UseEnforceReadiness {
  const [evidence, setEvidence] = useState<EvidenceState>({ phase: "loading" });
  const [acknowledged, setAcknowledged] = useState(false);
  const [recheck, setRecheck] = useState<RecheckState>({ phase: "idle" });
  const [changeNote, setChangeNote] = useState<string | null>(null);
  const [focusRequest, setFocusRequest] = useState<FocusRequest>(null);
  const [retrying, setRetrying] = useState(false);
  const mounted = useRef(true);
  // The view the operator saw; the re-check compares against it, not against later fetches.
  const shown = useRef<ClassReadinessView | null>(null);

  const load = useCallback(async (): Promise<boolean> => {
    try {
      const readiness = await fetchReadiness(cls);
      if (!mounted.current) return false;
      shown.current = readiness;
      setEvidence({ phase: "loaded", readiness });
      return true;
    } catch (err) {
      console.error("[useEnforceReadiness] getWorkflowEngineStatus:", err);
      if (mounted.current) setEvidence({ phase: "failed", error: reasonOf(err) });
      return false;
    }
  }, [cls]);

  useEffect(() => {
    mounted.current = true;
    void load();
    return () => {
      mounted.current = false;
    };
  }, [load]);

  const retryEvidence = useCallback(async () => {
    setRetrying(true);
    // An ack given for "no evidence" must not carry over to evidence the operator has not seen.
    setAcknowledged(false);
    const ok = await load();
    if (mounted.current) setRetrying(false);
    return ok;
  }, [load]);

  const confirm = useCallback(async (): Promise<boolean> => {
    setRecheck({ phase: "running" });
    setFocusRequest(null);
    let fresh: ClassReadinessView;
    try {
      fresh = await fetchReadiness(cls);
    } catch (err) {
      console.error("[useEnforceReadiness] re-check:", err);
      if (mounted.current) setRecheck({ phase: "failed", error: reasonOf(err) });
      return false;
    }
    if (!mounted.current) return false;
    const worse = worseningOf(shown.current, fresh);
    if (worse) {
      shown.current = fresh;
      setEvidence({ phase: "loaded", readiness: fresh });
      setAcknowledged(false);
      setChangeNote(`Readiness evidence changed since you opened this dialog: ${worse}. Review it and confirm again.`);
      setRecheck({ phase: "idle" });
      if (!isClean(fresh)) setFocusRequest("ack");
      return false;
    }
    setRecheck({ phase: "idle" });
    return true;
  }, [cls]);

  const loaded = evidence.phase === "loaded" ? evidence.readiness : null;
  const clean = loaded !== null && isClean(loaded);
  const needsAck = evidence.phase === "failed" || (loaded !== null && !clean);
  const canEnforce = evidence.phase !== "loading" && (clean || (needsAck && acknowledged));

  return {
    evidence,
    clean,
    needsAck,
    acknowledged,
    setAcknowledged,
    canEnforce,
    recheck,
    changeNote,
    focusRequest,
    retrying,
    retryEvidence,
    confirm,
  };
}
