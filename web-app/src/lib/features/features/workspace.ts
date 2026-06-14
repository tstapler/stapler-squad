import type { Feature } from '../types';

export const workspaceFeatures = {
  'workspace-get-info': {
    id: 'workspace-get-info',
    title: 'Get Workspace Info',
    description: 'Retrieves workspace metadata and configuration for a given session.',
    rpcIds: ['workspace:get-info'],
    componentPaths: [],
    testIds: [
      'TestGetWorkspaceInfo_EmptyID',
      'TestGetWorkspaceInfo_SessionNotFound',
      'TestGetWorkspaceInfo_NoSession',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'workspace-list-targets': {
    id: 'workspace-list-targets',
    title: 'List Workspace Targets',
    description: 'Lists available workspace targets that a session can switch to.',
    rpcIds: ['workspace:list-targets'],
    componentPaths: [],
    testIds: [
      'TestListWorkspaceTargets_EmptyID',
      'TestListWorkspaceTargets_SessionNotFound',
      'TestListWorkspaceTargets_NoConfig',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'workspace-switch': {
    id: 'workspace-switch',
    title: 'Switch Workspace',
    description: 'Switches a session to a different workspace target.',
    rpcIds: ['workspace:switch'],
    componentPaths: [],
    testIds: [
      'TestWorkspaceService_SwitchWorkspace_MissingID',
      'TestWorkspaceService_SwitchWorkspace_MissingTarget',
      'TestWorkspaceService_ConcurrentSwitchReturnsUnavailable',
      'TestWorkspaceService_SwitchGuardCleansUpOnCompletion',
      'TestWorkspaceService_SwitchGuardIsPerSession',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
