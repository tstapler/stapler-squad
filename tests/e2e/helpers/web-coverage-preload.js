// Opt-in Chromium JS-coverage and request recorder for the S14 capture (tools/bzl/e2ecapture).
// Loaded with NODE_OPTIONS=--require only when WEBCOV_DIR is set; it touches no spec and is inert otherwise.
const fs = require('fs');
const path = require('path');

const outDir = process.env.WEBCOV_DIR;
if (outDir) {
  try {
    const from = process.env.WEBCOV_RESOLVE_FROM || process.cwd();
    // the package's exports map hides lib/, so load the client class by absolute path (same module instance the runner uses)
    const pkgDir = path.dirname(require.resolve('playwright-core', { paths: [from] }));
    const { BrowserContext } = require(path.join(pkgDir, 'lib/client/browserContext.js'));
    const onPage = BrowserContext.prototype._onPage;
    const close = BrowserContext.prototype.close;
    let seq = 0;
    BrowserContext.prototype._onPage = function (page) {
      try {
        const st = this.__cov || (this.__cov = { pages: [], paths: new Set() });
        st.pages.push(page);
        page.coverage.startJSCoverage({ resetOnNavigation: false }).catch(() => {});
        page.on('request', (r) => {
          try { st.paths.add(new URL(r.url()).pathname); } catch (e) { /* non-http url */ }
        });
      } catch (e) { /* coverage is best effort */ }
      return onPage.apply(this, arguments);
    };
    BrowserContext.prototype.close = async function () {
      const st = this.__cov;
      if (st && !st.flushed) {
        st.flushed = true;
        const entries = [];
        for (const p of st.pages) {
          try {
            if (p.isClosed()) continue;
            for (const e of await p.coverage.stopJSCoverage()) entries.push({ url: e.url, functions: e.functions });
          } catch (e) { /* page already gone */ }
        }
        try {
          fs.mkdirSync(outDir, { recursive: true });
          fs.writeFileSync(path.join(outDir, `${process.pid}-${seq++}.json`), JSON.stringify({ entries, paths: [...st.paths] }));
        } catch (e) { /* ignore */ }
      }
      return close.apply(this, arguments);
    };
  } catch (e) { /* playwright-core not resolvable in this process */ }
}
