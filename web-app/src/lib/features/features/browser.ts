import type { Feature } from '../types';

export const browserFeatures = {
  'browser-cdp-stream': {
    id: 'browser-cdp-stream',
    title: 'CDP Stream',
    description: 'Streams Chrome DevTools Protocol events for a session\'s embedded browser.',
    rpcIds: ['browser:cdp-stream'],
    componentPaths: [],
    testIds: [
      'TestCDPStreamHandler_MissingSessionID_Returns400',
      'TestCDPStreamHandler_UnknownSessionID_Returns404',
      'TestCDPStreamHandler_CDPPortZero_Returns503',
    ],
    status: 'stable',
    since: '1.0.0',
  },
  'browser-proxy': {
    id: 'browser-proxy',
    title: 'Browser Proxy',
    description: 'Proxies VNC connections to the embedded browser for a session.',
    rpcIds: ['browser:proxy'],
    componentPaths: [],
    testIds: [
      'TestVNCProxyHandler_MissingSessionID_Returns400',
      'TestVNCProxyHandler_UnknownSessionID_Returns404',
      'TestVNCProxyHandler_VNCPortZero_Returns503',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
