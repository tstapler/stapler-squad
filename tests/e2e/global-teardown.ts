import { FullConfig } from '@playwright/test';
import { stopServerPool } from './helpers/server-pool';

/**
 * Global teardown runs once after all tests
 * Stops the test servers and cleans up test data
 */
async function globalTeardown(config: FullConfig) {
  console.log('\n🧹 Cleaning up test servers...\n');

  try {
    await stopServerPool();
  } catch (error) {
    console.error('Failed to stop test servers:', error);
  }
}

export default globalTeardown;
