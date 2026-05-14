// @feature backlog:create-item, backlog:list-items, backlog:transition-status, backlog:spawn-session
import { test, expect } from '@playwright/test';
import { BacklogPage } from './pages/BacklogPage';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

test.describe('Backlog', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto(`${BASE_URL}/backlog`, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('[data-testid="backlog-page"]', { timeout: 15000 });
  });

  test.describe('Empty State', () => {
    test('e2e:backlog-empty-state-renders - Empty state displays headline, lifecycle diagram, and CTA button', async ({ page }) => {
      const backlogPage = new BacklogPage(page);

      // Verify page title
      await expect(page).toHaveTitle(/Stapler Squad/);

      // Check if backlog is empty. Fail loudly if items exist — the test environment
      // must be clean for empty-state tests to be meaningful.
      const tableRows = await backlogPage.getTableRows().count();
      if (tableRows > 0) {
        test.fail(true, `Empty-state test requires a clean backlog, but found ${tableRows} item(s). Clear the backlog before running this test.`);
        return;
      }

      // Verify empty state is visible
      await expect(backlogPage.emptyState).toBeVisible();

      // Verify headline is present and accessible
      await expect(backlogPage.emptyHeadline).toBeVisible();
      const headlineText = await backlogPage.emptyHeadline.textContent();
      expect(headlineText).toContain('Your backlog is empty');

      // Verify lifecycle diagram is present
      await expect(backlogPage.lifecycleDiagram).toBeVisible();

      // Verify at least one lifecycle node exists
      const lifecycleNodes = await page.locator('[data-testid^="backlog-lifecycle-node-"]').count();
      expect(lifecycleNodes).toBeGreaterThan(0);

      // Verify CTA button is present and accessible
      await expect(backlogPage.emptyCtaButton).toBeVisible();
      const ctaText = await backlogPage.emptyCtaButton.textContent();
      expect(ctaText).toContain('Create First Item');
    });

    test('e2e:backlog-empty-form-opens - Clicking CTA button reveals inline form', async ({ page }) => {
      const backlogPage = new BacklogPage(page);

      // Check if backlog is empty. Fail loudly so the environment issue is surfaced.
      const tableRows = await backlogPage.getTableRows().count();
      if (tableRows > 0) {
        test.fail(true, `Empty-state test requires a clean backlog, but found ${tableRows} item(s).`);
        return;
      }

      // Verify form is not initially visible
      const form = page.locator('[data-testid="backlog-empty-form"]');
      await expect(form).not.toBeVisible();

      // Click CTA button to open form
      await backlogPage.emptyCtaButton.click();

      // Verify form becomes visible
      await expect(form).toBeVisible();

      // Verify form inputs are present
      const titleInput = page.locator('[data-testid="backlog-empty-form-title"]');
      const prioritySelect = page.locator('[data-testid="backlog-empty-form-priority"]');
      const submitButton = page.locator('[data-testid="backlog-empty-form-submit"]');
      const cancelButton = page.locator('[data-testid="backlog-empty-form-cancel"]');

      await expect(titleInput).toBeVisible();
      await expect(prioritySelect).toBeVisible();
      await expect(submitButton).toBeVisible();
      await expect(cancelButton).toBeVisible();

      // Verify title input is focused
      const focusedElement = await page.evaluate(() => document.activeElement?.getAttribute('data-testid'));
      expect(focusedElement).toBe('backlog-empty-form-title');
    });

    test('e2e:backlog-empty-form-cancel - Clicking cancel hides form', async ({ page }) => {
      const backlogPage = new BacklogPage(page);

      // Check if backlog is empty. Fail loudly so the environment issue is surfaced.
      const tableRows = await backlogPage.getTableRows().count();
      if (tableRows > 0) {
        test.fail(true, `Empty-state test requires a clean backlog, but found ${tableRows} item(s).`);
        return;
      }

      // Open form
      await backlogPage.emptyCtaButton.click();
      const form = page.locator('[data-testid="backlog-empty-form"]');
      await expect(form).toBeVisible();

      // Cancel form
      await backlogPage.cancelEmptyStateForm();

      // Verify form is hidden
      await expect(form).not.toBeVisible();

      // Verify CTA button is refocused
      const focusedElement = await page.evaluate(() => document.activeElement?.getAttribute('data-testid'));
      expect(focusedElement).toBe('backlog-empty-cta-button');
    });

    test('e2e:backlog-empty-form-submit - Form requires title; submit button disabled when empty', async ({ page }) => {
      const backlogPage = new BacklogPage(page);

      // Check if backlog is empty. Fail loudly so the environment issue is surfaced.
      const tableRows = await backlogPage.getTableRows().count();
      if (tableRows > 0) {
        test.fail(true, `Empty-state test requires a clean backlog, but found ${tableRows} item(s).`);
        return;
      }

      // Open form
      await backlogPage.openEmptyStateForm();

      // Verify submit button is disabled when title is empty
      const submitButton = page.locator('[data-testid="backlog-empty-form-submit"]');
      await expect(submitButton).toHaveAttribute('aria-disabled', 'true');

      // Type a title
      await backlogPage.fillEmptyStateForm('Test Backlog Item');

      // Verify submit button is now enabled
      await expect(submitButton).not.toHaveAttribute('aria-disabled', 'true');
    });
  });

  test.describe('Item Creation and List', () => {
    test('e2e:backlog-create-item - Creating first item via empty state form', async ({ page }) => {
      const backlogPage = new BacklogPage(page);

      // Check if backlog is empty. Fail loudly so the environment issue is surfaced.
      const tableRows = await backlogPage.getTableRows().count();
      if (tableRows > 0) {
        test.fail(true, `This test requires a clean backlog, but found ${tableRows} item(s).`);
        return;
      }

      const itemTitle = `Test Item ${Date.now()}`;

      // Create item from empty state
      await backlogPage.createItemFromEmptyState(itemTitle, 2);

      // Verify item appears in the table
      const itemRow = backlogPage.getTableRows().filter({ hasText: itemTitle });
      await expect(itemRow.first()).toBeVisible();

      // Verify the item has expected content
      const titleCell = itemRow.locator('td').first();
      const text = await titleCell.textContent();
      expect(text).toContain(itemTitle);
    });

    test('e2e:backlog-item-card-visible - Created item is displayed in the list after empty state', async ({ page }) => {
      const backlogPage = new BacklogPage(page);

      // If already has items, verify they are visible
      const itemCards = await backlogPage.getTableRows().count();

      if (itemCards === 0) {
        // Create an item
        const itemTitle = `Test Item ${Date.now()}`;
        await backlogPage.createItemFromEmptyState(itemTitle, 3);

        // Verify it appears
        const createdRow = backlogPage.getTableRows().filter({ hasText: itemTitle });
        await expect(createdRow.first()).toBeVisible();
      } else {
        // Verify at least one item row is visible
        const firstRow = backlogPage.getTableRows().first();
        await expect(firstRow).toBeVisible();
      }
    });

    test('e2e:backlog-default-priority - Items created with default priority (P3)', async ({ page }) => {
      const backlogPage = new BacklogPage(page);

      const tableRows = await backlogPage.getTableRows().count();
      if (tableRows > 0) {
        test.fail(true, `This test requires a clean backlog, but found ${tableRows} item(s).`);
        return;
      }

      const itemTitle = `Test Item Priority ${Date.now()}`;

      // Create item without specifying priority (should default to P3)
      await backlogPage.openEmptyStateForm();
      await backlogPage.fillEmptyStateForm(itemTitle);
      await backlogPage.submitEmptyStateForm();

      // Wait for item to appear
      await page.waitForSelector('[data-testid="backlog-table-row"]');

      // Verify item appears in list
      const itemRow = backlogPage.getTableRows().filter({ hasText: itemTitle });
      await expect(itemRow.first()).toBeVisible();

      // Verify priority badge shows P3
      const priorityBadge = itemRow.first().locator('[data-testid="priority-badge"]');
      const priorityText = await priorityBadge.textContent();
      expect(priorityText?.trim()).toMatch(/P3/);
    });
  });

  test.describe('Filter Zero State', () => {
    test('e2e:backlog-filter-zero-state - No items match filters shows empty message', async ({ page }) => {
      const backlogPage = new BacklogPage(page);

      // Get item count before filtering
      const beforeCount = await backlogPage.getTableRows().count();

      if (beforeCount === 0) {
        // Need items to filter; create one
        await backlogPage.createItemFromEmptyState(`Test Item ${Date.now()}`, 1);
      }

      // Apply a filter that should have no matches (if all items are P1, filter for P5)
      // First, let's check what priorities exist
      const priorityBadges = await page.locator('[data-testid="priority-badge"]').allTextContents();
      const existingPriorities = new Set(priorityBadges);

      let targetPriority: number | null = null;
      for (let p = 1; p <= 5; p++) {
        if (!existingPriorities.has(`P${p}`)) {
          targetPriority = p;
          break;
        }
      }

      if (targetPriority === null) {
        // All priorities exist, so filter strategy won't work; skip
        test.skip();
      }

      // Apply filter for non-existent priority
      await backlogPage.applyPriorityFilter(targetPriority!);

      // Wait for filter zero state to appear (replaces waitForTimeout).
      await expect(backlogPage.filterZeroState).toBeVisible();

      // Verify filter zero state appears
      await expect(backlogPage.filterZeroState).toBeVisible();

      // Verify clear filters button is present
      await expect(backlogPage.clearFiltersButton).toBeVisible();

      // Verify no table rows are shown
      const filteredRows = await backlogPage.getTableRows().count();
      expect(filteredRows).toBe(0);
    });

    test('e2e:backlog-clear-filters - Clear filters button resets filters', async ({ page }) => {
      const backlogPage = new BacklogPage(page);

      const beforeCount = await backlogPage.getTableRows().count();

      if (beforeCount === 0) {
        // Create an item first
        await backlogPage.createItemFromEmptyState(`Test Item ${Date.now()}`, 2);
      }

      // Apply a status filter
      await backlogPage.applyStatusFilter('done');

      // If filter resulted in zero items, proceed to clear
      const filteredCount = await backlogPage.getTableRows().count();
      if (filteredCount === 0) {
        // Click clear filters
        await backlogPage.clearAllFilters();

        // Wait for items to reappear after clearing filters.
        await expect(backlogPage.getTableRows().first()).toBeVisible();

        // Verify items reappear
        const clearedCount = await backlogPage.getTableRows().count();
        expect(clearedCount).toBeGreaterThan(0);
      }
    });
  });

  test.describe('Page Navigation', () => {
    test('e2e:backlog-page-loads - Backlog page loads and is accessible', async ({ page }) => {
      const backlogPage = new BacklogPage(page);

      // Verify page title
      await expect(page).toHaveTitle(/Stapler Squad/);

      // Verify page content is present
      await expect(backlogPage.pageContent).toBeVisible();

      // Verify navigation elements exist
      await expect(backlogPage.newItemButton).toBeVisible();
      await expect(backlogPage.searchInput).toBeVisible();
    });

    test('e2e:backlog-search-input-accessible - Search input is accessible and functional', async ({ page }) => {
      const backlogPage = new BacklogPage(page);

      // Verify search input is visible
      await expect(backlogPage.searchInput).toBeVisible();

      // Type in search
      await backlogPage.searchItems('test query');

      // Verify value is set
      await expect(backlogPage.searchInput).toHaveValue('test query');

      // Clear search
      await backlogPage.searchInput.clear();
      await expect(backlogPage.searchInput).toHaveValue('');
    });
  });

  test.describe('Status Transitions', () => {
    test('e2e:backlog-item-appears-in-list-after-creation - Item created via "+ New Item" button appears in list', async ({ page }) => {
      const backlogPage = new BacklogPage(page);

      const itemTitle = `New Item Button Test ${Date.now()}`;

      // "+ New Item" button must be visible regardless of empty/non-empty state
      await expect(backlogPage.newItemButton).toBeVisible();

      // Open the new-item modal
      await backlogPage.openNewItemForm();

      // Verify the modal is visible with the form
      const modal = page.locator('[data-testid="backlog-form-modal"]');
      await expect(modal).toBeVisible();

      const titleInput = page.locator('[data-testid="backlog-title-input"]');
      await expect(titleInput).toBeVisible();

      // Fill and submit the form
      await backlogPage.fillNewItemForm(itemTitle);
      await backlogPage.submitNewItemForm();

      // Modal should close
      await expect(modal).not.toBeVisible();

      // Item must appear in the list
      const itemRow = backlogPage.getTableRows().filter({ hasText: itemTitle });
      await expect(itemRow.first()).toBeVisible();
    });

    test('e2e:backlog-transition-idea-to-ready - Item created as idea can be transitioned to ready via detail pane', async ({ page }) => {
      const backlogPage = new BacklogPage(page);

      const itemTitle = `Transition Test ${Date.now()}`;

      // Create the item with one acceptance criterion so "Mark Ready" is enabled
      // We must use the new-item modal form which supports adding AC criteria
      await backlogPage.openNewItemForm();
      await backlogPage.fillNewItemForm(itemTitle, {
        addAcCriterion: 'At least one criterion to enable Mark Ready',
      });
      await backlogPage.submitNewItemForm();

      // Wait for item to appear in list
      const itemRow = backlogPage.getTableRows().filter({ hasText: itemTitle });
      await expect(itemRow.first()).toBeVisible();

      // Verify initial status is "Idea"
      const statusBadge = backlogPage.getTableRowStatusBadge(itemTitle);
      await expect(statusBadge).toHaveAttribute('aria-label', 'Status: Idea');

      // Open the detail pane by clicking the row
      await backlogPage.openItemDetail(itemTitle);

      // Detail pane must be visible
      await expect(backlogPage.getItemDetailPane()).toBeVisible();

      // The "Mark Ready" button must be enabled (item has an AC criterion)
      const markReadyBtn = page.locator('[data-testid="backlog-action-mark-ready"]');
      await expect(markReadyBtn).toBeVisible();
      await expect(markReadyBtn).not.toBeDisabled();

      // Click "Mark Ready"
      await markReadyBtn.click();

      // Status badge in the detail pane must update to "Ready"
      const detailStatus = backlogPage.getDetailStatusBadge();
      await expect(detailStatus).toHaveAttribute('aria-label', 'Status: Ready', { timeout: 10000 });

      // The status badge in the list row must also show "Ready" after closing the detail pane
      await backlogPage.closeItemDetail();

      // Re-check the row (it may have been re-rendered after the transition)
      const updatedStatusBadge = backlogPage.getTableRowStatusBadge(itemTitle);
      await expect(updatedStatusBadge).toHaveAttribute('aria-label', 'Status: Ready', { timeout: 10000 });
    });

    test('e2e:backlog-suggest-next-item - Suggest Next feature (not yet exposed in UI)', async () => {
      // The SuggestNextItem RPC exists in the backend
      // (gen/proto/go/session/v1/sessionv1connect/backlog.connect.go) but
      // there is currently no "Suggest Next" button or data-testid in the
      // frontend UI (web-app/src/app/backlog/page.tsx). This test is marked
      // fixme until the feature is surfaced in the UI.
      test.fixme(true, 'SuggestNextItem RPC is implemented but has no UI button yet — add data-testid="backlog-suggest-next-button" and implement the test once the feature is exposed');
    });
  });
});
