// @feature session:detail, session:pull-requests, backlog:item-detail, upnext:pr-card
/**
 * Multi-PR session tracking in a real browser (project_plans/session-multi-pr-tracking, T6.3.2 and T6.3.4):
 * the card/row chip with "+N", the detail "Pull requests" list and its dialogs, held lines, the VCS widget
 * switcher, the backlog item "k of N merged" section, the Up Next "session: name" chip, light and dark Axe
 * runs on each, and the WCAG 1.4.1 check under emulated achromatopsia.
 *
 * Server-computed fields (`pullRequests`, `prHolds`) and the `session_multi_pr` flag read are injected by route
 * interception on a real API-created session or backlog item (helpers/multi-pr.ts states why). The 390px
 * assertions are in multi-pr-session-mobile.spec.ts.
 */
import { test, expect, type Page } from "@playwright/test";
import { SessionClient } from "./helpers/session-client";
import { waitUntilSettled } from "./helpers/wait";
import { baseUrl } from "./helpers/base-url";
import { axeLightAndDark, axeAaViolations, tabOrderNames, visibleTexts, withAchromatopsia } from "./helpers/a11y";
import {
  HOLD_KINDS,
  collectConsoleErrors,
  createGitRepo,
  holdJson,
  mockPrRpc,
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
const rpc = (name: string) => `${BASE_URL}/api/session.v1.SessionService/${name}`;

let client: SessionClient;
let consoleErrors: string[];
const created: string[] = [];

async function newSession(title: string, path = "/tmp"): Promise<string> {
  const session = await client.createSession({ title, path, program: "bash" });
  created.push(session.id);
  await waitUntilSettled(client, session.id);
  return session.id;
}

async function seedSession(page: Page, title: string, prs: PrSeed[], extra: { holds?: Record<string, unknown>[]; fields?: Record<string, unknown> } = {}): Promise<string> {
  const id = await newSession(title);
  await seedSessionFields(page, { sessionId: id, pullRequests: prs.map(prJson), prHolds: extra.holds, fields: extra.fields });
  return id;
}

test.describe("multi-pr-session", () => {
  test.use({ viewport: { width: 1280, height: 1000 } });

  test.beforeEach(async ({ page, request }) => {
    // Without the backlog flag every page load logs a "feature backlog is not enabled" console error.
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

  test("chip_should_showActivePrWithPlusN_AndPlusNShouldOpenTheListFocusingItsHeading", async ({ page }) => {
    const title = `e2e-multipr-chip-${Date.now()}`;
    const id = await seedSession(page, title, threePrs());
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
    const detail = new SessionDetailPage(page);
    const row = new SessionsPage(page).getSessionCard(title);
    await expect(row).toBeVisible();

    const chip = detail.getPrChip(row);
    await expect(chip).toContainText("#12");
    await expect(chip).toContainText("Open");
    await expect(chip).toHaveAttribute("title", /GitHub PR #12 acme\/app.*State: open/);
    const more = detail.getPrMoreButton(row);
    await expect(more).toHaveText("+2");

    await more.click();
    await expect(detail.getPullRequestsHeading()).toBeFocused();
    expect(page.url()).toContain("#pull-requests");
    expect(id).toBeTruthy();
  });

  test("list_should_showEveryPrWithTheActiveOneMarked_AndKeepRowsInKeyboardOrder", async ({ page }) => {
    const id = await seedSession(page, `e2e-multipr-list-${Date.now()}`, threePrs());
    const detail = new SessionDetailPage(page);
    await detail.openPullRequests(id);

    await expect(detail.getPullRequestRow(12)).toContainText("Active");
    await expect(detail.getPullRequestRow(11)).toContainText("Merged");
    await expect(detail.getPullRequestRow(10)).toContainText("Merged");
    await expect(detail.getPullRequestsHeading()).toContainText("(3)");

    await detail.getPullRequestsHeading().focus();
    const order = await tabOrderNames(page, 5);
    expect(order[0]).toBe("Refresh");
    expect(order[1]).toBe("More pull request actions");
    expect(order[2]).toMatch(/^Pull request #12 acme\/app/);
    expect(order.findIndex((n) => /^Merge #12/.test(n))).toBeGreaterThan(2);
  });

  test("list_should_passLightAndDarkAxe_AndEveryDialogShouldPassLightAxeAndCloseOnEscReturningFocus", async ({ page }) => {
    const id = await seedSession(page, `e2e-multipr-axe-${Date.now()}`, threePrs());
    const detail = new SessionDetailPage(page);
    await detail.openPullRequests(id);

    expect(await axeLightAndDark(page, '[data-testid="pr-section"]', () => detail.openPullRequests(id))).toEqual([]);

    const dialogs: Array<{ open: () => Promise<void>; testId: string }> = [
      { testId: "pr-merge-dialog", open: () => detail.getPullRequestRow(12).getByRole("button", { name: /^Merge #12/ }).click() },
      { testId: "pr-close-dialog", open: async () => { await detail.getPullRequestRowMenuButton(12).click(); await detail.getPullRequestRowMenuItem("close", 12).click(); } },
      { testId: "pr-detach-dialog", open: async () => { await detail.getPullRequestRowMenuButton(11).click(); await detail.getPullRequestRowMenuItem("detach", 11).click(); } },
      { testId: "pr-track-dialog", open: async () => { await page.getByTestId("pr-header-menu").click(); await page.getByTestId("pr-track-by-url").click(); } },
    ];
    for (const d of dialogs) {
      await d.open();
      const dialog = page.getByTestId(d.testId);
      await expect(dialog).toBeVisible();
      expect(await axeAaViolations(page, `[data-testid="${d.testId}"]`), d.testId).toEqual([]);
      await page.keyboard.press("Escape");
      await expect(dialog).toBeHidden();
      await expect(page.locator("body")).not.toBeFocused();
    }
  });

  test("notMine_should_sendTheRowsOwnRefAndOfferUndoThatReattachesIt", async ({ page }) => {
    const id = await seedSession(page, `e2e-multipr-detach-${Date.now()}`, threePrs());
    const detached = await mockPrRpc(page, "DetachSessionPullRequest", { pullRequests: [prJson(threePrs()[0]), prJson(threePrs()[2])] });
    const attached = await mockPrRpc(page, "AttachSessionPullRequest", { pullRequests: threePrs().map(prJson) });
    const detail = new SessionDetailPage(page);
    await detail.openPullRequests(id);

    await detail.getPullRequestRowMenuButton(11).click();
    await detail.getPullRequestRowMenuItem("detach", 11).click();
    const dialog = page.getByTestId("pr-detach-dialog");
    await expect(dialog).toContainText("It won't be added back automatically");
    await dialog.getByRole("button", { name: "Not mine" }).click();

    await expect(detail.getPullRequestRow(11)).toHaveCount(0);
    expect(detached[0]).toMatchObject({ id, pr: { owner: "acme", repo: "app", number: 11 } });
    const toast = detail.getPullRequestToast();
    await expect(toast).toContainText("Stopped tracking #11.");
    await toast.getByRole("button", { name: /Undo/ }).click();
    await expect.poll(() => attached.length).toBe(1);
    expect(attached[0]).toMatchObject({ id, url: "https://github.com/acme/app/pull/11" });
  });

  test("pin_should_moveFocusToTheToastUndo_When_StartedFromTheKeyboard", async ({ page }) => {
    const id = await seedSession(page, `e2e-multipr-pin-${Date.now()}`, [{ number: 12, isActive: true }, { number: 14 }]);
    const pinned = await mockPrRpc(page, "PinSessionPullRequest", { pullRequests: [prJson({ number: 12 }), prJson({ number: 14, isActive: true, pinned: true })] });
    const detail = new SessionDetailPage(page);
    await detail.openPullRequests(id);

    await detail.getPullRequestRowMenuButton(14).focus();
    await page.keyboard.press("Enter");
    await detail.getPullRequestRowMenuItem("pin", 14).focus();
    await page.keyboard.press("Enter");

    const toast = detail.getPullRequestToast();
    await expect(toast).toContainText("Pinned #14 as current.");
    await expect(toast.getByRole("button", { name: /Undo/ })).toBeFocused();
    expect(pinned[0]).toMatchObject({ id, pr: { number: 14 } });
  });

  test("undoToast_should_waitWhileFocusedAndExpireEightSecondsAfterFocusLeaves", async ({ page }) => {
    await page.clock.install();
    const id = await seedSession(page, `e2e-multipr-toast-${Date.now()}`, [{ number: 12, isActive: true }, { number: 14 }]);
    await mockPrRpc(page, "PinSessionPullRequest", { pullRequests: [prJson({ number: 12 }), prJson({ number: 14, isActive: true, pinned: true })] });
    const detail = new SessionDetailPage(page);
    await detail.openPullRequests(id);

    await detail.getPullRequestRowMenuButton(14).focus();
    await page.keyboard.press("Enter");
    await detail.getPullRequestRowMenuItem("pin", 14).focus();
    await page.keyboard.press("Enter");
    const toast = detail.getPullRequestToast();
    const undo = toast.getByRole("button", { name: /Undo/ });
    await expect(undo).toBeFocused();

    await page.clock.fastForward(9_000);
    await expect(toast).toBeVisible();

    await detail.getPullRequestsHeading().focus();
    await page.clock.fastForward(7_500);
    await expect(toast).toBeVisible();
    await page.clock.fastForward(1_000);
    await expect(toast).toBeHidden();
    // The untimed path outlives the toast: the row menu can still undo the pin.
    await detail.getPullRequestRowMenuButton(14).focus();
    await page.keyboard.press("Enter");
    await expect(detail.getPullRequestRowMenuItem("unpin", 14)).toBeVisible();
  });

  test("holds_should_wordEveryHoldKindAndNeverSayWorktree", async ({ page }) => {
    const numbers = [21, 22, 23, 24, 25, 26];
    const prs: PrSeed[] = numbers.map((n, i) => ({ number: n, state: i === 4 ? "merged" : i === 5 ? "merged" : i === 1 ? "closed" : "open" }));
    const holds = HOLD_KINDS.map((kind, i) => holdJson(kind, numbers[i], { idleWindowSeconds: 600 }));
    const id = await seedSession(page, `e2e-multipr-holds-${Date.now()}`, prs, { holds });
    const detail = new SessionDetailPage(page);
    await detail.openPullRequests(id);

    const expected = [
      "Local files kept: #21 is still open.",
      "Local files kept until #22 is confirmed closed or merged.",
      "Local files kept: GitHub can't be reached for #23",
      "Kept for #24 from an earlier round; it is still open.",
      "Merged. The session is still active, so the item isn't done yet.",
      "The session was still active 1 hour after #26 merged",
    ];
    for (const [i, text] of expected.entries()) {
      await expect(page.getByTestId(`pr-hold-${numbers[i]}`)).toContainText(text);
    }
    for (const n of numbers) expect(await page.getByTestId(`pr-hold-${n}`).innerText()).not.toMatch(/worktree/i);
    expect(await axeAaViolations(page, '[data-testid="pr-section"]')).toEqual([]);
  });

  test("flag_should_renderTheLegacySingleBadgeAndLink_When_Off", async ({ page }) => {
    const title = `e2e-multipr-flagoff-${Date.now()}`;
    const id = await newSession(title);
    await setMultiPrFlag(page, false);
    await seedSessionFields(page, {
      sessionId: id,
      pullRequests: threePrs().map(prJson),
      fields: { githubOwner: "acme", githubRepo: "app", githubPrNumber: 12, githubPrUrl: "https://github.com/acme/app/pull/12", githubPrState: "open" },
    });
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
    const row = new SessionsPage(page).getSessionCard(title);
    await expect(row).toBeVisible();
    await expect(row.getByTestId("pr-more-button")).toHaveCount(0);
    await expect(row.getByTestId("pr-chip-group")).toHaveCount(0);

    const detail = new SessionDetailPage(page);
    await detail.gotoSession(id);
    await expect(page.getByRole("link", { name: /#12/ }).first()).toBeVisible();
    await expect(detail.getPullRequestsSection()).toHaveCount(0);
  });

  test("states_should_exposeDistinctTextAndGlyphs_UnderAchromatopsia", async ({ page }, testInfo) => {
    const states: PrSeed[] = [
      { number: 31, isActive: true },
      { number: 32, isDraft: true },
      { number: 33, state: "merged", mergedAt: "2026-11-04T10:00:00Z" },
      { number: 34, state: "closed", closedAt: "2026-11-04T10:00:00Z" },
      { number: 35, state: "unknown" },
    ];
    const id = await seedSession(page, `e2e-multipr-achroma-${Date.now()}`, states);
    const detail = new SessionDetailPage(page);
    await detail.openPullRequests(id);

    await withAchromatopsia(page, async () => {
      const rows = [31, 32, 33, 34, 35].map((n) => detail.getPullRequestRow(n));
      const labels: string[] = [];
      const glyphs: string[] = [];
      for (const row of rows) {
        labels.push((await row.locator("span", { hasText: /^(Open|Draft|Merged|Closed|State unknown|Checking)/ }).first().innerText()).split(" ")[0]);
        glyphs.push((await row.locator("svg[data-glyph]").first().getAttribute("data-glyph")) ?? "");
      }
      expect(new Set(labels).size, `labels ${labels}`).toBe(5);
      expect(new Set(glyphs).size, `glyphs ${glyphs}`).toBe(5);
      await testInfo.attach("pr-states-achromatopsia", { body: await page.getByTestId("pr-section").screenshot(), contentType: "image/png" });
    });
  });

  test("motion_should_beOff_When_ReducedMotionIsRequested", async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    const id = await seedSession(page, `e2e-multipr-motion-${Date.now()}`, threePrs());
    const detail = new SessionDetailPage(page);
    await detail.openPullRequests(id);
    const running = await page.evaluate(() =>
      document
        .getAnimations()
        .filter((a): a is CSSAnimation => a instanceof CSSAnimation && a.playState === "running" && !!(a.effect as KeyframeEffect | null)?.target?.closest?.('[data-testid="pr-section"]'))
        .map((a) => a.animationName),
    );
    expect(running).toEqual([]);
  });

  test("switcher_should_beSegmentedUpToFourOpenPrsAndANativeSelectAtFive", async ({ page }) => {
    const open = (n: number, active = false): PrSeed => ({ number: n, isActive: active, headBranch: `feat-${n}` });
    const legacy = { githubOwner: "acme", githubRepo: "app", githubPrNumber: 41, githubPrUrl: "https://github.com/acme/app/pull/41", githubPrState: "open" };
    const repo = createGitRepo("switch");
    const id = await newSession(`e2e-multipr-switch-${Date.now()}`, repo);
    await seedSessionFields(page, { sessionId: id, pullRequests: [open(41, true), open(42), open(43), open(44)].map(prJson), fields: legacy });
    const detail = new SessionDetailPage(page);
    await detail.gotoSession(id);
    await page.getByRole("tab", { name: /Version Control|VCS|Git/ }).first().click();

    await expect(detail.getPrSwitcherRadioGroup()).toBeVisible();
    await expect(page.getByTestId("pr-switcher-target")).toContainText("#41 acme/app");
    await page.getByRole("radio", { name: /#42/ }).click();
    await expect(page.getByTestId("pr-switcher-target")).toContainText("#42 acme/app");
    const openVcsTab = async () => {
      await detail.gotoSession(id);
      await page.getByRole("tab", { name: /Version Control|VCS|Git/ }).first().click();
      await expect(detail.getPrSwitcherRadioGroup()).toBeVisible();
    };
    expect(await axeLightAndDark(page, '[data-testid="pr-switcher"]', openVcsTab)).toEqual([]);
  });

  test("list_should_showServerTitleAndVerifyCounters_AndAskToFixShouldReportTheServerOutcome", async ({ page }) => {
    const prs: PrSeed[] = [
      { number: 12, isActive: true, title: "Add token refresh" },
      { number: 9, state: "unknown", outcomeUnverified: true, verifyAttempts: 2, verifyAttemptLimit: 5 },
      { number: 8, state: "unknown", outcomeUnverified: true, verifyAttempts: 5, verifyAttemptLimit: 5, verifyExhausted: true },
    ];
    const id = await seedSession(page, `e2e-multipr-server-fields-${Date.now()}`, prs);
    const asked = await mockPrRpc(page, "AskSessionToFixPullRequest", { outcome: "NUDGE_OUTCOME_DELIVERED", pr: { host: "github.com", owner: "acme", repo: "app", number: 12 } });
    const detail = new SessionDetailPage(page);
    await detail.openPullRequests(id);

    await expect(detail.getPullRequestRow(12).getByTestId("pr-title-12")).toHaveText("Add token refresh");
    await expect(detail.getPullRequestRow(9)).toContainText("Checking... (attempt 2 of 5)");
    await expect(detail.getPullRequestRow(8)).toContainText("Couldn't verify after 5 attempts. Treated as closed.");

    await detail.getPullRequestRow(12).getByRole("button", { name: "Ask to fix #12 acme/app" }).click();
    await expect.poll(() => asked.length).toBe(1);
    expect(asked[0]).toMatchObject({ id, pr: { owner: "acme", repo: "app", number: 12 } });
    await expect(page.getByTestId("pr-note")).toContainText("Request sent to");
  });

  test("backlogItem_should_showKofNmergedHeldLinesAndStopTracking", async ({ page, request }) => {
    await enableBacklogFeatureFlag(request);
    const title = `e2e-multipr-backlog-${Date.now()}`;
    const itemId = await createBacklogItemDirect(request, { title, status: "review" });
    const prs = [prJson({ number: 21, state: "merged" }), prJson({ number: 22, state: "merged" }), prJson({ number: 23, state: "unknown", itemDirect: true, title: "Wire the UI" })];
    await seedBacklogItemPrs(page, itemId, {
      pullRequests: prs,
      prHolds: [holdJson("KIND_SESSION_ACTIVE", 21, { idleWindowSeconds: 600 }), holdJson("KIND_UNKNOWN", 23)],
    });
    const detached = await mockPrRpc(page, "DetachItemPullRequest", {});

    const backlog = new BacklogPage(page);
    await backlog.goto();
    await backlog.waitForItemCards();
    const detail = new BacklogItemDetailPage(page);
    await detail.openItemByTitle(title);

    await expect(page.getByTestId("item-pr-summary")).toHaveText("2 of 3 merged");
    await expect(page.getByTestId("item-pr-progress")).toHaveAttribute("aria-valuetext", "2 of 3 pull requests merged");
    await expect(page.getByTestId("item-pr-rule")).toContainText("Waiting on #23 (unknown)");
    const holds = page.getByTestId("item-pr-hold");
    await expect(holds.first()).toContainText("#21 merged. Waiting for the session to go idle");
    await expect(holds.first()).toContainText("10 minutes idle");

    const reopenItem = async () => {
      await backlog.goto();
      await backlog.waitForItemCards();
      await detail.openItemByTitle(title);
      await expect(page.getByTestId("item-pr-list")).toBeVisible();
    };
    expect(await axeLightAndDark(page, '[data-testid="item-pr-list"]', reopenItem)).toEqual([]);

    await withAchromatopsia(page, async () => {
      await expect(page.getByTestId("item-pr-summary")).toBeVisible();
      await expect(page.getByTestId("item-pr-progress")).toBeVisible();
    });

    await expect(page.getByTestId("item-pr-title-23")).toHaveText("Wire the UI");
    await expect(page.getByTestId("item-pr-stop-22")).toHaveCount(0);
    await page.getByTestId("item-pr-stop-23").click();
    await page.getByTestId("item-pr-stop-dialog").getByRole("button", { name: "Stop tracking" }).click();
    await expect.poll(() => detached.length).toBe(1);
    expect(detached[0]).toMatchObject({ itemId, pr: { owner: "acme", repo: "app", number: 23 } });
  });

  test("upNextChip_should_nameOneSessionAndOpenAMenuForTwo_PassingLightAndDarkAxe", async ({ page }) => {
    const ts = Date.now();
    const a = await newSession(`e2e-multipr-up-a-${ts}`);
    const b = await newSession(`e2e-multipr-up-b-${ts}`);
    const shared = prJson({ number: 77 });
    await seedSessionFields(page, [{ sessionId: a, pullRequests: [shared] }, { sessionId: b, pullRequests: [shared] }]);
    await seedUserPrs(page, [{ owner: "acme", repo: "app", number: 77, title: "Shared PR" }, { owner: "acme", repo: "app", number: 78, title: "Lonely PR" }]);

    const upNext = new UpNextPage(page);
    await upNext.resetStorage();
    await upNext.goto("prs");

    const card = page.locator('[data-testid="github-pr-card"][data-pr-key]').filter({ hasText: "#77" });
    await expect(card).toBeVisible();
    const lonely = page.locator('[data-testid="github-pr-card"]').filter({ hasText: "#78" });
    await expect(lonely.getByTestId("pr-session-chip")).toHaveCount(0);

    const trigger = card.getByTestId("pr-session-chip-menu-button");
    await expect(trigger).toHaveText("2 sessions");
    await trigger.click();
    const menu = card.getByTestId("pr-session-chip-menu");
    await expect(menu.getByRole("link")).toHaveCount(2);
    const reopenMenu = async () => {
      await upNext.goto("prs");
      await expect(trigger).toBeVisible();
      await trigger.click();
      await expect(menu.getByRole("link")).toHaveCount(2);
    };
    expect(await axeLightAndDark(page, '[data-testid="pr-session-chip"]', reopenMenu)).toEqual([]);
    await page.keyboard.press("Escape");
    await expect(menu).toBeHidden();
    await expect(trigger).toBeFocused();
  });
});
