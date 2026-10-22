// @feature accessibility
/**
 * Task 7.1b guard: titles are asserted with auto-retrying `await expect(page).toHaveTitle(...)`,
 * never a one-shot read. On the Bun build React sets the title after navigation, so
 * `page.title()` / `document.title` followed by an assertion is flaky by construction.
 * Runs without a browser, so it costs nothing in the full suite.
 */
import * as fs from 'fs';
import * as path from 'path';
import { test, expect } from '@playwright/test';

const E2E_ROOT = path.join(__dirname, '..');
const SKIP_DIRS = new Set(['node_modules', 'test-results', 'playwright-report', 'allure-results', 'allure-report']);
const ONE_SHOT_TITLE = /\bpage\.title\(\)|\bdocument\.title\b/;

/** Files that record a settled title into a fixture (a measurement, not an assertion). */
const RECORDING_FILES = new Set([
  path.join('tools', 'capture-next-fixtures.spec.ts'),
  path.join('pages', 'BaselineProbes.ts'),
  path.join('tools', 'e2e-conventions.spec.ts'),
]);

function typeScriptFiles(dir: string): string[] {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    if (entry.isDirectory()) return SKIP_DIRS.has(entry.name) ? [] : typeScriptFiles(path.join(dir, entry.name));
    return entry.name.endsWith('.ts') ? [path.join(dir, entry.name)] : [];
  });
}

test('no spec or page helper reads the title one-shot', () => {
  const offenders = typeScriptFiles(E2E_ROOT)
    .map((file) => path.relative(E2E_ROOT, file))
    .filter((file) => !RECORDING_FILES.has(file))
    .flatMap((file) =>
      fs
        .readFileSync(path.join(E2E_ROOT, file), 'utf8')
        .split('\n')
        .map((line, index) => ({ file, line: index + 1, text: line.trim() }))
        .filter(({ text }) => ONE_SHOT_TITLE.test(text)),
    );
  expect(offenders.map((o) => `${o.file}:${o.line} ${o.text}`)).toEqual([]);
});
