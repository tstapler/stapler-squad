import { useCallback, useRef, useState } from "react";
import type { Session } from "@/gen/session/v1/types_pb";
import type { AnalyticsProvider } from "@/lib/analytics/types";

type ResumeSession = (id: string, updates?: { title?: string; tags?: string[] }) => Promise<Session | null>;

/**
 * "Resume session" modal flow — request/direct-resume/confirm/cancel.
 * Extracted from page.tsx's HomeContent — same behavior, isolated as its
 * own concern. `resumeTarget` is still returned so callers elsewhere in the
 * page (e.g. the Escape-key handler, j/k navigation guards) can gate on
 * whether the modal is open.
 */
export function useResumeSessionFlow(resumeSession: ResumeSession, track: AnalyticsProvider["track"]) {
  const [resumeTarget, setResumeTarget] = useState<Session | null>(null);
  const resumeTriggerRef = useRef<HTMLElement | null>(null);

  const handleResumeRequest = useCallback((session: Session) => {
    resumeTriggerRef.current = document.activeElement as HTMLElement;
    setResumeTarget(session);
  }, []);

  const handleDirectResume = useCallback(
    (session: Session) => {
      track({ name: "session_resumed", category: "user_action" });
      resumeSession(session.id, { title: session.title, tags: [...(session.tags || [])] });
    },
    [resumeSession, track]
  );

  const handleResumeConfirm = useCallback(
    async (updates: { title: string; tags: string[] }) => {
      if (!resumeTarget) return;
      try {
        track({ name: "session_resumed", category: "user_action" });
        await resumeSession(resumeTarget.id, updates);
        setResumeTarget(null);
      } catch {
        // resumeSession dispatches to Redux error state; modal stays open for retry
      }
    },
    [resumeTarget, resumeSession, track]
  );

  const handleResumeCancel = useCallback(() => {
    setResumeTarget(null);
  }, []);

  return {
    resumeTarget,
    resumeTriggerRef,
    handleResumeRequest,
    handleDirectResume,
    handleResumeConfirm,
    handleResumeCancel,
  };
}
