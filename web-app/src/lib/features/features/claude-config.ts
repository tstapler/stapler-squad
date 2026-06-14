import type { Feature } from '../types';

export const claudeConfigFeatures = {
  'claude-config-get': {
    id: 'claude-config-get',
    title: 'Get Claude Config',
    description: 'Retrieves the Claude configuration for a given context.',
    rpcIds: ['claude-config:get'],
    componentPaths: [],
    testIds: [
      'TestGetClaudeConfig_ReturnsConfig',
      'TestGetClaudeConfig_NotFound',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'claude-config-list': {
    id: 'claude-config-list',
    title: 'List Claude Configs',
    description: 'Lists all available Claude configurations.',
    rpcIds: ['claude-config:list'],
    componentPaths: [],
    testIds: [
      'TestListClaudeConfigs_ReturnsList',
      'TestListClaudeConfigs_EmptyDir',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'claude-config-update': {
    id: 'claude-config-update',
    title: 'Update Claude Config',
    description: 'Updates the Claude configuration for a given context.',
    rpcIds: ['claude-config:update'],
    componentPaths: [],
    testIds: [
      'TestUpdateClaudeConfig_NoOp',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
