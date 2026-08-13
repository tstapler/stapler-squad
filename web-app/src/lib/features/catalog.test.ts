import { FEATURE_CATALOG, getFeature } from './catalog';
import type { FeatureId } from './catalog';

const VALID_STATUSES = new Set(['stable', 'experimental', 'deprecated']);
const SEMVER_RE = /^\d+\.\d+\.\d+$/;
const KEBAB_RE = /^[a-z][a-z0-9]*(-[a-z0-9]+)*$/;

const entries = Object.entries(FEATURE_CATALOG);

describe('FEATURE_CATALOG', () => {
  test('catalog is non-empty', () => {
    expect(entries.length).toBeGreaterThan(0);
  });

  test.each(entries)(
    'feature "%s": stable features have non-empty testIds',
    (key, feature) => {
      if (feature.status === 'stable') {
        expect(feature.testIds.length).toBeGreaterThan(0);
      }
    }
  );

  test.each(entries)(
    'feature "%s": has non-empty rpcIds or non-empty componentPaths',
    (key, feature) => {
      const hasCoverage =
        feature.rpcIds.length > 0 || feature.componentPaths.length > 0;
      expect(hasCoverage).toBe(true);
    }
  );

  test.each(entries)(
    'feature "%s": status is a valid value',
    (key, feature) => {
      expect(VALID_STATUSES.has(feature.status)).toBe(true);
    }
  );

  test.each(entries)(
    'feature "%s": since matches semver pattern',
    (key, feature) => {
      expect(SEMVER_RE.test(feature.since)).toBe(true);
    }
  );

  test.each(entries)(
    'feature "%s": id is kebab-case',
    (key, feature) => {
      expect(KEBAB_RE.test(feature.id)).toBe(true);
    }
  );

  test.each(entries)(
    'feature "%s": id matches catalog key',
    (key, feature) => {
      expect(feature.id).toBe(key);
    }
  );
});

describe('getFeature', () => {
  test('returns the correct feature for a known id', () => {
    const feature = getFeature('session-create');
    expect(feature.id).toBe('session-create');
    expect(feature.title).toBe('Create Session');
  });

  test('returns the correct feature for every catalog entry', () => {
    for (const [key] of entries) {
      const feature = getFeature(key as FeatureId);
      expect(feature.id).toBe(key);
    }
  });
});

describe('FeatureId type coverage', () => {
  test('all known feature IDs are present', () => {
    const expectedIds = [
      'session-create',
      'session-list',
      'session-delete',
      'review-queue-list',
      'review-queue-acknowledge',
      'terminal-render',
      'unfinished-work',
    ];
    const actualIds = Object.keys(FEATURE_CATALOG);
    for (const id of expectedIds) {
      expect(actualIds).toContain(id);
    }
  });
});
