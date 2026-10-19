import type { NotificationHistoryItem } from "@/lib/types/notification";

/** Mutable state behind the mocked `useNotifications`; set `panelState.history` then `rerender()`. */
export const panelState = {
  history: [] as NotificationHistoryItem[],
  isOpen: true,
  acknowledge: jest.fn(),
  refresh: jest.fn(),
};

export const notificationsModule = () => ({
  useNotifications: () => ({
    notificationHistory: panelState.history,
    isPanelOpen: panelState.isOpen,
    togglePanel: jest.fn(),
    markAsRead: jest.fn(),
    acknowledgeNotification: panelState.acknowledge,
    clearHistoryByIds: jest.fn(),
    showActionToast: jest.fn(),
    getUnreadCount: () => panelState.history.length,
    historyLoading: false,
    historyHasMore: false,
    historyError: null,
    historyLastUpdatedAt: 1,
    loadMoreHistory: jest.fn(),
    refreshHistory: panelState.refresh,
    quietMode: false,
    setQuietMode: jest.fn(),
  }),
});

/** jsdom has no layout: window the first 20 rows like a real viewport would. */
export const virtualizerModule = () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getVirtualItems: () => Array.from({ length: Math.min(count, 20) }, (_, index) => ({ index, key: index, start: index * 100, size: 100 })),
    getTotalSize: () => count * 100,
    measureElement: () => {},
    scrollToIndex: jest.fn(),
  }),
});
