/**
 * Frecency scoring for repository path suggestions.
 *
 * Frecency = frequency × recency decay.
 * A path that is used often AND recently scores higher than one that is only
 * frequent (but stale) or only recent (but rare).
 *
 * Algorithm: half-life decay
 *   score = frequency × 0.5^(ageMs / halfLifeMs)
 *
 * With a 7-day half-life:
 *   - 4 sessions used today       → 4.0
 *   - 4 sessions used 7 days ago  → 2.0
 *   - 8 sessions used 7 days ago  → 4.0  (ties with 4 fresh sessions)
 *   - 1 session used today        → 1.0
 */

export const FRECENCY_HALF_LIFE_MS = 7 * 24 * 60 * 60 * 1000; // 7 days

export interface PathActivity {
  /** Number of sessions that use this path. */
  frequency: number;
  /** Unix timestamp (ms) of the most recent activity across all sessions for this path. */
  mostRecentMs: number;
}

/**
 * Compute the frecency score for a single path given its activity record.
 * Higher is better. Returns 0 if mostRecentMs is 0 (no known activity).
 */
export function computeFrecencyScore(
  activity: PathActivity,
  nowMs: number,
  halfLifeMs: number = FRECENCY_HALF_LIFE_MS,
): number {
  if (activity.mostRecentMs === 0) return 0;
  const ageMs = Math.max(0, nowMs - activity.mostRecentMs);
  return activity.frequency * Math.pow(0.5, ageMs / halfLifeMs);
}

export interface SessionForFrecency {
  path?: string;
  /** All relevant activity timestamps for this session (ms). Zeros are ignored. */
  timestampsMs: number[];
}

/**
 * Rank repository paths by frecency score (highest first).
 *
 * @param sessions - flat list of sessions to aggregate
 * @param nowMs    - current time in ms (injectable for testing; defaults to Date.now())
 * @param halfLifeMs - decay half-life in ms (injectable for testing)
 */
export function rankPathsByFrecency(
  sessions: SessionForFrecency[],
  nowMs: number = Date.now(),
  halfLifeMs: number = FRECENCY_HALF_LIFE_MS,
): string[] {
  const activityMap = new Map<string, PathActivity>();

  for (const session of sessions) {
    if (!session.path) continue;
    const existing = activityMap.get(session.path) ?? { frequency: 0, mostRecentMs: 0 };
    existing.frequency += 1;
    for (const ts of session.timestampsMs) {
      if (ts > 0 && ts > existing.mostRecentMs) {
        existing.mostRecentMs = ts;
      }
    }
    activityMap.set(session.path, existing);
  }

  return Array.from(activityMap.entries())
    .map(([path, activity]) => ({ path, score: computeFrecencyScore(activity, nowMs, halfLifeMs) }))
    .sort((a, b) => b.score - a.score)
    .map(({ path }) => path);
}
