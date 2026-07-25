import type { Feature } from '../types';

export const checkpointFeatures = {
  'checkpoint-create': {
    id: 'checkpoint-create',
    title: 'Create Checkpoint',
    description: 'Creates a named checkpoint snapshot for a running session.',
    rpcIds: ['checkpoint:create'],
    componentPaths: [],
    testIds: [
      'TestCreateCheckpoint_MissingSessionID',
      'TestCreateCheckpoint_MissingLabel',
      'TestCreateCheckpoint_SessionNotFound',
      'TestCreateCheckpoint_SessionNotStarted',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'checkpoint-list': {
    id: 'checkpoint-list',
    title: 'List Checkpoints',
    description: 'Lists all checkpoints associated with a given session.',
    rpcIds: ['checkpoint:list'],
    componentPaths: [],
    testIds: [
      'TestListCheckpoints_MissingSessionID',
      'TestListCheckpoints_SessionNotFound',
      'TestListCheckpoints_ReturnsExistingCheckpoints',
      'TestListCheckpoints_EmptyWhenNoCheckpoints',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
