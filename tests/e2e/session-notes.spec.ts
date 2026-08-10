// @feature session:update, session-notes-panel, session-notes-card-indicator
import { test, expect } from "@playwright/test";
import { SessionsPage } from "./pages/SessionsPage";
import { SessionDetailPage } from "./pages/SessionDetailPage";

test.describe("session-notes", () => {
  test(
    "should_PersistNoteAndBadgeAcrossPageReload",
    async ({ page }) => {
      const sessionsPage = new SessionsPage(page);
      const detail = new SessionDetailPage(page);

      await sessionsPage.goto();
      await expect(sessionsPage.searchInput).toBeVisible({ timeout: 15000 });

      // Create a trivial one-off session (no worktree/program lifecycle needed
      // to exercise the note panel).
      await sessionsPage.newSessionButton.click();
      await page.getByRole("radio", { name: /one.off/i }).click();

      const sessionTitle = `e2e-notes-${Date.now()}`;
      await page.getByLabel("Session Name").fill(sessionTitle);

      await page.getByText("Advanced Options").click();
      await page.getByLabel("Program").selectOption("bash");

      const createRequest = page.waitForRequest(
        (req) => req.url().includes("CreateSession") && req.method() === "POST",
      );
      await page.getByRole("button", { name: /create|start/i }).click();
      await createRequest;
      await page.waitForURL(/[?&]session=/, { timeout: 15000 });

      // --- Open the Info tab and attach a note. ---
      await detail.getInfoTab().click();
      await expect(detail.getNotePanel()).toBeVisible({ timeout: 10000 });
      await detail.getNoteAddButton().click();

      const noteText = "**Blocked** — spike, don't merge";
      await detail.getNoteTextarea().fill(noteText);

      const updateRequest = page.waitForRequest(
        (req) => req.url().includes("UpdateSession") && req.method() === "POST",
      );
      await detail.getNoteSaveButton().click();
      await updateRequest;

      // Renders as markdown in read mode.
      await expect(detail.getNoteRenderedBody()).toBeVisible({ timeout: 10000 });
      await expect(detail.getNoteRenderedBody().locator("strong")).toHaveText("Blocked");

      // --- Reload the page (proxy for a server restart — both discard the
      // in-memory Instance and rebuild it from the same ent/SQLite-backed
      // LoadInstances() read path). ---
      await page.reload({ waitUntil: "domcontentloaded" });
      await detail.getInfoTab().click();
      await expect(detail.getNoteRenderedBody()).toBeVisible({ timeout: 10000 });
      await expect(detail.getNoteRenderedBody().locator("strong")).toHaveText("Blocked");

      // --- The SessionCard badge is visible from the list view without
      // opening the session. ---
      await sessionsPage.goto();
      await expect(sessionsPage.searchInput).toBeVisible({ timeout: 15000 });
      await sessionsPage.searchInput.fill(sessionTitle);
      const card = sessionsPage.getSessionCard(sessionTitle);
      await expect(card.getByTestId("badge-has-note")).toBeVisible({ timeout: 10000 });
    },
  );
});
