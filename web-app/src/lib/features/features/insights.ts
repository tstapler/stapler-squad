import type { Feature } from '../types';

export const insightsFeatures = {
  'watch-insights': {
    id: 'watch-insights',
    title: 'Watch Insights',
    description: 'Streams real-time token usage and cost insight events as they are recorded.',
    rpcIds: ['WatchInsights'],
    componentPaths: [],
    testIds: [],
    status: 'experimental',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
