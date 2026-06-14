import type { Feature } from './types';
import { sessionFeatures } from './features/session';
import { reviewQueueFeatures } from './features/review-queue';
import { terminalFeatures } from './features/terminal';
import { unfinishedWorkFeatures } from './features/unfinished-work';
import { analyticsFeatures } from './features/analytics';
import { approvalFeatures } from './features/approval';
import { backlogFeatures } from './features/backlog';
import { browserFeatures } from './features/browser';
import { checkpointFeatures } from './features/checkpoint';
import { claudeConfigFeatures } from './features/claude-config';
import { clientEventFeatures } from './features/client-event';
import { databaseFeatures } from './features/database';
import { debugFeatures } from './features/debug';
import { defaultsFeatures } from './features/defaults';
import { directoryRuleFeatures } from './features/directory-rule';
import { errorFeatures } from './features/error';
import { fileFeatures } from './features/file';
import { flagsFeatures } from './features/flags';
import { historyFeatures } from './features/history';
import { insightsFeatures } from './features/insights';
import { interactionFeatures } from './features/interaction';
import { logsFeatures } from './features/logs';
import { miscSessionFeatures } from './features/misc-session';
import { notificationFeatures } from './features/notification';
import { pathFeatures } from './features/path';
import { profileFeatures } from './features/profile';
import { projectFeatures } from './features/project';
import { prFeatures } from './features/pr';
import { rulesFeatures } from './features/rules';
import { shellFeatures } from './features/shell';
import { unfinishedFeatures } from './features/unfinished';
import { uploadFeatures } from './features/upload';
import { windowFeatures } from './features/window';
import { workflowFeatures } from './features/workflow';
import { workspaceFeatures } from './features/workspace';
import { worktreeFeatures } from './features/worktree';

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
  ...analyticsFeatures,
  ...approvalFeatures,
  ...backlogFeatures,
  ...browserFeatures,
  ...checkpointFeatures,
  ...claudeConfigFeatures,
  ...clientEventFeatures,
  ...databaseFeatures,
  ...debugFeatures,
  ...defaultsFeatures,
  ...directoryRuleFeatures,
  ...errorFeatures,
  ...fileFeatures,
  ...flagsFeatures,
  ...historyFeatures,
  ...insightsFeatures,
  ...interactionFeatures,
  ...logsFeatures,
  ...miscSessionFeatures,
  ...notificationFeatures,
  ...pathFeatures,
  ...profileFeatures,
  ...projectFeatures,
  ...prFeatures,
  ...rulesFeatures,
  ...shellFeatures,
  ...unfinishedFeatures,
  ...uploadFeatures,
  ...windowFeatures,
  ...workflowFeatures,
  ...workspaceFeatures,
  ...worktreeFeatures,
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
