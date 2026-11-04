import { expect, type Page } from '@playwright/test';
import { SessionDetailPage } from './SessionDetailPage';
import { dismissOnboarding, openAndWaitConnected } from '../helpers/scroll-forward-fixture';

/** Browser-only terminal observations; no capture-pane or injected xterm.write calls. */
export class TerminalEditingPage extends SessionDetailPage {
  constructor(page: Page) { super(page); }

  async open(id: string): Promise<void> {
    await dismissOnboarding(this.page);
    await this.gotoSession(id);
    await this.page.reload();
    await expect(this.page.getByRole('tab', { name: 'Terminal', exact: true })).toBeVisible();
    for (const button of await this.page.getByRole('button', { name: 'Not now', exact: true }).all()) {
      if (await button.isVisible()) await button.click();
    }
    await openAndWaitConnected(this.page, this);
    await this.focus();
  }

  async focus(): Promise<void> {
    await this.page.getByRole('tabpanel', { name: /terminal/i }).click();
  }

  async text(): Promise<string> {
    // xterm owns these renderer internals and provides no ARIA row API. Read-only
    // DOM inspection is scoped to the public Terminal tabpanel (not a page locator).
    return this.page.getByRole('tabpanel', { name: /terminal/i }).evaluate(el =>
      Array.from(el.querySelectorAll('.xterm-rows > div')).map(row => row.textContent ?? '').join('\n'));
  }

  async contains(value: string | RegExp): Promise<void> {
    await expect.poll(() => this.text(), { timeout: 15000 }).toMatch(value);
  }

  async command(value: string): Promise<void> {
    await this.focus();
    await this.page.keyboard.type(value);
    await this.page.keyboard.press('Enter');
  }

  async paste(value: string): Promise<void> {
    await this.focus();
    // Exercise xterm's real clipboard handler, including bracketed-paste wrapping,
    // without depending on OS clipboard permissions in headless CI.
    await this.page.getByRole('tabpanel', { name: /terminal/i }).evaluate((el, text) => {
      const input = el.querySelector('textarea');
      if (!input) throw new Error('xterm input not mounted');
      const clipboardData = new DataTransfer();
      clipboardData.setData('text/plain', text);
      input.dispatchEvent(new ClipboardEvent('paste', { clipboardData, bubbles: true, cancelable: true }));
    }, value);
  }

  async toolbarPage(direction: 'up' | 'down'): Promise<void> {
    const toggle = this.page.getByRole('button', { name: /keyboard/i });
    if (!(await this.page.getByRole('button', { name: 'Page up', exact: true }).isVisible())) {
      await toggle.click();
    }
    await this.page.getByRole('button', { name: `Page ${direction}`, exact: true }).click();
    await this.focus();
  }

  async dimensions(): Promise<{ cols: number; rows: number }> {
    return this.page.getByRole('tabpanel', { name: /terminal/i }).evaluate(el => {
      const screen = el.querySelector('.xterm-screen') as HTMLElement;
      const row = el.querySelector('.xterm-rows > div') as HTMLElement;
      const cell = el.querySelector('.xterm-char-measure-element') as HTMLElement;
      if (!screen || !row || !cell) throw new Error('xterm measurement elements missing');
      return {
        cols: Math.round(screen.getBoundingClientRect().width / cell.getBoundingClientRect().width),
        rows: Math.round(screen.getBoundingClientRect().height / row.getBoundingClientRect().height),
      };
    });
  }
}
