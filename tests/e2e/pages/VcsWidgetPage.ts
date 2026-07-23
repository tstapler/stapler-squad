import { Page, Locator, expect } from '@playwright/test';

/**
 * Page-helper for the unified VcsWidget (web-app/src/components/shared/VcsWidget.tsx),
 * shared across Session detail (VcsPanel), Backlog item detail, and Unfinished item
 * detail (compact mode). Locators use data-testid or ARIA roles only, per
 * .claude/rules/e2e-test-conventions.md — VcsWidgetFileList/VcsWidgetCommitList render
 * native <ul>/<li> elements with no explicit `role` attribute, which still resolve via
 * getByRole('list')/getByRole('listitem') through the browser's implicit ARIA mapping.
 */
export class VcsWidgetPage {
  readonly page: Page;
  readonly widget: Locator;

  constructor(page: Page) {
    this.page = page;
    this.widget = page.getByTestId('vcs-widget-loaded');
  }

  async waitForLoaded(timeout = 10000) {
    await expect(this.widget).toBeVisible({ timeout });
  }

  /** The aria-live="polite" / role="status" region wrapping the MergeabilityPill. */
  getMergeabilityPill(): Locator {
    return this.widget.getByRole('status').first();
  }

  async getMergeabilityPillText(): Promise<string> {
    return (await this.getMergeabilityPill().textContent())?.trim() ?? '';
  }

  getSnapshotTimestamp(): Locator {
    return this.page.getByTestId('vcs-widget-snapshot-timestamp');
  }

  getViewDiffButton(): Locator {
    return this.page.getByTestId('vcs-widget-view-diff');
  }

  async clickViewDiff() {
    await this.getViewDiffButton().click();
  }

  /**
   * A file-list row for `path`. VcsWidgetFileList renders each row inside a native
   * <li> (implicit ARIA role "listitem") with the file path as its text content —
   * as a <button> when onNavigateToFile is wired, or a plain <span> otherwise, so
   * this is scoped by role + text rather than assuming either element type.
   */
  getFileRow(path: string): Locator {
    return this.widget.getByRole('listitem').filter({ hasText: path });
  }

  getCommitList(): Locator {
    return this.widget.getByRole('list', { name: 'Commits' });
  }

  getCommitRows(): Locator {
    return this.getCommitList().getByRole('listitem');
  }

  getCommitRow(summaryText: string): Locator {
    return this.getCommitRows().filter({ hasText: summaryText });
  }

  /** The compact-mode aggregate stat line ("N files changed +X -Y"). */
  getAggregateStatLine(): Locator {
    return this.widget.getByText(/files changed/);
  }

  /** The neutral "no history captured" copy shown for a historical snapshot with no data. */
  getNoHistoryMessage(): Locator {
    return this.widget.getByText(/No history captured for this item/);
  }
}
