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
    // Propagate the isolated test data directory the same way, so specs that need to seed a
    // config file (e.g. launcher-presets.json) directly on disk — rather than through an RPC —
    // know where to write it. getGlobalTestServer() itself is a live singleton scoped to this
    // global-setup process and is NOT reachable from worker processes.
    process.env.TEST_SERVER_TESTDIR = getGlobalTestServer().getTestDir();
    console.log(`Test server data dir exported: ${process.env.TEST_SERVER_TESTDIR}`);

    // Note: remote-workspaces.spec.ts's SSH target (tests/e2e/sshd) is deliberately NOT started
    // here. A single global sshd shared across every project in this run (chromium AND
    // chromium-dom both execute this same spec file against the SAME test server) would mean
    // only the FIRST project to dial it ever sees an unknown host key -- every later
    // project/retry sees an ALREADY-trusted host key (KnownHostsStore is keyed by host:port,
    // server-side, for the whole run), skipping the TOFU dialog the test is meant to exercise.
    // remote-workspaces.spec.ts starts its own dedicated sshd in a file-scoped beforeAll/afterAll
    // instead, so every project's run of that file gets a fresh port + fresh host key.

    // Rewrite storageState fixture files with the actual server origin so
    // Playwright applies localStorage to the correct origin regardless of
    // which dynamic port findFreePort() assigned. These files are gitignored
    // (regenerated every run, see .gitignore's tests/e2e/fixtures/*-theme.json
    // entry), so git checks out no tracked files here and the directory itself
    // doesn't exist on a fresh checkout — create it before writing into it.
    const fixturesDir = path.join(__dirname, 'fixtures');
    fs.mkdirSync(fixturesDir, { recursive: true });
    const themeFixtures: Record<string, string> = {
      'matrix-theme.json': 'matrix',
      'cyberpunk77-theme.json': 'cyberpunk77',
      'wh40k-theme.json': 'wh40k',
      'clean-theme.json': 'clean',
    };
    const origin = process.env.TEST_SERVER_URL!;
    // fixtures/*-theme.json are gitignored (rewritten every run), so the
    // directory itself doesn't exist on a fresh checkout — ensure it exists
    // before writing into it.
    fs.mkdirSync(fixturesDir, { recursive: true });
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
