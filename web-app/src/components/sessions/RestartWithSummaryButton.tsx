"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { ConnectError, Code } from "@connectrpc/connect";
import { useHandoffSummary, isGenerating } from "@/lib/hooks/useHandoffSummary";
import { HandoffSummaryStatus } from "@/gen/session/v1/handoff_summary_pb";
import { useSessionService } from "@/lib/hooks/useSessionService";
import type { CreateSessionRequest } from "@/gen/session/v1/session_pb";
import { routes } from "@/lib/routes";
import { srOnly } from "@/components/ui/LiveRegion.css";
import * as styles from "./RestartWithSummaryButton.css";

export interface RestartWithSummaryButtonProps {
  /** The source/current session this button lives on. */
  sessionId: string;
}

type ButtonPhase = "idle" | "generating" | "ready" | "error";

/**
 * Drives the trigger -> poll -> create-session restart flow for a session's
 * handoff summary. Renders `null` when the backend reports the feature
 * disabled -- there is no dedicated feature-flag read RPC in this plan's
 * scope, so a `TriggerHandoffSummary` call that fails with
 * `Code.FailedPrecondition` is treated as the disabled signal (see Story
 * 3.2.1 task notes).
 */
export function RestartWithSummaryButton({ sessionId }: RestartWithSummaryButtonProps) {
  const { data, neverResolved, trigger } = useHandoffSummary(sessionId);
  const { createSession } = useSessionService();
  const router = useRouter();

  const [featureDisabled, setFeatureDisabled] = useState(false);
  const [triggering, setTriggering] = useState(false);
  const [triggerErrorMessage, setTriggerErrorMessage] = useState<string | null>(null);
  const [restarting, setRestarting] = useState(false);
  const [restartErrorMessage, setRestartErrorMessage] = useState<string | null>(null);
  const [liveMessage, setLiveMessage] = useState("");
  const prevPhaseRef = useRef<ButtonPhase | null>(null);

  const handleTrigger = useCallback(async () => {
    setTriggering(true);
    setTriggerErrorMessage(null);
    try {
      await trigger();
    } catch (err) {
      if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
        setFeatureDisabled(true);
      } else {
        setTriggerErrorMessage(err instanceof Error ? err.message : "Failed to generate summary");
      }
    } finally {
      setTriggering(false);
    }
  }, [trigger]);

  const handleRestart = useCallback(async () => {
    if (!data) return;
    setRestarting(true);
    setRestartErrorMessage(null);
    setLiveMessage("Starting new session…");
    const baseRequest: Partial<CreateSessionRequest> = {
      prompt: data.summaryText,
      restartFromSessionId: sessionId,
    };
    try {
      let session;
      try {
        session = await createSession(baseRequest);
      } catch (err) {
        // The source session is normally still open when this button is
        // clicked -- that's the entire point of the feature (the source is
        // degrading mid-session) -- so the backend's still-live-source guard
        // (connect.CodeFailedPrecondition; see CreateSession in
        // server/services/session_service.go) fires on essentially every
        // real click. Retry once with the explicit confirmation flag rather
        // than surfacing a dead-end error: clicking "restart" here already
        // IS the user's confirmation that the source is live.
        if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
          session = await createSession({ ...baseRequest, confirmRestartWithLiveSource: true });
        } else {
          throw err;
        }
      }
      if (session) {
        router.push(routes.sessionDetail(session.id));
      }
    } catch (err) {
      const message =
        err instanceof Error ? err.message : "Failed to start a new session from this summary";
      setRestartErrorMessage(message);
      setLiveMessage(`Couldn't start the new session: ${message}`);
    } finally {
      setRestarting(false);
    }
  }, [createSession, data, router, sessionId]);

  let phase: ButtonPhase;
  if (triggerErrorMessage !== null || (data && data.status === HandoffSummaryStatus.ERROR)) {
    phase = "error";
  } else if (triggering || (data && !neverResolved && isGenerating(data.status))) {
    phase = "generating";
  } else if (data && !neverResolved && data.status === HandoffSummaryStatus.READY) {
    phase = "ready";
  } else {
    phase = "idle";
  }

  // Announces phase transitions through the single shared aria-live region
  // below, mirroring SessionSummaryPanel.tsx's pattern (design/ux.md Surface
  // 3's Accessibility section): idle->generating and generating->ready/error
  // are driven off `phase` here; ready->creating and creating->error are
  // announced imperatively inside handleRestart above, since "creating" is
  // not one of this button's four render phases.
  useEffect(() => {
    if (prevPhaseRef.current === phase) return;
    prevPhaseRef.current = phase;
    if (phase === "generating") {
      setLiveMessage("Generating handoff summary…");
    } else if (phase === "ready") {
      setLiveMessage("Handoff summary ready.");
    } else if (phase === "error") {
      const message =
        triggerErrorMessage || data?.errorMessage || "Something went wrong while generating this summary.";
      setLiveMessage(`Couldn't generate handoff summary: ${message}`);
    }
    // triggerErrorMessage/data are read for their current-render value only
    // within the "error" branch above -- intentionally not dependencies so
    // this doesn't re-run on every poll tick (mirrors SessionSummaryPanel's
    // identical phase-transition effect).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [phase]);

  if (featureDisabled) {
    return null;
  }

  const liveRegion = (
    <div role="status" aria-live="polite" aria-atomic="true" className={srOnly}>
      {liveMessage}
    </div>
  );

  if (phase === "idle") {
    return (
      <>
        <button
          type="button"
          className={styles.button}
          onClick={handleTrigger}
          data-testid="restart-with-summary-button"
        >
          Generate restart summary
        </button>
        {liveRegion}
      </>
    );
  }

  if (phase === "generating") {
    return (
      <>
        <button
          type="button"
          className={styles.button}
          disabled
          data-testid="restart-with-summary-button"
        >
          Generating summary...
        </button>
        {liveRegion}
      </>
    );
  }

  if (phase === "error") {
    const errorMessage =
      triggerErrorMessage || data?.errorMessage || "Something went wrong while generating this summary.";
    return (
      <div className={styles.errorContainer} data-testid="restart-with-summary-error">
        <div className={styles.errorText}>{errorMessage}</div>
        <button
          type="button"
          className={styles.button}
          onClick={handleTrigger}
          data-testid="restart-with-summary-retry"
        >
          Try again
        </button>
        {liveRegion}
      </div>
    );
  }

  // phase === "ready"
  return (
    <div className={styles.container}>
      <button
        type="button"
        className={styles.button}
        onClick={handleRestart}
        disabled={restarting}
        data-testid="restart-with-summary-button"
      >
        {restarting ? "Starting session..." : "Start new session from this summary"}
      </button>
      {restartErrorMessage && (
        <div className={styles.errorText} data-testid="restart-with-summary-restart-error">
          {restartErrorMessage}
        </div>
      )}
      {liveRegion}
    </div>
  );
}
