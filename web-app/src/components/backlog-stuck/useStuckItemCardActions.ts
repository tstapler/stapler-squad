import { useCallback, useEffect, useState, type RefObject } from "react";
import { StuckReason } from "@/gen/session/v1/backlog_pb";
import { isRemediationParked } from "./stuckReason";

export type SnoozeDuration = "1h" | "1d" | "3d";

export const SNOOZE_DURATION_MS: Record<SnoozeDuration, number> = {
  "1h": 60 * 60 * 1000,
  "1d": 24 * 60 * 60 * 1000,
  "3d": 3 * 24 * 60 * 60 * 1000,
};

/**
 * Snooze popover state (open/duration/confirm) for one StuckItem card.
 * Extracted from StuckItem.tsx — same behavior, isolated as its own concern.
 */
export function useSnoozeControl(
  itemId: string,
  reason: StuckReason,
  onSnooze: ((itemId: string, reason: StuckReason, until: Date) => Promise<boolean>) | undefined,
  containerRef: RefObject<HTMLDivElement | null>
) {
  const [snoozeOpen, setSnoozeOpen] = useState(false);
  const [snoozeDuration, setSnoozeDuration] = useState<SnoozeDuration>("1d");
  const [snoozeState, setSnoozeState] = useState<"idle" | "pending" | "error">("idle");

  // Surface 10: clicking outside the open picker closes it with no request sent.
  useEffect(() => {
    if (!snoozeOpen) return;
    const handleClickOutside = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setSnoozeOpen(false);
        setSnoozeState("idle");
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [snoozeOpen, containerRef]);

  const handleSnoozeTriggerClick = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    setSnoozeOpen((open) => !open);
    setSnoozeState("idle");
  }, []);

  const handleSnoozeCancel = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    setSnoozeOpen(false);
    setSnoozeState("idle");
  }, []);

  const handleSnoozeConfirm = useCallback(
    async (e: React.MouseEvent) => {
      e.stopPropagation();
      if (!onSnooze) return;
      setSnoozeState("pending");
      const until = new Date(Date.now() + SNOOZE_DURATION_MS[snoozeDuration]);
      const applied = await onSnooze(itemId, reason, until);
      if (applied) {
        // Success: the hook refetches and this card is removed from the
        // parent's list on the next render — nothing further to do here.
        setSnoozeOpen(false);
        setSnoozeState("idle");
      } else {
        setSnoozeState("error");
      }
    },
    [onSnooze, itemId, reason, snoozeDuration]
  );

  const handleSnoozePickerKeyDown = useCallback((e: React.KeyboardEvent) => {
    e.stopPropagation();
    if (e.key === "Escape") {
      setSnoozeOpen(false);
      setSnoozeState("idle");
    }
  }, []);

  return {
    snoozeOpen,
    snoozeDuration,
    setSnoozeDuration,
    snoozeState,
    handleSnoozeTriggerClick,
    handleSnoozeCancel,
    handleSnoozeConfirm,
    handleSnoozePickerKeyDown,
  };
}

/**
 * "Retry now" (TriggerRemediationNow) state for one StuckItem card.
 * Extracted from StuckItem.tsx — same behavior, isolated as its own concern.
 */
export function useRetryRemediation(
  itemId: string,
  reason: StuckReason,
  remediationAttempts: number,
  onTriggerRemediationNow: ((itemId: string, reason: StuckReason) => Promise<void>) | undefined
) {
  const [retryState, setRetryState] = useState<"idle" | "pending" | "error">("idle");
  const [retryErrorMessage, setRetryErrorMessage] = useState<string | null>(null);
  const isParked = isRemediationParked({ remediationAttempts });

  const handleRetryNow = useCallback(
    async (e: React.MouseEvent) => {
      e.stopPropagation();
      if (!onTriggerRemediationNow) return;
      setRetryState("pending");
      setRetryErrorMessage(null);
      try {
        await onTriggerRemediationNow(itemId, reason);
        // Success: the hook refetches; this item's remediation_attempts will
        // reflect the new attempt on the next render. No local "success"
        // state needed beyond clearing pending.
        setRetryState("idle");
      } catch (err) {
        setRetryState("error");
        setRetryErrorMessage(err instanceof Error ? err.message : "Retry failed");
      }
    },
    [onTriggerRemediationNow, itemId, reason]
  );

  return { retryState, retryErrorMessage, handleRetryNow, isParked };
}

/**
 * "Override claim" (OverrideClaimBlock) form state for one StuckItem card.
 * Extracted from StuckItem.tsx — same behavior, isolated as its own concern.
 */
export function useClaimOverrideControl(
  itemId: string,
  reason: StuckReason,
  onOverrideClaimBlock: ((itemId: string, reason: string) => Promise<void>) | undefined
) {
  const [overrideOpen, setOverrideOpen] = useState(false);
  const [overrideBusy, setOverrideBusy] = useState(false);
  const [overrideError, setOverrideError] = useState<string | null>(null);
  const canOverrideClaim = reason === StuckReason.BLOCKED_BY_CLAIM && onOverrideClaimBlock !== undefined;

  const handleOverrideConfirm = useCallback(
    async (overrideReason: string) => {
      if (!onOverrideClaimBlock) return;
      setOverrideBusy(true);
      setOverrideError(null);
      try {
        await onOverrideClaimBlock(itemId, overrideReason);
        setOverrideOpen(false);
      } catch (err) {
        setOverrideError(err instanceof Error ? err.message : "Override failed");
      } finally {
        setOverrideBusy(false);
      }
    },
    [onOverrideClaimBlock, itemId]
  );

  return {
    canOverrideClaim,
    overrideOpen,
    setOverrideOpen,
    overrideBusy,
    overrideError,
    handleOverrideConfirm,
  };
}
