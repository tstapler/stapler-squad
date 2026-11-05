// @feature session:artifacts
/**
 * Artifacts tab at 390px on a touch device (T8.2.2, T6.3.4): groups collapse behind full-width disclosure
 * buttons, every target is at least 44px and works with `tap()`, there is no horizontal scroll and nothing
 * depends on hover. Runs on `mobile-chromium` only (playwright.config.ts MOBILE_ONLY_SPECS).
 */
import { test, expect } from "@playwright/test";
import { SessionClient } from "./helpers/session-client";
import { waitUntilSettled } from "./helpers/wait";
import { baseUrl } from "./helpers/base-url";
import { assertReflow, mobilePreconditionFacts, touchTargetProblems } from "./helpers/a11y";
import { artifactsJson, collectConsoleErrors, commitRecord, prCreated, prJson, seedSessionFields, skipOnboarding } from "./helpers/multi-pr";
import { enableBacklogFeatureFlag } from "./pages/StuckItemsPage";
import { SessionDetailPage } from "./pages/SessionDetailPage";

const BASE_URL = baseUrl();
let client: SessionClient;
let consoleErrors: string[];
const created: string[] = [];

test.describe("artifacts-tab-mobile", () => {
  test.beforeEach(async ({ page, request }) => {
    await enableBacklogFeatureFlag(request);
    client = new SessionClient(BASE_URL);
    consoleErrors = collectConsoleErrors(page);
    await skipOnboarding(page);
  });

  test.afterEach(async () => {
    for (const id of created.splice(0)) await client.deleteSession(id, true).catch(() => undefined);
    expect(consoleErrors, "no console or page errors").toEqual([]);
  });

  test("precondition_should_beACoarsePointerTouchDevice", async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
    const facts = await mobilePreconditionFacts(page);
    expect(facts.coarse).toBe(true);
    expect(facts.touchPoints).toBeGreaterThan(0);
    expect(facts.ua).toMatch(/Mobile|Android/);
  });

  test("groups_should_collapseWith44pxTapTargetsAndNoSidewaysScroll", async ({ page }) => {
    const session = await client.createSession({ title: `e2e-artifacts-m-${Date.now()}`, path: "/tmp", program: "bash" });
    created.push(session.id);
    await waitUntilSettled(client, session.id);
    // Four created PRs: beyond three groups only the active PR's group starts open (ux.md S7).
    const records = [
      prCreated("2026-11-05T09:00:00Z", 11),
      commitRecord("2026-11-05T09:10:00Z", "1111111aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "fix: eleven with a very long subject that must wrap rather than push the page sideways at 390px"),
      prCreated("2026-11-05T10:00:00Z", 12),
      commitRecord("2026-11-05T10:10:00Z", "2222222bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "fix: twelve"),
      prCreated("2026-11-05T11:00:00Z", 13),
      prCreated("2026-11-05T12:00:00Z", 14),
    ];
    await seedSessionFields(page, {
      sessionId: session.id,
      artifacts: artifactsJson("has_data", records),
      pullRequests: [prJson({ number: 11, state: "merged" }), prJson({ number: 12, state: "merged" }), prJson({ number: 13, state: "closed" }), prJson({ number: 14, isActive: true })],
    });
    const detail = new SessionDetailPage(page);
    await detail.gotoSession(session.id);
    await page.getByRole("tab", { name: "Artifacts", exact: true }).tap();

    const toggle = (n: number) => detail.getArtifactsGroup(n).getByRole("button", { name: new RegExp(`^#${n} `) });
    await expect(toggle(14)).toHaveAttribute("aria-expanded", "true");
    for (const n of [11, 12, 13]) await expect(toggle(n)).toHaveAttribute("aria-expanded", "false");

    expect(await touchTargetProblems(...(await Promise.all([11, 12, 13, 14].map(async (n) => toggle(n)))))).toEqual([]);
    await toggle(11).tap();
    await expect(toggle(11)).toHaveAttribute("aria-expanded", "true");
    const copy = detail.getArtifactsGroup(11).getByRole("button", { name: /^Copy commit SHA 1111111/ });
    await expect(copy).toBeVisible();
    expect(await touchTargetProblems(copy)).toEqual([]);
    expect(await assertReflow(page)).toEqual([]);
    expect(await page.evaluate(() => document.scrollingElement!.scrollWidth <= window.innerWidth)).toBe(true);

    await toggle(11).tap();
    await expect(toggle(11)).toHaveAttribute("aria-expanded", "false");
  });
});
