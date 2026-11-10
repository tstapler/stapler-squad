// @feature session:artifacts
/**
 * Artifacts tab in a real browser (T8.2.2, T6.3.4): the five reliability states, one-click Rescan, the grouped
 * view (commit under the PR created before it, plan doc under the later PR, mentioned PRs collapsed), the
 * redaction marker, light/dark Axe per state and the WCAG 1.4.1 check under emulated achromatopsia.
 *
 * Scan results are injected into a real API-created session (helpers/multi-pr.ts): no transcript exists in the
 * isolated server. The 390px assertions are in artifacts-tab-mobile.spec.ts.
 */
import { test, expect } from "@playwright/test";
import { SessionClient } from "./helpers/session-client";
import { waitUntilSettled } from "./helpers/wait";
import { baseUrl } from "./helpers/base-url";
import { axeLightAndDark, visibleTexts, withAchromatopsia } from "./helpers/a11y";
import {
  artifactsJson,
  ciRecord,
  collectConsoleErrors,
  groupedRecords,
  prJson,
  prCreated,
  seedSessionFields,
  skipOnboarding,
  type ScanState,
} from "./helpers/multi-pr";
import { SessionDetailPage } from "./pages/SessionDetailPage";
import { enableBacklogFeatureFlag } from "./pages/StuckItemsPage";

const BASE_URL = baseUrl();
let client: SessionClient;
let consoleErrors: string[];
const created: string[] = [];

async function openTab(page: import("@playwright/test").Page, artifacts: Record<string, unknown>, pullRequests?: Record<string, unknown>[]) {
  const session = await client.createSession({ title: `e2e-artifacts-${Date.now()}`, path: "/tmp", program: "bash" });
  created.push(session.id);
  await waitUntilSettled(client, session.id);
  await seedSessionFields(page, { sessionId: session.id, artifacts, pullRequests });
  const detail = new SessionDetailPage(page);
  await detail.gotoSession(session.id);
  await detail.openArtifactsTab();
  return { detail, id: session.id };
}

test.describe("artifacts-tab", () => {
  test.use({ viewport: { width: 1280, height: 1000 } });

  test.beforeEach(async ({ page, request }) => {
    // Without the backlog flag every page load logs a "feature backlog is not enabled" console error.
    await enableBacklogFeatureFlag(request);
    client = new SessionClient(BASE_URL);
    consoleErrors = collectConsoleErrors(page);
    await skipOnboarding(page);
  });

  test.afterEach(async () => {
    for (const id of created.splice(0)) await client.deleteSession(id, true).catch(() => undefined);
    expect(consoleErrors, "no console or page errors").toEqual([]);
  });

  const states: Array<{ state: ScanState; text: RegExp; rescan: boolean }> = [
    { state: "not_linked", text: /no linked transcript yet, so there is nothing to scan/, rescan: false },
    { state: "scanning", text: /Scanning transcript/, rescan: false },
    { state: "none_found", text: /No artifacts found/, rescan: true },
    { state: "failed", text: /Scan failed/, rescan: false },
  ];
  for (const { state, text, rescan } of states) {
    test(`tab_should_showDistinctTextForState_${state}_AndPassLightAndDarkAxe`, async ({ page }) => {
      const { detail, id } = await openTab(page, artifactsJson(state));
      const panel = detail.getArtifactsPanel();
      await expect(panel).toContainText(text);
      await expect(panel).not.toContainText("Extraction pending");
      await expect(page.getByRole("button", { name: "Rescan" })).toHaveCount(rescan ? 1 : 0);
      const reopen = async () => {
        await detail.gotoSession(id);
        await detail.openArtifactsTab();
        await expect(panel).toBeVisible();
      };
      expect(await axeLightAndDark(page, '[aria-labelledby="tab-artifacts"]', reopen)).toEqual([]);
    });
  }

  test("rescan_should_beOneClickDisabledWithSpinnerWhileRunning_AndIgnoreDoubleClicks", async ({ page }) => {
    let calls = 0;
    await page.route("**/api/session.v1.SessionService/RescanSessionArtifacts", async (route) => {
      calls += 1;
      await new Promise((r) => setTimeout(r, 1500));
      await route.fulfill({ status: 200, contentType: "application/json", body: "{}" });
    });
    await openTab(page, artifactsJson("has_data", groupedRecords()));
    const rescan = page.getByRole("button", { name: "Rescan" });
    await rescan.click();
    const busy = page.getByRole("button", { name: /Rescanning/ });
    await expect(busy).toBeDisabled();
    await expect(busy).toHaveAttribute("aria-busy", "true");
    await busy.click({ force: true }).catch(() => undefined);
    await expect.poll(() => calls).toBe(1);
  });

  test("grouped_should_putCommitUnderEarlierPrAndPlanUnderLaterPr_AndCollapseMentionedPrs", async ({ page }) => {
    const { detail, id } = await openTab(page, artifactsJson("has_data", groupedRecords()), [prJson({ number: 11, state: "merged" }), prJson({ number: 12, isActive: true })]);
    await expect(page.getByText("Grouped by when each item appeared relative to PR creation.")).toBeVisible();

    await expect(detail.getArtifactsGroup(11).getByRole("button", { name: "Copy commit SHA 1a2b3c4" })).toBeVisible();
    await expect(detail.getArtifactsGroup(12).getByRole("button", { name: "Copy plan path project_plans/x/plan.md" })).toBeVisible();
    await expect(detail.getArtifactsGroup("unassigned").getByRole("button", { name: "Copy commit SHA aaaaaaa" })).toBeVisible();

    const mentioned = detail.getArtifactsGroup("mentioned");
    const toggle = mentioned.getByRole("button", { name: /Mentioned pull requests/ });
    await expect(toggle).toHaveAttribute("aria-expanded", "false");
    await toggle.click();
    await expect(mentioned.getByText("Mentioned, not created by this session.")).toBeVisible();
    const reopen = async () => {
      await detail.gotoSession(id);
      await detail.openArtifactsTab();
      await detail.getArtifactsGroup("mentioned").getByRole("button", { name: /Mentioned pull requests/ }).click();
    };
    expect(await axeLightAndDark(page, '[data-testid="artifacts-grouped"]', reopen)).toEqual([]);
  });

  test("tab_should_showRedactedMarkerNeverTheTokenValue", async ({ page }) => {
    const { detail } = await openTab(page, artifactsJson("has_data", [prCreated("2026-11-05T10:00:00Z", 11), ciRecord("2026-11-05T10:05:00Z", "https://ci.example/run/9?token=[redacted]")]));
    await expect(page.getByTestId("artifact-redacted")).toHaveText("[redacted]");
    await expect(detail.getArtifactsPanel()).not.toContainText("ci.example");
    await expect(page.getByRole("link", { name: /ci\.example/ })).toHaveCount(0);
  });

  test("groupHeaders_should_exposeDistinctStateTextAndGlyphs_UnderAchromatopsia", async ({ page }, testInfo) => {
    const records = [prCreated("2026-11-05T10:00:00Z", 11), prCreated("2026-11-05T11:00:00Z", 12), prCreated("2026-11-05T12:00:00Z", 13)];
    const { detail } = await openTab(page, artifactsJson("has_data", records), [prJson({ number: 11, state: "merged" }), prJson({ number: 12, isActive: true }), prJson({ number: 13, state: "closed" })]);
    await withAchromatopsia(page, async () => {
      const labels: string[] = [];
      const glyphs: string[] = [];
      for (const n of [11, 12, 13]) {
        const chip = detail.getArtifactsGroup(n).getByTestId("pr-chip");
        labels.push(((await visibleTexts(page, `[data-testid="artifact-group-${n}"] [data-testid="pr-chip"]`))[0] ?? "").replace(/^#\d+\s*/, ""));
        glyphs.push((await chip.locator("svg[data-glyph]").getAttribute("data-glyph")) ?? "");
      }
      expect(new Set(labels).size, `labels ${labels}`).toBe(3);
      expect(new Set(glyphs).size, `glyphs ${glyphs}`).toBe(3);
      await testInfo.attach("artifact-chips-achromatopsia", { body: await page.getByTestId("artifacts-grouped").screenshot(), contentType: "image/png" });
    });
  });
});
