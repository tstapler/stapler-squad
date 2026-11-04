// Task 0.1e: records, per spec file, every pathname navigated to during the Next baseline capture
// run, written to E2E_ROUTES_VISITED_FILE (JSONL) and later merged into next-e2e-baseline.json's
// per-spec `routesVisited`. Loaded only for that run, via `NODE_OPTIONS=--require` (see
// spikes/s0/run-e2e-baseline.sh) -- never during a normal `npm test`.
//
// Playwright's public API has no global "a page navigated" hook that applies without editing
// every one of the ~180 spec files, so this patches BrowserContext.newPage at the internal
// playwright-core client module (pinned to playwright-core@1.56.1's module layout; a Playwright
// upgrade that moves BrowserContext may need this path updated -- it fails loudly (MODULE_NOT_FOUND)
// rather than silently recording nothing, since require() throws if the path is gone).
// test.info() resolves because newPage() runs inside the `page` fixture's setup, which executes
// within the running test's async context.
'use strict';
const fs = require('fs');
const path = require('path');

const OUT_FILE = process.env.E2E_ROUTES_VISITED_FILE;
if (OUT_FILE) {
  const { test } = require('@playwright/test');
  const browserContextPath = require.resolve('playwright-core').replace(/index\.js$/, 'lib/client/browserContext.js');
  const { BrowserContext } = require(browserContextPath);
  const origNewPage = BrowserContext.prototype.newPage;

  BrowserContext.prototype.newPage = async function (...args) {
    const page = await origNewPage.apply(this, args);
    let specFile = 'unknown';
    try {
      specFile = path.relative(path.join(__dirname, '..'), test.info().file);
    } catch {
      // test.info() throws outside a running test (e.g. a context created by global-setup); leave 'unknown'.
    }
    const routes = new Set();
    page.on('framenavigated', (frame) => {
      if (frame !== page.mainFrame()) return;
      try {
        const u = new URL(frame.url());
        if (u.protocol === 'http:' || u.protocol === 'https:') routes.add(u.pathname);
      } catch {
        // non-http navigation (about:blank, etc.) -- not a route.
      }
    });
    page.once('close', () => {
      if (routes.size === 0) return;
      fs.appendFileSync(OUT_FILE, JSON.stringify({ specFile, routes: [...routes] }) + '\n');
    });
    return page;
  };
}
