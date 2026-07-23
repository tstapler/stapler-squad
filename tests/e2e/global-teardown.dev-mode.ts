import { FullConfig } from '@playwright/test';
import { stopDevStack } from './global-setup.dev-mode';

/**
 * Global teardown for the opt-in Playwright dev-mode harness (Epic 5.1).
 * Mirrors global-teardown.ts's shape, but calls the stop() function
 * returned by startDevStack() (held via global-setup.dev-mode.ts's
 * module-level `stopDevStack` export) instead of the single-server
 * helpers/test-server.ts's stopGlobalTestServer(). stop() tears down both
 * the BackendChild and FrontendChild process groups (Epic 3.2, Task 3.2.1d)
 * and removes the dev-stack.json manifest.
 */
async function globalTeardown(config: FullConfig) {
  console.log('\n🧹 Cleaning up dev-mode DevStack...\n');

  try {
    if (stopDevStack) {
      await stopDevStack();
    }
  } catch (error) {
    console.error('Failed to stop dev-mode DevStack:', error);
  }
}

export default globalTeardown;
