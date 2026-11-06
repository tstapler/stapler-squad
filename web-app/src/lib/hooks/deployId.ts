/**
 * Compares a watch-stream event's deploy_id with the last one seen on that stream. A change means
 * the server process restarted (seq reset), so the caller should drop its afterSeq cursor and resync.
 * An empty id (older server) is ignored; the caller's seq-backwards-jump heuristic still covers it.
 */
export function noteDeployId(lastSeen: { current: string }, incoming: string, stream: string): boolean {
  if (!incoming) return false;
  const previous = lastSeen.current;
  lastSeen.current = incoming;
  if (!previous || previous === incoming) return false;
  console.warn(`[deploy] stream=${stream} server redeployed ${previous} -> ${incoming}`);
  return true;
}
