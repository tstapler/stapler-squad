import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import type { CDPSession, Locator, Page } from '@playwright/test';
import { baseUrl, tmuxSocket } from '../helpers/base-url';

const run = promisify(execFile);

export type RendererKind = 'xterm' | 'ghostty';

/**
 * Page object for a session's terminal as the phone input layer sees it (mobile-input-layer, Story 5.1.1).
 * Pane text is read from the session's tmux pane on the run's private tmux socket, so an assertion proves
 * the bytes reached the shell, not just that the page drew something.
 */
export class TerminalPage {
  constructor(
    readonly page: Page,
    /** Title the session was created with; alphanumerics only so it survives tmux name sanitising. */
    readonly title: string,
  ) {}

  async open(sessionId: string): Promise<void> {
    await this.page.goto(`${baseUrl()}/?session=${sessionId}`, { waitUntil: 'domcontentloaded' });
    const tab = this.page.getByRole('tab', { name: /terminal/i });
    if (await tab.isVisible().catch(() => false)) await tab.click();
    await this.surface().waitFor({ state: 'visible', timeout: 20_000 });
  }

  /** The visible terminal surface that takes the tap. */
  surface(): Locator {
    return this.page.locator('[data-context="terminal"]').first();
  }

  renderer(kind: RendererKind): Locator {
    const surface = this.page.locator('[data-context="terminal"]');
    return kind === 'ghostty' ? surface.and(this.page.locator('[data-renderer="ghostty"]')) : surface.and(this.page.locator(':not([data-renderer])'));
  }

  layerTextarea(): Locator {
    return this.page.getByTestId('mobile-input-textarea');
  }

  flagOffHint(): Locator {
    return this.page.getByTestId('mobile-input-flag-off-hint');
  }

  /** A single-finger tap at the surface centre (CDP touch events through Playwright's touchscreen). */
  async tapSurface(): Promise<void> {
    const box = await this.surface().boundingBox();
    if (!box) throw new Error('terminal surface has no bounding box');
    await this.page.touchscreen.tap(box.x + box.width / 2, box.y + box.height / 2);
  }

  /** A vertical touch drag over the surface via CDP `Input.dispatchTouchEvent` (real touch events). */
  async dragSurface(cdp: CDPSession, dy: number): Promise<void> {
    const box = await this.surface().boundingBox();
    if (!box) throw new Error('terminal surface has no bounding box');
    const x = box.x + box.width / 2;
    const y0 = box.y + box.height / 2;
    const steps = 8;
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y: y0 }] });
    for (let i = 1; i <= steps; i++) {
      await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x, y: y0 + (dy * i) / steps }] });
    }
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  }

  private async paneTarget(): Promise<string> {
    const socket = tmuxSocket();
    const args = socket ? ['-L', socket] : [];
    const { stdout } = await run('tmux', [...args, 'list-sessions', '-F', '#{session_name}']);
    const name = stdout.split('\n').find((n) => n.includes(this.title));
    if (!name) throw new Error(`no tmux session for "${this.title}" among: ${stdout.trim()}`);
    return name;
  }

  /** Replaces the shell prompt with `$ ` and clears the screen, so the prompt line never wraps at phone width. */
  async useShortPrompt(): Promise<void> {
    const socket = tmuxSocket();
    const args = socket ? ['-L', socket] : [];
    await run('tmux', [...args, 'send-keys', '-t', await this.paneTarget(), "PS1='$ '; clear", 'Enter']);
  }

  /** Current text of the session's pane (trailing blank lines trimmed). */
  async paneText(): Promise<string> {
    const socket = tmuxSocket();
    const args = socket ? ['-L', socket] : [];
    const { stdout } = await run('tmux', [...args, 'capture-pane', '-p', '-t', await this.paneTarget()]);
    return stdout.replace(/\s+$/, '');
  }

  /** The pane's last non-empty line, i.e. the prompt line with whatever was typed at it. */
  async promptLine(): Promise<string> {
    const lines = (await this.paneText()).split('\n');
    return lines[lines.length - 1] ?? '';
  }
}
