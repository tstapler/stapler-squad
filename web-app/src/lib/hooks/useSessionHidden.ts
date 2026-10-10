"use client";

import { useContext, useEffect, useState } from "react";
import { ReactReduxContext } from "react-redux";
import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { selectAllSessions } from "@/lib/store/sessionsSlice";
import type { RootState } from "@/lib/store/store";

// A session never becomes visible after it was created hidden, so both answers are cached.
const hiddenBySessionId = new Map<string, boolean>();
const inFlight = new Map<string, Promise<boolean | undefined>>();

/** Test seam: forget cached answers. */
export function resetSessionHiddenCache(): void {
  hiddenBySessionId.clear();
  inFlight.clear();
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
      return res.session.hidden;
    })
    .catch(() => undefined)
    .finally(() => inFlight.delete(sessionId));
  inFlight.set(sessionId, request);
  return request;
}

/**
 * Whether the session behind a notification is hidden (a Background session). The live
 * list never carries hidden sessions, so an id absent from it is asked of the server once.
 * `undefined` while unknown, for a deleted session, and outside a Redux Provider. Toasts
 * render above the session-service context, so this reads the store directly.
 */
export function useSessionHidden(sessionId: string | undefined): boolean | undefined {
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
      setHidden(live.hidden);
      return;
    }
    let cancelled = false;
    void lookupViaRpc(sessionId).then((answer) => {
      if (!cancelled) setHidden(answer);
    });
    return () => {
      cancelled = true;
    };
  }, [sessionId, redux]);

  return hidden;
}
