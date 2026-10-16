import type { Session } from "@/gen/session/v1/types_pb";

/**
 * Resolves a session reference that may be a full id, an id prefix, a tmux
 * session name, an external-session worktree path (or its basename), an
 * `_`-prefixed external id, a title, or a repo-dir basename — in that order
 * of preference. Extracted from page.tsx's HomeContent — same behavior,
 * isolated as a pure lookup so the component doesn't carry the fallback chain.
 */
export function findSessionById(sessions: Session[], sessionId: string): Session | undefined {
  let session = sessions.find((s) => s.id === sessionId);
  if (session) return session;

  session = sessions.find((s) => {
    if (s.id.startsWith(sessionId)) return true;
    if (s.externalMetadata?.tmuxSessionName === sessionId) return true;
    if (sessionId.includes("/") && s.existingDir && s.existingDir.includes(sessionId)) return true;
    if (s.existingDir && s.existingDir.endsWith(`/${sessionId}`)) return true;
    return false;
  });

  if (!session && sessionId.includes("_")) {
    const withoutPrefix = sessionId.split("_").slice(1).join("_");
    session = sessions.find((s) => s.id === withoutPrefix || s.title === withoutPrefix);
  }

  if (!session) {
    const searchLower = sessionId.toLowerCase();
    session = sessions.find((s) => {
      if (s.title.toLowerCase() === searchLower) return true;
      const pathBasename = s.existingDir?.split("/").pop()?.toLowerCase();
      if (pathBasename === searchLower) return true;
      return false;
    });
  }

  if (!session) {
    console.warn(`[findSessionById] No session found for ID: ${sessionId}`, {
      availableSessions: sessions.map((s) => ({ id: s.id, title: s.title, path: s.existingDir })),
    });
  }

  return session;
}
