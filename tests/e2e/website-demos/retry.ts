// @feature website-demos
// Bounded scene-level retry. A flaky locator timeout is retried on a fresh page; a
// wrong theme or a leak is a content defect that a second attempt cannot fix.

/** Thrown for failures that must fail the shard immediately, never retried. */
export class NoRetryError extends Error {
  readonly noRetry = true;
  constructor(message: string) {
    super(message);
    this.name = new.target.name;
  }
}

export const MAX_ATTEMPTS = 2;

export interface RetryResult<T> {
  value: T;
  /** 1 when the first attempt passed. */
  attempts: number;
}

export interface RetryOptions {
  attempts?: number;
  /** Runs between attempts (server-state check); a throw aborts the retry. */
  between?: (failedAttempt: number, error: unknown) => Promise<void>;
}

function isNoRetry(error: unknown): boolean {
  return error instanceof NoRetryError || (typeof error === 'object' && error !== null && (error as { noRetry?: boolean }).noRetry === true);
}

/** Error thrown when every attempt failed; carries the attempt count for the manifest row. */
export class RetriesExhaustedError extends Error {
  constructor(
    readonly attempts: number,
    readonly cause: unknown,
  ) {
    super(`failed after ${attempts} attempt(s): ${cause instanceof Error ? cause.message : String(cause)}`);
    this.name = 'RetriesExhaustedError';
  }
}

export async function withRetry<T>(run: (attempt: number) => Promise<T>, opts: RetryOptions = {}): Promise<RetryResult<T>> {
  const max = opts.attempts ?? MAX_ATTEMPTS;
  let last: unknown;
  for (let attempt = 1; attempt <= max; attempt++) {
    try {
      return { value: await run(attempt), attempts: attempt };
    } catch (error) {
      if (isNoRetry(error)) throw error;
      last = error;
      if (attempt < max) await opts.between?.(attempt, error);
    }
  }
  throw new RetriesExhaustedError(max, last);
}
