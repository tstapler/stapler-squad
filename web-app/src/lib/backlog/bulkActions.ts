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
