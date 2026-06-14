import type { Feature } from '../types';

export const sessionFeatures = {
  'session-create': {
    id: 'session-create',
    title: 'Create Session',
    description: 'Creates a new AI agent session in a directory, worktree, or one-off mode.',
    rpcIds: ['session:create'],
    componentPaths: [
      'components/sessions/Omnibar.tsx',
      'components/sessions/OmnibarCreationPanel.tsx',
    ],
    testIds: [
      'Session Lifecycle > e2e:session-create - Session create UI is accessible',
      'Session Create Directory > creates a session in a directory',
      'One-Off Session > creates a one-off session',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'session-list': {
    id: 'session-list',
    title: 'Session List',
    description: 'Lists and streams updates for all sessions.',
    rpcIds: ['session:list', 'session:watch'],
    componentPaths: ['components/sessions/SessionList.tsx'],
    testIds: ['Smoke Tests > home page loads successfully'],
    status: 'stable',
    since: '1.0.0',
  },
  'session-delete': {
    id: 'session-delete',
    title: 'Delete Session',
    description: 'Deletes a session and its associated worktree.',
    rpcIds: ['session:delete'],
    componentPaths: ['components/sessions/SessionList.tsx'],
    testIds: [
      'Session Lifecycle > e2e:session-delete - Session management page loads',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
