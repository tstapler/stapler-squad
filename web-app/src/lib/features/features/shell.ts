import type { Feature } from '../types';

export const shellFeatures = {
  'delete-shell': {
    id: 'delete-shell',
    title: 'Delete Shell',
    description: 'Deletes a shell pane from a session, terminating its process.',
    rpcIds: ['DeleteShell'],
    componentPaths: [],
    testIds: [
      'TestDeleteShell_Success',
      'TestDeleteShell_SessionNotFound',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'list-shells': {
    id: 'list-shells',
    title: 'List Shells',
    description: 'Lists all active shell panes associated with a given session.',
    rpcIds: ['ListShells'],
    componentPaths: [],
    testIds: [
      'TestListShells_ReturnsAll',
      'TestListShells_EmptySession',
      'TestListShells_SessionNotRunning',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'restart-shell': {
    id: 'restart-shell',
    title: 'Restart Shell',
    description: 'Restarts a specific shell pane within a session.',
    rpcIds: ['RestartShell'],
    componentPaths: [],
    testIds: [
      'TestRestartShell_SessionNotFound',
      'TestRestartShell_ShellNotFound',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'spawn-shell': {
    id: 'spawn-shell',
    title: 'Spawn Shell',
    description: 'Spawns a new shell pane inside a running session.',
    rpcIds: ['SpawnShell'],
    componentPaths: [],
    testIds: [
      'TestSpawnShell_SessionNotFound',
      'TestSpawnShell_SessionNotRunning',
      'TestSpawnShell_EmptySessionId',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'stop-shell': {
    id: 'stop-shell',
    title: 'Stop Shell',
    description: 'Stops a running shell pane within a session without deleting it.',
    rpcIds: ['StopShell'],
    componentPaths: [],
    testIds: [
      'TestStopShell_Success',
      'TestStopShell_SessionNotFound',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
