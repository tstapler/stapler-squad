// Ships browser console warn/error, window.onerror and unhandled rejections to the server log
// (POST /api/client-logs) so UI faults can be correlated with server-side stream events.

const ENDPOINT = "/api/client-logs";
const FLUSH_INTERVAL_MS = 5000;
const MAX_BUFFER = 200; // oldest entries drop first if the server is unreachable
const MAX_BATCH = 100; // matches the server's per-request cap
const MAX_ARG_LEN = 2000;
const MAX_STACK_LEN = 4000;
// fetch keepalive bodies are capped at ~64KB by browsers; stay well under it.
const MAX_BODY_BYTES = 48 * 1024;
const MAX_CONSECUTIVE_FAILURES = 3;

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
    return (JSON.stringify(arg) ?? String(arg)).slice(0, MAX_ARG_LEN);
  } catch {
    try {
      return String(arg).slice(0, MAX_ARG_LEN);
    } catch {
      return "[unprintable]"; // e.g. Object.create(null) or a throwing toJSON
    }
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
  let failures = 0;

  const push = (level: ClientLogEntry["level"], message: string, stack?: string) => {
    buffer.push({ level, message, stack: stack?.slice(0, MAX_STACK_LEN), url: location.pathname + location.search, timeMs: now() });
    if (buffer.length > MAX_BUFFER) buffer = buffer.slice(-MAX_BUFFER);
  };
  // Logging must never break the caller's own console.* call.
  const safePush = (level: ClientLogEntry["level"], build: () => [string, string?]) => {
    try {
      const [message, stack] = build();
      push(level, message, stack);
    } catch {
      /* drop the entry */
    }
  };

  // Takes entries until the serialized body would pass MAX_BODY_BYTES; always takes at least one.
  const takeBatch = (): { body: string; count: number } => {
    let count = Math.min(buffer.length, MAX_BATCH);
    for (;;) {
      const body = JSON.stringify({ windowId: deps.windowId(), userAgent: navigator.userAgent, entries: buffer.slice(0, count) });
      if (body.length <= MAX_BODY_BYTES || count === 1) return { body, count };
      count = Math.max(1, Math.floor(count / 2));
    }
  };

  const flush = async () => {
    if (sending || buffer.length === 0) return;
    sending = true;
    const { body, count } = takeBatch();
    const batch = buffer.slice(0, count);
    buffer = buffer.slice(count);
    try {
      await deps.send(body);
      failures = 0;
    } catch {
      // Never console.* here: that would feed the shipper. Give up on a batch that keeps failing.
      failures += 1;
      if (failures < MAX_CONSECUTIVE_FAILURES) buffer = [...batch, ...buffer].slice(-MAX_BUFFER);
      else failures = 0;
    } finally {
      sending = false;
    }
  };

  const origWarn = console.warn;
  const origError = console.error;
  console.warn = (...args: unknown[]) => {
    safePush("warn", () => [formatArgs(args)]);
    origWarn.apply(console, args);
  };
  console.error = (...args: unknown[]) => {
    safePush("error", () => [formatArgs(args), args.find((a): a is Error => a instanceof Error)?.stack]);
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

/** Rejects on network errors and 5xx (retry); a 4xx (bad or over-budget batch) is dropped, not retried. */
export async function defaultSend(body: string): Promise<void> {
  const res = await fetch(ENDPOINT, { method: "POST", headers: { "Content-Type": "application/json" }, body, keepalive: true });
  if (res.status >= 500) throw new Error(`client-logs ${res.status}`);
}

export function windowIdFromUrl(): string {
  return new URLSearchParams(location.search).get("window") ?? "default";
}
