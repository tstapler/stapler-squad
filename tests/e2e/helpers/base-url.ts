/**
 * Which e2e server this process talks to.
 *
 * global-setup starts one server per Playwright worker slot and publishes their origins as a JSON
 * array in TEST_SERVER_URLS. Playwright sets TEST_PARALLEL_INDEX (stable 0..workers-1, surviving
 * worker restarts) in every worker before it loads the config and the spec files, so these
 * accessors resolve synchronously, even at module scope. Outside a worker (global-setup, listing)
 * the index is unset and the single-server env vars apply.
 */
function workerEntry(envName: string): string | undefined {
  const raw = process.env[envName];
  const index = process.env.TEST_PARALLEL_INDEX;
  if (!raw || index === undefined) return undefined;
  const list: string[] = JSON.parse(raw);
  const entry = list[Number(index)];
  if (entry === undefined) {
    throw new Error(`${envName} has ${list.length} entries but TEST_PARALLEL_INDEX=${index}; was the pool sized below the worker count?`);
  }
  return entry;
}

export function baseUrl(): string {
  return workerEntry('TEST_SERVER_URLS') || process.env.TEST_SERVER_URL || process.env.BASE_URL || 'http://localhost:8544';
}

export function testDir(): string | undefined {
  return workerEntry('TEST_SERVER_TESTDIRS') || process.env.TEST_SERVER_TESTDIR;
}

/** Slot whose server this process uses; 0 outside a worker. */
export function serverSlot(): number {
  return Number(process.env.TEST_PARALLEL_INDEX ?? 0);
}

export function tmuxSocket(): string | undefined {
  return workerEntry('TEST_SERVER_TMUX_SOCKETS');
}

/**
 * Re-point the single-server env vars at this worker's own server. Call from playwright.config.ts:
 * Playwright re-evaluates the config in every worker (TEST_PARALLEL_INDEX already set) before it
 * loads any spec, so specs that still read process.env.TEST_SERVER_URL / BASE_URL /
 * TEST_SERVER_TESTDIR at module scope get their own slot's values instead of slot 0's.
 * No-op outside a worker and with a single-server run.
 */
export function pinWorkerEnv(): void {
  if (process.env.TEST_PARALLEL_INDEX === undefined || !process.env.TEST_SERVER_URLS) return;
  process.env.TEST_SERVER_URL = baseUrl();
  process.env.BASE_URL = baseUrl();
  const dir = testDir();
  if (dir) process.env.TEST_SERVER_TESTDIR = dir;
}
