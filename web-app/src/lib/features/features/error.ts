import type { Feature } from '../types';

export const errorFeatures = {
  'error-acknowledge': {
    id: 'error-acknowledge',
    title: 'Acknowledge Error',
    description: 'Acknowledges an error, marking it as seen and dismissing it from the active error list.',
    rpcIds: ['error:acknowledge'],
    componentPaths: [],
    testIds: [],
    status: 'experimental',
    since: '1.0.0',
  },
  'error-list': {
    id: 'error-list',
    title: 'List Errors',
    description: 'Lists all active errors reported by sessions or the server.',
    rpcIds: ['error:list'],
    componentPaths: [],
    testIds: [],
    status: 'experimental',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
