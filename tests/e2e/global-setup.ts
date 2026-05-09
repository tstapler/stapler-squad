import { FullConfig } from '@playwright/test';
import { startGlobalTestServer } from './helpers/test-server';

/**
 * Global setup runs once before all tests
 * Starts the test server with isolated data directory
 */
async function globalSetup(config: FullConfig) {
  console.log('\n🚀 Starting test server in isolated mode...\n');

  try {
    await startGlobalTestServer();
  } catch (error) {
    console.error('Failed to start test server:', error);
    throw error;
  }
}

export default globalSetup;
