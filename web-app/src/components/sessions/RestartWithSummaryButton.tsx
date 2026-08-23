"use client";

import { useCallback, useState } from "react";
import { useRouter } from "next/navigation";
import { ConnectError, Code } from "@connectrpc/connect";
import { useHandoffSummary, isGenerating } from "@/lib/hooks/useHandoffSummary";
import { HandoffSummaryStatus } from "@/gen/session/v1/handoff_summary_pb";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { routes } from "@/lib/routes";
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
    try {
      const session = await createSession({
        prompt: data.summaryText,
        restartFromSessionId: sessionId,
      });
      if (session) {
        router.push(routes.sessionDetail(session.id));
      }
    } catch (err) {
      setRestartErrorMessage(
        err instanceof Error ? err.message : "Failed to start a new session from this summary",
      );
    } finally {
      setRestarting(false);
    }
  }, [createSession, data, router, sessionId]);

  if (featureDisabled) {
    return null;
  }

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

  if (phase === "idle") {
    return (
      <button
        type="button"
        className={styles.button}
        onClick={handleTrigger}
        data-testid="restart-with-summary-button"
      >
        Generate restart summary
      </button>
    );
  }

  if (phase === "generating") {
    return (
      <button
        type="button"
        className={styles.button}
        disabled
        data-testid="restart-with-summary-button"
      >
        Generating summary...
      </button>
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
    </div>
  );
}
