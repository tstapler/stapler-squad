/**
 * Coalesces concurrent `request()` calls into at most one in-flight fetch
 * plus at most one queued rerun, so callers never trigger two concurrent
 * fetches for the same resource. A fetch that's been superseded by a newer
 * request before it starts is discarded (its fetcher never runs); its
 * `onResult` is correspondingly never invoked. Every caller's own returned
 * promise settles (resolve or reject) exactly once, including callers whose
 * request was coalesced away — this is what lets a caller's own
 * `finally { setLoading(false) }` always run.
 *
 * Never dispatch the return value of `request()` — it resolves to
 * `Promise<void>`. All side effects happen inside `onResult`, invoked by the
 * coordinator itself, only for the fetcher whose result actually wins.
 *
 * Modeled after herdr-web's `createSnapshotRefreshController`, adapted to a
 * strictly serialized (never >1 fetch in flight) design and a discriminated
 * `CoordinatorState<T>` in place of two independent booleans.
 */

type Fetcher<T> = () => Promise<T>;
type ResultHandler<T> = (result: T) => void;

export interface RequestOptions {
  /**
   * Marks this request's fetcher as one that must not be silently dropped
   * (e.g. a stream-reconnect flush gated on `streamGenerationRef`, whose RPC
   * must actually fire — see useSessionService.ts's watch-stream call sites).
   * A guarded pending fetcher is never overwritten by a later *non-guarded*
   * request: the later caller's own promise still settles once the guarded
   * fetch resolves, but its `onResult` is not invoked for it. Guarded vs.
   * guarded is still last-caller-wins — this option only protects a guarded
   * fetcher from an unguarded one, not from another guarded one.
   */
  guarded?: boolean;
}

interface Waiter {
  resolve: () => void;
  reject: (err: unknown) => void;
}

interface PendingRequest<T> {
  fetcher: Fetcher<T>;
  onResult: ResultHandler<T>;
  guarded: boolean;
  waiters: Waiter[];
}

type CoordinatorState<T> =
  | { kind: "idle" }
  | { kind: "inFlight" }
  | { kind: "inFlightWithPending"; pending: PendingRequest<T> };

export class RefreshCoordinator<T> {
  private state: CoordinatorState<T> = { kind: "idle" };
  // Per-fetch-start counter; see the myGeneration check in run() for why.
  private generation = 0;

  request(fetcher: Fetcher<T>, onResult: ResultHandler<T>, opts: RequestOptions = {}): Promise<void> {
    const guarded = opts.guarded ?? false;

    if (this.state.kind === "idle") {
      return this.run(fetcher, onResult);
    }

    const existingPending = this.state.kind === "inFlightWithPending" ? this.state.pending : undefined;

    // Blocker 2 fix: never let a non-guarded caller overwrite an
    // already-queued guarded fetcher — the guarded RPC must still fire.
    const pending: PendingRequest<T> =
      existingPending && existingPending.guarded && !guarded
        ? existingPending
        : { fetcher, onResult, guarded, waiters: existingPending?.waiters ?? [] };

    const promise = new Promise<void>((resolve, reject) => {
      pending.waiters.push({ resolve, reject });
    });
    this.state = { kind: "inFlightWithPending", pending };
    return promise;
  }

  private async run(fetcher: Fetcher<T>, onResult: ResultHandler<T>): Promise<void> {
    const myGeneration = ++this.generation;
    this.state = { kind: "inFlight" };

    try {
      const result = await fetcher();
      // Defensive: under the strictly serialized design (at most 1 fetch in
      // flight, see request()'s pending overwrite) a superseded fetcher is
      // never even invoked, so this can't currently mismatch — retained as
      // a guard against a future refactor breaking that invariant.
      if (myGeneration === this.generation) {
        onResult(result);
      }
    } finally {
      // No await between reading/clearing state and kicking off the queued
      // rerun (if any) — this runs synchronously so no other request() call
      // can observe a torn state between "current fetch done" and "next
      // fetch started."
      this.drain();
    }
  }

  private drain(): void {
    const pending = this.state.kind === "inFlightWithPending" ? this.state.pending : undefined;
    this.state = { kind: "idle" };
    if (!pending) return;

    this.run(pending.fetcher, pending.onResult).then(
      () => pending.waiters.forEach((w) => w.resolve()),
      (err) => pending.waiters.forEach((w) => w.reject(err))
    );
  }
}
