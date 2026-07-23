import { Page, Locator, expect } from '@playwright/test';

export class BacklogPage {
  readonly page: Page;
  readonly newItemButton: Locator;
  readonly searchInput: Locator;
  readonly pageTitle: Locator;
  readonly pageContent: Locator;
  readonly emptyState: Locator;
  readonly emptyHeadline: Locator;
  readonly emptyCtaButton: Locator;
  readonly lifecycleDiagram: Locator;
  readonly filterZeroState: Locator;
  readonly clearFiltersButton: Locator;
  readonly tourButton: Locator;
  readonly tourModal: Locator;

  constructor(page: Page) {
    this.page = page;
    this.newItemButton = page.locator('[data-testid="backlog-new-item-button"]');
    this.searchInput = page.locator('[data-testid="backlog-search-input"]');
    this.pageTitle = page.locator('[data-testid="backlog-page"] h1');
    this.pageContent = page.locator('[data-testid="backlog-page"]');
    this.emptyState = page.locator('[data-testid="backlog-empty-state"]');
    this.emptyHeadline = page.locator('[data-testid="backlog-empty-headline"]');
    this.emptyCtaButton = page.locator('[data-testid="backlog-empty-cta-button"]');
    this.lifecycleDiagram = page.locator('[data-testid="backlog-lifecycle-diagram"]');
    this.filterZeroState = page.locator('[data-testid="backlog-filter-zero-state"]');
    this.clearFiltersButton = page.locator('[data-testid="backlog-clear-filters-button"]');
    this.tourButton = page.locator('[data-testid="backlog-tour-button"]');
    this.tourModal = page.locator('[data-testid="backlog-tour-modal"]');
  }

  async goto() {
    await this.page.goto('/backlog');
    await this.page.waitForLoadState('domcontentloaded');
  }

  async waitForPageLoad() {
    await this.page.waitForSelector('[data-testid="backlog-page"]', { timeout: 10000 });
  }

  async waitForEmptyState() {
    await this.page.waitForSelector('[data-testid="backlog-empty-state"]', { timeout: 5000 });
  }

  async waitForItemCards() {
    await this.page.waitForSelector('[data-testid="backlog-table-row"]', { timeout: 5000 });
  }

  getItemCard(title: string): Locator {
    return this.page.locator('[data-testid="backlog-table-row"]').filter({ hasText: title });
  }

  getItemCards(): Locator {
    return this.page.locator('[data-testid="backlog-table-row"]');
  }

  getTableRows(): Locator {
    return this.page.locator('[data-testid="backlog-table-row"]');
  }

  // ---------------------------------------------------------------------------
  // Sort / group by repository
  // ---------------------------------------------------------------------------

  getRepositoryColumnHeader(): Locator {
    return this.page.locator('[data-testid="backlog-col-repo-path"]');
  }

  getGroupBySelect(): Locator {
    return this.page.locator('[data-testid="backlog-group-by-select"]');
  }

  async selectGroupBy(value: 'none' | 'repoPath') {
    await this.getGroupBySelect().selectOption(value);
  }

  getGroupHeaders(): Locator {
    return this.page.locator('[data-testid="backlog-group-header"]');
  }

  async getRowRepoPaths(): Promise<string[]> {
    return this.getTableRows().locator('[data-testid="backlog-repo-path-cell"]').allTextContents();
  }

  async openEmptyStateForm() {
    await this.emptyCtaButton.click();
    await this.page.waitForSelector('[data-testid="backlog-empty-form"]', { timeout: 5000 });
  }

  async fillEmptyStateForm(title: string, priority?: number) {
    const titleInput = this.page.locator('[data-testid="backlog-empty-form-title"]');
    await titleInput.fill(title);

    if (priority !== undefined) {
      const prioritySelect = this.page.locator('[data-testid="backlog-empty-form-priority"]');
      await prioritySelect.selectOption({ value: String(priority) });
    }
  }

  async submitEmptyStateForm() {
    const submitButton = this.page.locator('[data-testid="backlog-empty-form-submit"]');
    await submitButton.click();
  }

  async cancelEmptyStateForm() {
    const cancelButton = this.page.locator('[data-testid="backlog-empty-form-cancel"]');
    await cancelButton.click();
  }

  async createItemFromEmptyState(title: string, priority?: number) {
    await this.openEmptyStateForm();
    await this.fillEmptyStateForm(title, priority);
    await this.submitEmptyStateForm();
    // Wait for the item to appear in the list
    await this.page.waitForSelector('[data-testid="backlog-table-row"]', { timeout: 5000 });
  }

  getLifecycleNode(label: string): Locator {
    return this.page.locator(`[data-testid="backlog-lifecycle-node-${label}"]`);
  }

  getStatusFilterChip(status: string): Locator {
    return this.page.locator(`[data-testid="backlog-filter-status-${status}"]`);
  }

  getPriorityFilterChip(priority: number): Locator {
    return this.page.locator(`[data-testid="backlog-filter-priority-${priority}"]`);
  }

  async applyStatusFilter(status: string) {
    const filterChip = this.getStatusFilterChip(status);
    await filterChip.click();
  }

  async applyPriorityFilter(priority: number) {
    const filterChip = this.getPriorityFilterChip(priority);
    await filterChip.click();
  }

  async clearAllFilters() {
    if (await this.clearFiltersButton.isVisible({ timeout: 1000 }).catch(() => false)) {
      await this.clearFiltersButton.click();
    }
  }

  async searchItems(query: string) {
    await this.searchInput.fill(query);
  }

  // ---------------------------------------------------------------------------
  // New-item modal form (launched from backlog-new-item-button)
  // ---------------------------------------------------------------------------

  async openNewItemForm() {
    await this.newItemButton.click();
    await this.page.waitForSelector('[data-testid="backlog-form-modal"]', { timeout: 5000 });
  }

  async fillNewItemForm(title: string, options?: { priority?: number; addAcCriterion?: string; repoPath?: string }) {
    const titleInput = this.page.locator('[data-testid="backlog-title-input"]');
    await titleInput.fill(title);

    if (options?.priority !== undefined) {
      const prioritySelect = this.page.locator('[data-testid="backlog-priority-select"]');
      await prioritySelect.selectOption({ value: String(options.priority) });
    }

    if (options?.addAcCriterion) {
      const addBtn = this.page.locator('[data-testid="backlog-add-criterion"]');
      await addBtn.click();
      const criterionInput = this.page.locator('[data-testid="backlog-criterion-text-0"]');
      await criterionInput.fill(options.addAcCriterion);
    }

    if (options?.repoPath) {
      const repoPathInput = this.page.locator('[data-testid="backlog-repo-path-input"]');
      await repoPathInput.fill(options.repoPath);
    }
  }

  async submitNewItemForm() {
    const submitButton = this.page.locator('[data-testid="backlog-form-submit"]');
    await submitButton.click();
  }

  /**
   * Selects a pipeline mode option in the new/edit item form's radio group
   * (Epic 3.2 — BacklogItemForm.tsx). `slug` is the PipelineMode's slug; use
   * "default" for the built-in default option.
   */
  getPipelineModeOption(slug: string): Locator {
    return this.page.getByTestId(`backlog-pipeline-mode-${slug}`);
  }

  async selectPipelineMode(slug: string) {
    await this.getPipelineModeOption(slug).click();
  }

  async cancelNewItemForm() {
    const cancelButton = this.page.locator('[data-testid="backlog-form-cancel"]');
    await cancelButton.click();
  }

  async createItemViaNewItemButton(
    title: string,
    options?: { priority?: number; addAcCriterion?: string }
  ) {
    await this.openNewItemForm();
    await this.fillNewItemForm(title, options);
    await this.submitNewItemForm();
    // Wait for modal to close and item to appear
    await this.page.waitForSelector('[data-testid="backlog-form-modal"]', {
      state: 'hidden',
      timeout: 5000,
    });
  }

  // ---------------------------------------------------------------------------
  // Item detail pane
  // ---------------------------------------------------------------------------

  async openItemDetail(itemTitle: string) {
    const row = this.getTableRows().filter({ hasText: itemTitle });
    await row.first().click();
    await this.page.waitForSelector('[data-testid="backlog-item-detail"]', { timeout: 5000 });
  }

  getItemDetailPane(): Locator {
    return this.page.locator('[data-testid="backlog-item-detail"]');
  }

  getDetailStatusBadge(): Locator {
    // In the detail pane the status badge uses aria-label "Status: <label>"
    return this.page.locator('[data-testid="backlog-item-detail"] [aria-label^="Status:"]');
  }

  getTableRowStatusBadge(itemTitle: string): Locator {
    const row = this.getTableRows().filter({ hasText: itemTitle });
    return row.locator('[aria-label^="Status:"]');
  }

  async closeItemDetail() {
    const closeBtn = this.page.locator('[data-testid="backlog-detail-close"]');
    await closeBtn.click();
  }

  // ---------------------------------------------------------------------------
  // Status transition helpers
  // ---------------------------------------------------------------------------

  async transitionItemToReady(itemTitle: string) {
    // Open the detail pane for the item, then click "Mark Ready"
    await this.openItemDetail(itemTitle);
    const markReadyBtn = this.page.locator('[data-testid="backlog-action-mark-ready"]');
    await expect(markReadyBtn).toBeVisible();
    await markReadyBtn.click();
    // Wait for status to update in the detail pane
    await expect(this.getDetailStatusBadge()).toContainText('Ready', { timeout: 10000 });
  }

  // ---------------------------------------------------------------------------
  // backlog-event-driven-updates: live-update surfaces (list row, connection
  // indicator, board columns)
  // ---------------------------------------------------------------------------

  getConnectionIndicator(): Locator {
    return this.page.getByTestId('connection-indicator');
  }

  getRowById(itemId: string): Locator {
    return this.page.locator(`[data-testid="backlog-table-row"][data-item-id="${itemId}"]`);
  }

  async gotoBoard() {
    await this.page.goto('/backlog/board');
    await this.page.waitForSelector('[data-testid="backlog-board"]', { timeout: 15000 });
  }

  getColumn(status: string): Locator {
    return this.page.locator(`[data-testid="backlog-column-${status}"]`);
  }

  getCardInColumn(status: string, itemId: string): Locator {
    return this.getColumn(status).locator(`[data-item-id="${itemId}"]`);
  }

  getExitingCards(): Locator {
    return this.page.getByTestId('backlog-card-exiting');
  }
}
