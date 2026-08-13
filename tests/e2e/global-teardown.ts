import { FullConfig } from '@playwright/test';
import { stopGlobalTestServer } from './helpers/test-server';

/**
 * Global teardown runs once after all tests
 * Stops the test server and cleans up test data
 */
async function globalTeardown(config: FullConfig) {
  console.log('\n🧹 Cleaning up test server...\n');

  try {
    await stopGlobalTestServer();
  } catch (error) {
    console.error('Failed to stop test server:', error);
  }
}

export default globalTeardown;
