// @feature session:vcs-diff, session:get-diff
/**
 * E2E coverage for the VCS/Diff IntelliJ-parity features: hunk navigation, split view,
 * inline per-file diff in the VCS tab, and context expansion.
 *
 * A real session is created via the API (so the session list/detail UI is real), but
 * GetSessionDiff and GetVCSStatus are fulfilled with a fixed two-file diff — the same
 * documented precedent as ci-status-badge.spec.ts (a bash session in /tmp has no git
 * worktree, so the server would return an empty diff). GetSessionDiff mocks echo the
 * request's contextLines back through a counter so the expand tests can assert the
 * frontend actually requested more context.
 */

import { test, expect, Page } from "@playwright/test";
import { SessionClient } from "./helpers/session-client";
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();

const DIFF = [
  "diff --git a/src/alpha.go b/src/alpha.go",
  "--- a/src/alpha.go",
  "+++ b/src/alpha.go",
  "@@ -3,4 +3,3 @@",
  " ctx",
  "-old one",
  "-old two",
  "+new one",
  " ctx2",
  "@@ -30,2 +29,3 @@",
  " tail",
  "+added tail",
  " end",
  "diff --git a/src/beta.go b/src/beta.go",
  "--- a/src/beta.go",
  "+++ b/src/beta.go",
  "@@ -1,1 +1,1 @@",
  "-package a",
  "+package b",
  "",
].join("\n");

async function mockVcs(page: Page): Promise<{ contextLinesRequested: number[] }> {
  const seen = { contextLinesRequested: [] as number[] };
  await page.route("**/api/session.v1.SessionService/GetSessionDiff", async (route) => {
    const body = route.request().postDataJSON() as { contextLines?: number } | null;
    seen.contextLinesRequested.push(body?.contextLines ?? 0);
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ diffStats: { added: 3, removed: 3, content: DIFF } }),
    });
  });
  await page.route("**/api/session.v1.SessionService/GetVCSStatus", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        vcsStatus: {
          branch: "feat/e2e",
          isClean: false,
          unstagedFiles: [
            { path: "src/alpha.go", status: "FILE_STATUS_MODIFIED", additions: 2, deletions: 2 },
            { path: "src/beta.go", status: "FILE_STATUS_MODIFIED", additions: 1, deletions: 1 },
          ],
        },
      }),
    });
  });
  return seen;
}

async function openSessionTab(page: Page, title: string, tab: "Diff" | "VCS") {
  await page.addInitScript(() => {
    localStorage.setItem("stapler-squad:onboarded", "true");
  });
  await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
  const card = page.locator('[data-testid="session-card"], [data-testid="session-row"]').filter({ hasText: title });
  await expect(card).toBeVisible({ timeout: 10000 });
  await card.click();
  await page.getByRole("tab", { name: tab }).click();
}

test.describe("vcs-diff-parity", () => {
  let client: SessionClient;
  let title: string;

  test.beforeEach(async () => {
    client = new SessionClient(BASE_URL);
    title = `e2e-vcs-diff-${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
    await client.createSession({ title, path: "/tmp", program: "bash" });
  });

  test("n / p and the buttons move the active hunk across files and wrap", async ({ page }) => {
    await mockVcs(page);
    await openSessionTab(page, title, "Diff");

    const counter = page.getByTestId("diff-hunk-counter");
    await expect(counter).toHaveText("–/3");
    await expect(page.getByTestId("diff-pane")).toBeFocused();

    await page.keyboard.press("n");
    await expect(page.getByTestId("diff-hunk-0-0")).toHaveAttribute("data-active", "true");
    await expect(counter).toHaveText("1/3");

    await page.getByTestId("diff-next-hunk").click();
    await page.getByTestId("diff-next-hunk").click();
    await expect(page.getByTestId("diff-hunk-1-0")).toHaveAttribute("data-active", "true");

    await page.getByTestId("diff-next-hunk").click(); // wraps to the first hunk
    await expect(page.getByTestId("diff-hunk-0-0")).toHaveAttribute("data-active", "true");

    await page.getByTestId("diff-pane").focus();
    await page.keyboard.press("p"); // wraps backwards to the last hunk
    await expect(page.getByTestId("diff-hunk-1-0")).toHaveAttribute("data-active", "true");
  });

  test("Split shows old and new columns, aligned, and Unified restores the single column", async ({ page }) => {
    await mockVcs(page);
    await openSessionTab(page, title, "Diff");

    await page.getByTestId("diff-mode-split").click();
    const grid = page.getByTestId("diff-split-grid").first();
    await expect(grid).toBeVisible();
    await expect(grid.locator('[data-side="left"]').nth(1)).toHaveText("old one");
    await expect(grid.locator('[data-side="right"]').nth(1)).toHaveText("new one");
    // 2 deletes vs 1 add: one placeholder on the right, none on the left.
    await expect(grid.locator('[data-testid="diff-split-placeholder"][data-side="right"]')).toHaveCount(1);

    await page.getByTestId("diff-mode-unified").click();
    await expect(page.getByTestId("diff-split-grid")).toHaveCount(0);

    await page.getByTestId("diff-pane").focus();
    await page.keyboard.press("s");
    await expect(page.getByTestId("diff-split-grid").first()).toBeVisible();
  });

  test("the ? overlay lists the diff shortcuts", async ({ page }) => {
    await mockVcs(page);
    await openSessionTab(page, title, "Diff");

    await page.getByTestId("diff-pane").focus();
    await page.keyboard.press("?");
    const diffSection = page.getByRole("region", { name: "Diff View" });
    await expect(diffSection.getByText("Next change")).toBeVisible();
    await expect(diffSection.getByText("Previous change")).toBeVisible();
    await expect(diffSection.getByText("Toggle split / unified view")).toBeVisible();
  });

  test("clicking a file in the VCS tab shows its diff inline and Back restores the list", async ({ page }) => {
    await mockVcs(page);
    await openSessionTab(page, title, "VCS");

    await page.getByRole("button", { name: /src\/beta\.go/ }).click();
    const view = page.getByTestId("file-diff-view");
    await expect(view).toBeVisible();
    await expect(view.getByText("+package b")).toBeVisible();
    await expect(view.getByText("src/alpha.go")).toHaveCount(0); // scoped to this file only
    await expect(page.getByRole("tab", { name: "VCS" })).toHaveAttribute("aria-selected", "true"); // no tab switch

    await page.getByTestId("file-diff-back").click();
    await expect(view).toHaveCount(0);
    await expect(page.getByRole("button", { name: /src\/alpha\.go/ })).toBeVisible();
  });

  test("Show more lines and Expand all re-request the diff with more context", async ({ page }) => {
    const seen = await mockVcs(page);
    await openSessionTab(page, title, "Diff");

    await expect(page.getByTestId("diff-pane")).toBeVisible();
    expect(seen.contextLinesRequested.every((n) => n === 0)).toBe(true);

    await page.getByTestId("diff-expand-above").first().click();
    await expect.poll(() => seen.contextLinesRequested.at(-1)).toBe(13);

    await page.getByTestId("diff-expand-all").click();
    await expect.poll(() => seen.contextLinesRequested.at(-1)).toBe(-1);
    await expect(page.getByTestId("diff-expand-all")).toHaveText("Collapse");
  });
});
