import type { Feature } from '../types';

export const reviewQueueFeatures = {
  'review-queue-list': {
    id: 'review-queue-list',
    title: 'Review Queue',
    description: 'Lists and streams the review queue of sessions needing attention.',
    rpcIds: ['review-queue:list', 'review-queue:watch'],
    componentPaths: ['components/review-queue/ReviewQueuePanel.tsx'],
    testIds: [
      'Review Queue Smoke Tests > review queue page loads successfully',
      'Smoke Tests > review queue page loads successfully',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'review-queue-acknowledge': {
    id: 'review-queue-acknowledge',
    title: 'Acknowledge Review Item',
    description: 'Acknowledges (skips) an item in the review queue.',
    rpcIds: ['review-queue:acknowledge'],
    componentPaths: ['components/review-queue/ReviewQueuePanel.tsx'],
    testIds: [
      'Review Queue Acknowledge Flow — UI Contract > review-queue-loaded sentinel is present after page renders',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'review-queue-get': {
    id: 'review-queue-get',
    title: 'Get Review Queue',
    description: 'Fetches the current review queue of sessions requiring user attention.',
    rpcIds: ['review-queue:get'],
    componentPaths: [],
    testIds: [
      'TestGetReviewQueue_ReturnsEmpty',
      'TestGetReviewQueue_WithItems',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
