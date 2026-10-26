"use client";

import { createClient } from "@connectrpc/connect";
import { SessionService } from "@/gen/session/v1/session_pb";
import { getConnectTransport, getKeepaliveConnectTransport } from "@/lib/api/transport";

/**
 * Process-wide browser log shipping: one refcounted console patch feeding one
 * LogBuffer, so N mounted terminals produce one entry per console call and one
 * LogClientEvents per flush instead of N.
 */

export interface BrowserLogEntry {
  level: "log" | "warn" | "error" | "debug";
  message: string;
  timestamp: string;
  url: string;
  userAgent: string;
  sessionId?: string;
}

const MAX_BUFFER = 50;
const MAX_MSG_LEN = 200;
const FLUSH_INTERVAL_MS = 5_000;

type ConsoleLevel = BrowserLogEntry["level"];
const LEVELS: ConsoleLevel[] = ["log", "warn", "error", "debug"];

const buffer: BrowserLogEntry[] = [];
let flushTimer: ReturnType<typeof setTimeout> | null = null;
let enqueuing = false;
let focusedSessionId: string | undefined;
let debugConsumers = 0;

function truncate(s: string, max: number): string {
  return s.length <= max ? s : s.slice(0, max) + "…";
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

/** The tag follows the last session to claim focus. */
export function setFocusedSessionId(sessionId: string | undefined): void {
  if (sessionId !== undefined) focusedSessionId = sessionId;
}

/** console.debug streams only while at least one consumer asked for it. */
export function setDebugConsumer(active: boolean): () => void {
  if (!active) return () => {};
  debugConsumers += 1;
  let released = false;
  return () => {
    if (released) return;
    released = true;
    debugConsumers -= 1;
  };
}

function flush(keepalive: boolean): void {
  if (flushTimer) {
    clearTimeout(flushTimer);
    flushTimer = null;
  }
  if (buffer.length === 0) return;
  const entries = buffer.splice(0).map((e) => ({
    level: e.level,
    message: e.message,
    timestamp: e.timestamp,
    url: e.url,
    userAgent: e.userAgent,
    sessionId: e.sessionId ?? "",
  }));
  try {
    const transport = keepalive ? getKeepaliveConnectTransport() : getConnectTransport();
    // Fire-and-forget and never logs: buffered entries must ship even if the
    // mount that scheduled them is gone. abort-signal-exempt
    createClient(SessionService, transport)
      .logClientEvents({ entries })
      .catch(() => {});
  } catch {
    // Dropping a log batch is acceptable; recursing into console is not.
  }
}

/** Test/diagnostic hook: flush whatever is buffered now. */
export function flushLogBuffer(keepalive = false): void {
  flush(keepalive);
}

export function enqueueLog(level: ConsoleLevel, args: unknown[]): void {
  if (level === "debug" && debugConsumers === 0) return;
  if (enqueuing) return;
  enqueuing = true;
  try {
    buffer.push({
      level,
      message: truncate(argsToMessage(args), MAX_MSG_LEN),
      timestamp: new Date().toISOString(),
      url: window.location.href,
      userAgent: navigator.userAgent,
      sessionId: focusedSessionId,
    });
    if (buffer.length >= MAX_BUFFER) {
      flush(false);
    } else if (!flushTimer) {
      flushTimer = setTimeout(() => flush(false), FLUSH_INTERVAL_MS);
    }
  } finally {
    enqueuing = false;
  }
}

interface Installed {
  originals: Record<ConsoleLevel, (...a: unknown[]) => void>;
  patched: Record<ConsoleLevel, (...a: unknown[]) => void>;
  prevOnError: OnErrorEventHandler;
  onError: OnErrorEventHandler;
  onUnhandled: (e: PromiseRejectionEvent) => void;
  onPageHide: () => void;
  onVisibility: () => void;
}

let installed: Installed | null = null;
let refCount = 0;

function install(): Installed {
  const originals = {} as Installed["originals"];
  const patched = {} as Installed["patched"];
  for (const level of LEVELS) {
    originals[level] = console[level].bind(console);
    patched[level] = (...a: unknown[]) => {
      originals[level](...a);
      enqueueLog(level, a);
    };
    console[level] = patched[level];
  }

  const prevOnError = window.onerror;
  const onError: OnErrorEventHandler = (msg, src, line, col, err) => {
    originals.error("[onerror]", msg, src, line, col, err?.stack);
    enqueueLog("error", [String(msg), `${src}:${line}:${col}`, err?.stack ?? ""]);
    return prevOnError?.(msg, src, line, col, err) ?? false;
  };
  window.onerror = onError;

  const onUnhandled = (e: PromiseRejectionEvent): void => {
    originals.error("[unhandledrejection]", e.reason);
    enqueueLog("error", ["UnhandledRejection", String(e.reason)]);
  };
  window.addEventListener("unhandledrejection", onUnhandled);

  const onPageHide = (): void => flush(true);
  const onVisibility = (): void => {
    if (document.visibilityState === "hidden") flush(true);
  };
  window.addEventListener("pagehide", onPageHide);
  document.addEventListener("visibilitychange", onVisibility);

  return { originals, patched, prevOnError, onError, onUnhandled, onPageHide, onVisibility };
}

function uninstall(state: Installed): void {
  // Restore only what is still ours; a later patch by someone else is left alone.
  for (const level of LEVELS) {
    if (console[level] === state.patched[level]) console[level] = state.originals[level];
  }
  if (window.onerror === state.onError) window.onerror = state.prevOnError;
  window.removeEventListener("unhandledrejection", state.onUnhandled);
  window.removeEventListener("pagehide", state.onPageHide);
  document.removeEventListener("visibilitychange", state.onVisibility);
}

/** Installs the shared console patch on first acquire; the returned release unwinds it on last release. */
export function acquireConsolePatch(): () => void {
  if (typeof window === "undefined") return () => {};
  if (refCount === 0) installed = install();
  refCount += 1;
  let released = false;
  return () => {
    if (released) return;
    released = true;
    refCount -= 1;
    if (refCount === 0 && installed) {
      uninstall(installed);
      installed = null;
      if (flushTimer) {
        clearTimeout(flushTimer);
        flushTimer = null;
      }
      buffer.length = 0;
    }
  };
}

export function resetLogBufferForTest(): void {
  if (installed) uninstall(installed);
  installed = null;
  refCount = 0;
  buffer.length = 0;
  if (flushTimer) clearTimeout(flushTimer);
  flushTimer = null;
  enqueuing = false;
  focusedSessionId = undefined;
  debugConsumers = 0;
}
