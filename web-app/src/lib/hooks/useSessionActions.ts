"use client";

import type { UpdateSessionRequest } from "@/gen/session/v1/session_pb";
import { useSessionService } from "./useSessionService";

/**
 * Thin adapter that binds all session-mutating service calls to a specific
 * sessionId. Eliminates the need for each component to thread the id through
 * every call site, and provides a single place to add cross-cutting concerns
 * (optimistic updates, analytics, etc.) in the future.
 */
export function useSessionActions(sessionId: string) {
  const {
    pauseSession,
    resumeSession,
    resumeCrashedSession,
    deleteSession,
    renameSession,
    restartSession,
    createCheckpoint,
    updateSession,
  } = useSessionService();

  return {
    pause: () => pauseSession(sessionId),
    resume: (updates?: { title?: string; tags?: string[] }) =>
      resumeSession(sessionId, updates),
    resumeFromCrash: () => resumeCrashedSession(sessionId),
    delete: (force?: boolean) => deleteSession(sessionId, force),
    rename: (title: string) => renameSession(sessionId, title),
    restart: () => restartSession(sessionId),
    createCheckpoint: (label: string) => createCheckpoint(sessionId, label),
    updateTags: (tags: string[]) => updateSession(sessionId, { tags }),
    update: (updates: Partial<UpdateSessionRequest>) =>
      updateSession(sessionId, updates),
  };
}
