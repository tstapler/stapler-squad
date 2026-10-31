"use client";

import { readWsFlag } from "./wsUnaryPolicy";
import { createTerminalSocketPool, type PooledSocketLike, type TerminalSocketPool } from "./terminal-socket-pool";

export const POOL_FLAG = "terminal_ws_pool";
export const POOL_SIZE_KEY = "ssq:terminal-pool-size";

/** User override: localStorage `ssq:terminal-pool-size` (0 disables the pool for this browser). */
export function readUserPoolSize(): number | undefined {
  try {
    const raw = typeof localStorage === "undefined" ? null : localStorage.getItem(POOL_SIZE_KEY);
    if (raw === null) return undefined;
    const n = Number.parseInt(raw, 10);
    return Number.isFinite(n) && n >= 0 ? n : undefined;
  } catch {
    return undefined;
  }
}

let shared: TerminalSocketPool | null = null;

/**
 * A pooled virtual socket for one terminal stream. Callers gate on terminalPoolEnabled();
 * with the flag off they keep opening a plain WebSocket per stream (today's behavior).
 */
export function openTerminalSocket(url: string): PooledSocketLike {
  shared ??= createTerminalSocketPool({ size: readUserPoolSize() });
  return shared.openStream(url);
}

/** True when terminal streams should go through the pool (flag on and the user has not set size 0). */
export function terminalPoolEnabled(): boolean {
  return readWsFlag(POOL_FLAG) && readUserPoolSize() !== 0;
}

/** @internal tests */
export function _resetSharedPoolForTesting(): void {
  shared?.dispose();
  shared = null;
}
