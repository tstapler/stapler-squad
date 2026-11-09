// @feature session:detail, session:pull-requests, backlog:item-detail, upnext:pr-card
/**
 * The 390px assertions for session-multi-pr-tracking (T6.3.2 + T6.3.4): real touch taps, 44px targets, no
 * horizontal scroll, wrapped held lines, the native PR switcher select, the backlog section and the Up Next
 * "N sessions" menu. Runs on `mobile-chromium` only (registered in playwright.config.ts MOBILE_ONLY_SPECS).
 * Seeding is route interception on real API-created sessions and items (helpers/multi-pr.ts).
 */
import { test, expect } from "@playwright/test";
import { SessionClient } from "./helpers/session-client";
import { waitUntilSettled } from "./helpers/wait";
import { baseUrl } from "./helpers/base-url";
import { assertReflow, mobilePreconditionFacts, touchTargetProblems } from "./helpers/a11y";
import {
  HOLD_KINDS,
  collectConsoleErrors,
  createGitRepo,
  holdJson,
  prJson,
  seedBacklogItemPrs,
  seedSessionFields,
  seedUserPrs,
  setMultiPrFlag,
  skipOnboarding,
  threePrs,
  type PrSeed,
} from "./helpers/multi-pr";
import { SessionsPage } from "./pages/SessionsPage";
import { SessionDetailPage } from "./pages/SessionDetailPage";
import { BacklogPage } from "./pages/BacklogPage";
import { BacklogItemDetailPage } from "./pages/BacklogItemDetailPage";
import { createBacklogItemDirect } from "./pages/BacklogMutations";
import { enableBacklogFeatureFlag } from "./pages/StuckItemsPage";
import { UpNextPage } from "./pages/UpNextPage";

const BASE_URL = baseUrl();
let client: SessionClient;
let consoleErrors: string[];
const created: string[] = [];

async function seedSession(page: import("@playwright/test").Page, title: string, prs: PrSeed[], holds?: Record<string, unknown>[], fields?: Record<string, unknown>) {
  const session = await client.createSession({ title, path: "/tmp", program: "bash" });
  created.push(session.id);
  await waitUntilSettled(client, session.id);
  await seedSessionFields(page, { sessionId: session.id, pullRequests: prs.map(prJson), prHolds: holds, fields });
  return session.id;
}

test.describe("multi-pr-session-mobile", () => {
  test.beforeEach(async ({ page, request }) => {
    await enableBacklogFeatureFlag(request);
    client = new SessionClient(BASE_URL);
    consoleErrors = collectConsoleErrors(page);
    await skipOnboarding(page);
    await setMultiPrFlag(page, true);
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

  test("plusN_should_beTappableAt44pxAndOpenTheListFocusingItsHeading", async ({ page }) => {
    const title = `e2e-multipr-m-chip-${Date.now()}`;
    await seedSession(page, title, threePrs());
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
    // On a phone only one pane shows; the list is the "Sessions" pane tab beside the empty detail pane.
    await page.getByRole("tab", { name: "Sessions", exact: true }).or(page.getByRole("button", { name: "Sessions", exact: true })).first().tap();
    const detail = new SessionDetailPage(page);
    const row = new SessionsPage(page).getSessionCard(title);
    await expect(row).toBeVisible();

    expect(await touchTargetProblems(detail.getPrChip(row), detail.getPrMoreButton(row))).toEqual([]);
    expect(await assertReflow(page)).toEqual([]);
    // The state is in the row's own text, not only in the chip's hover title (no hover-only affordance).
    await expect(detail.getPrChip(row)).toContainText("Open");

    await detail.getPrMoreButton(row).tap();
    await expect(detail.getPullRequestsHeading()).toBeFocused();
  });

  test("list_should_notScrollSidewaysAndKeepControlsAt44px_AtEveryReflowWidth", async ({ page }) => {
    const id = await seedSession(page, `e2e-multipr-m-list-${Date.now()}`, threePrs());
    const detail = new SessionDetailPage(page);
    await detail.openPullRequests(id);

    expect(await assertReflow(page)).toEqual([]);
    expect(await touchTargetProblems(detail.getPullRequestRowMenuButton(12), page.getByTestId("pr-refresh"), page.getByTestId("pr-header-menu"))).toEqual([]);
    for (const width of [640, 320]) {
      await page.setViewportSize({ width, height: 844 });
      expect(await assertReflow(page), `${width}px`).toEqual([]);
    }
  });

  test("heldLines_should_wrapWithoutTruncationOrSidewaysScroll", async ({ page }) => {
    const numbers = [21, 22, 23, 24, 25, 26];
    const prs: PrSeed[] = numbers.map((n) => ({ number: n }));
    const holds = HOLD_KINDS.map((kind, i) => holdJson(kind, numbers[i], { idleWindowSeconds: 600 }));
    const id = await seedSession(page, `e2e-multipr-m-holds-${Date.now()}`, prs, holds);
    const detail = new SessionDetailPage(page);
    await detail.openPullRequests(id);
    await page.getByTestId("pr-disclosure").tap().catch(() => undefined);

    for (const n of numbers) {
      const line = page.getByTestId(`pr-hold-${n}`);
      await expect(line).toBeVisible();
      const fits = await line.evaluate((el) => el.scrollWidth <= el.clientWidth + 1);
      expect(fits, `hold line #${n} overflows`).toBe(true);
    }
    expect(await assertReflow(page)).toEqual([]);
  });

  test("notMine_should_openABottomSheetByTapTrapFocusAndReturnItOnEscape", async ({ page }) => {
    const id = await seedSession(page, `e2e-multipr-m-sheet-${Date.now()}`, threePrs());
    const detail = new SessionDetailPage(page);
    await detail.openPullRequests(id);

    const menu = detail.getPullRequestRowMenuButton(11);
    await menu.tap();
    await detail.getPullRequestRowMenuItem("detach", 11).tap();
    const dialog = page.getByTestId("pr-detach-dialog");
    await expect(dialog).toBeVisible();
    expect(await touchTargetProblems(dialog.getByRole("button", { name: "Cancel" }), dialog.getByRole("button", { name: "Not mine" }))).toEqual([]);
    const box = await dialog.boundingBox();
    expect(box && box.x + box.width <= 391).toBe(true);
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(menu).toBeFocused();
  });

  test("switcher_should_beANativeSelectAt390pxAndAnnounceTheTargetChange", async ({ page }) => {
    const legacy = { githubOwner: "acme", githubRepo: "app", githubPrNumber: 41, githubPrUrl: "https://github.com/acme/app/pull/41", githubPrState: "open" };
    const session = await client.createSession({ title: `e2e-multipr-m-switch-${Date.now()}`, path: createGitRepo("m-switch"), program: "bash" });
    created.push(session.id);
    await waitUntilSettled(client, session.id);
    await seedSessionFields(page, { sessionId: session.id, pullRequests: [{ number: 41, isActive: true }, { number: 42 }].map(prJson), fields: legacy });
    const id = session.id;
    const detail = new SessionDetailPage(page);
    await detail.gotoSession(id);
    await page.getByRole("tab", { name: /Version Control|VCS|Git/ }).first().tap();

    const select = detail.getPrSwitcherSelect();
    await expect(select).toBeVisible();
    await expect(detail.getPrSwitcherRadioGroup()).toHaveCount(0);
    await select.selectOption({ index: 1 });
    await expect(page.getByTestId("pr-switcher-target")).toContainText("#42 acme/app");
    await expect(page.getByTestId("pr-switcher-target")).toHaveAttribute("aria-live", "polite");
    expect(await assertReflow(page)).toEqual([]);
  });

  test("backlogSection_should_wrapAndKeepStopTrackingAndTrackByUrlTappableAt44px", async ({ page, request }) => {
    const title = `e2e-multipr-m-backlog-${Date.now()}`;
    const itemId = await createBacklogItemDirect(request, { title, status: "review" });
    await seedBacklogItemPrs(page, itemId, {
      pullRequests: [prJson({ number: 21, state: "merged" }), prJson({ number: 23, state: "unknown", itemDirect: true })],
      prHolds: [holdJson("KIND_SESSION_ACTIVE", 21, { idleWindowSeconds: 600 }), holdJson("KIND_MAX_HOLD", 21)],
    });
    const backlog = new BacklogPage(page);
    await backlog.goto();
    await backlog.waitForItemCards();
    await new BacklogItemDetailPage(page).openItemByTitle(title);

    const list = page.getByTestId("item-pr-list");
    await expect(list).toBeVisible();
    expect(await touchTargetProblems(page.getByTestId("item-pr-stop-23"), page.getByTestId("item-pr-track"))).toEqual([]);
    for (const hold of await page.getByTestId("item-pr-hold").all()) {
      expect(await hold.evaluate((el) => el.scrollWidth <= el.clientWidth + 1)).toBe(true);
    }
    expect(await assertReflow(page)).toEqual([]);
    await page.getByTestId("item-pr-track").tap();
    await expect(page.getByTestId("pr-track-dialog")).toBeVisible();
  });

  test("upNextSessionChip_should_tapOpenTheSessionMenuWithTargetsAt44pxAndReturnFocus", async ({ page }) => {
    const ts = Date.now();
    const a = await client.createSession({ title: `e2e-multipr-m-up-a-${ts}`, path: "/tmp", program: "bash" });
    const b = await client.createSession({ title: `e2e-multipr-m-up-b-${ts}`, path: "/tmp", program: "bash" });
    created.push(a.id, b.id);
    const shared = prJson({ number: 77 });
    await seedSessionFields(page, [{ sessionId: a.id, pullRequests: [shared] }, { sessionId: b.id, pullRequests: [shared] }]);
    await seedUserPrs(page, [{ owner: "acme", repo: "app", number: 77, title: "Shared PR" }]);

    const upNext = new UpNextPage(page);
    await upNext.resetStorage();
    await upNext.goto("prs");
    const card = page.locator('[data-testid="github-pr-card"]').filter({ hasText: "#77" });
    const trigger = card.getByTestId("pr-session-chip-menu-button");
    await expect(trigger).toBeVisible();
    expect(await touchTargetProblems(trigger)).toEqual([]);
    await trigger.tap();
    const links = card.getByTestId("pr-session-chip-menu").getByRole("link");
    await expect(links).toHaveCount(2);
    expect(await touchTargetProblems(...(await links.all()))).toEqual([]);
    expect(await assertReflow(page)).toEqual([]);
    await page.keyboard.press("Escape");
    await expect(trigger).toBeFocused();
    await trigger.tap();
    await page.locator("body").tap({ position: { x: 5, y: 400 } });
    await expect(card.getByTestId("pr-session-chip-menu")).toBeHidden();
  });
});
