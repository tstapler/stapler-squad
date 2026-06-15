import type { Feature } from '../types';

export const databaseFeatures = {
  'database-get-current': {
    id: 'database-get-current',
    title: 'Get Current Database',
    description: 'Returns the currently active database for the session service.',
    rpcIds: ['database:get-current'],
    componentPaths: [],
    testIds: ['TestGetCurrentDatabase_ReturnsDatabase'],
    status: 'stable',
    since: '1.0.0',
  },
  'database-list': {
    id: 'database-list',
    title: 'List Databases',
    description: 'Lists all available databases known to the session service.',
    rpcIds: ['database:list'],
    componentPaths: [],
    testIds: ['TestListDatabases_ReturnsList'],
    status: 'stable',
    since: '1.0.0',
  },
  'database-merge': {
    id: 'database-merge',
    title: 'Merge Database',
    description: 'Merges a source database into the current database.',
    rpcIds: ['database:merge'],
    componentPaths: [],
    testIds: [
      'TestMergeDatabase_EmptySource',
      'TestMergeDatabase_SourceOutsideBaseDir',
      'TestMergeDatabase_SourceDBNotFound',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'database-switch': {
    id: 'database-switch',
    title: 'Switch Database',
    description: 'Switches the session service to use a different database at the given path.',
    rpcIds: ['database:switch'],
    componentPaths: [],
    testIds: [
      'TestSwitchDatabase_EmptyPath',
      'TestSwitchDatabase_PathOutsideBaseDir',
      'TestSwitchDatabase_NonExistentDir',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
