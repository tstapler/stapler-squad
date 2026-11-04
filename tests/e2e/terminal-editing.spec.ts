// @feature session:create, session:terminal
import { test, expect } from '@playwright/test';
import { createHash, randomUUID } from 'crypto';
import { existsSync, readFileSync } from 'fs';
import * as path from 'path';
import { SessionClient } from './helpers/session-client';
import { baseUrl, testDir } from './helpers/base-url';
import { TerminalEditingPage } from './pages/TerminalEditingPage';

const fixture = path.resolve(__dirname, 'fixtures/terminal-editing.py');
const hash = (text: string) => createHash('sha256').update(text).digest('hex').slice(0, 16);

for (const backend of ['tmux', 'tymux'] as const) {
  test.describe(`terminal editing (${backend})`, () => {
    test.describe.configure({ timeout: 90000 });
    let client: SessionClient;
    let terminal: TerminalEditingPage;
    let id: string;
    let pid: string;
    let ack: number;

    test.beforeEach(async ({ page }) => {
      await page.setViewportSize({ width: 1400, height: 1000 });
      if (backend === 'tymux') expect(existsSync(process.env.TYMUXD_BIN || path.resolve(__dirname, '../../session/tymux/embed/tymuxd')), 'pinned tymuxd prerequisite').toBe(true);
      client = new SessionClient(baseUrl());
      terminal = new TerminalEditingPage(page);
      ack = 0;
      const title = `editing-${backend}-${randomUUID().slice(0, 8)}`;
      id = '';
      const session = await client.createSession({
        title,
        path: path.dirname(fixture),
        program: 'python3 -u terminal-editing.py',
        backendOverride: backend,
      });
      id = session.id;
      await client.waitForSessionsActive([session.title]);
      await terminal.open(id);
      await terminal.command(':ready');
      await terminal.contains(/FIXTURE_READY pid=\d+/);
      pid = (await terminal.text()).match(/FIXTURE_READY pid=(\d+)/)![1];
      // Session's public proto has no backend field. The isolated server's stream
      // attachment log records the *actual* Snapshot.Backend, not the requested pin.
      await expect.poll(() => {
        const records = readFileSync(path.join(testDir(), 'logs/staplersquad.log'), 'utf8').split('\n');
        return records.some(line => {
          try {
            const record = JSON.parse(line);
            return record.msg === '[streamViaHub] starting' && record.session === session.title && record.backend === backend;
          } catch { return false; }
        });
      }, { message: `actual stream backend must be ${backend}` }).toBe(true);
    });

    test.afterEach(async () => {
      if (id) await client.deleteSession(id, true);
    });

    async function submitted(expected: string): Promise<void> {
      ack += 1;
      await terminal.contains(`ACK ${ack} pid=${pid} chars=${Array.from(expected).length} sha=${hash(expected)}`);
    }

    test('printable Unicode, destructive editing, arrows, Home/End and control editing', async ({ page }) => {
      await page.keyboard.type('hello café 世界 🦊');
      await page.keyboard.press('Enter');
      await submitted('hello café 世界 🦊');
      await page.keyboard.type('abXYcd');
      await page.keyboard.press('ArrowLeft');
      await page.keyboard.press('ArrowLeft');
      await page.keyboard.press('Backspace');
      await page.keyboard.press('Delete');
      await page.keyboard.press('Home');
      await page.keyboard.type('>');
      await page.keyboard.press('End');
      await page.keyboard.type('<');
      await page.keyboard.press('Enter');
      await submitted('>abXd<');
      await page.keyboard.type('one two three');
      await page.keyboard.press('Control+w');
      await page.keyboard.press('Control+a');
      await page.keyboard.type('start ');
      await page.keyboard.press('Control+e');
      await page.keyboard.type('end');
      await page.keyboard.press('Enter');
      await submitted('start one two end');
      await page.keyboard.type('discard me');
      await page.keyboard.press('Control+u');
      await page.keyboard.type('keep tail');
      await page.keyboard.press('Home');
      for (let n = 0; n < 4; n++) await page.keyboard.press('ArrowRight');
      await page.keyboard.press('Control+k');
      await page.keyboard.press('Enter');
      await submitted('keep');
      await page.keyboard.press('ArrowUp');
      await page.keyboard.press('Enter');
      await submitted('keep');
      await page.keyboard.type('fresh');
      await page.keyboard.press('ArrowUp');
      await page.keyboard.press('ArrowDown');
      await page.keyboard.press('Enter');
      await submitted('fresh');
    });

    test('Ctrl+C cancellation and safe Ctrl+D preserve the same process', async ({ page }) => {
      await page.keyboard.type('must-not-submit');
      await page.keyboard.press('Control+c');
      await terminal.contains(`INTERRUPTED pid=${pid}`);
      await page.keyboard.press('Control+d');
      await terminal.contains(`EOF_ACK pid=${pid}`);
      await terminal.command('still-alive');
      await submitted('still-alive');
    });

    test('bracketed Unicode, multiline and large paste arrive intact', async ({ page }) => {
      for (const payload of ['paste café 世界 🦊', 'first line\nsecond line\nthird line', 'xY09'.repeat(4096)]) {
        await terminal.paste(payload);
        await page.keyboard.press('Enter');
        await submitted(payload);
      }
    });

    test('alternate screen receives raw PageUp/PageDown, toolbar keys and bracketed paste bytes', async ({ page }) => {
      await terminal.command(':raw');
      await terminal.contains('RAW_READY');
      await page.keyboard.press('PageUp');
      await page.keyboard.press('PageDown');
      await terminal.toolbarPage('up');
      await terminal.toolbarPage('down');
      await terminal.paste('a\nb');
      await page.keyboard.press('Control+]');
      const bytes = '\x1b[5~\x1b[6~\x1b[5~\x1b[6~\x1b[200~a\rb\x1b[201~';
      await terminal.contains(`RAW_ACK ${Buffer.from(bytes).toString('hex')}`);
      await terminal.command('after-alt');
      await submitted('after-alt');
    });

    test('normal buffer keyboard and toolbar paging show numbered history after reload', async ({ page }) => {
      await terminal.command(':history');
      await terminal.contains('HISTORY_END');
      // Discard browser-resident output, so paging must hydrate backend history.
      await terminal.open(id);
      await terminal.contains('HISTORY_END');
      const live = await terminal.text();
      const liveNumbers = [...live.matchAll(/HISTORY-(\d{4})/g)].map(match => Number(match[1]));
      expect(liveNumbers.length).toBeGreaterThan(0);
      const liveFirst = Math.min(...liveNumbers);
      await page.keyboard.press('Shift+PageUp');
      await expect.poll(async () => {
        const numbers = [...(await terminal.text()).matchAll(/HISTORY-(\d{4})/g)].map(match => Number(match[1]));
        return numbers.length > 0 && Math.min(...numbers) < liveFirst;
      }, { message: `Shift+PageUp must show history older than ${liveFirst}` }).toBe(true);
      const older = await terminal.text();
      await page.keyboard.press('Shift+PageDown');
      await terminal.contains('HISTORY_END');
      await terminal.toolbarPage('up');
      await expect.poll(() => terminal.text()).not.toBe(live);
      await expect.poll(() => terminal.text()).toContain(older.match(/HISTORY-\d{4}/)![0]);
      await terminal.toolbarPage('down');
      await terminal.contains('HISTORY_END');
      await terminal.command('after-history');
      await submitted('after-history');
      for (let pageNumber = 0; pageNumber < 30; pageNumber++) {
        if ((await terminal.text()).includes('HISTORY-0001')) break;
        await terminal.toolbarPage('up');
      }
      await terminal.contains('HISTORY-0001');
      await terminal.toolbarPage('up');
      await terminal.contains('HISTORY-0001');
    });

    test('resize updates exact PTY columns and rows, then reload/reconnect accepts fresh I/O', async ({ page, context }) => {
      let previous = await terminal.dimensions();
      for (const viewport of [{ width: 1000, height: 700 }, { width: 1400, height: 900 }]) {
        await page.setViewportSize(viewport);
        await expect.poll(() => terminal.dimensions()).not.toEqual(previous);
        await expect(async () => {
          const dims = await terminal.dimensions();
          await terminal.command(':dims');
          await terminal.contains(`DIMS ${dims.cols}x${dims.rows}`);
        }).toPass({ timeout: 15000 });
        previous = await terminal.dimensions();
      }
      await terminal.command('before-reload');
      await submitted('before-reload');
      await terminal.open(id);
      await terminal.command('after-reload');
      await submitted('after-reload');
      await context.setOffline(true);
      await expect(page.getByText('Connected', { exact: true })).not.toBeVisible();
      await context.setOffline(false);
      await expect(page.getByText('Connected', { exact: true })).toBeVisible({ timeout: 20000 });
      await terminal.command('after-reconnect');
      await submitted('after-reconnect');
    });
  });
}
