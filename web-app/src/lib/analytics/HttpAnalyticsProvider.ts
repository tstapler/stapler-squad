import type { AnalyticsEvent, AnalyticsProvider, AnalyticsProviderMetadata } from "./types";

const BATCH_SIZE = 25;
const FLUSH_INTERVAL_MS = 2000;
const MAX_QUEUE_SIZE = 200;

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

  async flush(): Promise<void> {
    if (this.flushTimer !== undefined) {
      clearTimeout(this.flushTimer);
      this.flushTimer = undefined;
    }

    const batch = this.queue.splice(0, this.queue.length);
    if (batch.length === 0) {
      return;
    }

    const body = JSON.stringify({ events: batch });
    await fetch("/api/analytics", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body,
      keepalive: true,
    }).catch(() => {});
  }

  async initialize(): Promise<void> {
    if (typeof window !== "undefined") {
      window.addEventListener("pagehide", () => {
        const batch = this.queue.splice(0, this.queue.length);
        if (batch.length === 0) return;
        const body = JSON.stringify({ events: batch });
        navigator.sendBeacon("/api/analytics", body);
      });
    }
  }

  async onClose(): Promise<void> {
    await this.flush();
  }
}
