// @feature session:list
/**
 * Proves the per-worker server pool is isolated: every worker slot has its own port, data dir,
 * instance.lock and tmux socket, and a session created on one slot's server is invisible to the others.
 * Meaningful at E2E_WORKERS>1; at 1 it degrades to checking the single server's own wiring.
 */
import * as fs from 'fs';
import * as path from 'path';
import { execFileSync } from 'child_process';
import { test, expect } from '@playwright/test';
import { baseUrl, serverSlot, testDir, tmuxSocket } from './helpers/base-url';
import { SessionClient } from './helpers/session-client';

const urls: string[] = JSON.parse(process.env.TEST_SERVER_URLS ?? '[]');
const dirs: string[] = JSON.parse(process.env.TEST_SERVER_TESTDIRS ?? '[]');
const sockets: string[] = JSON.parse(process.env.TEST_SERVER_TMUX_SOCKETS ?? '[]');


test.describe('worker server pool isolation', () => {
  // One project is enough: the pool is shared by all of them.
  test.beforeEach(({}, testInfo) => {
    test.skip(testInfo.project.name !== 'chromium', 'pool is shared across projects');
  });

  test('every slot has a distinct port, data dir and tmux socket', async () => {
    expect(urls.length).toBeGreaterThanOrEqual(1);
    expect(urls).toHaveLength(dirs.length);
    expect(new Set(urls).size).toBe(urls.length);
    expect(new Set(dirs).size).toBe(dirs.length);
    expect(sockets).toHaveLength(urls.length);
    expect(new Set(sockets).size).toBe(sockets.length);
    for (const socket of sockets) {
      // Each server seeded live bash sessions into its own private tmux server, so every socket is live.
      const out = execFileSync('tmux', ['-L', socket, 'list-sessions'], { encoding: 'utf8' });
      expect(out.length, `${socket} list-sessions`).toBeGreaterThan(0);
    }
    for (const dir of dirs) {
      // instance.lock lives in the --test-dir, so a per-slot dir means a per-slot lock.
      expect(fs.existsSync(path.join(dir, 'instance.lock')), `${dir}/instance.lock`).toBe(true);
    }
  });

  test('this worker talks to the server of its own slot', async () => {
    expect(baseUrl()).toBe(urls[serverSlot()]);
    expect(testDir()).toBe(dirs[serverSlot()]);
    expect(tmuxSocket()).toBe(sockets[serverSlot()]);
    // Specs that still read the single-server env vars are re-pointed at this slot by pinWorkerEnv().
    expect(process.env.TEST_SERVER_URL).toBe(urls[serverSlot()]);
    expect(process.env.TEST_SERVER_TESTDIR).toBe(dirs[serverSlot()]);
  });

  for (const probe of ['a', 'b', 'c', 'd']) {
    test(`session created on own server is invisible to the other servers (${probe})`, async () => {
      console.log(`isolation probe=${probe} slot=${serverSlot()} baseUrl=${baseUrl()}`);
      const title = `isolation-${probe}-slot${serverSlot()}-${Date.now()}`;
      const own = new SessionClient(baseUrl());
      const dir = path.join(testDir()!, 'isolation', title);
      fs.mkdirSync(dir, { recursive: true });
      const created = await own.createSession({ title, path: dir, program: 'bash' });
      try {
        const ownTitles = (await own.listSessions()).map((s) => s.title);
        expect(ownTitles).toContain(title);
        for (const other of urls.filter((u) => u !== baseUrl())) {
          const titles = (await new SessionClient(other).listSessions()).map((s) => s.title);
          expect(titles, `${other} must not see ${title}`).not.toContain(title);
        }
      } finally {
        await own.deleteSession(created.id, true);
      }
    });
  }
});
