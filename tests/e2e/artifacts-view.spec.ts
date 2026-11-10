// @feature session:artifacts
/**
 * The /artifacts page at 1280px (T10.1.3, UX-S8-1..3, 5..8 and UX-S9 for the page side): filtered browse, search
 * with the matched type per row, distinct copy for every state, URL state, one announcer region, light and dark
 * Axe, the nav route in two clicks, and the PR lookup behind the omnibar's /artifacts?pr=... link.
 *
 * Browse, search and lookup RPCs are answered by route mocks (pages/ArtifactsPage.ts): the isolated server has no
 * scanned transcripts. One test runs against the real server to prove the browse RPC is wired end to end.
 * The 390px assertions are in artifacts-view-mobile.spec.ts.
 */
import { test, expect } from "@playwright/test";
import { SessionClient } from "./helpers/session-client";
import { waitUntilSettled } from "./helpers/wait";
import { baseUrl } from "./helpers/base-url";
import { axeLightAndDark } from "./helpers/a11y";
import { collectConsoleErrors, skipOnboarding } from "./helpers/multi-pr";
import { enableBacklogFeatureFlag } from "./pages/StuckItemsPage";
import { ArtifactsPage, hitJson, rowJson, type MockRow } from "./pages/ArtifactsPage";

const BASE_URL = baseUrl();
let client: SessionClient;
let consoleErrors: string[];
let artifacts: ArtifactsPage;
const created: string[] = [];

const ALL_ROWS: MockRow[] = [
  { sessionId: "s1", sessionTitle: "refactor-auth", kind: "pr", value: 12, state: "open", source: "session_pr", at: "2026-11-05T10:00:00Z" },
  { sessionId: "s1", sessionTitle: "refactor-auth", kind: "pr", value: 11, state: "merged", source: "session_pr", at: "2026-11-04T10:00:00Z" },
  { sessionId: "s1", sessionTitle: "refactor-auth", kind: "commit", value: "1a2b3c4d5e6f1a2b3c4d5e6f1a2b3c4d5e6f1a2b", at: "2026-11-05T09:00:00Z" },
  { sessionId: "s2", sessionTitle: "other-repo-work", kind: "pr", value: 3, owner: "acme", repo: "other", state: "closed", at: "2026-11-03T10:00:00Z" },
];

const browseResponse = (rows: MockRow[], extra: Record<string, unknown> = {}) => ({
  rows: rows.map(rowJson),
  totalCount: rows.length,
  unscannedSessionCount: 0,
  ...extra,
});

/** Filters ALL_ROWS the way the server would for the request's types and repo. */
function serverFilter(req: Record<string, unknown>): MockRow[] {
  const types = (req.types as string[] | undefined) ?? [];
  const repo = String(req.repo ?? "").toLowerCase();
  return ALL_ROWS.filter((r) => (types.length === 0 || types.includes(r.kind)) && (!repo || `${r.owner ?? "acme"}/${r.repo ?? "app"}` === repo));
}

test.describe("artifacts-view", () => {
  test.use({ viewport: { width: 1280, height: 1000 } });

  test.beforeEach(async ({ page, request }) => {
    await enableBacklogFeatureFlag(request);
    client = new SessionClient(BASE_URL);
    consoleErrors = collectConsoleErrors(page);
    await skipOnboarding(page);
    artifacts = new ArtifactsPage(page);
  });

  test.afterEach(async () => {
    for (const id of created.splice(0)) await client.deleteSession(id, true).catch(() => undefined);
    expect(consoleErrors, "no console or page errors").toEqual([]);
  });

  test("page should list only acme/app PR records with session link and chip", async ({ page }) => {
    await artifacts.mockBrowse((req) => browseResponse(serverFilter(req)));
    await artifacts.goto("?type=pr&repo=acme/app");
    await expect(artifacts.rows).toHaveCount(2);
    expect(artifacts.requests.browse[0]).toMatchObject({ types: ["pr"], repo: "acme/app" });
    for (const row of await artifacts.rows.all()) {
      await expect(row.getByTestId("pr-chip")).toBeVisible();
      await expect(artifacts.sessionLink(row)).toHaveAttribute("href", "/?session=s1&tab=artifacts");
    }
    await expect(page.getByRole("columnheader")).toHaveCount(4);
    await expect(artifacts.count).toHaveText("Showing 2 of 2");
  });

  test("search should show matched type per row and Clear returns to browse in one action", async ({ page }) => {
    await artifacts.mockBrowse((req) => browseResponse(serverFilter(req)));
    await artifacts.mockSearch(() => ({
      hits: [hitJson({ sessionId: "s1", title: "refactor-auth", source: "artifact", matchType: "commit", matchText: "1a2b3c4d5e6f1a2b3c4d5e6f1a2b3c4d5e6f1a2b" })],
      unscannedSessionCount: 0,
    }));
    await artifacts.goto();
    await expect(artifacts.rows).toHaveCount(4);
    await artifacts.submitSearch("1a2b3c4");
    await expect(artifacts.rows).toHaveCount(1);
    expect(artifacts.requests.search[0]).toMatchObject({ query: "1a2b3c4" });
    await expect(artifacts.rows.first()).toContainText("commit");
    await expect(artifacts.rows.first()).toContainText("1a2b3c4d5e6f");
    await expect(page).toHaveURL(/[?&]q=1a2b3c4/);
    await expect(page.getByTestId("artifacts-search-sha-hint")).toHaveCount(0); // 7 characters: no hint

    await artifacts.clearSearch.click();
    await expect(artifacts.rows).toHaveCount(4);
    await expect(page).not.toHaveURL(/[?&]q=/);
    await expect(artifacts.search).toHaveValue("");
  });

  test("search box should reject one character inline and hint at a short SHA", async () => {
    await artifacts.mockBrowse((req) => browseResponse(serverFilter(req)));
    await artifacts.mockSearch(() => ({ hits: [], unscannedSessionCount: 0 }));
    await artifacts.goto();
    await artifacts.submitSearch("a");
    await expect(artifacts.root.getByTestId("artifacts-search-invalid")).toHaveText("Enter at least 2 characters");
    expect(artifacts.requests.search).toHaveLength(0);
    await artifacts.submitSearch("1a2b3c");
    await expect(artifacts.root.getByTestId("artifacts-search-sha-hint")).toHaveText("Looks like a commit SHA. Use at least 7 characters to match a commit.");
    await expect(artifacts.empty).toContainText('No sessions found for "1a2b3c".');
    expect(artifacts.requests.search).toHaveLength(1);
  });

  test("page should show distinct copy and exit action for loading empty-browse empty-filter empty-search search-error browse-error load-more-error", async ({ page }) => {
    // empty browse
    await artifacts.mockBrowse(() => browseResponse([]));
    await artifacts.goto();
    await expect(artifacts.empty).toContainText("No artifacts yet. Artifacts appear as sessions open PRs, make commits and write plans.");
    await expect(artifacts.empty.getByRole("link", { name: "Go to sessions" })).toBeVisible();

    // empty because of filters, with a Clear filters exit
    await page.goto(`${BASE_URL}/artifacts?repo=nobody/none`, { waitUntil: "domcontentloaded" });
    await expect(artifacts.empty).toContainText("No artifacts match these filters.");
    await artifacts.empty.getByRole("button", { name: "Clear filters" }).click();
    await expect(page).not.toHaveURL(/repo=/);

    // empty search
    await artifacts.mockSearch(() => ({ hits: [], unscannedSessionCount: 0 }));
    await artifacts.submitSearch("zebra");
    await expect(artifacts.empty).toContainText('No sessions found for "zebra".');
    await expect(artifacts.empty).toContainText("Try the PR number (acme/app#12), a branch name, or a SHA prefix of at least 7 characters");

    // search error: Retry and Clear search
    await page.unroute("**/api/session.v1.SessionService/SearchSessionArtifacts");
    await artifacts.mockSearch(() => null);
    await artifacts.submitSearch("zebra2");
    await expect(artifacts.error).toHaveText(/Search failed\. Browse still works\./);
    await expect(artifacts.retry).toBeVisible();
    await artifacts.error.getByRole("button", { name: "Clear search" }).click();
    await expect(artifacts.error).toHaveCount(0);

    // browse error: Retry reloads
    await page.unroute("**/api/session.v1.SessionService/ListSessionArtifacts");
    let fail = true;
    await artifacts.mockBrowse(() => (fail ? null : browseResponse(ALL_ROWS)));
    await page.goto(`${BASE_URL}/artifacts`, { waitUntil: "domcontentloaded" });
    await expect(artifacts.error).toContainText("Couldn't load artifacts");
    fail = false;
    await artifacts.retry.click();
    await expect(artifacts.rows).toHaveCount(4);

    // load more error keeps the loaded rows
    await page.unroute("**/api/session.v1.SessionService/ListSessionArtifacts");
    let calls = 0;
    await artifacts.mockBrowse(() => {
      calls += 1;
      if (calls === 1) return { rows: ALL_ROWS.slice(0, 2).map(rowJson), nextPageToken: "t1", totalCount: 4, unscannedSessionCount: 0 };
      if (calls === 2) return null;
      return { rows: ALL_ROWS.slice(2).map(rowJson), totalCount: 4, unscannedSessionCount: 0 };
    });
    await page.goto(`${BASE_URL}/artifacts`, { waitUntil: "domcontentloaded" });
    await expect(artifacts.rows).toHaveCount(2);
    await artifacts.loadMore.click();
    await expect(page.getByTestId("artifacts-load-more-error")).toContainText("Couldn't load more");
    await expect(artifacts.rows).toHaveCount(2);
    await page.getByTestId("artifacts-load-more-error").getByRole("button", { name: "Retry" }).click();
    await expect(artifacts.rows).toHaveCount(4);
    await expect(artifacts.loadMore).toHaveCount(0);
  });

  test("page should turn a 15 second wait into the browse error state", async ({ page }) => {
    await page.clock.install();
    await artifacts.hangBrowse();
    await page.goto(`${BASE_URL}/artifacts`, { waitUntil: "domcontentloaded" });
    await expect(page.getByTestId("artifacts-loading")).toBeVisible();
    await page.clock.fastForward(15_000);
    await expect(artifacts.error).toContainText("Couldn't load artifacts");
  });

  test("reload and Back should restore filters and query", async ({ page }) => {
    await artifacts.mockBrowse((req) => browseResponse(serverFilter(req)));
    await artifacts.goto();
    await artifacts.filterType.selectOption("pr");
    await expect(page).toHaveURL(/type=pr/);
    await artifacts.filterRepo.fill("acme/app");
    await artifacts.filterRepo.press("Enter");
    await expect(page).toHaveURL(/repo=acme%2Fapp/);
    await expect(artifacts.rows).toHaveCount(2);

    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(artifacts.filterType).toHaveValue("pr");
    await expect(artifacts.filterRepo).toHaveValue("acme/app");
    await expect(artifacts.rows).toHaveCount(2);

    await artifacts.sessionLink(artifacts.rows.first()).click();
    await expect(page).toHaveURL(/session=s1/);
    await page.goBack();
    await expect(artifacts.filterType).toHaveValue("pr");
    await expect(artifacts.rows).toHaveCount(2);
  });

  test("result count and loading should be announced via one role=status region", async ({ page }) => {
    await artifacts.mockBrowse((req) => browseResponse(serverFilter(req)));
    await artifacts.goto();
    await expect(artifacts.rows).toHaveCount(4);
    await expect(artifacts.root.locator("[role=status]")).toHaveCount(0);
    await expect(page.getByTestId("announcer-polite")).toHaveText(/Showing 4 of 4/);
    await expect(page.locator("[role=status]").filter({ hasText: "Showing 4 of 4" })).toHaveCount(1);
  });

  test("unscanned sessions should show the incomplete-results notice", async () => {
    await artifacts.mockBrowse(() => browseResponse(ALL_ROWS, { unscannedSessionCount: 2 }));
    await artifacts.goto();
    await expect(artifacts.unscanned).toContainText("2 sessions aren't scanned yet, so results may be incomplete");
  });

  test("page should have no WCAG AA violations in light and dark", async () => {
    await artifacts.mockBrowse(() => browseResponse(ALL_ROWS, { unscannedSessionCount: 1 }));
    await artifacts.goto();
    await expect(artifacts.rows).toHaveCount(4);
    const violations = await axeLightAndDark(artifacts.root.page(), "[data-testid=artifacts-page]", async () => {
      await expect(artifacts.rows).toHaveCount(4);
    });
    expect(violations).toEqual([]);
  });

  test("artifacts page should be reachable from main navigation in two clicks", async ({ page }) => {
    await artifacts.mockBrowse(() => browseResponse(ALL_ROWS));
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
    await page.getByTestId("drawer-nav").getByRole("link", { name: "Artifacts" }).click();
    await expect(page).toHaveURL(/\/artifacts$/);
    await expect(artifacts.heading).toBeVisible();
    await expect(page).toHaveTitle("Artifacts - Stapler Squad");
  });

  test("the omnibar link /artifacts?pr=acme/app%2312 should dispatch FindSessionsByPullRequest and list Tracked before Seen", async () => {
    await artifacts.mockFind(() => ({
      hits: [
        hitJson({ sessionId: "s1", title: "owner-session", source: "session_pr" }),
        hitJson({ sessionId: "s2", title: "mentioned-only", source: "artifact" }),
      ],
      unscannedSessionCount: 0,
    }));
    await artifacts.goto("?pr=acme/app%2312");
    expect(artifacts.requests.find[0]).toMatchObject({ pr: { owner: "acme", repo: "app", number: 12 } });
    await expect(artifacts.rows).toHaveCount(2);
    await expect(artifacts.rows.nth(0)).toContainText("Tracked");
    await expect(artifacts.rows.nth(1)).toContainText("Seen in transcript");
  });

  test("a single match should open that session with a View all toast that returns to the list", async ({ page }) => {
    const session = await client.createSession({ title: `e2e-artifacts-pr-${Date.now()}`, path: "/tmp", program: "bash" });
    created.push(session.id);
    await waitUntilSettled(client, session.id);
    await artifacts.mockFind(() => ({ hits: [hitJson({ sessionId: session.id, title: "only-one", source: "session_pr" })], unscannedSessionCount: 0 }));
    await page.goto(`${BASE_URL}/artifacts?pr=acme/app%2312`, { waitUntil: "domcontentloaded" });
    await expect(page).toHaveURL(new RegExp(`session=${session.id}`));
    await expect(page.getByText("Opened the session tracking acme/app#12 (Tracked)").last()).toBeVisible();
    await page.getByRole("button", { name: "View all" }).click();
    await expect(page).toHaveURL(/\/artifacts\?.*view=all/);
    await expect(artifacts.rows).toHaveCount(1);
    expect(artifacts.requests.find.length).toBeGreaterThanOrEqual(2);
  });

  test("browse should reach the real server and show the empty state for an unscanned workspace", async ({ page }) => {
    await artifacts.goto();
    await expect(page.getByTestId("artifacts-loading")).toHaveCount(0);
    await expect(artifacts.error).toHaveCount(0);
    await expect(artifacts.empty).toContainText("No artifacts yet.");
  });
});
