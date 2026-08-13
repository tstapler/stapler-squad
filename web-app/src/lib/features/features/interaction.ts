import type { Feature } from '../types';

export const interactionFeatures = {
  'interaction-log': {
    id: 'interaction-log',
    title: 'Log User Interaction',
    description: 'Records a user interaction event associated with a session for analytics and auditing.',
    rpcIds: ['interaction:log'],
    componentPaths: [],
    testIds: [
      'TestLogInteraction_Success',
      'TestLogInteraction_NoSessionID',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
