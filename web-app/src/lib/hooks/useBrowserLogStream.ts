"use client";

import { useEffect, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getApiBaseUrl } from "@/lib/config";

export interface BrowserLogEntry {
  level: "log" | "warn" | "error" | "debug";
  message: string;
  timestamp: string;
  url: string;
  userAgent: string;
  sessionId?: string;
}

export interface UseBrowserLogStreamOptions {
  /** Whether log streaming is active. Hook is a no-op when false. */
  enabled: boolean;
  /** Optional session ID to tag entries. */
  sessionId?: string;
  /** Override base URL for testing. Defaults to getApiBaseUrl(). */
  baseUrl?: string;
}

const MAX_BUFFER = 50;
const MAX_MSG_LEN = 200;
const FLUSH_INTERVAL_MS = 5_000;

// NOTE: If TerminalOutput is ever rendered more than once simultaneously
// (session pool scenario), multiple instances of this hook will each patch
// console.*. Since each install captures the then-current console.log as
// its originals.log, the interceptors chain automatically. Cleanup unwinds
// in reverse React effect order. A module-level reference counter should be
// added if multi-mount becomes a real scenario.

export function useBrowserLogStream(options: UseBrowserLogStreamOptions): void {
  const enabledRef = useRef(options.enabled);
  const sessionRef = useRef(options.sessionId);
  const baseUrlRef = useRef(options.baseUrl);

  // Keep refs current without triggering re-effects
  useEffect(() => {
    enabledRef.current = options.enabled;
  }, [options.enabled]);
  useEffect(() => {
    sessionRef.current = options.sessionId;
  }, [options.sessionId]);
  useEffect(() => {
    baseUrlRef.current = options.baseUrl;
  }, [options.baseUrl]);

  useEffect(() => {
    // SSR guard
    if (typeof window === "undefined") return;
    if (!options.enabled) return;

    const apiBase = baseUrlRef.current ?? getApiBaseUrl();
    const transport = createConnectTransport({ baseUrl: apiBase });
    const client = createClient(SessionService, transport);

    const buffer: BrowserLogEntry[] = [];
    let flushTimer: ReturnType<typeof setTimeout> | null = null;
    let intercepting = false;

    // Capture originals BEFORE patching
    const originals = {
      log: console.log.bind(console),
      warn: console.warn.bind(console),
      error: console.error.bind(console),
      debug: console.debug.bind(console),
    };

    function truncate(s: string, max: number): string {
      if (s.length <= max) return s;
      return s.slice(0, max) + "…";
    }

    function argsToMessage(args: unknown[]): string {
      try {
        return args
          .slice(0, 5)
          .map((a) => (typeof a === "object" ? JSON.stringify(a) : String(a)))
          .join(" ");
      } catch {
        return args.slice(0, 5).map(String).join(" ");
      }
    }

    function enqueue(level: BrowserLogEntry["level"], args: unknown[]): void {
      if (!enabledRef.current) return;
      if (intercepting) return; // reentrancy guard
      intercepting = true;
      try {
        const entry: BrowserLogEntry = {
          level,
          message: truncate(argsToMessage(args), MAX_MSG_LEN),
          timestamp: new Date().toISOString(),
          url: window.location.href,
          userAgent: navigator.userAgent,
          sessionId: sessionRef.current,
        };
        buffer.push(entry);
        if (buffer.length >= MAX_BUFFER) {
          flush();
        } else if (!flushTimer) {
          flushTimer = setTimeout(flush, FLUSH_INTERVAL_MS);
        }
      } finally {
        intercepting = false;
      }
    }

    function flush(): void {
      if (flushTimer) {
        clearTimeout(flushTimer);
        flushTimer = null;
      }
      if (buffer.length === 0) return;
      const entries = buffer.splice(0);
      // Use ConnectRPC client — fire-and-forget; never recurse into console
      client
        .logClientEvents({
          entries: entries.map((e) => ({
            level: e.level,
            message: e.message,
            timestamp: e.timestamp,
            url: e.url,
            userAgent: e.userAgent,
            sessionId: e.sessionId ?? "",
          })),
        })
        .catch(() => {});
    }

    // Install interceptors
    console.log = (...a) => {
      originals.log(...a);
      enqueue("log", a);
    };
    console.warn = (...a) => {
      originals.warn(...a);
      enqueue("warn", a);
    };
    console.error = (...a) => {
      originals.error(...a);
      enqueue("error", a);
    };
    console.debug = (...a) => {
      originals.debug(...a);
      enqueue("debug", a);
    };

    // window.onerror
    const prevOnError = window.onerror;
    window.onerror = (msg, src, line, col, err) => {
      originals.error("[onerror]", msg, src, line, col, err?.stack);
      enqueue("error", [String(msg), `${src}:${line}:${col}`, err?.stack ?? ""]);
      return prevOnError?.(msg, src, line, col, err) ?? false;
    };

    // unhandledrejection
    function onUnhandled(e: PromiseRejectionEvent): void {
      originals.error("[unhandledrejection]", e.reason);
      enqueue("error", ["UnhandledRejection", String(e.reason)]);
    }
    window.addEventListener("unhandledrejection", onUnhandled);

    // Page-unload beacon — flush remaining entries synchronously
    function onBeforeUnload(): void {
      if (buffer.length === 0) return;
      const entries = buffer.splice(0);
      // Use sendBeacon with JSON blob for keepalive semantics on unload.
      // ConnectRPC binary POST is not suitable here; fall back to plain JSON
      // matched by the same route (the server accepts both via HTTP/1.1 fallback).
      const body = JSON.stringify({
        entries: entries.map((e) => ({
          level: e.level,
          message: e.message,
          timestamp: e.timestamp,
          url: e.url,
          user_agent: e.userAgent,
          session_id: e.sessionId ?? "",
        })),
      });
      const blob = new Blob([body], { type: "application/json" });
      if (!navigator.sendBeacon(apiBase + "/session.v1.SessionService/LogClientEvents", blob)) {
        fetch(apiBase + "/session.v1.SessionService/LogClientEvents", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body,
          keepalive: true,
        }).catch(() => {});
      }
    }
    window.addEventListener("beforeunload", onBeforeUnload);

    // Start periodic flush
    flushTimer = setTimeout(flush, FLUSH_INTERVAL_MS);

    // Cleanup: restore originals, cancel timer, clear buffer
    return () => {
      console.log = originals.log;
      console.warn = originals.warn;
      console.error = originals.error;
      console.debug = originals.debug;
      window.onerror = prevOnError;
      window.removeEventListener("unhandledrejection", onUnhandled);
      window.removeEventListener("beforeunload", onBeforeUnload);
      if (flushTimer) {
        clearTimeout(flushTimer);
        flushTimer = null;
      }
      // Flush any buffered entries before tearing down so logs within the
      // batching window are not silently lost when the toggle is turned off.
      if (buffer.length > 0) {
        flush();
      }
    };
  }, [options.enabled]); // re-run only when enabled changes
}
