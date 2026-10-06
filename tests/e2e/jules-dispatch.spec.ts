// @feature backlog:dispatch-to-jules, jules-settings
/**
 * E2E coverage for the gated "Dispatch to Jules" flow
 * (project_plans/google-jules-integration, Epic 4.1 Story 4.1.3) — the one
 * path with a real privacy consequence (Jules runs on Google's
 * infrastructure), so a regression here matters more than most UI gating.
 *
 * Two scenarios:
 *   1. Jules disabled -> the trigger button isn't attached to the DOM at
 *      all on a ready item's detail page (never a disabled dead button).
 *   2. Jules enabled with a stubbed key and an unacknowledged repo -> the
 *      dialog opens, and Dispatch stays disabled until the egress
 *      confirmation checkbox is checked.
 *
 * Neither scenario ever reaches a real Jules API call — scenario 2 never
 * clicks Dispatch (BacklogService.DispatchToJules, which would need a
 * registered Jules source) — so no external stub/mock seam is needed beyond
 * the stubbed API key string UpdateJulesConfig stores in the test server's
 * own (test-instance-scoped) OS keychain.
 *
 * Prerequisites:
 *   STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local \
 *   ./stapler-squad --tmux-keep-server &
 *
 * VERIFICATION STATUS (2026-09-01): scenario 1 was run against a live
 * ./stapler-squad instance (npx playwright test jules-dispatch.spec.ts
 * --project=chromium) and passes reliably. Scenario 2 was ALSO run but
 * currently fails against a live instance — not because this spec or the
 * gating logic it exercises is wrong, but because of a pre-existing bug
 * found during that run: BacklogItemDetail.tsx's GetJulesConfig-fetching
 * useEffect (Story 3.2.2, ~line 638) never executes in a production
 * (`next build` static-export) build, so julesDispatchGate never leaves its
 * `hidden` default and the "Dispatch to Jules" trigger never renders even
 * when GetJulesConfig genuinely returns `enabled: true, has_api_key: true`
 * (confirmed correct via a direct HTTP call to the same running server).
 * The same symptom reproduces on /settings/jules (JulesSettings.tsx uses an
 * identical raw createConnectTransport+createClient(SessionService, ...)
 * pattern): the component's own content never mounts — the browser keeps
 * showing the root "/" session-cockpit view under that URL, even though the
 * server's static HTML for /settings/jules is confirmed correct on disk.
 * Ruled out as causes: backend/RPC correctness, stale build artifacts
 * (rebuilt `web-app` and copied to `server/web/dist` twice), and JS
 * exceptions (zero console/pageerror events surfaced). Root cause not
 * further isolated — likely a Next.js static-export client hydration/
 * routing issue — and is out of Epic 4.1's scope to fix; filed here as a
 * discovered gap rather than silently left unverified.
 *
 * SECOND PRE-EXISTING BUG found while adding §7.2/§7.9/§7.13/§7.15 below
 * (2026-09-01), unrelated to Jules: GetBacklogItem's handler
 * (backlog_service_query.go) enriches each returned ItemSession with
 * worktree_branch/worktree_path via a per-session GetWorktreeDataBySessionUUID
 * lookup AFTER the shared backlogItemToProto conversion — a step ListBacklogItems
 * (and the WatchBacklogItems push it shares backlogItemToProto with) does not
 * perform, presumably to avoid an N+1 query over a whole list/every event.
 * BacklogItemDetail.tsx's own useWatchBacklogItems() call fires an
 * unconditional mount-time ListBacklogItems refresh() (useWatchBacklogItems.ts
 * :190-220) alongside the panel's own full, enriched GetBacklogItem load() —
 * both write into the same shared item through an equal-updated_at-passes
 * staleness guard (BacklogItemDetail.tsx's liveRawItem effect), so whichever
 * response's dispatch lands last always overwrites, deterministically once it
 * does — not a one-time coin flip resolved at mount, which is why a bounded
 * reload-and-retry mitigation (tried first) still intermittently exhausted
 * its attempts. Independently verified correct via direct GetBacklogItem/
 * ListBacklogItems calls against the same server (a client-side merge bug,
 * not a data bug). Not fixed here (out of this file's scope, and a shared
 * useWatchBacklogItems.ts/backlog_service_query.go fix risks colliding with
 * concurrent work on those files) — openItemAvoidingWatchListRace below
 * neuters just the one competing ListBacklogItems call, scoped to a single
 * open, rather than trying to outrun it.
 */

import { test, expect, APIRequestContext, Page } from "@playwright/test";
import { BacklogPage } from "./pages/BacklogPage";
import { BacklogItemDetailPage } from "./pages/BacklogItemDetailPage";
import {
  seedWorkSessionWithWorktreeDirect,
  createBacklogItemDirect,
  enableBacklogFeatureFlag,
} from "./pages/BacklogMutations";
import {
  JulesDispatchPage,
  updateJulesConfigDirect,
  confirmEgressConsentDirect,
  interceptDispatchToJulesWithSeededSession,
} from "./pages/JulesDispatchPage";
import { dismissOnboardingIfPresent } from "./pages/OnboardingPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

async function waitForBacklogRPCsEnabled(request: APIRequestContext) {
  for (let attempt = 0; attempt < 20; attempt++) {
    const resp = await request.post(`${BASE_URL}/api/session.v1.BacklogService/ListBacklogItems`, {
      headers: { "Content-Type": "application/json" },
      data: {},
    });
    if (resp.ok()) return;
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error("BacklogService RPCs did not become enabled in time");
}

/**
 * Navigates to the backlog board and opens `title`'s detail panel, working
 * around the useWatchBacklogItems.ts race documented in this file's header:
 * BacklogItemDetail.tsx's own useWatchBacklogItems() call fires an
 * unconditional mount-time ListBacklogItems REST refresh() AND opens a
 * WatchBacklogItems stream — both deliver items via plain backlogItemToProto
 * (no worktree_branch enrichment; only GetBacklogItem's handler does that
 * per-session lookup), and the store's equal-updated_at-passes staleness
 * guard lets whichever response's dispatch lands last silently overwrite the
 * correct data the panel's own GetBacklogItem load() fetched moments earlier
 * — deterministically, once it does (not a one-time coin flip, so bounded
 * reload retries could not reliably outrun it, and blocking only the REST
 * refresh alone was not sufficient either — the stream's own initial push
 * carries the same gap). This neuters BOTH calls, and ONLY around this one
 * open: the routes are registered after the board's own (real,
 * unintercepted) ListBacklogItems-backed card render has already completed
 * (waitForItemCards), and removed again immediately after, so no other
 * caller of either RPC in this test (or a later test reusing
 * `backlogPage`/`page`) is affected. Aborting WatchBacklogItems is safe: the
 * hook's own resilience design (exponential backoff, REST fallback polling)
 * treats a dropped stream as a transient disconnect, not a fatal error, and
 * unrouting immediately after lets it reconnect normally.
 */
async function openItemAvoidingWatchListRace(
  page: Page,
  backlogPage: BacklogPage,
  detailPage: BacklogItemDetailPage,
  title: string
): Promise<void> {
  await backlogPage.goto();
  await dismissOnboardingIfPresent(page);
  await backlogPage.waitForItemCards();

  const listUrl = `${BASE_URL}/api/session.v1.BacklogService/ListBacklogItems`;
  const watchUrl = `${BASE_URL}/api/session.v1.BacklogService/WatchBacklogItems`;
  await page.route(listUrl, (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: '{"items":[]}' })
  );
  await page.route(watchUrl, (route) => route.abort("failed"));
  try {
    await detailPage.openItemByTitle(title);
  } finally {
    await page.unroute(listUrl);
    await page.unroute(watchUrl);
  }
}

test.describe("jules dispatch gating", () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
    await waitForBacklogRPCsEnabled(request);
  });

  // Pre-seed the first-visit backlog tour as already dismissed (matches
  // backlog.spec.ts's beforeEach) so it doesn't pop up and block clicks on
  // the backlog table row — this spec isn't testing the tour.
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem("stapler-squad:backlog-onboarded", "true");
    });
  });

  test("dispatch-to-jules is not attached when Jules is disabled", async ({ page, request }) => {
    await updateJulesConfigDirect(request, { enabled: false });

    const title = `e2e jules disabled ${Date.now()}`;
    await seedWorkSessionWithWorktreeDirect(request, { title, status: "ready" });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await dismissOnboardingIfPresent(page);
    await backlogPage.waitForItemCards();

    const detailPage = new BacklogItemDetailPage(page);
    await detailPage.openItemByTitle(title);
    await expect(detailPage.pane).toBeVisible();

    const julesPage = new JulesDispatchPage(page);
    await expect(julesPage.trigger).not.toBeAttached();
  });

  test("Dispatch stays disabled until the egress checkbox is checked, then enables", async ({ page, request }) => {
    await updateJulesConfigDirect(request, { enabled: true, apiKey: "AIzaSyD-E2E-STUB" });

    const title = `e2e jules egress gate ${Date.now()}`;
    await seedWorkSessionWithWorktreeDirect(request, { title, status: "ready" });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await dismissOnboardingIfPresent(page);
    await backlogPage.waitForItemCards();

    const detailPage = new BacklogItemDetailPage(page);
    await detailPage.openItemByTitle(title);

    const julesPage = new JulesDispatchPage(page);
    await expect(julesPage.trigger).toBeEnabled();
    await julesPage.openDialog();

    await julesPage.fillBranch("backlog/e2e-1");
    await julesPage.fillPrompt("Investigate and fix the flaky poller test.");

    await expect(julesPage.dispatchButton()).toBeDisabled();

    await julesPage.acknowledgeEgress();

    await expect(julesPage.dispatchButton()).toBeEnabled();
  });

  // §7.2 (ux.md AC #2 / validation.md's Dispatch-happy-path row): from an
  // item's detail page, with Jules configured and the repo already
  // acknowledged, dispatch completes in <=3 interactions with the trigger
  // (Dispatch to Jules -> confirm prefilled branch+prompt -> Dispatch) and a
  // new session row appears.
  //
  // The real DispatchToJules RPC's guard chain calls the real Jules
  // ListSources endpoint before it ever reserves a session
  // (jules_dispatch_service.go, jules/source_registry.go's Resolve) --
  // verified directly against https://jules.googleapis.com, no e2e-local
  // stub key can authenticate there, so a live call can only ever fail here.
  // interceptDispatchToJulesWithSeededSession (JulesDispatchPage.ts) leaves
  // every other layer real: it intercepts only that one browser->backend
  // POST and, in its place, creates the exact ItemSession row a successful
  // dispatch would have produced on the SAME item via the debug-only
  // seed-jules-work-session endpoint's itemId-attach mode -- so the row this
  // test asserts on is observed through the real WatchBacklogItems live-
  // update path, not injected client-side. computeJulesPhase
  // (SessionsSection.tsx) never resolves an open row to "queued" -- only
  // "running" is reachable for an open jules_work row -- so that's what's
  // asserted below rather than validation.md's more casual "queued" wording.
  test("dispatches to Jules in 3 clicks when repo already acknowledged", async ({ page, request }) => {
    await updateJulesConfigDirect(request, { enabled: true, apiKey: "AIzaSyD-E2E-STUB" });

    const title = `e2e jules happy path ${Date.now()}`;
    const repoPath = `/tmp/e2e-jules-repo-${Date.now()}`;
    const { itemId } = await seedWorkSessionWithWorktreeDirect(request, { title, status: "ready", repoPath });
    await confirmEgressConsentDirect(request, repoPath);

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await dismissOnboardingIfPresent(page);
    await backlogPage.waitForItemCards();

    const detailPage = new BacklogItemDetailPage(page);
    await detailPage.openItemByTitle(title);

    const julesPage = new JulesDispatchPage(page);
    await interceptDispatchToJulesWithSeededSession(page, request, itemId);

    // Click 1: open the dialog.
    await expect(julesPage.trigger).toBeEnabled();
    await julesPage.openDialog();

    // Repo already acknowledged -> no egress checkbox, branch+prompt already
    // prefilled from the tracked branch/item title -> Dispatch is already
    // enabled with no edits needed ("confirm prefilled branch+prompt").
    await expect(julesPage.egressCheckbox).not.toBeAttached();
    await expect(julesPage.branchInput).toHaveValue("main");
    await expect(julesPage.dispatchButton()).toBeEnabled();

    // Click 2: submit.
    await julesPage.dispatchButton().click();
    await expect(julesPage.dialog).not.toBeVisible();

    // The new jules_work row appears via the real live-update path.
    await expect(detailPage.sectionHeader("sessions")).toContainText("Sessions (2)");
    const julesBadge = page.getByTestId("jules-status-badge");
    await expect(julesBadge).toBeVisible();
    await expect(julesBadge).toHaveAttribute("aria-label", "Jules: Running");
  });

  // §7.9 (ux.md AC #9): the entire dispatch flow -- open dialog, check
  // confirmation, fill branch/prompt, submit -- is completable with keyboard
  // alone. Only page.keyboard.* (and Locator.focus(), not a mouse click) is
  // used from opening the dialog onward. Uses an UNacknowledged repo so the
  // egress checkbox is actually present to "check confirmation" against
  // (useFocusTrap moves focus to it first, per JulesDispatchDialog's DOM
  // order: checkbox -> branch -> prompt -> cancel -> dispatch). Submission is
  // routed through interceptDispatchToJulesWithSeededSession for the same
  // reason as the §7.2 test above -- a real Dispatch click cannot succeed
  // without a live Jules account.
  test("completes the full dispatch flow with keyboard only", async ({ page, request }) => {
    await updateJulesConfigDirect(request, { enabled: true, apiKey: "AIzaSyD-E2E-STUB" });

    const title = `e2e jules keyboard only ${Date.now()}`;
    const repoPath = `/tmp/e2e-jules-repo-kb-${Date.now()}`;
    // Not acknowledged (no confirmEgressConsentDirect call) -- the egress
    // checkbox needs to actually be present for "check confirmation" to mean
    // anything. repoPath must still be non-empty: JulesDispatchDialog's own
    // handleDispatch makes a REAL (unintercepted) ConfirmEgressConsent call
    // first for an unacknowledged repo, and that RPC 400s on an empty
    // repo_path -- unrelated to (and unmasked by) interceptDispatchToJules-
    // WithSeededSession, which only intercepts the later DispatchToJules call.
    const { itemId } = await seedWorkSessionWithWorktreeDirect(request, { title, status: "ready", repoPath });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await dismissOnboardingIfPresent(page);
    await backlogPage.waitForItemCards();

    const detailPage = new BacklogItemDetailPage(page);
    await detailPage.openItemByTitle(title);

    const julesPage = new JulesDispatchPage(page);
    await interceptDispatchToJulesWithSeededSession(page, request, itemId);

    await expect(julesPage.trigger).toBeEnabled();
    await julesPage.openDialogViaKeyboard();

    // Check confirmation: the focus trap put focus on the egress checkbox
    // first (unacknowledged repo), so Space toggles it directly.
    await expect(julesPage.egressCheckbox).toBeFocused();
    await page.keyboard.press("Space");
    await expect(julesPage.egressCheckbox).toBeChecked();

    // Fill branch/prompt: Tab to each field, select the prefilled value, and
    // type a replacement.
    await page.keyboard.press("Tab");
    await expect(julesPage.branchInput).toBeFocused();
    await page.keyboard.press("Control+A");
    await page.keyboard.type("backlog/e2e-keyboard-only");

    await page.keyboard.press("Tab");
    await expect(julesPage.promptInput).toBeFocused();
    await page.keyboard.press("Control+A");
    await page.keyboard.type("Investigate the flaky poller test, keyboard-driven.");

    // Submit: Tab past Cancel to Dispatch, then Enter.
    await page.keyboard.press("Tab");
    await expect(julesPage.cancelButton).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(julesPage.dispatchButton()).toBeFocused();
    await expect(julesPage.dispatchButton()).toBeEnabled();
    await page.keyboard.press("Enter");

    await expect(julesPage.dialog).not.toBeVisible();
  });

  // §7.13 (ux.md AC #13): revoking a repo's egress acknowledgment IN
  // SETTINGS, then opening the dispatch dialog for an item in that repo,
  // re-shows the confirmation checkbox -- no stale "already acknowledged"
  // carried over. Drives the real Revoke button on /settings/jules
  // (JulesSettings.tsx) rather than calling RevokeEgressConsent directly, and
  // navigates to the item fresh afterward (BacklogItemDetail.tsx fetches
  // GetJulesConfig once per mount, ux.md §3.3 -- Settings and the backlog
  // item detail are separate top-level routes, so this navigation is exactly
  // the "no stale cache" real-world path the criterion describes, not a
  // same-instance reload used to dodge the scenario).
  test("revoking egress consent immediately re-shows the confirmation in a freshly opened dialog", async ({
    page,
    request,
  }) => {
    await updateJulesConfigDirect(request, { enabled: true, apiKey: "AIzaSyD-E2E-STUB" });

    const title = `e2e jules revoke live ${Date.now()}`;
    const repoPath = `/tmp/e2e-jules-repo-${Date.now()}`;
    await seedWorkSessionWithWorktreeDirect(request, { title, status: "ready", repoPath });
    await confirmEgressConsentDirect(request, repoPath);

    const backlogPage = new BacklogPage(page);
    const detailPage = new BacklogItemDetailPage(page);
    const julesPage = new JulesDispatchPage(page);
    await openItemAvoidingWatchListRace(page, backlogPage, detailPage, title);
    await expect(julesPage.trigger).toBeEnabled();

    // Baseline: repo already acknowledged -> no checkbox.
    await julesPage.openDialog();
    await expect(julesPage.egressCheckbox).not.toBeAttached();
    await julesPage.cancelButton.click();
    await expect(julesPage.dialog).not.toBeVisible();

    // Revoke via the real Settings UI (JulesSettings.tsx's per-repo Revoke
    // button, aria-label mirrors shortRepoLabel's last-two-path-segments
    // display form).
    const repoLabel = repoPath.split("/").filter(Boolean).slice(-2).join("/");
    await page.goto("/settings/jules", { waitUntil: "domcontentloaded" });
    await dismissOnboardingIfPresent(page);
    const revokeButton = page.getByRole("button", { name: `Revoke cloud-egress consent for ${repoLabel}` });
    await expect(revokeButton).toBeVisible();
    await revokeButton.click();
    await expect(page.getByRole("status").filter({ hasText: `Removed ${repoLabel}.` })).toBeVisible();

    // Fresh navigation back to the item -- a new BacklogItemDetail mount,
    // fetching GetJulesConfig again.
    await openItemAvoidingWatchListRace(page, backlogPage, detailPage, title);
    await expect(julesPage.trigger).toBeEnabled();
    await julesPage.openDialog();
    await expect(julesPage.egressCheckbox).toBeVisible();
  });

  // §7.15 (ux.md AC #15): the Branch field prefills from the item's tracked
  // branch (never blank) when a prior local session exists; an item with no
  // prior local session at all gets a disabled Dispatch to Jules with the
  // no-branch reason, and the dialog cannot be opened through it.
  test("prefills the Branch field from the item's tracked branch, and disables Dispatch to Jules with a no-branch reason when none exists", async ({
    page,
    request,
  }) => {
    await updateJulesConfigDirect(request, { enabled: true, apiKey: "AIzaSyD-E2E-STUB" });

    const withSessionTitle = `e2e jules branch prefill ${Date.now()}`;
    await seedWorkSessionWithWorktreeDirect(request, { title: withSessionTitle, status: "ready" });

    const noSessionTitle = `e2e jules no branch ${Date.now()}`;
    await createBacklogItemDirect(request, { title: noSessionTitle, status: "ready" });

    const backlogPage = new BacklogPage(page);
    await backlogPage.goto();
    await dismissOnboardingIfPresent(page);
    await backlogPage.waitForItemCards();

    const detailPage = new BacklogItemDetailPage(page);
    const julesPage = new JulesDispatchPage(page);

    // Half 1: prior local session -> Branch prefills from its worktree_branch
    // (seedWorkSessionWithWorktreeDirect's fixture worktree is on "main").
    await detailPage.openItemByTitle(withSessionTitle);
    await expect(julesPage.trigger).toBeEnabled();
    await julesPage.openDialog();
    await expect(julesPage.branchInput).toHaveValue("main");
    await julesPage.cancelButton.click();
    await expect(julesPage.dialog).not.toBeVisible();

    // Half 2: fresh item, zero sessions -> disabled with the no-branch
    // reason, and clicking cannot open the dialog.
    await backlogPage.goto();
    await dismissOnboardingIfPresent(page);
    await backlogPage.waitForItemCards();
    await detailPage.openItemByTitle(noSessionTitle);
    await expect(julesPage.trigger).toBeDisabled();
    await expect(julesPage.triggerReason).toHaveText(
      "This item has no branch yet — spawn a local session (or push a branch) before dispatching to Jules."
    );
    await expect(julesPage.dialog).not.toBeAttached();
  });
});
