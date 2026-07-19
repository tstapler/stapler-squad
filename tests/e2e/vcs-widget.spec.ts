// @feature vcs-widget, backlog:get-item-ship-status
/**
 * E2E coverage for the unified VcsWidget (project_plans/unified-vcs-widget), Phase 5
 * Epic 5.2 Story 5.2.1. Per the plan, this covers the DB-backed, deterministic half of
 * the widget — the durable ship-status snapshot path (Backlog item detail) and the
 * compact mode (Unfinished item detail) — not the live-GitHub-API polling half, which
 * is lower priority for e2e determinism per pitfalls research §6.
 *
 * Durable-snapshot fields (ShippedSnapshotAt, ShippedFileStats, ShippedCheckConclusion,
 * etc.) are only ever populated server-side by CaptureShipSnapshot during a real
 * ReconcilePRPending reconciler tick against a real merged GitHub PR — there is no RPC
 * that lets a test set them directly (see proto/session/v1/backlog.proto's
 * BacklogItemShipStatus: every snapshot field is response-only). Reproducing that path
 * in e2e would require a real GitHub PR merge, which is exactly the slow/flaky
 * live-API cost this story's own rationale says to avoid. Following the same
 * documented precedent as backlog-pipeline-mode.spec.ts's
 * injectFakeSessionWithPipelineSnapshot (intercepting a real RPC response to inject
 * server-computed-only fields onto a real, API-created backlog item), the
 * durable-snapshot and no-snapshot tests below intercept GetBacklogItemShipStatus for
 * a real item and fulfill a fabricated response — exercising the real frontend
 * fromShipStatus adapter + VcsWidget rendering path end-to-end, not the backend
 * snapshot-write path (which has its own Go coverage — see Story 3.4.1's tests in
 * server/services/backlog_service_ship_status_test.go).
 *
 * The compact-mode test uses real seeded git data (bare repo + worktree + one ahead
 * commit + one uncommitted change), mirroring unfinished-work.spec.ts's established
 * pattern, since fromUnfinishedWorktree's inputs come from a live worktree scan that
 * is cheap and deterministic to reproduce for real.
 */

import { test, expect, APIRequestContext, Page } from "@playwright/test";
import { execSync } from "child_process";
import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import { VcsWidgetPage } from "./pages/VcsWidgetPage";

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

async function createBacklogItemViaApi(request: APIRequestContext, title: string): Promise<string> {
  const res = await request.post(`${BASE_URL}/api/session.v1.BacklogService/CreateBacklogItem`, {
    headers: { "Content-Type": "application/json" },
    data: { title, priority: 3, repoPath: "", skipTriage: true },
  });
  if (!res.ok()) {
    throw new Error(`CreateBacklogItem failed: ${res.status()} ${await res.text()}`);
  }
  const body = (await res.json()) as { item?: { id: string } };
  const id = body.item?.id;
  if (!id) throw new Error("CreateBacklogItem did not return an item id");
  return id;
}

async function archiveBacklogItemViaApi(request: APIRequestContext, id: string) {
  await request
    .post(`${BASE_URL}/api/session.v1.BacklogService/ArchiveBacklogItem`, {
      headers: { "Content-Type": "application/json" },
      data: { id },
    })
    .catch(() => {
      // Best-effort cleanup — do not fail the test on cleanup errors.
    });
}

/**
 * Intercepts GetBacklogItemShipStatus for `itemId` and fulfills a fabricated
 * BacklogItemShipStatus response — see the file-level doc comment for rationale.
 */
async function mockShipStatus(page: Page, itemId: string, status: Record<string, unknown>) {
  await page.route("**/api/session.v1.BacklogService/GetBacklogItemShipStatus", async (route) => {
    const postData = route.request().postDataJSON() as { itemId?: string };
    if (postData?.itemId !== itemId) {
      await route.continue();
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ status }),
    });
  });
}

test.describe("vcs-widget", () => {
  test.describe("Backlog item detail (durable snapshot)", () => {
    test.beforeAll(async ({ request }) => {
      await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
        headers: { "Content-Type": "application/json" },
        data: { name: "backlog", enabled: true },
      });
      await waitForBacklogRPCsEnabled(request);
    });

    test.afterAll(async ({ request }) => {
      await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
        headers: { "Content-Type": "application/json" },
        data: { name: "backlog", enabled: false },
      });
    });

    test.beforeEach(async ({ page }) => {
      await page.addInitScript(() => {
        localStorage.setItem("stapler-squad:backlog-onboarded", "true");
      });
    });

    test("VcsWidget_should_RenderPillFileListCommitListAndAsOfTimestamp_When_DurableSnapshotPresent", async ({
      page,
      request,
    }) => {
      const itemId = await createBacklogItemViaApi(request, `vcs-widget-snapshot-${Date.now()}`);
      try {
        await mockShipStatus(page, itemId, {
          shipped: true,
          shippedVia: "pr",
          prUrl: "https://github.com/acme/widget/pull/42",
          branchName: "feature/vcs-widget-e2e",
          branchExists: false,
          aheadOfMain: 0,
          behindMain: 0,
          lastCommitSha: "abc123def456",
          lastCommitMessage: "feat: add durable snapshot rendering",
          lastCommitAt: "2026-07-10T12:00:00Z",
          error: "",
          commits: [
            {
              sha: "abc123def456",
              summary: "feat: add durable snapshot rendering",
              authorName: "E2E Bot",
              authoredAt: "2026-07-10T12:00:00Z",
            },
            {
              sha: "def456abc789",
              summary: "fix: typo in snapshot copy",
              authorName: "E2E Bot",
              authoredAt: "2026-07-09T09:00:00Z",
            },
          ],
          shippedCheckConclusion: "success",
          shippedApprovedCount: 2,
          shippedChangesReqCount: 0,
          fileStats: [
            { path: "src/foo.ts", status: "FILE_STATUS_MODIFIED", additions: 12, deletions: 3 },
            { path: "src/bar.ts", status: "FILE_STATUS_ADDED", additions: 40, deletions: 0 },
          ],
          snapshotAt: "2026-07-10T12:05:00Z",
          snapshotCaptureFailed: false,
        });

        await page.goto(`${BASE_URL}/backlog?item=${itemId}`, { waitUntil: "domcontentloaded" });
        await page.waitForSelector('[data-testid="backlog-item-detail"]', { timeout: 10000 });

        const widget = new VcsWidgetPage(page);
        await widget.waitForLoaded();

        await expect(widget.getMergeabilityPill()).toBeVisible();
        await expect(widget.getMergeabilityPill()).toHaveText(/Shipped/);

        await expect(widget.getFileRow("src/foo.ts")).toBeVisible();
        await expect(widget.getFileRow("src/bar.ts")).toBeVisible();

        await expect(widget.getCommitList()).toBeVisible();
        await expect(widget.getCommitRow("feat: add durable snapshot rendering")).toBeVisible();
        await expect(widget.getCommitRow("fix: typo in snapshot copy")).toBeVisible();

        await expect(widget.getSnapshotTimestamp()).toBeVisible();
        await expect(widget.getSnapshotTimestamp()).toHaveText(/As of/);
      } finally {
        await archiveBacklogItemViaApi(request, itemId);
      }
    });

    // B7: a legitimately-shipped pre-feature item (ShippedSnapshotAt unset, no
    // backend error) must render the neutral "No history captured for this item"
    // copy rather than a blank/error widget. VcsWidget's neutral-copy gate fires on
    // `kind === "historical" && !snapshotAt` with a default fallback message when
    // `loadError` is empty (see VcsWidget.tsx's `showNeutralLoadError`).
    test("VcsWidget_should_RenderNoHistoryCapturedCopy_When_ShippedSnapshotAtUnset", async ({ page, request }) => {
      const itemId = await createBacklogItemViaApi(request, `vcs-widget-no-snapshot-${Date.now()}`);
      try {
        await mockShipStatus(page, itemId, {
          shipped: true,
          shippedVia: "direct",
          prUrl: "",
          branchName: "",
          branchExists: false,
          aheadOfMain: 0,
          behindMain: 0,
          lastCommitSha: "",
          lastCommitMessage: "",
          error: "",
          commits: [],
        });

        await page.goto(`${BASE_URL}/backlog?item=${itemId}`, { waitUntil: "domcontentloaded" });
        await page.waitForSelector('[data-testid="backlog-item-detail"]', { timeout: 10000 });

        const widget = new VcsWidgetPage(page);
        await widget.waitForLoaded();

        await expect(widget.getNoHistoryMessage()).toBeVisible();
      } finally {
        await archiveBacklogItemViaApi(request, itemId);
      }
    });
  });

  test.describe("Unfinished item detail (compact mode)", () => {
    let testRepoDir: string;
    let testWorktreeDir: string;
    let branchName: string;

    async function rpc(service: string, method: string, body: object): Promise<Response> {
      return fetch(`${BASE_URL}/${service}/${method}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
    }

    async function addPinnedRepoViaApi(repoPath: string): Promise<void> {
      const res = await rpc("session.v1.UnfinishedWorkService", "UpdateUnfinishedWorkConfig", {
        config: { autoSpiderSessions: false, watchDirs: [], pinnedRepos: [repoPath] },
      });
      if (!res.ok) throw new Error(`UpdateUnfinishedWorkConfig failed: ${res.status} ${await res.text()}`);
    }

    async function triggerScanAndWait(timeoutMs = 15000): Promise<void> {
      await rpc("session.v1.UnfinishedWorkService", "ScanUnfinishedWork", {});
      const deadline = Date.now() + timeoutMs;
      while (Date.now() < deadline) {
        const res = await rpc("session.v1.UnfinishedWorkService", "ListUnfinishedWork", {});
        if (res.ok) {
          const data = (await res.json()) as { worktrees?: Array<{ branch?: string }> };
          if (data.worktrees?.some((w) => w.branch === branchName)) return;
        }
        await new Promise((r) => setTimeout(r, 500));
      }
    }

    test.beforeAll(async () => {
      branchName = `e2e-vcs-widget-compact-${Date.now()}`;
      testRepoDir = fs.mkdtempSync(path.join(os.tmpdir(), "sq-e2e-vcs-repo-"));
      testWorktreeDir = fs.mkdtempSync(path.join(os.tmpdir(), "sq-e2e-vcs-wt-"));

      execSync(`git init --bare "${testRepoDir}"`);

      const seedDir = fs.mkdtempSync(path.join(os.tmpdir(), "sq-e2e-vcs-seed-"));
      execSync(`git clone "${testRepoDir}" "${seedDir}"`);
      execSync('git config user.email "test@example.com"', { cwd: seedDir });
      execSync('git config user.name "Test"', { cwd: seedDir });
      fs.writeFileSync(path.join(seedDir, "README.md"), "init\n");
      execSync("git add . && git commit -m \"init\"", { cwd: seedDir });
      execSync("git push origin main", { cwd: seedDir });

      execSync(`git worktree add -b ${branchName} "${testWorktreeDir}"`, { cwd: seedDir });

      // One real commit ahead of main — populates aheadCommitMessages -> the
      // compact commit list.
      fs.writeFileSync(path.join(testWorktreeDir, "feature.ts"), "export const feature = true;\n");
      execSync("git add . && git commit -m \"feat: add compact-mode e2e fixture file\"", { cwd: testWorktreeDir });

      // One uncommitted change on top — populates changedFiles/linesAdded ->
      // the compact aggregate stat line.
      fs.writeFileSync(path.join(testWorktreeDir, "README.md"), "init\nwork in progress\n");

      await addPinnedRepoViaApi(testRepoDir);
      await triggerScanAndWait();
    });

    test.afterAll(() => {
      try {
        fs.rmSync(testRepoDir, { recursive: true, force: true });
      } catch {
        /* ignore */
      }
      try {
        fs.rmSync(testWorktreeDir, { recursive: true, force: true });
      } catch {
        /* ignore */
      }
    });

    test("VcsWidget_should_RenderAggregateStatsAndCommitsWithNoPerFileRows_When_CompactModeExpanded", async ({
      page,
    }) => {
      await page.goto(`${BASE_URL}/unfinished`, { waitUntil: "domcontentloaded", timeout: 15000 });

      const item = page.locator('[data-testid="unfinished-item"]').filter({ hasText: branchName });
      await expect(item).toBeVisible({ timeout: 10000 });
      await item.click();
      await expect(item).toHaveAttribute("aria-expanded", "true");

      const widget = new VcsWidgetPage(page);
      await widget.waitForLoaded();

      await expect(widget.getAggregateStatLine()).toBeVisible();
      await expect(widget.getCommitList()).toBeVisible();
      await expect(widget.getCommitRow("feat: add compact-mode e2e fixture file")).toBeVisible();

      // Compact mode never renders VcsWidgetFileList — assert none of its
      // per-file section headings are present anywhere in the widget.
      await expect(widget.widget.getByText(/Unstaged Changes|Staged Changes|Untracked Files|Conflicts \(/)).toHaveCount(
        0
      );
    });
  });
});
