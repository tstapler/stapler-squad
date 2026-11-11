// @feature website-demos
import { describe, expect, test } from 'bun:test';
import * as fs from 'fs';
import * as path from 'path';
import { hold } from '../hold';

describe('hold', () => {
  test('waits inside the page for the requested time', async () => {
    const seen: unknown[] = [];
    await hold({ evaluate: async (_fn: unknown, arg: unknown) => void seen.push(arg) } as never, 250);
    expect(seen).toEqual([250]);
  });

  test('zero is a no-op and bad input is rejected', async () => {
    const page = { evaluate: async () => expect.unreachable() } as never;
    await hold(page, 0);
    await expect(hold(page, -1)).rejects.toThrow(RangeError);
    await expect(hold(page, Number.NaN)).rejects.toThrow(RangeError);
  });
});

describe('no raw timeouts in website-demos', () => {
  test('the Playwright fixed-sleep call does not appear anywhere under website-demos', () => {
    const banned = ['wait', 'For', 'Timeout'].join('');
    const root = path.resolve(__dirname, '..');
    const hits: string[] = [];
    const walk = (dir: string) => {
      for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
        const p = path.join(dir, e.name);
        if (e.isDirectory()) walk(p);
        else if (fs.readFileSync(p, 'utf8').includes(banned)) hits.push(path.relative(root, p));
      }
    };
    walk(root);
    expect(hits).toEqual([]);
    expect(fs.readFileSync(path.resolve(root, '..', 'website-demos.spec.ts'), 'utf8')).not.toContain(banned);
  });
});
