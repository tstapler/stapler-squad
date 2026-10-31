import { FullConfig } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';
import { startServerPool } from './helpers/server-pool';
import { themeFixturePath, THEME_FIXTURES } from './helpers/theme-fixtures';

/**
 * Global setup runs once before all tests.
 * Starts one isolated test server per worker slot (config.workers) with its own data directory.
 */
async function globalSetup(config: FullConfig) {
  const size = Math.max(1, config.workers);
  console.log(`\n🚀 Starting ${size} test server(s) in isolated mode...\n`);

  try {
    const { servers } = await startServerPool(size);
    const urls = servers.map((s) => s.getBaseUrl());
    const dirs = servers.map((s) => s.getTestDir());
    const sockets = servers.map((s) => s.tmuxSocket());
    // process.env mutations here are inherited by Playwright worker processes (spawned after
    // global-setup). Workers pick their entry by TEST_PARALLEL_INDEX (see helpers/base-url.ts).
    process.env.TEST_SERVER_URLS = JSON.stringify(urls);
    process.env.TEST_SERVER_TESTDIRS = JSON.stringify(dirs);
    process.env.TEST_SERVER_TMUX_SOCKETS = JSON.stringify(sockets);
    // Single-server consumers (and this process) see slot 0.
    process.env.TEST_SERVER_URL = urls[0];
    console.log(`Test server URLs exported: ${urls.join(', ')}`);
    // 5 specs (alias.spec.ts, alias-settings.spec.ts, omnibar-backlog-intent-confirm.spec.ts,
    // omnibar-backlog-compose.spec.ts, workflow-cron-schedule.spec.ts) read process.env.BASE_URL
    // directly instead of TEST_SERVER_URL, with a hardcoded :8544 fallback -- found running the
    // Task 0.1c baseline capture, where it produced systematic ERR_CONNECTION_REFUSED failures on
    // every run. Only set when the spec didn't already request a specific server by hand.
    process.env.BASE_URL ??= process.env.TEST_SERVER_URL;
    // Propagate the isolated test data directory the same way, so specs that need to seed a
    // config file (e.g. launcher-presets.json) directly on disk — rather than through an RPC —
    // know where to write it. The TestServer objects are live singletons scoped to this
    // global-setup process and are NOT reachable from worker processes.
    process.env.TEST_SERVER_TESTDIR = dirs[0];
    console.log(`Test server data dirs exported: ${dirs.join(', ')}`);

    // Note: remote-workspaces.spec.ts's SSH target (tests/e2e/sshd) is deliberately NOT started
    // here. A single global sshd shared across every project in this run (chromium AND
    // chromium-dom both execute this same spec file against the SAME test server) would mean
    // only the FIRST project to dial it ever sees an unknown host key -- every later
    // project/retry sees an ALREADY-trusted host key (KnownHostsStore is keyed by host:port,
    // server-side, for the whole run), skipping the TOFU dialog the test is meant to exercise.
    // remote-workspaces.spec.ts starts its own dedicated sshd in a file-scoped beforeAll/afterAll
    // instead, so every project's run of that file gets a fresh port + fresh host key.

    // Rewrite storageState fixture files with each server's origin so Playwright applies
    // localStorage to the correct origin regardless of which dynamic port findFreePort() assigned.
    // These files are gitignored (regenerated every run) and the directory doesn't exist on a
    // fresh checkout, so create it before writing into it.
    fs.mkdirSync(path.join(__dirname, 'fixtures'), { recursive: true });
    urls.forEach((origin, slot) => {
      for (const [name, themeName] of Object.entries(THEME_FIXTURES)) {
        const fixture = { origins: [{ origin, localStorage: [{ name: 'stapler-theme', value: themeName }] }] };
        fs.writeFileSync(themeFixturePath(name, slot), JSON.stringify(fixture));
      }
    });
  } catch (error) {
    console.error('Failed to start test server:', error);
    throw error;
  }
}

export default globalSetup;
