import type { Feature } from '../types';

export const terminalFeatures = {
  'terminal-render': {
    id: 'terminal-render',
    title: 'Terminal Rendering',
    description: 'Streams terminal output to the browser with RAF batching.',
    rpcIds: ['session:stream-terminal'],
    componentPaths: ['components/terminal/Terminal.tsx'],
    testIds: [
      'Terminal Flickering Fix > should maintain 60fps with incremental rendering',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
