"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { NotificationData } from "@/lib/types/notification";

export type DecisionPhase = "idle" | "confirming" | "sending" | "failed";
export type Verb = "approve" | "deny";

/** low and medium approve in one tap; high, critical and an unrecorded level ask first (fail safe). */
export function needsApproveConfirm(riskLevel: string | undefined): boolean {
  return riskLevel !== "low" && riskLevel !== "medium";
}

/**
 * The stacked card's Approve and Deny flow: both buttons disable together while a
 * request is in flight (a double tap sends one), a failure keeps the toast with
 * "Could not <verb> - Retry", and on a phone a risky approve asks first.
 */
export function useDecisionFlow(args: {
  notification: NotificationData;
  onClose: (options?: { acknowledge?: boolean }) => void;
  confirmApprove: boolean;
  onBusyChange?: (busy: boolean) => void;
  onActionFailed?: () => void;
}) {
  const { notification, onClose, confirmApprove, onBusyChange, onActionFailed } = args;
  const [phase, setPhase] = useState<DecisionPhase>("idle");
  const [verb, setVerb] = useState<Verb>("approve");
  const inFlight = useRef(false);

  useEffect(() => {
    onBusyChange?.(phase === "sending" || phase === "confirming");
  }, [phase, onBusyChange]);

  const run = useCallback(
    async (next: Verb) => {
      if (inFlight.current) return;
      inFlight.current = true;
      setVerb(next);
      setPhase("sending");
      try {
        await (next === "approve" ? notification.onApprove?.() : notification.onDeny?.());
        onClose({ acknowledge: true });
      } catch {
        setPhase("failed");
        onActionFailed?.();
      } finally {
        inFlight.current = false;
      }
    },
    [notification, onClose, onActionFailed],
  );

  const approve = useCallback(() => {
    if (confirmApprove && phase !== "confirming") setPhase("confirming");
    else void run("approve");
  }, [confirmApprove, phase, run]);

  return { phase, verb, run, approve, cancelConfirm: () => setPhase("idle") };
}

