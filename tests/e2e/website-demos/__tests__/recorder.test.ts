// @feature website-demos
import { describe, expect, test } from 'bun:test';
import * as fs from 'fs';
import * as path from 'path';
import { readParts } from '../manifest';
import { caption, ensureFlags, readClipMaxDurationMs, recordClip, type RecordEnv } from '../recorder';
import type { ClipSpec } from '../scene';
import { fakeBrowser, tmpDir } from './helpers';

const env = (outDir: string, extra: Partial<RecordEnv> = {}): RecordEnv => ({ outDir, commit: 'C', clipMaxDurationMs: 60_000, fetchFn: (async () => new Response('ok')) as typeof fetch, ...extra });
const okClip = (extra: Partial<ClipSpec> = {}): ClipSpec => ({
  name: 'demo',
  title: 'Demo',
  scenes: async (page, clip) => {
    await clip.ready();
    await caption(page, 'Approve the push', 1);
  },
  ...extra,
});

describe('recordClip', () => {
  test('records the clip, transcript, DOM dump, poster and a recorded manifest part', async () => {
    const out = tmpDir();
    const { browser } = fakeBrowser();
    await recordClip(browser, 'http://localhost:1', okClip(), env(out));
    expect(fs.existsSync(path.join(out, 'demo.webm'))).toBe(true);
    expect(JSON.parse(fs.readFileSync(path.join(out, 'demo.transcript.json'), 'utf8'))[0].text).toBe('Approve the push');
    expect(fs.readFileSync(path.join(out, 'dom-text', 'demo.txt'), 'utf8')).toContain('Six agents');
    const rows = readParts(out);
    expect(rows.map((r) => `${r.name}:${r.format}:${r.status}`)).toEqual(['demo:webm:recorded', 'demo-poster:png:recorded']);
    expect(rows[0]).toMatchObject({ attempts: 1, themeId: 'grimdark', src: 'clips/demo.webm' });
  });

  test('a wrong theme throws the documented message, is not retried and leaves a fatal marker and a failed row', async () => {
    const out = tmpDir();
    const { browser, calls } = fakeBrowser({ background: 'rgb(15, 17, 23)' });
    await expect(recordClip(browser, 'http://localhost:1', okClip(), env(out))).rejects.toThrow('expected theme class grimdark, got clean');
    expect(calls.contexts).toBe(1);
    expect(fs.existsSync(path.join(out, '.fatal-demo.txt'))).toBe(true);
    expect(readParts(out).map((r) => r.status)).toEqual(['failed']);
    expect(fs.existsSync(path.join(out, 'demo.webm'))).toBe(false);
  });

  test('a flaky scene is retried on a fresh context and records attempts 2', async () => {
    const out = tmpDir();
    const { browser, calls } = fakeBrowser();
    let n = 0;
    const clip = okClip({
      scenes: async (page, c) => {
        if (++n === 1) throw new Error('locator timeout');
        await c.ready();
        await caption(page, 'Second try', 1);
      },
    });
    await recordClip(browser, 'http://localhost:1', clip, env(out));
    expect(calls.contexts).toBe(2);
    expect(readParts(out)[0].attempts).toBe(2);
  });

  test('a scene that never calls ready(), has no captions, or misses a quote fails', async () => {
    const { browser } = fakeBrowser();
    await expect(recordClip(browser, 'http://x', okClip({ scenes: async () => {} }), env(tmpDir()))).rejects.toThrow(/ready/);
    await expect(recordClip(browser, 'http://x', okClip({ scenes: async (_p, c) => c.ready() }), env(tmpDir()))).rejects.toThrow(/no captions/);
    await expect(recordClip(browser, 'http://x', okClip({ quotes: ['Deny everything'] }), env(tmpDir()))).rejects.toThrow(/quote/);
  });

  test('a flag-gated page that redirected fails the landing assertion', async () => {
    const { browser } = fakeBrowser({ url: 'http://localhost/' });
    const clip = okClip({ landing: { path: '/backlog/board', landmark: '[data-testid="board"]' } });
    await expect(recordClip(browser, 'http://x', clip, env(tmpDir()))).rejects.toThrow(/expected final path \/backlog\/board/);
  });

  test('a clip longer than the budget cap fails without retry', async () => {
    const out = tmpDir();
    const { browser, calls } = fakeBrowser();
    await expect(recordClip(browser, 'http://x', okClip(), env(out, { clipMaxDurationMs: 1 }))).rejects.toThrow(/cap/);
    expect(calls.contexts).toBe(1);
  });
});

describe('ensureFlags', () => {
  test('reads the flag back and fails when a write was silently ignored', async () => {
    const ok = (async (url: string) => new Response(JSON.stringify(String(url).endsWith('GetFeatureFlags') ? { flags: [{ name: 'backlog', enabled: true }] } : {}))) as unknown as typeof fetch;
    await ensureFlags('http://x', ['backlog'], ok);
    const ignored = (async (url: string) => new Response(JSON.stringify(String(url).endsWith('GetFeatureFlags') ? { flags: [{ name: 'backlog', enabled: false }] } : {}))) as unknown as typeof fetch;
    await expect(ensureFlags('http://x', ['backlog'], ignored)).rejects.toThrow(/did not turn on/);
  });
});

describe('budgets', () => {
  test('the clip cap comes from tools/promo/budgets.json (60 s)', () => {
    expect(readClipMaxDurationMs()).toBe(60_000);
  });
});
