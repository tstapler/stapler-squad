"use client";

import { useContext, useEffect, useState } from "react";
import { ReactReduxContext } from "react-redux";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { selectAllSessions } from "@/lib/store/sessionsSlice";
import type { RootState } from "@/lib/store/store";
import { rememberHiddenSession } from "@/lib/utils/hiddenSessionRegistry";

// A session never becomes visible after it was created hidden, so both answers are cached.
const hiddenBySessionId = new Map<string, boolean>();
const inFlight = new Map<string, Promise<boolean | undefined>>();
// A deleted session stays deleted; only NotFound is remembered, so a network error retries.
const missingSessionIds = new Set<string>();

/** Test seam: forget cached answers. */
export function resetSessionHiddenCache(): void {
  hiddenBySessionId.clear();
  inFlight.clear();
  missingSessionIds.clear();
}

function lookupViaRpc(sessionId: string): Promise<boolean | undefined> {
  const pending = inFlight.get(sessionId);
  if (pending) return pending;
  const client = createClient(SessionService, getConnectTransport());
  // One lookup is shared by every toast of the session, so no single caller owns it.
  // abort-signal-exempt
  const lookup = client.getSession({ id: sessionId });
  const request = lookup
    .then((res) => {
      if (!res.session) return undefined;
      hiddenBySessionId.set(sessionId, res.session.hidden);
      if (res.session.hidden) rememberHiddenSession({ id: sessionId, title: res.session.title });
      return res.session.hidden;
    })
    .catch((err: { code?: unknown } | null) => {
      if (err?.code === 5 || err?.code === "not_found") missingSessionIds.add(sessionId);
      return undefined;
    })
    .finally(() => inFlight.delete(sessionId));
  inFlight.set(sessionId, request);
  return request;
}

/**
 * Whether the session behind a notification is hidden (a Background session). The live
 * list never carries hidden sessions, so an id absent from it is asked of the server once.
 * `undefined` while unknown, for a deleted session, and outside a Redux Provider. Toasts
 * render above the session-service context, so this reads the store directly.
 * With `lookup` false the server is never asked (a closed tray keeps its rows mounted);
 * cached and live-list answers still apply.
 */
export function useSessionHidden(sessionId: string | undefined, lookup = true): boolean | undefined {
  const redux = useContext(ReactReduxContext);
  const [hidden, setHidden] = useState<boolean | undefined>(() =>
    sessionId ? hiddenBySessionId.get(sessionId) : undefined,
  );

  useEffect(() => {
    if (!sessionId || !redux) return;
    const cached = hiddenBySessionId.get(sessionId);
    if (cached !== undefined) {
      setHidden(cached);
      return;
    }
    const live = selectAllSessions(redux.store.getState() as RootState).find((s) => s.id === sessionId);
    if (live) {
      hiddenBySessionId.set(sessionId, live.hidden);
      if (live.hidden) rememberHiddenSession({ id: sessionId, title: live.title });
      setHidden(live.hidden);
      return;
    }
    if (!lookup || missingSessionIds.has(sessionId)) return;
    let cancelled = false;
    void lookupViaRpc(sessionId).then((answer) => {
      if (!cancelled) setHidden(answer);
    });
    return () => {
      cancelled = true;
    };
  }, [sessionId, redux, lookup]);

  return hidden;
}
