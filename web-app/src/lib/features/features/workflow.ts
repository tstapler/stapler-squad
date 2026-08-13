import type { Feature } from '../types';

export const workflowFeatures = {
  'create-workflow': {
    id: 'create-workflow',
    title: 'Create Workflow',
    description: 'Creates a new reusable workflow with a slug, command, and metadata.',
    rpcIds: ['CreateWorkflow'],
    componentPaths: [],
    testIds: [
      'TestCreateWorkflow_HappyPath',
      'TestCreateWorkflow_InvalidSlug',
      'TestCreateWorkflow_DuplicateSlug',
      'TestCreateWorkflow_MissingCommand',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'delete-workflow': {
    id: 'delete-workflow',
    title: 'Delete Workflow',
    description: 'Deletes a workflow by its ID, removing it from the available workflow catalog.',
    rpcIds: ['DeleteWorkflow'],
    componentPaths: [],
    testIds: [
      'TestDeleteWorkflow',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'list-workflows': {
    id: 'list-workflows',
    title: 'List Workflows',
    description: 'Lists all defined workflows available for execution in the current workspace.',
    rpcIds: ['ListWorkflows'],
    componentPaths: [],
    testIds: [
      'TestListWorkflows',
      'TestSessionService_DelegatesListWorkflows',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'run-workflow': {
    id: 'run-workflow',
    title: 'Run Workflow',
    description: 'Executes a workflow by ID within a session, running its configured command.',
    rpcIds: ['RunWorkflow'],
    componentPaths: [],
    testIds: [],
    status: 'experimental',
    since: '1.0.0',
  },
  'update-workflow': {
    id: 'update-workflow',
    title: 'Update Workflow',
    description: 'Updates the configuration of an existing workflow such as its command or metadata.',
    rpcIds: ['UpdateWorkflow'],
    componentPaths: [],
    testIds: [
      'TestUpdateWorkflow',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
