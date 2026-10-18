export interface BulkResult {
  succeeded: string[];
  failed: string[];
}

/** Runs `action` for every id in parallel; a rejection marks that id failed without stopping the rest. */
export async function runBulk(
  ids: string[],
  action: (id: string) => Promise<unknown>
): Promise<BulkResult> {
  const settled = await Promise.allSettled(ids.map((id) => action(id)));
  const result: BulkResult = { succeeded: [], failed: [] };
  settled.forEach((s, i) => {
    (s.status === "fulfilled" ? result.succeeded : result.failed).push(ids[i]);
  });
  return result;
}

export function formatBulkFeedback(verb: string, { succeeded, failed }: BulkResult): string {
  if (failed.length === 0) {
    return `${verb} ${succeeded.length} ${succeeded.length === 1 ? "item" : "items"}`;
  }
  return `${verb} ${succeeded.length} of ${succeeded.length + failed.length} (${failed.length} failed)`;
}

/** Max concurrent RPCs for fan-outs that start LLM work or sessions. */
export const BULK_CHUNK_SIZE = 5;

interface BulkEligibilityFields {
  status: string;
  triageStatus?: string;
  skipPlanning?: boolean;
  planApproved?: boolean;
}

export function isBulkTriageEligible(item: BulkEligibilityFields): boolean {
  return (item.status === "idea" || item.status === "ready") && item.triageStatus !== "running";
}

export function isBulkAutoImplementEligible(item: BulkEligibilityFields): boolean {
  return item.status === "ready" && Boolean(item.skipPlanning || item.planApproved);
}

/** Like runBulk, but runs at most `chunkSize` actions concurrently. */
export async function runBulkChunked(
  ids: string[],
  action: (id: string) => Promise<unknown>,
  chunkSize: number = BULK_CHUNK_SIZE
): Promise<BulkResult> {
  const result: BulkResult = { succeeded: [], failed: [] };
  for (let i = 0; i < ids.length; i += chunkSize) {
    const part = await runBulk(ids.slice(i, i + chunkSize), action);
    result.succeeded.push(...part.succeeded);
    result.failed.push(...part.failed);
  }
  return result;
}

export interface SpawnBulkResult {
  started: string[];
  /** Hit the WIP cap; the server queued the item instead of starting a session. */
  queued: string[];
  failed: string[];
}

export async function runBulkSpawnChunked(
  ids: string[],
  spawn: (id: string) => Promise<{ queued: boolean } | null>,
  chunkSize: number = BULK_CHUNK_SIZE
): Promise<SpawnBulkResult> {
  const result: SpawnBulkResult = { started: [], queued: [], failed: [] };
  for (let i = 0; i < ids.length; i += chunkSize) {
    const part = ids.slice(i, i + chunkSize);
    const settled = await Promise.allSettled(part.map((id) => spawn(id)));
    settled.forEach((s, j) => {
      if (s.status === "rejected" || s.value === null) result.failed.push(part[j]);
      else (s.value.queued ? result.queued : result.started).push(part[j]);
    });
  }
  return result;
}

export function formatSpawnBulkFeedback({ started, queued, failed }: SpawnBulkResult): string {
  const parts: string[] = [];
  if (started.length > 0 || queued.length === 0) {
    parts.push(`Started ${started.length}${queued.length === 0 && failed.length === 0 ? ` autonomous session${started.length === 1 ? "" : "s"}` : ""}`);
  }
  if (queued.length > 0) parts.push(`queued ${queued.length}`);
  const text = parts.join(", ");
  const capitalized = text.charAt(0).toUpperCase() + text.slice(1);
  return failed.length > 0 ? `${capitalized} (${failed.length} failed)` : capitalized;
}

/** Triage-specific feedback: `skipped` counts selected items that were ineligible. */
export function formatTriageFeedback(result: BulkResult, skipped: number): string {
  if (result.succeeded.length === 0 && result.failed.length === 0) {
    return skipped > 0 ? `Nothing to triage (${skipped} ineligible)` : "Nothing to triage";
  }
  const base = result.failed.length === 0
    ? `Triggered triage for ${result.succeeded.length} ${result.succeeded.length === 1 ? "item" : "items"}`
    : `Triggered ${result.succeeded.length} of ${result.succeeded.length + result.failed.length} (${result.failed.length} failed)`;
  return skipped > 0 ? `${base}, ${skipped} skipped` : base;
}

/**
 * Runs `fn` unless another bulk action holds `busyRef`. The ref is checked synchronously
 * because React's busy state has not committed yet when a second tap arrives.
 * Returns undefined when skipped.
 */
export async function runGuarded<T>(
  busyRef: { current: boolean },
  setBusy: (busy: boolean) => void,
  fn: () => Promise<T>
): Promise<T | undefined> {
  if (busyRef.current) return undefined;
  busyRef.current = true;
  setBusy(true);
  try {
    return await fn();
  } finally {
    busyRef.current = false;
    setBusy(false);
  }
}
