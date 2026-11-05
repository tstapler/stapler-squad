// @feature vcs-widget
/**
 * E2E coverage for the unified VcsWidget compact mode (project_plans/unified-vcs-widget,
 * Phase 5 Epic 5.2 Story 5.2.1) on the Unfinished item detail. Uses real seeded git data
 * (bare repo + worktree + one ahead commit + one uncommitted change), mirroring
 * unfinished-work.spec.ts, since fromUnfinishedWorktree's inputs come from a live
 * worktree scan.
 *
 * The durable-snapshot half (Backlog item detail) was fully mocked-RPC-driven and moved to
 * Jest: VcsWidget.test.tsx (As-of timestamp, no-history copy) and adapters.test.ts
 * (fromShipStatus). See project_plans/e2e-test-parallelization/decisions/test-content-triage-sample-2.md.
 */

import { test, expect } from "@playwright/test";
import { execSync } from "child_process";
import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import { VcsWidgetPage } from "./pages/VcsWidgetPage";
import { baseUrl } from './helpers/base-url';

const BASE_URL = baseUrl();

test.describe("vcs-widget", () => {
  test.describe("Unfinished item detail (compact mode)", () => {
    let testRepoDir: string;
    let testWorktreeDir: string;
    let branchName: string;

    async function rpc(service: string, method: string, body: object): Promise<Response> {
      return fetch(`${BASE_URL}/api/${service}/${method}`, {
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

      // Pin the non-bare clone: a bare repo has no .git dir and is rejected.
      await addPinnedRepoViaApi(seedDir);
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
      await page.goto(`${BASE_URL}/unfinished?tab=worktrees`, { waitUntil: "domcontentloaded", timeout: 15000 });

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
