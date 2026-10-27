"use client";

import { useEffect } from "react";
import {
  acquireConsolePatch,
  setDebugConsumer,
  releaseFocusedSessionId,
  setFocusedSessionId,
  type BrowserLogEntry,
} from "@/lib/telemetry/logBuffer";

export type { BrowserLogEntry };

export interface UseBrowserLogStreamOptions {
  /**
   * Whether verbose console.debug output streams to the server. console.log,
   * console.warn, console.error, window.onerror, and unhandledrejection
   * always stream regardless of this flag (info-and-up, matching the
   * server's own log-level convention) — this only gates the noisiest,
   * per-frame debug tracing (e.g. per-WebSocket-message logs).
   */
  enabled: boolean;
  /** Session ID to tag entries; the most recently set one wins across mounts. */
  sessionId?: string;
}

/** Thin subscriber over the shared, refcounted console patch and LogBuffer in lib/telemetry/logBuffer. */
export function useBrowserLogStream(options: UseBrowserLogStreamOptions): void {
  const { enabled, sessionId } = options;

  useEffect(() => acquireConsolePatch(), []);
  useEffect(() => setDebugConsumer(enabled), [enabled]);
  useEffect(() => {
    setFocusedSessionId(sessionId);
    return () => {
      if (sessionId !== undefined) releaseFocusedSessionId(sessionId);
    };
  }, [sessionId]);
}
