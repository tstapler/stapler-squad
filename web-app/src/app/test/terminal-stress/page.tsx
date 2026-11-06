'use client';
// analytics-exempt

import { lazyComponent } from '@/lib/lazy';

// Code-split so @xterm/xterm (and its CSS side-effect import) lives in its own chunk,
// loaded only in the browser -- see TerminalStressTest.tsx's header comment for why a
// static top-level import here broke the Bun build.
const TerminalStressTest = lazyComponent(
  () => import('./TerminalStressTest').then((mod) => ({ default: mod.TerminalStressTest })),
  {
    loading: () => (
      <div style={{ padding: '20px', textAlign: 'center' }}>Loading terminal...</div>
    ),
  }
);

export default function TerminalStressTestPage() {
  return <TerminalStressTest />;
}
