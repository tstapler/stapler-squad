import type { Feature } from '../types';

export const debugFeatures = {
  'debug-create-snapshot': {
    id: 'debug-create-snapshot',
    title: 'Create Debug Snapshot',
    description: 'Creates a debug snapshot of the current session state for diagnostics.',
    rpcIds: ['debug:create-snapshot'],
    componentPaths: [],
    testIds: [
      'TestCreateDebugSnapshot_Succeeds',
      'TestCreateDebugSnapshot_WithNote',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
