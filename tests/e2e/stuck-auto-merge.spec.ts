// @feature backlog:list-stuck
/**
 * The repo auto-merge line in the expanded stuck-item detail (backlog-make-it-true, Story 6.1.1;
 * UX-27..30, 40, 42, 53). ListStuckBacklogItems is mocked per AutoMergeLookupStatus (the server's
 * RepoSettings lookup is the Go side's job); a Retry sends `refreshAutoMerge: true`.
 */
import { test, type Page } from "@playwright/test";
import {
  BASE_URL,
  STUCK_ITEM_TITLE,
  connectError,
  deferred,
  mockStuckList,
  setFlagViaApi,
  skipOnboarding,
  stuckReport,
  type LookupStatus,
  type ProcedureMock,
  type StuckItemSpec,
  expect,
} from "./helpers/backlog-settings-mocks";
import { StuckItemsPage } from "./pages/StuckItemsPage";
import { sendNotification } from "./pages/NotificationPanel";

const VIEWPORTS = [
  { name: "desktop-1280x800", width: 1280, height: 800 },
  { name: "mobile-360x800", width: 360, height: 800 },
] as const;

const MIN_TARGET_PX = 44;
const MINUTES_AGO_MS = 5 * 60_000;
const OK_ALLOWED = "Repo auto-merge: allowed";
const OK_DISALLOWED = "Repo auto-merge: not allowed on this repo";
const FAILURE_COPY: Array<[Exclude<LookupStatus, "OK" | "UNSPECIFIED">, string]> = [
  ["RATE_LIMITED", "Repo auto-merge: couldn't check (rate limited)"],
  ["NO_PERMISSION", "Repo auto-merge: couldn't check (no permission)"],
  ["NOT_FOUND", "Repo auto-merge: couldn't check (repo not found)"],
  ["ERROR", "Repo auto-merge: couldn't check (lookup error)"],
];

function minutesAgo(): string {
  return new Date(Date.now() - MINUTES_AGO_MS).toISOString();
}

/** Shows the stuck list for `spec` and expands its card; returns the page object and the RPC mock. */
async function openWith(page: Page, spec: StuckItemSpec, mock?: ProcedureMock) {
  const stuck = new StuckItemsPage(page);
  const list = mock ?? (await mockStuckList(page, stuckReport(spec)));
  if (mock) mock.use(stuckReport(spec));
  await stuck.goto();
  await stuck.openDetail(STUCK_ITEM_TITLE);
  return { stuck, list };
}

for (const vp of VIEWPORTS) {
  test.describe(`stuck auto-merge @ ${vp.name}`, () => {
    test.use({ viewport: { width: vp.width, height: vp.height } });
    test.describe.configure({ timeout: 90_000 }); // multi-step flows on a loaded machine outrun the 30 s default
    const isMobile = vp.width < 400;

    test.beforeAll(async ({ request }) => {
      await setFlagViaApi(request, "backlog", true);
    });

    test.afterAll(async ({ request }) => {
      await setFlagViaApi(request, "backlog", false);
    });

    test.beforeEach(async ({ page }) => {
      await skipOnboarding(page);
      await page.addInitScript(() => localStorage.removeItem("up-next-tab"));
    });

    // ===== UX-27 / UX-40 =====

    test("auto_merge_line_should_render_allowed_and_not_allowed_copy_with_noopener_link", async ({ page }) => {
      const { stuck, list } = await openWith(page, { lookup: "OK", allowAutoMerge: true, checkedAt: minutesAgo() });
      await expect(stuck.autoMergeLine()).toContainText(OK_ALLOWED);
      await expect(stuck.autoMergeLine()).toContainText(
        "GitHub setting only. Branch protection or required checks can still block it.",
      );
      await expect(stuck.autoMergeSettingsLink()).toHaveCount(0);

      const open = await openWith(page, { lookup: "OK", allowAutoMerge: false, checkedAt: minutesAgo() }, list);
      await expect(open.stuck.autoMergeLine()).toContainText(OK_DISALLOWED);
      await expect(open.stuck.autoMergeLine()).toContainText(
        "Merge this PR by hand, or enable Allow auto-merge in the repo settings.",
      );
      const link = open.stuck.autoMergeSettingsLink();
      await expect(link).toHaveAttribute("rel", /noopener/);
      await expect(link).toHaveAttribute("target", "_blank");
      await expect(link).toHaveAttribute("href", "https://github.com/tstapler/stapler-squad/settings");
      await expect(link).toHaveText(/\(opens in new tab\)$/); // UX-40
    });

    // ===== UX-28: failed lookups, never "unknown" =====

    test("auto_merge_line_should_show_couldnt_check_with_retry_and_never_unknown_When_failed_statuses", async ({ page }) => {
      let mock: ProcedureMock | undefined;
      for (const [status, copy] of FAILURE_COPY) {
        const shown = await openWith(page, { lookup: status, checkedAt: minutesAgo() }, mock);
        mock = shown.list;
        const line = shown.stuck.autoMergeLine();
        await expect(line, status).toContainText(copy);
        await expect(line, status).toContainText("Last tried");
        await expect(shown.stuck.autoMergeRetry(), status).toBeVisible();
        const text = (await line.innerText()).toLowerCase();
        expect(text).not.toContain("unknown");
        expect(text).not.toContain("(allow_auto_merge:");
      }

      // Retry: "Checking..." while the forced refresh is pending, then the new state.
      const held = deferred();
      mock!.use((_n, body) =>
        (body as { refreshAutoMerge?: boolean } | null)?.refreshAutoMerge
          ? held.promise
          : stuckReport({ lookup: "RATE_LIMITED", checkedAt: minutesAgo() }),
      );
      const stuck = new StuckItemsPage(page);
      await stuck.autoMergeRetry().click();
      await expect(stuck.autoMergeLine()).toContainText("Repo auto-merge: checking...");
      await expect(stuck.autoMergeRetry()).toHaveText("Checking...");
      await expect(stuck.autoMergeStatus()).toHaveText("Checking repo auto-merge");

      held.resolve(stuckReport({ lookup: "OK", allowAutoMerge: true, checkedAt: new Date().toISOString() }));
      await expect(stuck.autoMergeLine()).toContainText(OK_ALLOWED);
      await expect(stuck.autoMergeRetry()).toHaveCount(0);
      expect((mock!.requests.at(-1) as { refreshAutoMerge?: boolean }).refreshAutoMerge).toBe(true);
    });

    // ===== UX-29 =====

    test("auto_merge_line_should_be_absent_When_status_unspecified", async ({ page }) => {
      const { stuck } = await openWith(page, { lookup: "UNSPECIFIED" });
      await expect(page.getByTestId("stuck-item-detail")).toBeVisible();
      await expect(stuck.autoMergeLine()).toHaveCount(0);
    });

    // ===== UX-30 =====

    test("auto_merge_line_should_wrap_and_have_44px_targets_When_360", async ({ page }) => {
      const failed = await openWith(page, { lookup: "RATE_LIMITED", checkedAt: minutesAgo() });
      const retryBox = await failed.stuck.autoMergeRetry().boundingBox();
      const overflow = await failed.stuck.autoMergeLine().evaluate((el) => ({
        clipped: el.scrollWidth > el.clientWidth,
        page: (document.scrollingElement as HTMLElement).scrollWidth > (document.scrollingElement as HTMLElement).clientWidth,
      }));
      expect(overflow).toEqual({ clipped: false, page: false });
      expect(retryBox?.height).toBeGreaterThanOrEqual(MIN_TARGET_PX);

      const disallowed = await openWith(page, { lookup: "OK", allowAutoMerge: false, checkedAt: minutesAgo() }, failed.list);
      const linkBox = await disallowed.stuck.autoMergeSettingsLink().boundingBox();
      expect(linkBox?.height).toBeGreaterThanOrEqual(MIN_TARGET_PX);
      const after = await disallowed.stuck.autoMergeLine().evaluate((el) => ({
        clipped: el.scrollWidth > el.clientWidth,
        page: (document.scrollingElement as HTMLElement).scrollWidth > (document.scrollingElement as HTMLElement).clientWidth,
      }));
      expect(after, isMobile ? "360px layout" : "1280px layout").toEqual({ clipped: false, page: false });
    });

    // ===== UX-53 =====

    test("auto_merge_line_should_show_last_tried_omit_link_without_repo_url_and_keep_retry_focus", async ({ page }) => {
      const withTime = await openWith(page, { lookup: "ERROR", checkedAt: minutesAgo() });
      await expect(withTime.stuck.autoMergeLine()).toContainText("Last tried");
      const withoutTime = await openWith(page, { lookup: "ERROR" }, withTime.list);
      await expect(withoutTime.stuck.autoMergeLine()).toContainText("Repo auto-merge: couldn't check (lookup error)");
      await expect(withoutTime.stuck.autoMergeLine()).not.toContainText("Last tried");

      const noUrl = await openWith(page, { lookup: "OK", allowAutoMerge: false, prUrl: "" }, withTime.list);
      await expect(noUrl.stuck.autoMergeLine()).toContainText(OK_DISALLOWED);
      await expect(noUrl.stuck.autoMergeSettingsLink()).toHaveCount(0);
      await expect(noUrl.stuck.autoMergeLine()).not.toContainText("Open repo settings");

      const ghe = await openWith(
        page,
        { lookup: "OK", allowAutoMerge: false, prUrl: "https://ghe.example.com/acme/widgets/pull/5" },
        withTime.list,
      );
      await expect(ghe.stuck.autoMergeSettingsLink()).toHaveAttribute("href", "https://ghe.example.com/acme/widgets/settings");

      // Retry keeps the same button (and focus) through a repeat failure; success hands focus to the line.
      const stuck = (await openWith(page, { lookup: "RATE_LIMITED", checkedAt: minutesAgo() }, withTime.list)).stuck;
      const retry = stuck.autoMergeRetry();
      const handle = await retry.elementHandle();
      await retry.click();
      await expect(retry).toHaveText("Retry");
      await expect(retry).toBeFocused();
      expect(await handle!.evaluate((el) => el.isConnected)).toBe(true);
      expect(await retry.evaluate((el, original) => el === original, handle)).toBe(true);
      await expect(stuck.autoMergeStatus()).toHaveText("Repo auto-merge check failed again");

      withTime.list.use(stuckReport({ lookup: "OK", allowAutoMerge: true, checkedAt: new Date().toISOString() }));
      await retry.click();
      await expect(stuck.autoMergeLine()).toBeFocused();
      await expect(stuck.autoMergeLine()).toContainText(OK_ALLOWED);
    });

    // ===== UX-42: the pre-PR warning is an ordinary, removable notification =====

    // The producer (type INFO, urgent=false, important=false, one per repo per 24h) is pinned by
    // session/backlog_lifecycle_pr_test.go. Here a notification with the producer's title and message is
    // published through SendNotification and the e2e proves the surface and the dismissal control.
    test("preflight_notification_should_appear_in_notification_list_and_be_removable", async ({ page, request }) => {
      const title = `Auto-merge unavailable ${vp.width}`;
      await sendNotification(request, BASE_URL, {
        sessionId: `e2e-preflight-${vp.width}`,
        type: "INFO",
        title,
        message: "Repo tstapler/stapler-squad does not allow auto-merge. PR will still be created.",
      });

      // The history subscriber coalesces before persisting, so wait until the row is listed.
      await expect
        .poll(async () => {
          const resp = await request.post(`${BASE_URL}/api/session.v1.SessionService/GetNotificationHistory`, {
            headers: { "Content-Type": "application/json" },
            data: { limit: 500 },
          });
          return JSON.stringify(await resp.json()).includes(title);
        }, { timeout: 20_000 })
        .toBe(true);

      await page.goto(`${BASE_URL}/notifications`, { waitUntil: "domcontentloaded" });
      const recent = page.getByRole("button", { name: /Recent activity/ });
      if ((await recent.getAttribute("aria-expanded")) === "false") await recent.click();
      await page.getByRole("searchbox", { name: "Search notifications" }).fill(title);

      // The tray repeats the list, so judge the page's own content.
      const content = page.getByTestId("notifications-content");
      await expect(content.getByText(title)).toBeVisible();
      await expect(content.getByText("does not allow auto-merge. PR will still be created.")).toBeVisible();
      const remove = content.getByRole("button", { name: "Remove notification" });
      await expect(remove).toHaveCount(1);
      await remove.click();
      await expect(content.getByText(title)).toHaveCount(0);
    });

    // ===== A failed list keeps the page usable =====

    test("stuck_list_failure_should_not_render_an_auto_merge_line", async ({ page }) => {
      const stuck = new StuckItemsPage(page);
      await mockStuckList(page, connectError("unavailable", "list unavailable"));
      await page.goto(`${BASE_URL}/unfinished?tab=stuck`, { waitUntil: "domcontentloaded" });
      await expect(stuck.section).toBeVisible();
      await expect(stuck.autoMergeLine()).toHaveCount(0);
    });
  });
}
