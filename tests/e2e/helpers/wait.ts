import { SessionClient } from "./session-client";

/**
 * Polls GetSession until a freshly created session leaves
 * SESSION_STATUS_CREATING, for specs that abort WatchSessions (so there's no
 * live update to pick up CREATING -> ACTIVE) and need a settled session
 * before navigating. Shared by create-pull-request.spec.ts and
 * session-actions-steer-focus.spec.ts.
 */
export async function waitUntilSettled(client: SessionClient, sessionId: string, timeoutMs = 10000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const session = await client.getSession(sessionId);
    if (session.status !== "SESSION_STATUS_CREATING") return session;
    await new Promise((r) => setTimeout(r, 250));
  }
  throw new Error(`Session ${sessionId} still SESSION_STATUS_CREATING after ${timeoutMs}ms`);
}
