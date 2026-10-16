"use client";

import { useRef, useState, useEffect } from "react";
import { useSessionActions } from "./useSessionActions";

/**
 * Cancel/Retry lifecycle guards for an in-progress or failed session
 * creation (Epic 5.4, async-session-creation). Shared by SessionCard.tsx and
 * SessionRow.tsx, which previously each carried an identical copy of this
 * ref+state pattern (same as Omnibar.tsx's isSubmittingRef): the ref blocks a
 * second click before React re-renders with the disabled attribute, the
 * state actually disables/re-enables the button. Reset once the stream
 * carries the session past the in-flight status (Creating after a
 * successful Retry; anything other than Creating after a lost-race Cancel)
 * so a later failure/creation cycle on the SAME card/row gets a fresh guard
 * rather than staying disabled forever.
 */
export function useCreationLifecycleActions(sessionId: string, isCreating: boolean) {
  const sessionActions = useSessionActions(sessionId);

  const cancelInFlightRef = useRef(false);
  const [cancelDisabled, setCancelDisabled] = useState(false);
  const retryInFlightRef = useRef(false);
  const [retryDisabled, setRetryDisabled] = useState(false);

  useEffect(() => {
    if (!isCreating) {
      cancelInFlightRef.current = false;
      setCancelDisabled(false);
    }
  }, [isCreating]);

  useEffect(() => {
    if (isCreating) {
      retryInFlightRef.current = false;
      setRetryDisabled(false);
    }
  }, [isCreating]);

  const handleCancelCreation = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (cancelInFlightRef.current) return;
    cancelInFlightRef.current = true;
    setCancelDisabled(true);
    const result = await sessionActions.cancelCreation();
    if (!result.success) {
      // Either a lost-race FailedPrecondition or a transport error -- the
      // card/row either already got its real status from the stream (lost
      // race) or is still Creating (transport error), so re-enable Cancel.
      cancelInFlightRef.current = false;
      setCancelDisabled(false);
    }
    // On success the instance is deleted server-side; the card/row is
    // removed from the store (dispatch(removeSession) in useSessionService)
    // and this component unmounts, so no further local state update is
    // needed.
  };

  const handleRetryCreation = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (retryInFlightRef.current) return;
    retryInFlightRef.current = true;
    setRetryDisabled(true);
    const ok = await sessionActions.retryCreation();
    if (!ok) {
      // FailedPrecondition (no longer Failed) or transport error -- let the
      // user try again rather than leaving Retry disabled indefinitely.
      retryInFlightRef.current = false;
      setRetryDisabled(false);
    }
    // On success the same instance flips Failed -> Creating server-side;
    // the effect above clears the guard once that status arrives.
  };

  return {
    cancelDisabled,
    retryDisabled,
    handleCancelCreation,
    handleRetryCreation,
  };
}
