import type { Feature } from '../types';

export const flagsFeatures = {
  'get-feature-flags': {
    id: 'get-feature-flags',
    title: 'Get Feature Flags',
    description: 'Retrieves the current state of all feature flags for the running instance.',
    rpcIds: ['GetFeatureFlags'],
    componentPaths: [],
    testIds: [],
    status: 'experimental',
    since: '1.0.0',
  },
  'update-feature-flag': {
    id: 'update-feature-flag',
    title: 'Update Feature Flag',
    description: 'Updates the enabled state of a specific feature flag at runtime.',
    rpcIds: ['UpdateFeatureFlag'],
    componentPaths: [],
    testIds: [],
    status: 'experimental',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
