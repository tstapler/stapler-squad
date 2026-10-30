import type { AnalyticsEvent, AnalyticsProvider, AnalyticsProviderMetadata } from "./types";

// Serialize a camelCase AnalyticsEvent into the snake_case shape the Go backend
// expects. durationMs must be an integer — performance.now() arithmetic produces
// floats which Go silently drops when decoding into *int64.
function serializeEvent(e: AnalyticsEvent): Record<string, unknown> {
  const out: Record<string, unknown> = {
    name: e.name,
    category: e.category,
  };
  if (e.durationMs !== undefined) out.duration_ms = Math.round(e.durationMs);
  if (e.sessionId !== undefined) out.session_id = e.sessionId;
  if (e.page !== undefined) out.page = e.page;
  if (e.component !== undefined) out.component = e.component;
  if (e.labels !== undefined) out.labels = e.labels;
  return out;
}

// Size trigger sits at the server's per-request cap so the 20 s window, not the trigger, sets request volume.
const BATCH_SIZE = 100;
// Events are telemetry, not interaction: a 20 s window cuts idle POSTs from one per 2-5 s to ~3/min.
const FLUSH_INTERVAL_MS = 20_000;
const MAX_QUEUE_SIZE = 200;
// The server rejects more than 100 events per request, and keepalive fetch and sendBeacon cap the body near 64 KB.
const MAX_EVENTS_PER_REQUEST = 100;
const MAX_BODY_BYTES = 60_000;

interface Chunk {
  events: AnalyticsEvent[];
  body: string;
}

/** Splits events into request chunks that stay under both the server's event cap and the beacon size cap. */
function toChunks(batch: AnalyticsEvent[]): Chunk[] {
  const chunks: Chunk[] = [];
  let events: AnalyticsEvent[] = [];
  let wire: Record<string, unknown>[] = [];
  let bytes = 0;
  const close = () => {
    if (events.length > 0) chunks.push({ events, body: JSON.stringify({ events: wire }) });
    events = [];
    wire = [];
    bytes = 0;
  };
  for (const e of batch) {
    const w = serializeEvent(e);
    const size = JSON.stringify(w).length + 1;
    if (events.length > 0 && (events.length >= MAX_EVENTS_PER_REQUEST || bytes + size > MAX_BODY_BYTES)) close();
    events.push(e);
    wire.push(w);
    bytes += size;
  }
  close();
  return chunks;
}

/** keepalive only for the unload drain: browsers share one ~64 KiB in-flight quota across keepalive requests. */
function post(body: string, keepalive: boolean): Promise<Response> {
  return fetch("/api/analytics", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body,
    ...(keepalive ? { keepalive: true } : {}),
  });
}

export class HttpAnalyticsProvider implements AnalyticsProvider {
  readonly metadata: AnalyticsProviderMetadata = { name: "HttpAnalyticsProvider" };

  private queue: AnalyticsEvent[] = [];
  private flushTimer: ReturnType<typeof setTimeout> | undefined;

  track(event: AnalyticsEvent): void {
    // Enforce bounded queue: drop oldest if at capacity before pushing
    if (this.queue.length >= MAX_QUEUE_SIZE) {
      this.queue.shift();
    }
    this.queue.push(event);

    if (this.queue.length >= BATCH_SIZE) {
      if (this.flushTimer !== undefined) {
        clearTimeout(this.flushTimer);
        this.flushTimer = undefined;
      }
      void this.flush();
    } else if (this.flushTimer === undefined) {
      this.flushTimer = setTimeout(() => {
        void this.flush();
      }, FLUSH_INTERVAL_MS);
    }
  }

  /** Puts events back at the front (they are older), keeps the queue within its cap and re-arms the timer. */
  private requeue(events: AnalyticsEvent[]): void {
    this.queue.unshift(...events);
    if (this.queue.length > MAX_QUEUE_SIZE) this.queue.splice(0, this.queue.length - MAX_QUEUE_SIZE);
    if (this.queue.length > 0 && this.flushTimer === undefined) {
      this.flushTimer = setTimeout(() => void this.flush(), FLUSH_INTERVAL_MS);
    }
  }

  /** Sends chunks one at a time; a network failure re-queues that chunk and everything after it. */
  async flush(): Promise<void> {
    if (this.flushTimer !== undefined) {
      clearTimeout(this.flushTimer);
      this.flushTimer = undefined;
    }

    const batch = this.queue.splice(0, this.queue.length);
    const chunks = toChunks(batch);
    for (let i = 0; i < chunks.length; i++) {
      try {
        await post(chunks[i].body, false);
      } catch {
        // A 429 or 5xx resolves the fetch and is dropped, as before; only a network failure is retried.
        this.requeue(chunks.slice(i).flatMap((c) => c.events));
        return;
      }
    }
  }

  private listening = false;
  private readonly onPageHide = () => this.drainOnUnload();
  private readonly onVisibilityChange = () => {
    if (document.hidden) this.drainOnUnload();
  };

  async initialize(): Promise<void> {
    if (typeof window === "undefined" || this.listening) return;
    this.listening = true;
    // pagehide alone misses a mobile tab the OS discards while hidden, so a hidden tab drains too.
    window.addEventListener("pagehide", this.onPageHide);
    document.addEventListener("visibilitychange", this.onVisibilityChange);
  }

  /**
   * Unload drain: sends as many chunks by sendBeacon as fit the ~64 KiB aggregate quota (MAX_BODY_BYTES in
   * total). A refused beacon falls back to a keepalive fetch of that chunk (re-queued if it fails); chunks past
   * the quota stay queued for the timer.
   */
  private drainOnUnload(): void {
    if (this.flushTimer !== undefined) {
      clearTimeout(this.flushTimer);
      this.flushTimer = undefined;
    }
    const chunks = toChunks(this.queue.splice(0, this.queue.length));
    let budget = MAX_BODY_BYTES;
    let i = 0;
    for (; i < chunks.length; i++) {
      const { body, events } = chunks[i];
      if (i > 0 && body.length > budget) break;
      budget -= body.length;
      const beaconed = typeof navigator.sendBeacon === "function" && navigator.sendBeacon("/api/analytics", body);
      if (!beaconed) {
        post(body, true).catch(() => this.requeue(events));
        i++;
        break;
      }
    }
    this.requeue(chunks.slice(i).flatMap((c) => c.events));
  }

  async onClose(): Promise<void> {
    if (this.listening) {
      window.removeEventListener("pagehide", this.onPageHide);
      document.removeEventListener("visibilitychange", this.onVisibilityChange);
      this.listening = false;
    }
    await this.flush();
  }
}
