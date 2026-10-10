// push-sw-logic.js — pure helpers for push-sw.js (Story 5.5), kept free of SW globals so
// Jest can load them. Loaded in the worker via importScripts; exported via CommonJS for tests.
(function (root, factory) {
  var api = factory();
  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  } else {
    root.PushSwLogic = api;
  }
})(typeof self !== 'undefined' ? self : this, function () {
  var MESSAGE_TYPE = 'notification-click';

  // Only same-origin targets are routed in-app; anything else collapses to "/".
  function normalizeTargetUrl(raw, origin) {
    if (typeof raw !== 'string' || raw === '') return '/';
    try {
      var parsed = new URL(raw, origin);
      if (parsed.origin !== origin) return '/';
      return parsed.pathname + parsed.search + parsed.hash;
    } catch (e) {
      return '/';
    }
  }

  function isSameOriginClient(client, origin) {
    try {
      return new URL(client.url).origin === origin;
    } catch (e) {
      return false;
    }
  }

  // Prefer the focused window, then a visible one, then any same-origin window.
  function pickClient(clientList, origin) {
    var candidates = (clientList || []).filter(function (c) {
      return isSameOriginClient(c, origin);
    });
    if (candidates.length === 0) return null;
    return (
      candidates.find(function (c) { return c.focused; }) ||
      candidates.find(function (c) { return c.visibilityState === 'visible'; }) ||
      candidates[0]
    );
  }

  // Hands the click to an open window (focus + postMessage, no reload); opens one only
  // when none exists or focusing is refused.
  async function handleNotificationClick(opts) {
    var url = normalizeTargetUrl(opts.url, opts.origin);
    var clientList = await opts.matchAll();
    var client = pickClient(clientList, opts.origin);
    if (client) {
      try {
        client.postMessage({ type: MESSAGE_TYPE, url: url });
        if (typeof client.focus === 'function') await client.focus();
        return { handedOff: true, url: url };
      } catch (e) {
        // fall through to openWindow
      }
    }
    if (opts.openWindow) {
      await opts.openWindow(url);
      return { handedOff: false, url: url };
    }
    return { handedOff: false, url: url };
  }

  return {
    MESSAGE_TYPE: MESSAGE_TYPE,
    normalizeTargetUrl: normalizeTargetUrl,
    pickClient: pickClient,
    handleNotificationClick: handleNotificationClick,
  };
});
