import { FullConfig } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';
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

    // Rewrite storageState fixture files with the actual server origin so
    // Playwright applies localStorage to the correct origin regardless of
    // which dynamic port findFreePort() assigned.
    const fixturesDir = path.join(__dirname, 'fixtures');
    // The theme fixture files themselves are gitignored (rewritten every run with the
    // dynamic test-server origin, see below) -- git doesn't track empty directories, so a
    // fresh checkout (e.g. CI) has no fixtures/ directory at all until this creates it.
    fs.mkdirSync(fixturesDir, { recursive: true });
    const themeFixtures: Record<string, string> = {
      'matrix-theme.json': 'matrix',
      'cyberpunk77-theme.json': 'cyberpunk77',
      'wh40k-theme.json': 'wh40k',
      'clean-theme.json': 'clean',
    };
    const origin = process.env.TEST_SERVER_URL!;
    for (const [filename, themeName] of Object.entries(themeFixtures)) {
      const fixture = { origins: [{ origin, localStorage: [{ name: 'stapler-theme', value: themeName }] }] };
      fs.writeFileSync(path.join(fixturesDir, filename), JSON.stringify(fixture));
    }
  } catch (error) {
    console.error('Failed to start test server:', error);
    throw error;
  }
}

export default globalSetup;
