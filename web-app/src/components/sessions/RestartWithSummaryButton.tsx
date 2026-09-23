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
  /**
   * Result of the caller's single `useHandoffSummary(sessionId)` call.
   * HandoffSummarySection mounts up to two instances of this button (empty
   * state + row) alongside its own poll -- lifting the hook up to that one
   * shared call avoids running two independent 2s poll loops against the
   * same session (Finding 2).
   */
  handoff: ReturnType<typeof useHandoffSummary>;
}

type ButtonPhase = "idle" | "generating" | "ready" | "error";

/**
 * Maps a backend `HandoffSummary.error_stage` to a plain-language message,
 * per design/ux.md's UX acceptance criterion #3 ("every error state has a
 * plain-language message, not a raw stage string"). The raw `errorMessage`
 * (which can be a technical string like a Go error's `.Error()` text) is
 * shown separately, behind a disclosure -- never as the primary text.
 */
const STAGE_MESSAGES: Record<string, string> = {
  transcript: "Couldn't read this session's conversation history.",
  generation: "Failed while generating the handoff summary.",
  persist: "Generated the summary but couldn't save it.",
  stale: "Generation didn't complete (the server may have restarted).",
};

function friendlyStageMessage(stage: string | undefined): string {
  if (!stage) return "Something went wrong while generating this summary.";
  return STAGE_MESSAGES[stage] ?? "Something went wrong while generating this summary.";
}

/**
 * Plain-language reason for a failed `createSession` restart call, per
 * design/ux.md's "restart-session-creation failure" flow -- a gap the plan
 * left undesigned. `CodeNotFound` means `restart_from_session_id` pointed at
 * a source session that no longer exists (e.g. archived/deleted between
 * generating the summary and clicking restart); anything else collapses to
 * a generic retry prompt rather than surfacing a raw transport/RPC message.
 */
function restartFailureReason(err: unknown): string {
  if (err instanceof ConnectError && err.code === Code.NotFound) {
    return "The original session no longer exists.";
  }
  return "Something went wrong — try again.";
}

/**
 * Drives the trigger -> poll -> create-session restart flow for a session's
 * handoff summary. Renders `null` when the backend reports the feature
 * disabled -- there is no dedicated feature-flag read RPC in this plan's
 * scope, so a `TriggerHandoffSummary` call that fails with
 * `Code.FailedPrecondition` is treated as the disabled signal (see Story
 * 3.2.1 task notes).
 */
export function RestartWithSummaryButton({ sessionId, handoff }: RestartWithSummaryButtonProps) {
  const { data, neverResolved, trigger } = handoff;
  const { createSession } = useSessionService();
  const router = useRouter();

  const [featureDisabled, setFeatureDisabled] = useState(false);
  const [triggering, setTriggering] = useState(false);
  const [triggerErrorMessage, setTriggerErrorMessage] = useState<string | null>(null);
  const [restarting, setRestarting] = useState(false);
  const [restartErrorMessage, setRestartErrorMessage] = useState<string | null>(null);
  // Finding 1: the still-live-source guard (connect.CodeFailedPrecondition;
  // see resolveRestartSource in server/services/session_service.go) must be
  // a real, visible confirmation -- not silently retried -- since two live
  // CLI processes sharing the same working directory (SESSION_TYPE_DIRECTORY,
  // no worktree isolation) is a real git-corruption risk. True once the
  // first attempt is rejected; the user must click "Restart anyway" to
  // proceed.
  const [needsLiveSourceConfirm, setNeedsLiveSourceConfirm] = useState(false);
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
      const session = await createSession(baseRequest);
      if (session) {
        router.push(routes.sessionDetail(session.id));
      }
    } catch (err) {
      // The source session is normally still open when this button is
      // clicked -- that's the entire point of the feature (the source is
      // degrading mid-session) -- so the backend's still-live-source guard
      // (connect.CodeFailedPrecondition; see resolveRestartSource in
      // server/services/session_service.go) fires on essentially every real
      // click. This is NOT auto-retried: two live CLI processes sharing the
      // same working directory can corrupt git state, so proceeding needs a
      // real, visible confirmation click ("Restart anyway"), not the
      // original click reinterpreted as consent.
      if (err instanceof ConnectError && err.code === Code.FailedPrecondition) {
        setNeedsLiveSourceConfirm(true);
        setLiveMessage(
          "The source session is still running. Restarting shares its working directory, which can corrupt git state. Confirmation required.",
        );
      } else {
        const reason = restartFailureReason(err);
        setRestartErrorMessage(`Couldn't start the new session. ${reason}`);
        setLiveMessage(`Couldn't start the new session: ${reason}`);
      }
    } finally {
      setRestarting(false);
    }
  }, [createSession, data, router, sessionId]);

  // Fires only from the "Restart anyway" button below, i.e. only after the
  // user has explicitly seen and dismissed the live-source warning -- this
  // IS the confirmation the still-live-source guard exists to require.
  const handleConfirmRestart = useCallback(async () => {
    if (!data) return;
    setRestarting(true);
    setRestartErrorMessage(null);
    setLiveMessage("Starting new session…");
    try {
      const session = await createSession({
        prompt: data.summaryText,
        restartFromSessionId: sessionId,
        confirmRestartWithLiveSource: true,
      });
      if (session) {
        setNeedsLiveSourceConfirm(false);
        router.push(routes.sessionDetail(session.id));
      }
    } catch (err) {
      setNeedsLiveSourceConfirm(false);
      const reason = restartFailureReason(err);
      setRestartErrorMessage(`Couldn't start the new session. ${reason}`);
      setLiveMessage(`Couldn't start the new session: ${reason}`);
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
      const message = triggerErrorMessage || friendlyStageMessage(data?.errorStage);
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
    // triggerErrorMessage is a client-side catch (e.g. a network failure
    // calling trigger()) with no backend stage, so it's already plain text.
    // A data-driven ERROR row does have a stage -- map it to a friendly
    // message and keep the raw errorMessage behind a disclosure, never as
    // the primary text (design/ux.md UX AC #3).
    const errorMessage = triggerErrorMessage || friendlyStageMessage(data?.errorStage);
    const rawDetail = !triggerErrorMessage ? data?.errorMessage : undefined;
    return (
      <div className={styles.errorContainer} data-testid="restart-with-summary-error">
        <div className={styles.errorText} data-testid="restart-with-summary-error-message">{errorMessage}</div>
        {rawDetail && (
          <details className={styles.errorDetails}>
            <summary>Details</summary>
            <div className={styles.errorRawText}>{rawDetail}</div>
          </details>
        )}
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
  if (needsLiveSourceConfirm) {
    return (
      <div className={styles.container}>
        <div className={styles.errorContainer} data-testid="restart-with-summary-live-source-warning">
          <div className={styles.errorText}>
            The source session is still running. Restarting shares its working directory —
            concurrent writes can corrupt git state.
          </div>
          <button
            type="button"
            className={styles.button}
            onClick={handleConfirmRestart}
            disabled={restarting}
            data-testid="restart-with-summary-confirm-live-source"
          >
            {restarting ? "Starting session..." : "Restart anyway"}
          </button>
        </div>
        {restartErrorMessage && (
          <div className={styles.errorText} data-testid="restart-with-summary-restart-error">
            {restartErrorMessage}
          </div>
        )}
        {liveRegion}
      </div>
    );
  }

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
