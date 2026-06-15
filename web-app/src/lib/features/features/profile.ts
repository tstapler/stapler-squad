import type { Feature } from '../types';

export const profileFeatures = {
  'profile-delete': {
    id: 'profile-delete',
    title: 'Delete Profile',
    description: 'Deletes an existing profile by name from the session service.',
    rpcIds: ['profile:delete'],
    componentPaths: [],
    testIds: [
      'TestDeleteProfile_NotFound',
      'TestDeleteProfile_EmptyName',
      'TestDeleteProfile_Success',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'profile-upsert': {
    id: 'profile-upsert',
    title: 'Upsert Profile',
    description: 'Creates or updates a profile in the session service.',
    rpcIds: ['profile:upsert'],
    componentPaths: [],
    testIds: [
      'TestUpsertProfile_EmptyName',
      'TestUpsertProfile_NilProfile',
      'TestUpsertProfile_CreatesProfile',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
