// Ships browser console warn/error, window.onerror and unhandled rejections to the server log
// (POST /api/client-logs) so UI faults can be correlated with server-side stream events.

const ENDPOINT = "/api/client-logs";
const FLUSH_INTERVAL_MS = 5000;
const MAX_BUFFER = 200; // oldest entries drop first if the server is unreachable
const MAX_BATCH = 100; // matches the server's per-request cap
const MAX_ARG_LEN = 2000;

export interface ClientLogEntry {
  level: "warn" | "error";
  message: string;
  stack?: string;
  url: string;
  timeMs: number;
}

export function formatArg(arg: unknown): string {
  if (arg instanceof Error) return arg.message;
  if (typeof arg === "string") return arg;
  try {
    return JSON.stringify(arg) ?? String(arg);
  } catch {
    return String(arg);
  }
}

export function formatArgs(args: unknown[]): string {
  return args.map(formatArg).join(" ").slice(0, MAX_ARG_LEN);
}

export interface ShipperDeps {
  windowId: () => string;
  send: (body: string) => Promise<unknown> | void;
  now?: () => number;
}

/** Installs the hooks and returns an uninstall function. Idempotent per call site only. */
export function installClientLogShipper(deps: ShipperDeps): () => void {
  const now = deps.now ?? Date.now;
  let buffer: ClientLogEntry[] = [];
  let sending = false;

  const push = (level: ClientLogEntry["level"], message: string, stack?: string) => {
    buffer.push({ level, message, stack, url: location.pathname + location.search, timeMs: now() });
    if (buffer.length > MAX_BUFFER) buffer = buffer.slice(-MAX_BUFFER);
  };

  const flush = async () => {
    if (sending || buffer.length === 0) return;
    sending = true;
    const batch = buffer.slice(0, MAX_BATCH);
    buffer = buffer.slice(batch.length);
    try {
      await deps.send(JSON.stringify({ windowId: deps.windowId(), userAgent: navigator.userAgent, entries: batch }));
    } catch {
      buffer = [...batch, ...buffer].slice(-MAX_BUFFER); // never console.* here: that would feed the shipper
    } finally {
      sending = false;
    }
  };

  const origWarn = console.warn;
  const origError = console.error;
  console.warn = (...args: unknown[]) => {
    push("warn", formatArgs(args));
    origWarn.apply(console, args);
  };
  console.error = (...args: unknown[]) => {
    const err = args.find((a): a is Error => a instanceof Error);
    push("error", formatArgs(args), err?.stack);
    origError.apply(console, args);
  };

  const onError = (e: ErrorEvent) => push("error", e.message || "window error", e.error?.stack);
  const onRejection = (e: PromiseRejectionEvent) => {
    const reason = e.reason;
    push("error", `unhandled rejection: ${formatArg(reason)}`, reason instanceof Error ? reason.stack : undefined);
  };
  const onHide = () => void flush();
  window.addEventListener("error", onError);
  window.addEventListener("unhandledrejection", onRejection);
  window.addEventListener("pagehide", onHide);
  const timer = setInterval(() => void flush(), FLUSH_INTERVAL_MS);

  return () => {
    console.warn = origWarn;
    console.error = origError;
    window.removeEventListener("error", onError);
    window.removeEventListener("unhandledrejection", onRejection);
    window.removeEventListener("pagehide", onHide);
    clearInterval(timer);
    void flush();
  };
}

export function defaultSend(body: string): Promise<Response> {
  return fetch(ENDPOINT, { method: "POST", headers: { "Content-Type": "application/json" }, body, keepalive: true });
}

export function windowIdFromUrl(): string {
  return new URLSearchParams(location.search).get("window") ?? "default";
}
