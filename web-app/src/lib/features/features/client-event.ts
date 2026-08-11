import type { Feature } from '../types';

export const clientEventFeatures = {
  'client-event-log': {
    id: 'client-event-log',
    title: 'Log Client Events',
    description: 'Receives and records client-side events from the frontend for server-side processing.',
    rpcIds: ['client-event:log'],
    componentPaths: [],
    testIds: [],
    status: 'experimental',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
