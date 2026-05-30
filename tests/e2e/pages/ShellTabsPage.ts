import { Page, Locator, expect } from '@playwright/test';

/**
 * Page object for shell tabs interactions within a session detail view.
 * Assumes the page has already navigated to a session and the detail view is visible.
 */
export class ShellTabsPage {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  // --- Tab bar locators ---

  /** The "+" button in the tab bar that opens the New Shell dialog. */
  get addShellButton(): Locator {
    return this.page.getByRole('button', { name: 'Spawn new shell' }).first();
  }

  /** The "⋯" / "Session actions" button in the session header. */
  get moreActionsButton(): Locator {
    return this.page.getByRole('button', { name: 'Session actions' });
  }

  /** "Spawn new shell" item inside the action sheet. */
  get actionSheetSpawnShell(): Locator {
    return this.page.getByTestId('action-spawn-shell');
  }

  // --- Dialog locators ---

  /** The "New Shell" dialog (role="dialog"). */
  get newShellDialog(): Locator {
    return this.page.getByRole('dialog', { name: 'New Shell' });
  }

  /** Name input inside the New Shell dialog. */
  get shellNameInput(): Locator {
    return this.page.getByLabel('Name (optional)');
  }

  /** Command input inside the New Shell dialog. */
  get shellCommandInput(): Locator {
    return this.page.getByLabel('Command (optional)');
  }

  /** "Spawn Shell" submit button inside the dialog. */
  get spawnShellSubmitButton(): Locator {
    return this.page.getByRole('button', { name: 'Spawn Shell' });
  }

  /** "Cancel" button inside the New Shell dialog. */
  get cancelDialogButton(): Locator {
    return this.page.getByRole('button', { name: 'Cancel' });
  }

  // --- Shell tab locators ---

  /**
   * Returns the tab button for a shell by its display name.
   * Matches the name shown in ShellTabLabel (shell.name || shell.command || "shell").
   */
  getShellTab(name: string): Locator {
    return this.page.getByRole('tab').filter({ hasText: name });
  }

  /**
   * Returns the "Delete shell <name>" button inside the tab label.
   * aria-label is set to "Delete shell <name>" in ShellTab.tsx.
   */
  getDeleteShellButton(shellName: string): Locator {
    return this.page.getByRole('button', { name: `Delete shell ${shellName}` });
  }

  // --- Helpers ---

  /** Wait for the New Shell dialog to be visible. */
  async waitForDialog(): Promise<void> {
    await expect(this.newShellDialog).toBeVisible({ timeout: 5000 });
  }

  /** Wait for the New Shell dialog to be gone. */
  async waitForDialogClosed(): Promise<void> {
    await expect(this.newShellDialog).not.toBeVisible({ timeout: 10000 });
  }

  /**
   * Open the dialog via the "+" button in the tab bar and optionally fill fields.
   */
  async openDialogViaButton(opts?: { name?: string; command?: string }): Promise<void> {
    await this.addShellButton.click();
    await this.waitForDialog();
    if (opts?.name) {
      await this.shellNameInput.fill(opts.name);
    }
    if (opts?.command) {
      await this.shellCommandInput.fill(opts.command);
    }
  }

  /**
   * Open the dialog via the action menu ("⋯" → "Spawn new shell") and optionally fill fields.
   */
  async openDialogViaActionMenu(opts?: { name?: string; command?: string }): Promise<void> {
    await this.moreActionsButton.click();
    await expect(this.actionSheetSpawnShell).toBeVisible({ timeout: 5000 });
    await this.actionSheetSpawnShell.click();
    await this.waitForDialog();
    if (opts?.name) {
      await this.shellNameInput.fill(opts.name);
    }
    if (opts?.command) {
      await this.shellCommandInput.fill(opts.command);
    }
  }

  /**
   * Submit the "Spawn Shell" form and wait for the dialog to close.
   */
  async submitAndWait(): Promise<void> {
    await this.spawnShellSubmitButton.click();
    await this.waitForDialogClosed();
  }

  /**
   * Click a session card / row to open its detail view.
   * Tries data-testid="session-card" first, then falls back to session-row.
   */
  async openFirstSession(): Promise<void> {
    const card = this.page.locator('[data-testid="session-card"]').first();
    const row = this.page.locator('[data-testid="session-row"]').first();

    const hasCard = await card.isVisible({ timeout: 3000 }).catch(() => false);
    if (hasCard) {
      await card.click();
    } else {
      await expect(row).toBeVisible({ timeout: 5000 });
      await row.click();
    }

    // Wait until the session header is visible (tab bar rendered)
    await expect(this.addShellButton).toBeVisible({ timeout: 8000 });
  }
}
