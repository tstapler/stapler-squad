// Pure helpers for the `/?session=<id>&tab=terminal&notification=<id>` deep link (Story 5.3).

export interface SessionDeepLinkParams {
  sessionId: string | null;
  tab: string | null;
  newPane: string | null;
  notificationId: string | null;
}

export function parseSessionDeepLink(params: URLSearchParams): SessionDeepLinkParams {
  return {
    sessionId: params.get("session"),
    tab: params.get("tab"),
    newPane: params.get("newPane"),
    notificationId: params.get("notification"),
  };
}

/**
 * The live list does not carry hidden sessions, so a session id that is not in it is
 * resolved with `getSession`. The lookup waits until the list has settled, otherwise a
 * cold load would resolve against a list that is empty only because it has not arrived
 * (a populated list resolves in-list; an empty settled list must still fall through).
 */
export function shouldResolveViaGetSession(input: {
  sessionId: string | null;
  foundInList: boolean;
  listSettled: boolean;
}): boolean {
  return Boolean(input.sessionId) && !input.foundInList && input.listSettled;
}

export type SessionLookupOutcome = "found" | "not_found" | "failed";

/** Connect code 5 is NotFound; anything else (network, timeout, auth) is a retryable failure. */
export function classifyGetSessionFailure(err: unknown): SessionLookupOutcome {
  const code = (err as { code?: unknown } | null)?.code;
  return code === 5 || code === "not_found" ? "not_found" : "failed";
}
