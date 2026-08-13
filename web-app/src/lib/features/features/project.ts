import type { Feature } from '../types';

export const projectFeatures = {
  'project-create': {
    id: 'project-create',
    title: 'Create Project',
    description: 'Creates a new project for grouping and organizing sessions.',
    rpcIds: ['project:create'],
    componentPaths: [],
    testIds: [
      'TestCreateProject_Success',
      'TestCreateProject_EmptyName',
      'TestCreateProject_NilStorage',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'project-list': {
    id: 'project-list',
    title: 'List Projects',
    description: 'Lists all projects available in the current workspace.',
    rpcIds: ['project:list'],
    componentPaths: [],
    testIds: [
      'TestListProjects_EmptyInitially',
      'TestListProjects_AfterCreate',
      'TestListProjects_NilStorageReturnsEmpty',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'project-update': {
    id: 'project-update',
    title: 'Update Project',
    description: 'Updates the name or metadata of an existing project.',
    rpcIds: ['project:update'],
    componentPaths: [],
    testIds: [
      'TestUpdateProject_Success',
      'TestUpdateProject_NilStorage',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'project-delete': {
    id: 'project-delete',
    title: 'Delete Project',
    description: 'Deletes a project and disassociates its sessions.',
    rpcIds: ['project:delete'],
    componentPaths: [],
    testIds: [
      'TestDeleteProject_Success',
      'TestDeleteProject_EmptyID',
      'TestDeleteProject_NilStorage',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'project-assign-sessions': {
    id: 'project-assign-sessions',
    title: 'Assign Sessions To Project',
    description: 'Assigns one or more sessions to a project for organizational grouping.',
    rpcIds: ['project:assign-sessions'],
    componentPaths: [],
    testIds: [
      'TestAssignSessionsToProject_EmptyProjectID',
      'TestAssignSessionsToProject_NilStorage',
      'TestAssignSessionsToProject_EmptySessions',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
