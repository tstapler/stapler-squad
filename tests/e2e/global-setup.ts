import { FullConfig } from '@playwright/test';
import { startGlobalTestServer, getGlobalTestServer } from './helpers/test-server';

/**
 * Global setup runs once before all tests
 * Starts the test server with isolated data directory
 */
async function globalSetup(config: FullConfig) {
  console.log('\n🚀 Starting test server in isolated mode...\n');

  try {
    await startGlobalTestServer();
    // Propagate dynamic URL to workers — process.env mutations here are
    // inherited by Playwright worker processes (spawned after global-setup).
    process.env.TEST_SERVER_URL = getGlobalTestServer().getBaseUrl();
    console.log(`Test server URL exported: ${process.env.TEST_SERVER_URL}`);
  } catch (error) {
    console.error('Failed to start test server:', error);
    throw error;
  }
}

export default globalSetup;
