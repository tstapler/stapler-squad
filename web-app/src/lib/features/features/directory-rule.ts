import type { Feature } from '../types';

export const directoryRuleFeatures = {
  'directory-rule-delete': {
    id: 'directory-rule-delete',
    title: 'Delete Directory Rule',
    description: 'Deletes a directory rule by path, removing any associated session configuration for that directory.',
    rpcIds: ['directory-rule:delete'],
    componentPaths: [],
    testIds: [
      'TestDeleteDirectoryRule_NotFound',
      'TestDeleteDirectoryRule_EmptyPath',
      'TestDeleteDirectoryRule_Success',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'directory-rule-upsert': {
    id: 'directory-rule-upsert',
    title: 'Upsert Directory Rule',
    description: 'Creates or updates a directory rule, associating session configuration with a given directory path.',
    rpcIds: ['directory-rule:upsert'],
    componentPaths: [],
    testIds: [
      'TestUpsertDirectoryRule_EmptyPath',
      'TestUpsertDirectoryRule_NilRule',
      'TestUpsertDirectoryRule_ValidPath',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
