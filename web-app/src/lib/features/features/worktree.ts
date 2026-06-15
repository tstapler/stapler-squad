import type { Feature } from '../types';

export const worktreeFeatures = {
  'worktree-list': {
    id: 'worktree-list',
    title: 'List Worktrees',
    description: 'Lists all git worktrees available for a given repository path.',
    rpcIds: ['worktree:list'],
    componentPaths: [],
    testIds: [
      'TestListWorktrees_EmptyPath',
      'TestListWorktrees_NonGitPath',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
