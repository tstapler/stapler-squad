"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { Buffer } from "@/lib/files/bufferReducer";
import { useFilesAnalytics } from "@/lib/files/analytics/filesAnalytics";
import { useFileTransport } from "./useFileTransport";
import { pollIntervalMs, type PollMode } from "./pollMode";

const POLL_MS = 10_000;
const SLOW_POLL_MS = 100_000;
const BACKOFF_MS = 30_000;
const CONFIRM_MS = 1_000;
const BACKOFF_AFTER_FAILURES = 3;

export type TokenChange = "same" | "mtime_only" | "changed";

/** Tokens are `v1:<size>:<mtime>:<hash>`; an equal hash with a different mtime is not a content change. */
export function compareTokens(base: string, next: string): TokenChange {
  if (base === next) return "same";
  const a = base.split(":");
  const b = next.split(":");
  const hashed = a.length === 4 && b.length === 4 && a[3] !== "-" && b[3] !== "-";
  return hashed && a[3] === b[3] ? "mtime_only" : "changed";
}

export interface StaleInfo {
  path: string;
  /** Token the file has on disk now; shown in place when it moves again. */
  token: string;
  dirty: boolean;
  actor: "agent_likely" | "unknown";
}

export type StaleAction = "reload" | "review" | "dismiss";

interface Options {
  active: Buffer | null;
  /** files:viewer-v2 on and the Files tab mounted. */
  enabled: boolean;
  /** "off" (hidden window) stops all checks; "slow" (unfocused pane) polls at SLOW_POLL_MS. Defaults to "fast". */
  pollMode?: PollMode;
  sessionRunning: boolean;
  /** Adopts a token whose content is unchanged (mtime-only). */
  onAdoptToken: (path: string, token: string) => void;
}

const canCheck = () => document.visibilityState === "visible" && navigator.onLine !== false;

/**
 * Detects on-disk changes to the active buffer: on focus, on becoming visible and every 10 s.
 * A change must be seen on two checks (one second apart) before the banner shows; it never touches the buffer.
 */
export function useBufferFreshness({ active, enabled, pollMode = "fast", sessionRunning, onAdoptToken }: Options) {
  const transport = useFileTransport();
  const analytics = useFilesAnalytics({ component: "BannerStack" });
  const [stale, setStale] = useState<StaleInfo | null>(null);
  const live = useRef({ active, sessionRunning, onAdoptToken, stale, analytics });
  live.current = { active, sessionRunning, onAdoptToken, stale, analytics };
  const pending = useRef<string | null>(null);
  const dismissed = useRef<string | null>(null);

  const loaded = active?.status === "loaded" ? active : null;
  const path = loaded?.path ?? null;
  const sessionId = loaded?.sessionId ?? null;
  const token = loaded?.token ?? null;

  // A new baseline (save, reload, adopt) or another active buffer ends the current banner.
  useEffect(() => {
    pending.current = null;
    dismissed.current = null;
    setStale(null);
  }, [path, token]);

  const lastPath = useRef<string | null>(null);
  useEffect(() => {
    const pollMs = pollIntervalMs(pollMode, POLL_MS, SLOW_POLL_MS);
    if (!enabled || pollMs === null || !path || !sessionId) return;
    const abort = new AbortController();
    let inflight = false;
    let failures = 0;
    let pollTimer: ReturnType<typeof setTimeout> | undefined;
    let confirmTimer: ReturnType<typeof setTimeout> | undefined;

    const show = (change: string) => {
      const { active: cur, sessionRunning: running, analytics: a } = live.current;
      const info: StaleInfo = {
        path, token: change, dirty: cur?.status === "loaded" && cur.dirty, actor: running ? "agent_likely" : "unknown",
      };
      if (!live.current.stale) a.emit("files.stale.shown", { dirty: String(info.dirty) as "true" | "false", actor: info.actor });
      setStale(info);
    };
    const handle = (next: string) => {
      const cur = live.current.active;
      if (cur?.status !== "loaded" || cur.path !== path) return;
      const change = compareTokens(cur.token, next);
      if (change === "same") return;
      if (change === "mtime_only") {
        pending.current = null;
        live.current.onAdoptToken(path, next);
        return;
      }
      if (next === dismissed.current) return;
      if (live.current.stale) {
        if (live.current.stale.token !== next) show(next);
      } else if (pending.current === next) {
        pending.current = null;
        show(next);
      } else {
        pending.current = next;
        confirmTimer = setTimeout(() => void check(), CONFIRM_MS);
      }
    };
    const check = async () => {
      if (inflight || abort.signal.aborted || !canCheck()) return;
      inflight = true;
      try {
        const r = await transport.stat({ sessionId, path }, undefined, abort.signal);
        failures = 0;
        if (!abort.signal.aborted) handle(r.token);
      } catch {
        if (!abort.signal.aborted) failures += 1;
      } finally {
        inflight = false;
      }
    };
    const schedule = () => {
      pollTimer = setTimeout(async () => {
        await check();
        if (!abort.signal.aborted) schedule();
      }, failures >= BACKOFF_AFTER_FAILURES ? BACKOFF_MS : pollMs);
    };
    const onWake = () => void check();
    const onVisibility = () => { if (document.visibilityState === "visible") onWake(); };

    if (lastPath.current !== null && lastPath.current !== path) void check();
    lastPath.current = path;
    schedule();
    window.addEventListener("focus", onWake);
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      abort.abort();
      clearTimeout(pollTimer);
      clearTimeout(confirmTimer);
      window.removeEventListener("focus", onWake);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [enabled, pollMode, path, sessionId, transport]);

  const resolve = useCallback((action: StaleAction) => {
    const cur = live.current.stale;
    live.current.analytics.emit("files.stale.action", { action });
    if (action === "dismiss" && cur) dismissed.current = cur.token;
    setStale(null);
  }, []);

  return { stale, resolve };
}
