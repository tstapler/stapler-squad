// @feature session-board-view, session:update
//
// E2E coverage for project_plans/board-kanban-view — the List/Board toggle and the
// kanban-style Board view (implementation/plan.md Phase 8, Epic 8.2).
//
// The toggle + column-rendering + keyboard-shortcut assertions need no session fixture (an
// empty board still renders its 4 column shells with "0" count badges). The mutation tests
// below use SessionClient.createIdleSession — the same fast CreateSession-RPC-backed fixture
// already used by tests/e2e/input-drop-badge.spec.ts and backlog-session-steer.spec.ts — rather
// than a raw pointer-drag: a real dnd-kit PointerSensor drag has no native HTML5 drag events
// for Playwright's dragTo() to hook, and simulating its activation-constraint timing directly
// would be fragile. MoveToMenu is not a lesser substitute here — SessionBoard.tsx's
// attemptColumnMove is the single function both a completed drag and a MoveToMenu selection
// call (Phase 4's explicit design), so this exercises the identical mutation path a drag would.
// The dnd-kit onDragEnd handler itself already has direct Jest coverage
// (SessionBoard.dragdrop.test.tsx).
import { test, expect } from "@playwright/test";
import { SessionClient } from "./helpers/session-client";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

// The board's grouping-strategy select persists to the same localStorage key SessionList
// uses (Phase 6's cross-view persistence). A shared test server can carry a prior run's
// (or SessionList's own default) "category" value into a fresh page, which renders one
// swimlane row per category and makes "board-column-running" etc. match once per row —
// pin it to the flat, single-row layout so column locators stay unambiguous.
async function ensureFlatBoard(page: import("@playwright/test").Page) {
  await page.getByTestId("board-grouping-select").selectOption("none");
}

test.describe("session-board-view", () => {
  test("session-board-view_should_toggleToBoardView_When_boardButtonClicked", async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });

    const boardToggle = page.getByTestId("session-view-mode-board");
    await expect(boardToggle).toBeVisible({ timeout: 10000 });
    await expect(boardToggle).toHaveAttribute("aria-pressed", "false");

    await boardToggle.click();

    await expect(boardToggle).toHaveAttribute("aria-pressed", "true");
    await expect(page.getByTestId("session-view-mode-list")).toHaveAttribute("aria-pressed", "false");
  });

  test("session-board-view_should_renderFourColumnsWithCountBadges_When_boardViewActive", async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
    await page.getByTestId("session-view-mode-board").click();
    await ensureFlatBoard(page);

    for (const key of ["running", "needs_review", "paused", "complete"]) {
      const column = page.getByTestId(`board-column-${key}`);
      await expect(column).toBeVisible({ timeout: 10000 });
      // Count is announced as "N sessions" for accessibility (backlog AC2), not a bare number.
      await expect(column.getByLabel(/\d+ sessions?/)).toBeVisible();
    }
  });

  test("session-board-view_should_toggleBackToList_When_bKeyboardShortcutPressed", async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
    const boardToggle = page.getByTestId("session-view-mode-board");
    const listToggle = page.getByTestId("session-view-mode-list");

    await boardToggle.click();
    await expect(boardToggle).toHaveAttribute("aria-pressed", "true");

    await page.locator("body").press("b");
    await expect(listToggle).toHaveAttribute("aria-pressed", "true");
    await expect(boardToggle).toHaveAttribute("aria-pressed", "false");
  });

  test("session-board-view_should_moveCardAndUpdateSessionStatus_When_movedViaMoveToMenu", async ({ page }) => {
    const client = new SessionClient(BASE_URL);
    const title = `e2e-board-move-${Date.now()}`;
    const session = await client.createIdleSession(title, "/tmp");

    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
    await page.getByTestId("session-view-mode-board").click();
    await ensureFlatBoard(page);

    const card = page.locator(`[data-testid="board-card"][data-session-id="${session.id}"]`);
    await expect(page.getByTestId("board-column-running")).toContainText(title);

    await card.getByTestId("move-to-menu-trigger").click();
    const menu = page.getByTestId("move-to-menu");
    await expect(menu).toBeVisible();
    await menu.getByRole("menuitem", { name: "Paused" }).click();

    // Optimistic move: the card re-renders under Paused before the RPC round-trip resolves.
    await expect(page.getByTestId("board-column-paused")).toContainText(title);
    await expect(page.getByTestId("board-column-running")).not.toContainText(title);

    // Verify server-side persistence via a read-back, not just DOM state.
    await expect
      .poll(async () => (await client.getSession(session.id)).status, { timeout: 10000 })
      .toBe("SESSION_STATUS_PAUSED");
  });

  test("session-board-view_should_requireConfirmationAndLeaveSessionUnchanged_When_moveToCompleteCancelled", async ({
    page,
  }) => {
    const client = new SessionClient(BASE_URL);
    const title = `e2e-board-complete-cancel-${Date.now()}`;
    const session = await client.createIdleSession(title, "/tmp");

    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
    await page.getByTestId("session-view-mode-board").click();
    await ensureFlatBoard(page);

    const card = page.locator(`[data-testid="board-card"][data-session-id="${session.id}"]`);
    await card.getByTestId("move-to-menu-trigger").click();
    await page.getByTestId("move-to-menu").getByRole("menuitem", { name: "Complete" }).click();

    // Scoped by name: the page can also have an unrelated "Notification Panel" dialog open.
    const dialog = page.getByRole("dialog", { name: /^Stop / });
    await expect(dialog).toBeVisible();
    await page.getByTestId("board-complete-confirm-cancel").click();
    await expect(dialog).not.toBeVisible();

    // Cancelling must leave the session untouched -- still in Running, tmux pane still alive
    // (backlog AC12: the stop mutation must not fire before confirmation).
    await expect(page.getByTestId("board-column-running")).toContainText(title);
    const stillActive = await client.getSession(session.id);
    expect(stillActive.status).not.toBe("SESSION_STATUS_STOPPED");
  });
});
