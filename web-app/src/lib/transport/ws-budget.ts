// Per-tab budget for non-pooled (Watch*) WebSockets. Chrome fails new sockets
// with "Insufficient resources" past ~256 per process, so excess opens queue
// here instead of reaching the browser.

export const MAX_WATCH_SOCKETS = 32;

export class WsBudget {
  private inUse = 0;
  private readonly waiters: Array<() => void> = [];

  constructor(private readonly limit: number = MAX_WATCH_SOCKETS) {}

  get active(): number {
    return this.inUse;
  }

  get queued(): number {
    return this.waiters.length;
  }

  /** Resolves with a one-shot release fn; rejects with `signal.reason` if aborted while queued. */
  acquire(signal?: AbortSignal): Promise<() => void> {
    if (signal?.aborted) return Promise.reject(signal.reason ?? new Error("aborted"));
    if (this.inUse < this.limit) return Promise.resolve(this.take());

    return new Promise((resolve, reject) => {
      const onAbort = () => {
        const i = this.waiters.indexOf(grant);
        if (i >= 0) this.waiters.splice(i, 1);
        reject(signal?.reason ?? new Error("aborted"));
      };
      const grant = () => {
        signal?.removeEventListener("abort", onAbort);
        resolve(this.take());
      };
      signal?.addEventListener("abort", onAbort, { once: true });
      this.waiters.push(grant);
    });
  }

  private take(): () => void {
    this.inUse++;
    let released = false;
    return () => {
      if (released) return;
      released = true;
      this.inUse--;
      this.waiters.shift()?.();
    };
  }
}

export const watchSocketBudget = new WsBudget();
