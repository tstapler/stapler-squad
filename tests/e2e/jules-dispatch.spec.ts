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
 */

import { test, expect, APIRequestContext } from "@playwright/test";
import { BacklogPage } from "./pages/BacklogPage";
import { BacklogItemDetailPage } from "./pages/BacklogItemDetailPage";
import { seedWorkSessionWithWorktreeDirect, enableBacklogFeatureFlag } from "./pages/BacklogMutations";
import { JulesDispatchPage, updateJulesConfigDirect } from "./pages/JulesDispatchPage";
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
});
