// @feature launcher_presets:get, launcher-presets-list
/**
 * E2E coverage for the launcher presets feature (project_plans/launcher-presets).
 *
 * launcher-presets.json is seeded directly onto disk in the shared test server's isolated
 * data directory (TEST_SERVER_TESTDIR, exported by global-setup.ts) rather than through an
 * RPC — GetLauncherPresets re-reads and re-validates the file fresh on every call (no
 * caching), so writing the fixture immediately before opening the Omnibar is sufficient to
 * prove Success Criterion 1 ("without restarting the server") without any server restart.
 *
 * The "true" program (a real, harmless no-op binary already on every CI runner's PATH) is
 * used as the preset's argv[0] to avoid depending on ssh/codex being installed in CI.
 */

import * as fs from "fs";
import * as path from "path";
import { test, expect } from "@playwright/test";
import { SessionClient } from "./helpers/session-client";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";
const TEST_DIR = process.env.TEST_SERVER_TESTDIR;

function writePresetsFixture(presets: unknown[]) {
  if (!TEST_DIR) {
    throw new Error("TEST_SERVER_TESTDIR is not set — global-setup.ts must export it");
  }
  fs.writeFileSync(
    path.join(TEST_DIR, "launcher-presets.json"),
    JSON.stringify({ version: 1, presets })
  );
}

test.describe("launcher-presets", () => {
  test.afterEach(() => {
    // Reset to no presets so later tests/specs in the shared server aren't affected.
    if (TEST_DIR) {
      fs.rmSync(path.join(TEST_DIR, "launcher-presets.json"), { force: true });
    }
  });

  test("launcher-presets_should_CreateSessionWithPresetProgram_When_RowClickedAndFormSubmitted", async ({ page }) => {
    writePresetsFixture([
      { id: "e2e-true", label: "E2E No-Op", argv: ["true"] },
    ]);

    // Pre-seed the first-visit onboarding dialog as dismissed so it doesn't intercept clicks
    // (same pattern as ci-status-badge.spec.ts / backlog-pipeline-mode.spec.ts).
    await page.addInitScript(() => {
      localStorage.setItem("stapler-squad:onboarded", "true");
    });
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded", timeout: 10000 });
    await page.waitForSelector('[data-testid="session-card"], [data-testid="session-row"]', { timeout: 15000 });

    // Ctrl+Shift+K opens the Omnibar directly in creation mode, where OmnibarCreationPanel
    // (and its Presets section) is rendered.
    await page.keyboard.press("Control+Shift+K");
    await page.waitForSelector('[aria-label="Session source input"]', { timeout: 5000 });

    // "Temporary (no git)" needs only a session name — no repo path/detection required —
    // the simplest session type for exercising the preset-select-then-submit flow.
    await page.getByRole("radio", { name: /Temporary \(no git\)/ }).click();

    const sessionTitle = `preset-e2e-${Date.now()}`;
    await page.getByLabel("Session Name *").fill(sessionTitle);

    const presetRow = page.getByTestId("preset-row").filter({ hasText: "E2E No-Op" });
    await expect(presetRow).toBeVisible({ timeout: 10000 });
    await presetRow.click();

    const chip = page.getByTestId("preset-resolution-chip");
    await expect(chip).toBeVisible();
    await expect(chip).toContainText("E2E No-Op");
    await expect(chip).toContainText("true");

    await page.getByTestId("omnibar-footer-submit").click();
    await page.waitForURL(/[?&]session=/, { timeout: 15000 });

    const sessionId = new URL(page.url()).searchParams.get("session");
    expect(sessionId).toBeTruthy();

    const client = new SessionClient(BASE_URL);
    const session = await client.getSession(sessionId!);
    expect(session.program).toBe("true");
    expect(session.title).toBe(sessionTitle);
  });

  test("launcher-presets_should_ShowNotFoundWithTypedId_When_PresetShorthandIdUnknown", async ({ page }) => {
    writePresetsFixture([{ id: "known-one", label: "Known One", argv: ["true"] }]);

    await page.addInitScript(() => {
      localStorage.setItem("stapler-squad:onboarded", "true");
    });
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded", timeout: 10000 });
    await page.waitForSelector('[data-testid="session-card"], [data-testid="session-row"]', { timeout: 15000 });

    await page.keyboard.press("Control+Shift+K");
    await page.waitForSelector('[aria-label="Session source input"]', { timeout: 5000 });

    const input = page.locator('[aria-label="Session source input"]');
    await input.fill("preset:doesnotexist");

    const alert = page.getByTestId("preset-not-found");
    await expect(alert).toBeVisible({ timeout: 5000 });
    await expect(alert).toContainText("preset:doesnotexist");

    // No dead end: input stays editable, the user can correct it.
    await expect(input).toBeEditable();
  });
});
