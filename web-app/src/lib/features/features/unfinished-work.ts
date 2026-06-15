import type { Feature } from '../types';

export const unfinishedWorkFeatures = {
  'unfinished-work': {
    id: 'unfinished-work',
    title: 'Unfinished Work',
    description: 'Surfaces pending changes across git worktrees.',
    rpcIds: ['unfinished:list', 'unfinished:watch'],
    componentPaths: ['components/unfinished/UnfinishedWorkPanel.tsx'],
    testIds: ['Unfinished Work > shows unfinished work panel'],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
