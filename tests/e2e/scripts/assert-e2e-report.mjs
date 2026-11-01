#!/usr/bin/env node
/**
 * CI guard for a Playwright JSON report (one shard's, or the merged one): fail when the run did not
 * actually execute tests. Raw test failures are NOT a failure here (the suite is report-only, the
 * baseline comparator judges them); a server that never started, a global-setup error, or zero
 * executed tests is — each of those once produced a green shard that ran nothing.
 *
 *   node assert-e2e-report.mjs <report.json> [--label "<name>"]
 */
import fs from 'node:fs';

function checkReport(report) {
  const problems = [];
  const stats = report?.stats ?? {};
  const executed = (stats.expected ?? 0) + (stats.unexpected ?? 0) + (stats.flaky ?? 0);
  if (executed === 0) problems.push('zero tests executed');
  for (const err of report?.errors ?? []) {
    problems.push(`runner error: ${(err.message ?? JSON.stringify(err)).split('\n')[0].slice(0, 300)}`);
  }
  return { executed, stats, problems };
}

function main(argv) {
  const file = argv[2];
  const label = argv.includes('--label') ? argv[argv.indexOf('--label') + 1] : file;
  if (!file || !fs.existsSync(file)) {
    console.error(`::error::${label}: Playwright JSON report missing (${file}); the run did not reach reporting`);
    return 1;
  }
  let report;
  try {
    report = JSON.parse(fs.readFileSync(file, 'utf8'));
  } catch (err) {
    console.error(`::error::${label}: Playwright JSON report unreadable (${err.message})`);
    return 1;
  }
  const { executed, stats, problems } = checkReport(report);
  const summary = `${label}: executed=${executed} expected=${stats.expected ?? 0} unexpected=${stats.unexpected ?? 0} flaky=${stats.flaky ?? 0} skipped=${stats.skipped ?? 0}`;
  if (problems.length > 0) {
    console.error(`::error::${summary}; ${problems.join('; ')}`);
    return 1;
  }
  console.log(summary);
  return 0;
}

process.exit(main(process.argv));
