import type { Feature } from '../types';

export const defaultsFeatures = {
  'defaults-get': {
    id: 'defaults-get',
    title: 'Get Session Defaults',
    description: 'Retrieves the current session defaults configuration.',
    rpcIds: ['defaults:get'],
    componentPaths: [],
    testIds: ['TestGetSessionDefaults_ReturnsDefaults'],
    status: 'stable',
    since: '1.0.0',
  },
  'defaults-resolve': {
    id: 'defaults-resolve',
    title: 'Resolve Defaults',
    description: 'Resolves effective session defaults for a given path or context.',
    rpcIds: ['defaults:resolve'],
    componentPaths: [],
    testIds: ['TestResolveDefaults_NoPath'],
    status: 'stable',
    since: '1.0.0',
  },
  'defaults-update-global': {
    id: 'defaults-update-global',
    title: 'Update Global Defaults',
    description: 'Updates the global session defaults such as the default program.',
    rpcIds: ['defaults:update-global'],
    componentPaths: [],
    testIds: ['TestUpdateGlobalDefaults_UpdatesProgram'],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
