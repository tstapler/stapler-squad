// @feature accessibility
/**
 * Story 7.9 self-tests: the parity harness must fail on a seeded CSS difference and on an allow-list
 * entry without a traceable reason. The pure-logic cases need no browser; the end-to-end case drives
 * the real CLI against two throwaway servers whose only difference is one CSS rule.
 */
import * as http from 'http';
import { execFile } from 'child_process';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import { promisify } from 'util';
import { test, expect } from '@playwright/test';
import {
  diffSnapshots,
  seededShuffle,
  unallowedDifferences,
  validateAllowList,
  type StyleSnapshot,
} from './parity-core';
import { walkOrders } from './nav-walk';

const run = promisify(execFile);
const PAGE_BODY = '<main><h1 data-testid="probe">hello</h1><a href="/x">link</a></main>';

const snapshot = (color: string): StyleSnapshot => ({
  rootVars: { '--bg': '#000' },
  styles: { 'main>h1[data-testid=probe]|base|color': color },
});

test.describe('parity harness self-test', () => {
  test('identical snapshots produce no difference', () => {
    expect(diffSnapshots('/', snapshot('red'), snapshot('red'))).toEqual([]);
  });

  test('a seeded CSS difference is reported and not allowed', () => {
    const diffs = diffSnapshots('/', snapshot('red'), snapshot('blue'));
    expect(diffs).toHaveLength(1);
    expect(unallowedDifferences(diffs, [])).toHaveLength(1);
  });

  test('a root custom-property difference is reported', () => {
    const changed = { ...snapshot('red'), rootVars: { '--bg': '#fff' } };
    expect(diffSnapshots('/', snapshot('red'), changed).map((d) => d.property)).toEqual(['--bg']);
  });

  test('an allow-list entry with a reason is honoured; one without is invalid', () => {
    const diffs = diffSnapshots('/', snapshot('red'), snapshot('blue'));
    const entry = { route: '/', selector: 'main>h1[data-testid=probe]', property: 'color', reason: 'https://example.com/issue/1' };
    expect(validateAllowList([entry])).toEqual([]);
    expect(unallowedDifferences(diffs, [entry])).toEqual([]);

    expect(validateAllowList([{ ...entry, reason: '' }])).not.toEqual([]);
    expect(validateAllowList([{ ...entry, reason: 'looks fine to me' }])).not.toEqual([]);
    expect(validateAllowList([{ ...entry, reason: '"main > h1 { color: blue }"' }])).toEqual([]);
  });

  test('seeded orderings replay from the seed and the walk has 3 fixed + 3 seeded orders', () => {
    const routes = ['/a', '/b', '/c', '/d', '/e', '/f'];
    expect(seededShuffle(routes, 7)).toEqual(seededShuffle(routes, 7));
    expect(seededShuffle(routes, 7)).not.toEqual(seededShuffle(routes, 8));
    const orders = walkOrders(routes, 100);
    expect(orders.map((o) => o.label)).toEqual([
      'fixed:forward', 'fixed:reverse', 'fixed:outside-in', 'seeded:100', 'seeded:101', 'seeded:102',
    ]);
    expect(orders.every((o) => [...o.routes].sort().join() === routes.join())).toBe(true);
  });

  test('the CLI exits non-zero on a seeded difference and zero on identical servers', async () => {
    test.setTimeout(120_000);
    const serve = (css: string) =>
      new Promise<http.Server>((resolve) => {
        const server = http.createServer((_req, res) => {
          res.setHeader('content-type', 'text/html');
          res.end(`<!doctype html><title>t</title><style>h1{color:${css}}</style>${PAGE_BODY}`);
        });
        server.listen(0, '127.0.0.1', () => resolve(server));
      });
    const port = (s: http.Server) => (s.address() as { port: number }).port;
    const [red, redToo, blue] = await Promise.all([serve('red'), serve('red'), serve('blue')]);
    const out = fs.mkdtempSync(path.join(os.tmpdir(), 'parity-selftest-'));
    const cli = (a: http.Server, b: http.Server, extra: string[] = []) =>
      run('node', ['--no-warnings', path.join(__dirname, 'parity-diff.ts'), '--a', `http://127.0.0.1:${port(a)}`, '--b', `http://127.0.0.1:${port(b)}`, '--quick', '--out', path.join(out, 'r.json'), ...extra], {
        cwd: path.join(__dirname, '..'),
      });
    try {
      await cli(red, redToo);
      const failing = await cli(red, blue).then(
        () => 0,
        (err: { code: number }) => err.code,
      );
      expect(failing, 'exit status for a seeded difference').toBe(1);
      const report = JSON.parse(fs.readFileSync(path.join(out, 'r.json'), 'utf8')) as { unallowed: { property: string }[] };
      expect(report.unallowed.some((d) => d.property === 'color')).toBe(true);

      // An allow-list entry without a reason makes even an identical pair fail (exit 2, invalid list).
      const badAllow = path.join(out, 'allow.json');
      fs.writeFileSync(badAllow, JSON.stringify([{ route: '/', selector: '*', property: 'color', reason: '' }]));
      const invalid = await cli(red, redToo, ['--allow', badAllow]).then(
        () => 0,
        (err: { code: number }) => err.code,
      );
      expect(invalid, 'exit status for an allow-list entry with no reason').toBe(2);
    } finally {
      for (const s of [red, redToo, blue]) s.close();
      fs.rmSync(out, { recursive: true, force: true });
    }
  });
});
