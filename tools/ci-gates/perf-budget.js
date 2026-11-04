'use strict';

// Decision logic for the frontend-responsiveness CI perf job (plan Story 7.1.3).
// Count-based request budgets are deterministic and can block; timing metrics depend on shared-runner
// speed and stay advisory. Everything degrades to advisory when results or baseline are absent.

const fs = require('fs');

const MARKER = '<!-- benchmark-perf-budget -->';
const DEFAULT_LOAD_WINDOW_S = 16; // LOAD_WINDOW_MS in tests/e2e/perf/frontend-responsiveness.spec.ts

// Mirrors loadBudgets() in the perf spec.
const COUNT_BUDGETS = (durationS) => ({
  GetFeatureFlags: 1,
  ListProgramsConfig: 1,
  LogClientEvents: Math.ceil(durationS / 5) + 1,
  GetFileTransportCapabilities: 1,
  ListWorkflows: 1,
});

const ADVISORY_METRICS = ['peakInFlight', 'longTaskMaxMs', 'queuedOver1s', 'wsQueuedOver1s'];
const isTimingMedian = (name) => /Ms$/.test(name) && !ADVISORY_METRICS.includes(name);
const REGRESSION_PCT = 0.2;

const med = (a) => (a && typeof a.median === 'number' ? a.median : null);

function advisoryRegression(cur, base) {
  const c = med(cur);
  const b = med(base);
  if (c === null || b === null) return null;
  const slack = Math.max(3 * (base.mad || 0), REGRESSION_PCT * b);
  return c > b + slack ? { current: c, baseline: b } : null;
}

function evaluate(results, baseline, opts = {}) {
  const blocking = Boolean(opts.blocking);
  const durationS = opts.loadWindowSeconds || DEFAULT_LOAD_WINDOW_S;
  const out = { blockingFailures: [], advisoryRegressions: [], rows: [], exitCode: 0, markdown: '' };
  const mode = blocking ? 'blocking (count-based subset)' : 'advisory';
  const head = `${MARKER}\n## Frontend responsiveness perf budget (${mode})\n`;

  if (!results || !results.scenarios) {
    out.markdown = `${head}\nNo perf results were produced; nothing to compare. Check the "Run perf project" step.\n`;
    return out;
  }

  const budgets = COUNT_BUDGETS(durationS);
  for (const [name, s] of Object.entries(results.scenarios)) {
    if (name.startsWith('load@')) {
      for (const [rpc, max] of Object.entries(budgets)) {
        const v = med(s.requestsByRpc && s.requestsByRpc[rpc]) ?? 0;
        if (v > max) out.blockingFailures.push({ scenario: name, metric: rpc, current: v, limit: max });
      }
      const s0 = med(s.metrics && s.metrics.status0Count);
      if (s0 !== null && s0 !== 0) {
        out.blockingFailures.push({ scenario: name, metric: 'status0Count', current: s0, limit: 0 });
      }
    }
    const baseMetrics = baseline && baseline.scenarios && baseline.scenarios[name] && baseline.scenarios[name].metrics;
    if (!baseMetrics) continue;
    for (const metric of Object.keys(s.metrics || {})) {
      if (!ADVISORY_METRICS.includes(metric) && !isTimingMedian(metric)) continue;
      const r = advisoryRegression(s.metrics[metric], baseMetrics[metric]);
      if (r) out.advisoryRegressions.push({ scenario: name, metric, ...r });
      out.rows.push({ scenario: name, metric, current: med(s.metrics[metric]), baseline: med(baseMetrics[metric]) });
    }
  }

  const lines = [head];
  if (!baseline) lines.push('No baseline (`tests/e2e/perf/baseline.json`) is committed yet, so medians are not compared; count budgets below still apply.\n');
  if (out.blockingFailures.length) {
    lines.push(blocking ? '**Count budgets exceeded (this check fails):**' : 'Count budgets exceeded (advisory, check not failed):');
    for (const f of out.blockingFailures) lines.push(`- \`${f.scenario}\` ${f.metric}: ${f.current} > ${f.limit}`);
  } else {
    lines.push('Count budgets: all within limits.');
  }
  if (out.rows.length) {
    lines.push('', 'Timing metrics (always advisory):', '', '| scenario | metric | current | baseline | |', '|---|---|---|---|---|');
    const flagged = new Set(out.advisoryRegressions.map((r) => `${r.scenario}/${r.metric}`));
    for (const r of out.rows) lines.push(`| ${r.scenario} | ${r.metric} | ${r.current} | ${r.baseline} | ${flagged.has(`${r.scenario}/${r.metric}`) ? 'slower' : ''} |`);
  }
  out.markdown = lines.join('\n') + '\n';
  out.exitCode = blocking && out.blockingFailures.length ? 1 : 0;
  return out;
}

const readJson = (p) => {
  try {
    return p ? JSON.parse(fs.readFileSync(p, 'utf8')) : null;
  } catch {
    return null;
  }
};

if (require.main === module) {
  const args = process.argv.slice(2);
  const arg = (k) => {
    const i = args.indexOf(k);
    return i >= 0 ? args[i + 1] : undefined;
  };
  const out = evaluate(readJson(arg('--results')), readJson(arg('--baseline')), {
    blocking: args.includes('--blocking') || process.env.PERF_BUDGET_BLOCKING === '1',
    loadWindowSeconds: Number(arg('--load-window-seconds')) || undefined,
  });
  if (arg('--out-md')) fs.writeFileSync(arg('--out-md'), out.markdown);
  console.log(out.markdown);
  process.exit(out.exitCode);
}

module.exports = { evaluate, COUNT_BUDGETS, MARKER };
