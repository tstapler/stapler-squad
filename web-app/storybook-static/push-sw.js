// push-sw.js — Web Push delivery only.
// Cache management is handled separately by cache-sw.js.
// This service worker intentionally has no fetch handler so its update cycle
// is independent of the cache layer.

self.addEventListener('push', (event) => {
  let data = {
    title: 'Stapler Squad',
    body: 'Notification',
    icon: '/icons/icon-192.png',
    tag: 'stapler-notification',
    data: {},
    requireInteraction: false,
    renotify: false,
  };

  if (event.data) {
    try {
      const parsed = event.data.json();
      data = { ...data, ...parsed };
    } catch {
      data.body = event.data.text();
    }
  }

  // Read actions from the server payload; fall back to sensible defaults.
  const actions = Array.isArray(data.data?.actions)
    ? data.data.actions
    : [
        { action: 'open', title: 'Open' },
        { action: 'dismiss', title: 'Dismiss' },
      ];

  const options = {
    body: data.body,
    icon: data.icon || '/icons/icon-192.png',
    badge: '/icons/icon-72.png',
    tag: data.tag,
    data: data.data || {},
    vibrate: [100, 50, 100],
    requireInteraction: data.requireInteraction === true,
    renotify: data.renotify === true,
    actions,
  };

  event.waitUntil(
    self.registration.showNotification(data.title, options)
  );
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();

  // Dispatch by action value.
  switch (event.action) {
    case 'dismiss':
    case 'later':
      // Close without opening — done.
      return;

    case 'review':
    case 'open':
    default: {
      const urlToOpen = event.notification.data?.url || '/';

      event.waitUntil(
        clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clientList) => {
          for (const client of clientList) {
            if (client.url.includes(urlToOpen) && 'focus' in client) {
              return client.focus();
            }
          }
          if (clients.openWindow) {
            return clients.openWindow(urlToOpen);
          }
        })
      );
    }
  }
});

self.addEventListener('message', (event) => {
  if (event.data && event.data.type === 'SKIP_WAITING') {
    self.skipWaiting();
  }
});
