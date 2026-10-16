import type { Locator, Page } from '@playwright/test';

/** The desktop edge bell and its "Reset notification button position" setting. */
export class TrayHandlePage {
  readonly handle: Locator;
  readonly tray: Locator;
  readonly overflow: Locator;
  readonly settingsItem: Locator;
  readonly resetButton: Locator;

  constructor(private readonly page: Page) {
    this.handle = page.getByTestId('tray-handle');
    this.tray = page.getByTestId('notification-tray');
    this.overflow = this.tray.getByTestId('tray-overflow');
    this.settingsItem = page.getByRole('menuitem', { name: 'Tray settings' });
    this.resetButton = page.getByRole('button', { name: 'Reset notification button position' });
  }

  async top(): Promise<number> {
    const box = await this.handle.boundingBox();
    if (!box) throw new Error('tray handle has no bounding box');
    return box.y;
  }

  /** Press-move-release with intermediate steps so the pointer crosses the drag threshold. */
  async dragBy(dy: number): Promise<void> {
    const box = await this.handle.boundingBox();
    if (!box) throw new Error('tray handle has no bounding box');
    const x = box.x + box.width / 2;
    const y = box.y + box.height / 2;
    await this.page.mouse.move(x, y);
    await this.page.mouse.down();
    await this.page.mouse.move(x, y + dy, { steps: 12 });
    await this.page.mouse.up();
  }

  async openSettings(): Promise<void> {
    await this.overflow.click();
    await this.settingsItem.click();
  }
}
