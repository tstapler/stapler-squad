// @feature session:artifacts
import { test, expect } from "@playwright/test";

test.describe("session-artifacts", () => {
  test("T-E2E-ARTIFACTS-001 Artifacts tab visible in session detail", async ({
    page,
  }) => {
    await page.goto("http://localhost:8544");

    // Navigate to first available session — skip test if none present.
    const firstSession = page.getByRole("listitem").first();
    if (!(await firstSession.isVisible())) {
      test.skip(true, "No sessions available in test environment");
      return;
    }
    await firstSession.click();

    // Click Artifacts tab
    await page.getByRole("tab", { name: "Artifacts" }).click();

    // Assert panel is present (any of the three sub-states)
    await expect(
      page.getByText(
        /Extraction pending|No artifacts found|Pull Requests|Commits|External URLs/
      )
    ).toBeVisible();
  });
});
