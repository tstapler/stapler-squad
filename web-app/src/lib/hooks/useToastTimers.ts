"use client";

import { useEffect, useRef } from "react";

/**
 * The single owner of every toast timer (auto-close, minimize, pinned collapse,
 * exit animation, undo windows). Components never call setTimeout for toast
 * lifetimes: they register here so one place can pause, resume and cancel them.
 *
 * Pausing has two scopes, both preserving the remaining time:
 *  - per toast id, by reason ("hover", "focus", "swipe"): the timer runs only
 *    while no reason is held;
 *  - global, by source ("tray-open"): holds every timer.
 */
export type ToastTimerKind = "close" | "minimize" | "collapse" | "exit" | "expire" | "undo-window";

export interface ToastTimerRegistry {
  /** Replaces any timer with the same id and kind. */
  register(id: string, kind: ToastTimerKind, ms: number, fire: () => void): void;
  cancel(id: string, kind?: ToastTimerKind): void;
  cancelAll(): void;
  pause(id: string, reason: string): void;
  resume(id: string, reason: string): void;
  hold(source: string): void;
  release(source: string): void;
  has(id: string, kind: ToastTimerKind): boolean;
  pendingCount(): number;
}

interface Entry {
  id: string;
  kind: ToastTimerKind;
  remaining: number;
  startedAt: number;
  handle: ReturnType<typeof setTimeout> | null;
  fire: () => void;
}

export function createToastTimerRegistry(): ToastTimerRegistry {
  const entries = new Map<string, Entry>();
  const idReasons = new Map<string, Set<string>>();
  const holds = new Set<string>();

  const keyOf = (id: string, kind: ToastTimerKind) => `${id}\u0000${kind}`;
  const isHeld = (id: string) => holds.size > 0 || (idReasons.get(id)?.size ?? 0) > 0;

  const stop = (entry: Entry) => {
    if (entry.handle === null) return;
    clearTimeout(entry.handle);
    entry.handle = null;
    entry.remaining = Math.max(0, entry.remaining - (Date.now() - entry.startedAt));
  };

  const start = (entry: Entry) => {
    if (entry.handle !== null) return;
    entry.startedAt = Date.now();
    entry.handle = setTimeout(() => {
      entry.handle = null;
      entries.delete(keyOf(entry.id, entry.kind));
      entry.fire();
    }, entry.remaining);
  };

  const sync = (entry: Entry) => (isHeld(entry.id) ? stop(entry) : start(entry));
  const syncId = (id: string) => entries.forEach((e) => e.id === id && sync(e));

  return {
    register(id, kind, ms, fire) {
      const key = keyOf(id, kind);
      const previous = entries.get(key);
      if (previous) stop(previous);
      const entry: Entry = { id, kind, remaining: ms, startedAt: 0, handle: null, fire };
      entries.set(key, entry);
      sync(entry);
    },
    cancel(id, kind) {
      entries.forEach((entry, key) => {
        if (entry.id !== id || (kind !== undefined && entry.kind !== kind)) return;
        stop(entry);
        entries.delete(key);
      });
      if (kind === undefined) idReasons.delete(id);
    },
    cancelAll() {
      entries.forEach(stop);
      entries.clear();
      idReasons.clear();
    },
    pause(id, reason) {
      const reasons = idReasons.get(id) ?? new Set<string>();
      reasons.add(reason);
      idReasons.set(id, reasons);
      syncId(id);
    },
    resume(id, reason) {
      const reasons = idReasons.get(id);
      if (!reasons) return;
      reasons.delete(reason);
      if (reasons.size === 0) idReasons.delete(id);
      syncId(id);
    },
    hold(source) {
      holds.add(source);
      entries.forEach(sync);
    },
    release(source) {
      holds.delete(source);
      entries.forEach(sync);
    },
    has: (id, kind) => entries.has(keyOf(id, kind)),
    pendingCount: () => entries.size,
  };
}

/** One stable registry per provider; everything it owns is cancelled on unmount. */
export function useToastTimers(): ToastTimerRegistry {
  const ref = useRef<ToastTimerRegistry | null>(null);
  if (ref.current === null) ref.current = createToastTimerRegistry();
  useEffect(() => {
    const registry = ref.current;
    return () => registry?.cancelAll();
  }, []);
  return ref.current;
}
