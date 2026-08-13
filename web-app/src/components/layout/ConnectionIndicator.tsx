"use client";

import React, { useRef } from "react";
import { useAppSelector } from "@/lib/store";
import { selectConnectionState, type ConnectionState } from "@/lib/store/sessionsSlice";
import { useSessionServiceContext } from "@/lib/contexts/SessionServiceContext";
import { button, dots, spinner, labels, ariaLiveRegion, tooltip, tooltipReloadLink, wrapper } from "./ConnectionIndicator.css";

const STATE_LABEL: Record<ConnectionState, string> = {
  connected: "Live",
  stale: "Reconnecting…",
  disconnected: "Reconnecting…",
};

const STATE_ANNOUNCE: Record<ConnectionState, string> = {
  connected: "Connection restored",
  stale: "Reconnecting…",
  disconnected: "Reconnecting…",
};

export function ConnectionIndicator() {
  const connectionState = useAppSelector(selectConnectionState);
  const { watchSessions, reconnectAttemptCount } = useSessionServiceContext();
  const isActionable = connectionState !== "connected";
  const isReconnecting = connectionState === "stale" || connectionState === "disconnected";

  const prevStateRef = useRef<ConnectionState>(connectionState);
  const [announcement, setAnnouncement] = React.useState<string>("");

  // Announce on state transitions
  React.useEffect(() => {
    if (prevStateRef.current !== connectionState) {
      setAnnouncement(STATE_ANNOUNCE[connectionState]);
      prevStateRef.current = connectionState;
    }
  }, [connectionState]);

  const handleClick = () => {
    if (isActionable) {
      watchSessions();
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      handleClick();
    }
  };

  const ariaLabel = isReconnecting
    ? `Reconnecting… attempt ${reconnectAttemptCount}. Click to reconnect now.`
    : "Live — session data is up to date";

  const titleText = isReconnecting
    ? `Reconnecting… attempt ${reconnectAttemptCount}`
    : "Live — session data is up to date";

  return (
    <>
      {/* Visually-hidden live region — separate from button so screen readers announce it */}
      <div
        className={ariaLiveRegion}
        aria-live="polite"
        aria-atomic="true"
      >
        {announcement}
      </div>
      <div className={wrapper}>
        <button
          className={button}
          aria-label={ariaLabel}
          title={titleText}
          onClick={isActionable ? handleClick : undefined}
          onKeyDown={isActionable ? handleKeyDown : undefined}
          disabled={!isActionable}
        >
          {isReconnecting ? (
            <span className={spinner} aria-hidden="true" />
          ) : (
            <span
              className={dots[connectionState]}
              aria-hidden="true"
            />
          )}
          <span className={labels[connectionState]}>
            {STATE_LABEL[connectionState]}
          </span>
        </button>
        {isReconnecting && (
          <div className={tooltip}>
            <a
              href="#"
              className={tooltipReloadLink}
              onClick={(e) => { e.preventDefault(); window.location.reload(); }}
            >
              Reload page (resets state)
            </a>
          </div>
        )}
      </div>
    </>
  );
}
