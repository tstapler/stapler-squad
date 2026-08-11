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

// Every test here starts from a fresh browser context (Playwright's default),
// so localStorage is empty and useOnboarding.ts's 800ms timer would otherwise
// pop a full-viewport onboarding modal mid-test, intercepting clicks on
// whatever it happens to render over -- see repo-path-picker-parity.spec.ts,
// which established this pattern.
const ONBOARDED_KEY = "stapler-squad:onboarded";

test.describe("session-completion-summary", () => {
  test.beforeEach(async ({ context, page }) => {
    // Established e2e clipboard-testing pattern — see
    // backlog-item-id-deep-link.spec.ts — required so navigator.clipboard
    // read/writeText work under Playwright's headless Chromium.
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);

    await page.addInitScript((key) => {
      try {
        window.localStorage.setItem(key, "true");
      } catch {
        /* ignore */
      }
    }, ONBOARDED_KEY);
  });

  test(
    "session-completion-summary_should_showTrivialFallbackAndCopyMarkdown_When_oneOffSessionEndsNaturally",
    async ({ page }) => {
      // remain-on-exit means a plain in-pane `exit` does NOT fire the fast
      // control-mode exit path (session/instance.go's instanceOnExitCallback) --
      // only SessionHealthChecker's polling (session/health.go) detects the dead
      // pane. That poller ticks every sessionHealthCheckInterval=15s and requires
      // failureThreshold=2 consecutive dead-pane observations before acting, so
      // detection can legitimately take up to ~30s from a bad tick-phase
      // alignment. The default 30s Playwright test timeout doesn't leave enough
      // room for that plus the setup steps before it, so extend it.
      test.setTimeout(60_000);

      const sessionsPage = new SessionsPage(page);
      const detail = new SessionDetailPage(page);

      await sessionsPage.goto();
      await expect(sessionsPage.searchInput).toBeVisible({ timeout: 15000 });

      // --- Create a trivial one-off session via the UI. Program is set to
      // "bash" (the omnibar's "Terminal" option) instead of the default AI
      // CLI so the session's lifecycle is fully controllable from the test:
      // a plain shell exits deterministically on `exit`, whereas a real
      // Claude Code process would not exit on its own. ---
      await sessionsPage.newSessionButton.click();
      // SESSION_TYPES' one_off entry's visible label is "Temporary (no git)"
      // (OmnibarCreationPanel.tsx) -- not literally "one-off" anymore. See
      // session-notes.spec.ts for the same pattern.
      await page.getByRole("radio", { name: /temporary \(no git\)/i }).click();

      const sessionTitle = `e2e-summary-${Date.now()}`;
      await page.getByLabel("Session Name").fill(sessionTitle);

      await page.getByText("Advanced Options").click();
      // exact:true -- non-exact substring matching also matches session-row
      // aria-labels containing "program: <name>" (SessionsPage lists demo
      // sessions), making the plain getByLabel("Program") ambiguous.
      await page.getByLabel("Program", { exact: true }).selectOption("bash");

      // Collapse Advanced Options back: the expanded panel pushes the Create
      // Session footer button below the viewport with no internal scroll,
      // making it unclickable. formState.program persists across the toggle.
      await page.getByText("Advanced Options").click();

      const createRequest = page.waitForRequest(
        (req) => req.url().includes("CreateSession") && req.method() === "POST",
      );
      // Exact "Create Session" (not the broader /create|start/i) to avoid matching
      // the omnibar's own "Create new session" trigger button; `.first()` because
      // the creation panel renders a submit button in both its mobile and desktop
      // layouts simultaneously (see `.claude/rules/feature-testing-registry.md`
      // and session-notes.spec.ts's identical pattern).
      await page.getByRole("button", { name: "Create Session", exact: true }).last().click();
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

      // --- Session reaches STOPPED (terminal) status → Summary tab enables.
      // 40s comfortably covers the ~30s worst-case health-checker debounce
      // described above (see test.setTimeout call). ---
      const summaryTab = detail.getSummaryTab();
      await expect(summaryTab).toBeEnabled({ timeout: 40000 });
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
