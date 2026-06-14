import type { Feature } from '../types';

export const logsFeatures = {
  'logs-get': {
    id: 'logs-get',
    title: 'Get Logs',
    description: 'Retrieves log output for a session by ID or title.',
    rpcIds: ['logs:get'],
    componentPaths: [],
    testIds: [
      'TestGetLogs_NoSessionID_NilPoller',
      'TestGetLogs_WithUUID_NilPoller',
      'TestGetLogs_WithUUID_MatchingInstance',
      'TestGetLogs_WithUUID_NoMatchingInstance',
      'TestGetLogs_WithTitle_FindsLogByTitle',
      'TestGetLogs_EmptySessionID',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
