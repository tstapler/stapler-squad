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

      // Pre-seed the first-visit onboarding dialog as dismissed so it doesn't
      // intercept clicks later in the flow (same pattern as ci-status-badge.spec.ts).
      await page.addInitScript(() => {
        localStorage.setItem("stapler-squad:onboarded", "true");
      });

      await sessionsPage.goto();
      await expect(sessionsPage.searchInput).toBeVisible({ timeout: 15000 });

      // Create a trivial one-off session (no worktree/program lifecycle needed
      // to exercise the note panel).
      await sessionsPage.newSessionButton.click();
      await page.getByRole("radio", { name: /temporary \(no git\)/i }).click();

      const sessionTitle = `e2e-notes-${Date.now()}`;
      await page.getByLabel("Session Name").fill(sessionTitle);

      const createRequest = page.waitForRequest(
        (req) => req.url().includes("CreateSession") && req.method() === "POST",
      );
      // Exact "Create Session" (not the broader /create|start/i) to avoid matching
      // the omnibar's own "Create new session" trigger button; `.first()` because
      // the creation panel renders a submit button in both its mobile and desktop
      // layouts simultaneously (see `.claude/rules/feature-testing-registry.md`).
      await page.getByRole("button", { name: "Create Session", exact: true }).first().click();
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

      // The sidebar list (same page, no navigation) already reflects the badge
      // immediately after save — no reload needed.
      await expect(sessionsPage.getSessionCard(sessionTitle).getByTestId("badge-has-note")).toBeVisible({ timeout: 10000 });

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
