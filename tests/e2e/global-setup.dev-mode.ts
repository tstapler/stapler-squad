import { FullConfig } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';
import { startDevStack } from '../../scripts/dev-stack/launch';

/**
 * Global setup for the opt-in Playwright dev-mode harness (Epic 5.1,
 * plan.md Story 5.1.1). Unlike global-setup.ts (which starts a single,
 * statically-ported backend via helpers/test-server.ts), this starts a full
 * DevStack — a real Go backend AND a real `next dev` frontend on
 * independently OS-assigned ports — via scripts/dev-stack/launch.ts's
 * startDevStack(), the same launcher Epic 3.2's manual `make dev-stack` CLI
 * uses. This is what lets backlog.dev-mode.spec.ts (Epic 5.2) prove the
 * cross-origin CORS / API-base-URL / hook-URL wiring (Phases 1-2) actually
 * works end-to-end, not just in isolation.
 *
 * seedData is passed explicitly as `true` — the manual CLI's default
 * (`false`, Task 3.2.1i's fast path) is deliberately NOT relied upon here,
 * since the e2e specs (e.g. backlog.dev-mode.spec.ts) need seeded demo data
 * to assert against.
 *
 * Instance name: a fixed 'e2e-dev-mode' string. This harness's config
 * (playwright.dev-mode.config.ts) runs with fullyParallel: false /
 * workers: 1, so at most one dev-mode run is ever in flight — a fixed name
 * is simpler than a workspace-hash-derived one, and it doubles as a stable
 * target for startDevStack()'s built-in orphan-reconciliation sweep
 * (Task 3.2.1g) if a previous run was hard-killed (e.g. Ctrl-C mid local
 * debug) without tearing down cleanly.
 */
const DEV_STACK_INSTANCE_NAME = 'e2e-dev-mode';

/**
 * Set by globalSetup below once startDevStack() resolves; read by
 * global-teardown.dev-mode.ts's globalTeardown. This mirrors the shape of
 * the existing global-setup.ts/global-teardown.ts pair, which threads
 * server lifecycle through a shared module (there, helpers/test-server.ts's
 * getGlobalTestServer() singleton) rather than duplicating startup logic in
 * teardown. Here, startDevStack()'s stop() is returned per-call rather than
 * exposed via a separate singleton-holder module, so this module's own
 * exported variable is the shared handoff point instead.
 */
export let stopDevStack: (() => Promise<void>) | undefined;

async function globalSetup(config: FullConfig) {
  console.log('\n🚀 Starting dev-mode DevStack (backend + next dev)...\n');

  try {
    const { backendUrl, frontendUrl, stop } = await startDevStack(DEV_STACK_INSTANCE_NAME, {
      seedData: true,
    });
    stopDevStack = stop;

    // Propagate the FRONTEND URL to workers — tests must load pages from
    // the next dev origin, not the backend, matching global-setup.ts's
    // existing TEST_SERVER_URL contract (process.env mutations here are
    // inherited by Playwright worker processes spawned after global-setup).
    process.env.TEST_SERVER_URL = frontendUrl;
    console.log(`Dev-mode frontend URL exported: ${frontendUrl}`);
    console.log(`Dev-mode backend URL: ${backendUrl}`);

    // Rewrite storageState fixture files with the actual server origin so
    // Playwright applies localStorage to the correct origin regardless of
    // which dynamic port allocatePort() assigned — identical to
    // global-setup.ts's block; the dynamic-origin handling is the same
    // regardless of which launcher produced the URL.
    const fixturesDir = path.join(__dirname, 'fixtures');
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
    console.error('Failed to start dev-mode DevStack:', error);
    throw error;
  }
}

export default globalSetup;
