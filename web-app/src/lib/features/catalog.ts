import type { Feature } from './types';
import { sessionFeatures } from './features/session';
import { reviewQueueFeatures } from './features/review-queue';
import { terminalFeatures } from './features/terminal';
import { unfinishedWorkFeatures } from './features/unfinished-work';

/**
 * The authoritative typed feature catalog.
 *
 * Every entry is type-checked against the Feature interface at compile time.
 * Adding a feature here is the single act of registration — no separate
 * annotation files or scanner outputs required.
 */
export const FEATURE_CATALOG = {
  ...sessionFeatures,
  ...reviewQueueFeatures,
  ...terminalFeatures,
  ...unfinishedWorkFeatures,
} as const satisfies Record<string, Feature>;

/**
 * Union of all valid feature IDs, e.g. "session-create" | "session-list" | ...
 * TypeScript will error at compile time if code references an unknown ID.
 */
export type FeatureId = keyof typeof FEATURE_CATALOG;

/**
 * Look up a feature by its typed ID.
 * Because the parameter type is FeatureId (not string), typos are caught at
 * compile time.
 */
export function getFeature(id: FeatureId): Feature {
  return FEATURE_CATALOG[id];
}
