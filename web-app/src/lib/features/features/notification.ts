import type { Feature } from '../types';

export const notificationFeatures = {
  'notification-clear-history': {
    id: 'notification-clear-history',
    title: 'Clear Notification History',
    description: 'Clears the notification history for the current user.',
    rpcIds: ['notification:clear-history'],
    componentPaths: [],
    testIds: ['TestClearNotificationHistory_Success'],
    status: 'stable',
    since: '1.0.0',
  },
  'notification-get-history': {
    id: 'notification-get-history',
    title: 'Get Notification History',
    description: 'Retrieves the notification history for the current user.',
    rpcIds: ['notification:get-history'],
    componentPaths: [],
    testIds: ['TestGetNotificationHistory_ReturnsEmpty'],
    status: 'stable',
    since: '1.0.0',
  },
  'notification-mark-read': {
    id: 'notification-mark-read',
    title: 'Mark Notification Read',
    description: 'Marks one or more notifications as read by their IDs.',
    rpcIds: ['notification:mark-read'],
    componentPaths: [],
    testIds: ['TestMarkNotificationRead_EmptyIDs'],
    status: 'stable',
    since: '1.0.0',
  },
  'notification-send': {
    id: 'notification-send',
    title: 'Send Notification',
    description: 'Sends a push notification to subscribed clients and manages subscription lifecycle.',
    rpcIds: ['notification:send'],
    componentPaths: [],
    testIds: [
      'TestSendNotification410RemovesSubscription',
      'TestSendNotification404RemovesSubscription',
      'TestSendNotificationClosesBody',
      'TestSendNotification201RetainsSubscription',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
