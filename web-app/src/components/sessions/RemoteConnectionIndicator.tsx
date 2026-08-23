"use client";
// +feature: remote-connection-indicator

import { useEffect, useRef, useState } from "react";
import { useAppSelector } from "@/lib/store";
import { selectRemoteHealthEntry } from "@/lib/store/remotesSlice";
import { RemoteConnectionState } from "@/gen/session/v1/remote_pb";
import {
  badge,
  statusConnected,
  statusReconnecting,
  statusDisconnected,
  dots,
  spinner,
  ariaLiveRegion,
} from "./RemoteConnectionIndicator.css";

// RemoteConnectionIndicator shows a remote session's live SSH connection
// health (Story 6.2.2, requirements.md AC5) -- distinct from a session's own
// lifecycle status badge (SessionCard.tsx's getStatusText/Active-Paused-etc,
// which reflects the tmux session, not the SSH transport underneath it).
// State comes exclusively from remotesSlice (fed by RemoteHealthChangedEvent
// arriving over the WatchSessions stream useSessionService.ts already
// subscribes to) -- this component issues no RPC or fetch of its own, ever.

const STATE_LABEL: Record<RemoteConnectionState, string> = {
  [RemoteConnectionState.UNSPECIFIED]: "",
  [RemoteConnectionState.CONNECTED]: "Connected",
  [RemoteConnectionState.RECONNECTING]: "Reconnecting…",
  [RemoteConnectionState.DISCONNECTED]: "Disconnected",
};

// Announcement text for the persistent aria-live="polite" region. DISCONNECTED
// is deliberately absent here -- it's announced through the separate
// role="alert" element below instead (assertive), matching this repo's
// inlineEditError convention (SessionCard.tsx's inline-rename error span) for
// a terminal failure state that needs the user's attention, not a polite
// background update.
const POLITE_ANNOUNCE: Partial<Record<RemoteConnectionState, string>> = {
  [RemoteConnectionState.CONNECTED]: "Remote connection restored",
  [RemoteConnectionState.RECONNECTING]: "Remote reconnecting…",
};

const ALERT_ANNOUNCE = "Remote disconnected — action may be required";

const STATE_CLASS: Record<RemoteConnectionState, string> = {
  [RemoteConnectionState.UNSPECIFIED]: "",
  [RemoteConnectionState.CONNECTED]: statusConnected,
  [RemoteConnectionState.RECONNECTING]: statusReconnecting,
  [RemoteConnectionState.DISCONNECTED]: statusDisconnected,
};

const DOT_CLASS: Record<RemoteConnectionState, string | undefined> = {
  [RemoteConnectionState.UNSPECIFIED]: undefined,
  [RemoteConnectionState.CONNECTED]: dots.connected,
  [RemoteConnectionState.RECONNECTING]: dots.reconnecting,
  [RemoteConnectionState.DISCONNECTED]: dots.disconnected,
};

interface RemoteConnectionIndicatorProps {
  remoteName: string;
}

export function RemoteConnectionIndicator({ remoteName }: RemoteConnectionIndicatorProps) {
  const entry = useAppSelector(selectRemoteHealthEntry(remoteName));
  const state = entry?.state ?? RemoteConnectionState.UNSPECIFIED;

  const prevStateRef = useRef<RemoteConnectionState>(state);
  const [politeAnnouncement, setPoliteAnnouncement] = useState("");

  // Announce on state transitions -- mirrors layout/ConnectionIndicator.tsx's
  // identical prevStateRef + effect pattern for the session-stream indicator.
  useEffect(() => {
    if (prevStateRef.current !== state) {
      const text = POLITE_ANNOUNCE[state];
      if (text) setPoliteAnnouncement(text);
      prevStateRef.current = state;
    }
  }, [state]);

  // No health-change event observed yet -- render nothing rather than a
  // misleading default. Story 6.2.2's data comes from Redux/the push stream
  // only, and there is no synchronous initial value to show.
  if (state === RemoteConnectionState.UNSPECIFIED) {
    return null;
  }

  const label = STATE_LABEL[state];

  return (
    <>
      {/* Persistent aria-live="polite" region — separate from the badge so
          screen readers announce connecting/reconnecting transitions without
          needing focus on the card (requirements.md AC5). */}
      <div
        className={ariaLiveRegion}
        role="status"
        aria-live="polite"
        aria-atomic="true"
      >
        {politeAnnouncement}
      </div>
      <span
        className={`${badge} ${STATE_CLASS[state]}`}
        role="img"
        title={`Remote ${remoteName}: ${label}`}
        aria-label={`Remote connection: ${label}`}
        data-testid="remote-connection-indicator"
      >
        {state === RemoteConnectionState.RECONNECTING ? (
          <span className={spinner} aria-hidden="true" />
        ) : (
          <span className={DOT_CLASS[state]} aria-hidden="true" />
        )}
        <span>{label}</span>
      </span>
      {/* Visually hidden -- the visible badge above already shows "Disconnected";
          this element exists solely to fire an assertive (role="alert")
          screen-reader announcement, distinct from the polite region above. */}
      {state === RemoteConnectionState.DISCONNECTED && (
        <span role="alert" className={ariaLiveRegion}>{ALERT_ANNOUNCE}</span>
      )}
    </>
  );
}
