import type { Feature } from '../types';

export const prFeatures = {
  'pr-close': {
    id: 'pr-close',
    title: 'Close PR',
    description: 'Closes a pull request associated with a session.',
    rpcIds: ['pr:close'],
    componentPaths: [],
    testIds: [
      'TestClosePR_EmptySessionID',
      'TestClosePR_UnknownSessionID',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'pr-get-comments': {
    id: 'pr-get-comments',
    title: 'Get PR Comments',
    description: 'Retrieves all review comments for a pull request associated with a session.',
    rpcIds: ['pr:get-comments'],
    componentPaths: [],
    testIds: [
      'TestGetPRComments_EmptySessionID',
      'TestGetPRComments_UnknownSessionID',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'pr-get-info': {
    id: 'pr-get-info',
    title: 'Get PR Info',
    description: 'Retrieves metadata and status information for a pull request associated with a session.',
    rpcIds: ['pr:get-info'],
    componentPaths: [],
    testIds: [
      'TestGetPRInfo_EmptySessionID',
      'TestGetPRInfo_UnknownSessionID',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'pr-merge': {
    id: 'pr-merge',
    title: 'Merge PR',
    description: 'Merges a pull request associated with a session.',
    rpcIds: ['pr:merge'],
    componentPaths: [],
    testIds: [
      'TestMergePR_EmptySessionID',
      'TestMergePR_UnknownSessionID',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'pr-post-comment': {
    id: 'pr-post-comment',
    title: 'Post PR Comment',
    description: 'Posts a review comment on a pull request associated with a session.',
    rpcIds: ['pr:post-comment'],
    componentPaths: [],
    testIds: [
      'TestPostPRComment_EmptySessionID',
      'TestPostPRComment_EmptyComment',
      'TestPostPRComment_UnknownSessionID',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
