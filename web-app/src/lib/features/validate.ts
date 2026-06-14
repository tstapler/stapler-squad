/**
 * Runtime validator for the typed feature catalog.
 *
 * Run via: npm run validate-catalog
 * Exits with code 1 if any feature has empty testIds, missing rpcIds+componentPaths,
 * invalid status, or invalid since/id format.
 */
import { FEATURE_CATALOG } from './catalog';
import type { Feature } from './types';

const VALID_STATUSES = new Set(['stable', 'experimental', 'deprecated']);
const SEMVER_RE = /^\d+\.\d+\.\d+$/;
const KEBAB_RE = /^[a-z][a-z0-9]*(-[a-z0-9]+)*$/;

let errors: string[] = [];

// Cast to Feature to avoid literal-type narrowing on readonly tuple lengths
for (const [key, feature] of Object.entries(FEATURE_CATALOG) as [string, Feature][]) {
  const prefix = `Feature "${key}"`;

  if (!KEBAB_RE.test(feature.id)) {
    errors.push(`${prefix}: id "${feature.id}" is not kebab-case`);
  }

  if (feature.id !== key) {
    errors.push(`${prefix}: id "${feature.id}" does not match catalog key "${key}"`);
  }

  if (!feature.title) {
    errors.push(`${prefix}: title is empty`);
  }

  if (!VALID_STATUSES.has(feature.status)) {
    errors.push(`${prefix}: status "${feature.status}" is not one of stable|experimental|deprecated`);
  }

  if (!SEMVER_RE.test(feature.since)) {
    errors.push(`${prefix}: since "${feature.since}" does not match semver pattern (e.g. "1.0.0")`);
  }

  if (feature.status === 'stable' && feature.testIds.length === 0) {
    errors.push(`${prefix}: stable feature has no testIds — add tests or set status to "experimental"`);
  }

  if (feature.rpcIds.length === 0 && feature.componentPaths.length === 0) {
    errors.push(`${prefix}: both rpcIds and componentPaths are empty — at least one is required`);
  }
}

if (errors.length > 0) {
  console.error('\nFeature catalog validation FAILED:\n');
  for (const e of errors) {
    console.error(`  ✗ ${e}`);
  }
  console.error(`\n${errors.length} error(s) found.`);
  process.exit(1);
}

const count = Object.keys(FEATURE_CATALOG).length;
console.log(`Feature catalog validation passed — ${count} feature(s) OK.`);
