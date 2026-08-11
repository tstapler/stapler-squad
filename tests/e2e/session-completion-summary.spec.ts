// @feature session-summary-tab, session-summary-standalone-route
import { test, expect } from "@playwright/test";
import { SessionsPage } from "./pages/SessionsPage";
import { SessionDetailPage } from "./pages/SessionDetailPage";

// Deterministic trivial-session fallback narrative — narrativeFallbackTrivial
// constant in session/session_summary_service.go. isTrivialSession() (empty
// diff, zero decisions, duration < 30s) selects this instead of calling the
// LLM, so a session that ends almost immediately after creation reaches
// READY quickly and reproducibly.
const TRIVIAL_NARRATIVE = "This session ended before any work was recorded.";

test.describe("session-completion-summary", () => {
  test.beforeEach(async ({ context }) => {
    // Established e2e clipboard-testing pattern — see
    // backlog-item-id-deep-link.spec.ts — required so navigator.clipboard
    // read/writeText work under Playwright's headless Chromium.
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  });

  test(
    "session-completion-summary_should_showTrivialFallbackAndCopyMarkdown_When_oneOffSessionEndsNaturally",
    async ({ page }) => {
      const sessionsPage = new SessionsPage(page);
      const detail = new SessionDetailPage(page);

      // Pre-seed the first-visit onboarding dialog as dismissed so it doesn't
      // intercept clicks later in the flow (same pattern as session-notes.spec.ts,
      // ci-status-badge.spec.ts).
      await page.addInitScript(() => {
        localStorage.setItem("stapler-squad:onboarded", "true");
      });

      await sessionsPage.goto();
      await expect(sessionsPage.searchInput).toBeVisible({ timeout: 15000 });

      // --- Create a trivial one-off session via the UI. Program is set to
      // "bash" (the omnibar's "Terminal" option) instead of the default AI
      // CLI so the session's lifecycle is fully controllable from the test:
      // a plain shell exits deterministically on `exit`, whereas a real
      // Claude Code process would not exit on its own. ---
      await sessionsPage.newSessionButton.click();
      await page.getByRole("radio", { name: /temporary \(no git\)/i }).click();

      const sessionTitle = `e2e-summary-${Date.now()}`;
      await page.getByLabel("Session Name").fill(sessionTitle);

      await page.getByText("Advanced Options").click();
      await page.getByLabel("Program", { exact: true }).selectOption("bash");

      // --- Regression check for the modal-clipping bug: with Advanced Options
      // expanded, the footer can exceed the viewport unless .modal scrolls
      // (Omnibar.css.ts). Assert the submit button is actually within the
      // viewport bounds after scrolling, not just DOM-attached/"visible". ---
      const submitButton = page.getByTestId("omnibar-footer-submit");
      await submitButton.scrollIntoViewIfNeeded();
      const viewport = page.viewportSize();
      await expect(async () => {
        const box = await submitButton.boundingBox();
        expect(box).not.toBeNull();
        expect(box!.y + box!.height).toBeLessThanOrEqual(viewport!.height);
        expect(box!.y).toBeGreaterThanOrEqual(0);
      }).toPass({ timeout: 5000 });

      const createRequest = page.waitForRequest(
        (req) => req.url().includes("CreateSession") && req.method() === "POST",
      );
      await submitButton.click();
      await createRequest;

      // OmnibarContext's handleCreateSession navigates to /?session=<id> on
      // success — confirms creation succeeded without needing to parse the
      // CreateSession response body for the new session's id.
      await page.waitForURL(/[?&]session=/, { timeout: 15000 });

      // --- Stop it via the UI: focus the Terminal tab and type `exit` +
      // Enter, ending the plain-shell process naturally. session/instance.go's
      // instanceOnExitCallback fires EventExited on this exit, which
      // session_summary_listener.go handles identically to an explicit stop
      // (ADR-002) — dispatching SessionSummaryGenerator.GenerateAndPersist. ---
      await detail.getTerminalTab().click();
      await expect(detail.getTerminalToolbarToggle()).toBeVisible({ timeout: 10000 });

      await detail.getTerminalPanel().click();
      await page.keyboard.type("exit");
      await page.keyboard.press("Enter");

      // --- Session reaches STOPPED (terminal) status → Summary tab enables. ---
      const summaryTab = detail.getSummaryTab();
      await expect(summaryTab).toBeEnabled({ timeout: 20000 });
      await summaryTab.click();

      // --- GENERATING → READY. Trivial session takes the deterministic
      // fallback-narrative path with no LLM call, so this should resolve
      // well within the 2s poll interval on a healthy server. ---
      const markdownBody = detail.getSummaryMarkdownBody();
      await expect(markdownBody).toBeVisible({ timeout: 20000 });
      await expect(markdownBody).toContainText(TRIVIAL_NARRATIVE);

      // --- Copy as Markdown: assert both the actual clipboard content and
      // the aria-live success announcement SessionSummaryPanel.tsx emits. ---
      await detail.getSummaryCopyButton().click();

      const liveRegion = detail.getSummaryLiveRegion();
      await expect(liveRegion).toHaveText("Summary copied to clipboard.");

      const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
      expect(clipboardText).toContain(TRIVIAL_NARRATIVE);
    },
  );
});
