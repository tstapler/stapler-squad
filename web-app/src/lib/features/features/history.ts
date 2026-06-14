import type { Feature } from '../types';

export const historyFeatures = {
  'history-get-detail': {
    id: 'history-get-detail',
    title: 'Get Claude History Detail',
    description: 'Retrieves the full detail of a single Claude conversation history entry by ID.',
    rpcIds: ['history:get-detail'],
    componentPaths: [],
    testIds: ['TestGetHistoryDetail_EmptyID'],
    status: 'stable',
    since: '1.0.0',
  },
  'history-get-messages': {
    id: 'history-get-messages',
    title: 'Get Claude History Messages',
    description: 'Retrieves the individual messages from a Claude conversation history entry.',
    rpcIds: ['history:get-messages'],
    componentPaths: [],
    testIds: ['TestGetHistoryMessages_EmptyID'],
    status: 'stable',
    since: '1.0.0',
  },
  'history-list': {
    id: 'history-list',
    title: 'List Claude History',
    description: 'Lists all available Claude conversation history entries.',
    rpcIds: ['history:list'],
    componentPaths: [],
    testIds: ['TestListClaudeHistory_EmptyDir'],
    status: 'stable',
    since: '1.0.0',
  },
  'history-search': {
    id: 'history-search',
    title: 'Search Claude History',
    description: 'Searches Claude conversation history entries by query string.',
    rpcIds: ['history:search'],
    componentPaths: [],
    testIds: ['TestSearchHistory_EmptyQuery'],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
