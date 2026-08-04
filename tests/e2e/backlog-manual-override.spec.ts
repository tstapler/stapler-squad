// @feature backlog:update-item, backlog:transition-status, backlog-manual-override-control, backlog-link-existing-pr-control
/**
 * E2E tests for the manual escape-hatch feature (project 7a383b3b — "manual
 * escape hatch: associate a PR / override status on a backlog item by
 * hand"): the always-visible "Manual Override" status control
 * (ManualOverrideSection.tsx) and the "Link existing PR" form added to
 * PullRequestSection.tsx.
 *
 * Prerequisites:
 *   STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local \
 *   ./stapler-squad --tmux-keep-server &
 *
 * Both flows are exercised against a real, API-created backlog item
 * (createBacklogItemDirect) in "review" status — the only status the PR
 * association write currently supports (SetBacklogItemPRAndTransition's
 * hardcoded ExpectedStatus=review, session/storage.go) — and asserted via
 * StageTracker's `aria-current="step"` on the target stage node, per this
 * repo's ARIA/data-testid-only locator convention.
 */

import { test, expect } from "@playwright/test";
import { createBacklogItemDirect } from "./pages/BacklogMutations";

test.describe("backlog-manual-override", () => {
  test("manual status override transitions a backlog item directly, separate from the automated pipeline's buttons", async ({
    page,
    request,
  }) => {
    const itemId = await createBacklogItemDirect(request, {
      title: `e2e-manual-override-${Date.now()}`,
      status: "review",
    });

    await page.goto(`/backlog?item=${itemId}`, { waitUntil: "domcontentloaded" });
    await page.getByTestId("backlog-item-detail").waitFor({ state: "visible" });

    const overrideHeader = page.getByTestId("collapsible-header-manual-override");
    if ((await overrideHeader.getAttribute("aria-expanded")) !== "true") {
      await overrideHeader.click();
    }

    const toggle = page.getByTestId("backlog-manual-override-toggle");
    await expect(toggle).toBeEnabled();
    await toggle.click();

    const statusSelect = page.getByTestId("backlog-manual-override-status");
    await statusSelect.selectOption("in_progress");

    const reasonTextarea = page.getByTestId("backlog-manual-override-reason");
    await reasonTextarea.fill("Recovering a wedged item — automation stopped responding.");

    const submit = page.getByTestId("backlog-manual-override-submit");
    await expect(submit).toBeEnabled();
    await submit.click();

    await expect(page.getByTestId("stage-node-in_progress")).toHaveAttribute("aria-current", "step");
  });

  test("manual override submit stays disabled until both a target status and a reason are provided", async ({
    page,
    request,
  }) => {
    const itemId = await createBacklogItemDirect(request, {
      title: `e2e-manual-override-disabled-${Date.now()}`,
      status: "review",
    });

    await page.goto(`/backlog?item=${itemId}`, { waitUntil: "domcontentloaded" });
    await page.getByTestId("backlog-item-detail").waitFor({ state: "visible" });

    const overrideHeader = page.getByTestId("collapsible-header-manual-override");
    if ((await overrideHeader.getAttribute("aria-expanded")) !== "true") {
      await overrideHeader.click();
    }
    await page.getByTestId("backlog-manual-override-toggle").click();

    const submit = page.getByTestId("backlog-manual-override-submit");
    await expect(submit).toBeDisabled();

    await page.getByTestId("backlog-manual-override-status").selectOption("in_progress");
    await expect(submit).toBeDisabled();

    // Below the 5-character minimum — still disabled.
    await page.getByTestId("backlog-manual-override-reason").fill("hi");
    await expect(submit).toBeDisabled();

    await page.getByTestId("backlog-manual-override-reason").fill("A sufficiently detailed reason.");
    await expect(submit).toBeEnabled();
  });

  test("linking an existing PR associates it with a review-status item, no live session required", async ({
    page,
    request,
  }) => {
    const itemId = await createBacklogItemDirect(request, {
      title: `e2e-link-existing-pr-${Date.now()}`,
      status: "review",
    });

    await page.goto(`/backlog?item=${itemId}`, { waitUntil: "domcontentloaded" });
    await page.getByTestId("backlog-item-detail").waitFor({ state: "visible" });

    const prHeader = page.getByTestId("collapsible-header-pull-request");
    if ((await prHeader.getAttribute("aria-expanded")) !== "true") {
      await prHeader.click();
    }

    const toggle = page.getByTestId("backlog-link-existing-pr-toggle");
    await toggle.click();

    await page.getByTestId("backlog-link-existing-pr-url").fill("https://github.com/acme/widgets/pull/321");
    await page.getByTestId("backlog-link-existing-pr-number").fill("321");

    const submit = page.getByTestId("backlog-link-existing-pr-submit");
    await expect(submit).toBeEnabled();
    await submit.click();

    await expect(page.getByTestId("stage-node-pr_pending")).toHaveAttribute("aria-current", "step");
  });
});
