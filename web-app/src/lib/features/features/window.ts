import type { Feature } from '../types';

export const windowFeatures = {
  'window-focus': {
    id: 'window-focus',
    title: 'Focus Window',
    description: 'Brings the application window to the foreground on the host platform.',
    rpcIds: ['window:focus'],
    componentPaths: [],
    testIds: [
      'TestFocusWindow_EmptyID',
      'TestFocusWindow_NonDarwinPlatform',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
