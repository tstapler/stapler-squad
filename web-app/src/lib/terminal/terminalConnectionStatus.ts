import { useSyncExternalStore } from "react";

export type TerminalConnectionStatus = "connecting" | "connected" | "failed";

interface Entry {
  status: TerminalConnectionStatus;
  retry: () => void;
}

const entries = new Map<string, Entry>();
const listeners = new Set<() => void>();
let version = 0;

function notify(): void {
  version += 1;
  for (const l of listeners) l();
}

/** Published by `TerminalOutput`; the keep-alive "Connecting <title>..." placeholder reads it. */
export function setTerminalConnectionStatus(sessionId: string, status: TerminalConnectionStatus, retry: () => void): void {
  const prev = entries.get(sessionId);
  if (prev?.status === status && prev.retry === retry) return;
  entries.set(sessionId, { status, retry });
  notify();
}

export function clearTerminalConnectionStatus(sessionId: string): void {
  if (entries.delete(sessionId)) notify();
}

export function retryTerminalConnection(sessionId: string): void {
  entries.get(sessionId)?.retry();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** A session with no published status has no terminal mounted yet, which reads as `connecting`. */
export function useTerminalConnectionStatuses(sessionIds: readonly string[]): TerminalConnectionStatus[] {
  useSyncExternalStore(subscribe, () => version, () => version);
  return sessionIds.map((id) => entries.get(id)?.status ?? "connecting");
}
