// @feature session:update, session:list
/**
 * E2E coverage for the tag-pill provenance surfaces (design/ux.md Surfaces 1–2,
 * AC1–AC6, AC8; validation.md's "UX Acceptance Tests" table).
 *
 * `RuleTagProvenance`/the `Unclassified` sentinel are only ever populated server-side
 * by the tagging engine/LLM-fallback poller (see project_plans/session-classifier-pipeline's
 * plan.md). Reproducing that live classification path in e2e would be slow/flaky, so —
 * following the same documented precedent as ci-status-badge.spec.ts's mockCIStatus —
 * this spec creates a real session via the API and a real TaggingRule via the real
 * UpsertTaggingRule RPC (so the tooltip's rule-name resolution is genuine, not mocked),
 * then intercepts ListSessions to inject the `tags`/`ruleTagProvenance` fields the real
 * classification pipeline would have set. WatchSessions is blocked for the page's
 * lifetime so its real events can't race with and clobber the injected fields.
 */

import { test, expect, Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { SessionClient } from "./helpers/session-client";
import { TagEditorPage } from "./pages/TagEditorPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";
const LLM_SENTINEL = "llm";
const UNCLASSIFIED_TAG = "Unclassified";

async function mockSessionTags(
  page: Page,
  sessionId: string,
  tags: string[],
  ruleTagProvenance: Record<string, string>,
) {
  await page.route("**/api/session.v1.SessionService/ListSessions", async (route) => {
    const response = await route.fetch();
    const json = await response.json();
    const sessions = (json?.sessions ?? []) as Array<Record<string, unknown>>;
    const target = sessions.find((s) => s.id === sessionId);
    if (target) {
      Object.assign(target, { tags, ruleTagProvenance });
    }
    await route.fulfill({ response, json });
  });
  await page.route("**/api/session.v1.SessionService/WatchSessions", async (route) => {
    await route.abort();
  });
}

/**
 * Navigates to the dashboard and switches to Board view, where `SessionCard` (and its
 * tag-pill/provenance UI) actually renders — the default List view renders `SessionRow`
 * instead, which has no tag UI at all (see TagEditorPage.ts's class doc). Returns a
 * ready-to-use page object.
 *
 * `expectSessionTitle`/`expectTag`, if given, guard against a rare race (observed under
 * the chromium-dom project, ~2 in 50 runs): the very first ListSessions fetch issued
 * during hydration can occasionally resolve before this session's mocked tags land. A
 * reload re-issues the fetch through the same still-registered route, giving the mock a
 * second real chance rather than just re-asserting against already-stale data.
 */
async function gotoDashboard(page: Page, expectSessionTitle?: string, expectTag?: string): Promise<TagEditorPage> {
  await page.addInitScript(() => {
    localStorage.setItem("stapler-squad:onboarded", "true");
  });
  await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
  const tags = new TagEditorPage(page);
  await tags.switchToBoardView();
  if (expectSessionTitle && expectTag) {
    await expect(async () => {
      if (!(await tags.getTagPill(expectSessionTitle, expectTag).isVisible().catch(() => false))) {
        await page.reload({ waitUntil: "domcontentloaded" });
        await tags.switchToBoardView();
      }
      await expect(tags.getTagPill(expectSessionTitle, expectTag)).toBeVisible({ timeout: 3000 });
    }).toPass({ timeout: 20000 });
  }
  return tags;
}

test.describe("session-tag-provenance", () => {
  let client: SessionClient;

  test.beforeEach(() => {
    client = new SessionClient(BASE_URL);
  });

  test("AC1: pill is Tab-focusable before Edit Tags button", async ({ page }) => {
    const title = `e2e-tag-prov-tabfocus-${Date.now()}`;
    const session = await client.createSession({ title, path: "/tmp", program: "bash" });
    await mockSessionTags(page, session.id, ["MyTag"], {});

    const tags = await gotoDashboard(page, title, "MyTag");
    const pill = tags.getTagPill(title, "MyTag");
    await expect(pill).toBeVisible({ timeout: 10000 });
    await expect(pill).toHaveAttribute("tabindex", "0");

    await pill.focus();
    await expect(pill).toBeFocused();
    await page.keyboard.press("Tab");
    await expect(tags.getEditTagsButton(title)).toBeFocused();
  });

  test("AC2: provenance pill accessible name includes auto-applied text, manual tag's does not", async ({ page }) => {
    const title = `e2e-tag-prov-arialabel-${Date.now()}`;
    const session = await client.createSession({ title, path: "/tmp", program: "bash" });
    await mockSessionTags(page, session.id, ["RuleTag", "ManualTag"], { RuleTag: "some-rule-id" });

    const tags = await gotoDashboard(page, title, "RuleTag");
    await expect(tags.getTagPill(title, "RuleTag")).toHaveAccessibleName(/auto-applied/);
    await expect(tags.getTagPill(title, "ManualTag")).not.toHaveAccessibleName(/auto-applied/);
  });

  test("AC3: hover or focus shows tooltip, manual tag shows neither", async ({ page }) => {
    const title = `e2e-tag-prov-tooltip-${Date.now()}`;
    const ruleId = `e2e-rule-${Date.now()}`;
    try {
      const ruleResp = await fetch(`${BASE_URL}/api/session.v1.SessionService/UpsertTaggingRule`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          rule: { id: ruleId, name: "e2e Bugfix rule", branchPattern: "^bugfix/", outputTag: "Bugfix", priority: 50, enabled: true, source: "user" },
        }),
      });
      expect(ruleResp.ok).toBe(true);

      const session = await client.createSession({ title, path: "/tmp", program: "bash" });
      await mockSessionTags(page, session.id, ["Bugfix", "ManualTag"], { Bugfix: ruleId });

      const tags = await gotoDashboard(page, title, "Bugfix");
      const rulePill = tags.getTagPill(title, "Bugfix");
      const manualPill = tags.getTagPill(title, "ManualTag");

      await expect(rulePill).toHaveAttribute("title", "Applied by rule: e2e Bugfix rule");
      await expect(manualPill).not.toHaveAttribute("title", /.+/);

      await rulePill.focus();
      await expect(rulePill).toHaveAttribute("title", "Applied by rule: e2e Bugfix rule");
    } finally {
      await fetch(`${BASE_URL}/api/session.v1.SessionService/DeleteTaggingRule`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id: ruleId }),
      });
    }
  });

  test("AC4: tooltip dismisses on blur/mouse-out with no lingering state", async ({ page }) => {
    const title = `e2e-tag-prov-dismiss-${Date.now()}`;
    const session = await client.createSession({ title, path: "/tmp", program: "bash" });
    await mockSessionTags(page, session.id, ["RuleTag"], { RuleTag: LLM_SENTINEL });

    const tags = await gotoDashboard(page, title, "RuleTag");
    const pill = tags.getTagPill(title, "RuleTag");
    await pill.hover();
    await expect(pill).toHaveAttribute("title", "Applied by AI classification");

    // Move away — the browser-native title tooltip carries no app-managed open/closed
    // state to leak; assert the page has no visible tooltip/popover element left behind.
    await page.mouse.move(0, 0);
    await expect(page.locator('[role="tooltip"]')).toHaveCount(0);
  });

  test("AC6: Unclassified shows dashed-border/icon glyph, not color alone", async ({ page }) => {
    const title = `e2e-tag-prov-unclassified-${Date.now()}`;
    const session = await client.createSession({ title, path: "/tmp", program: "bash" });
    await mockSessionTags(page, session.id, [UNCLASSIFIED_TAG], { [UNCLASSIFIED_TAG]: LLM_SENTINEL });

    const tags = await gotoDashboard(page, title, UNCLASSIFIED_TAG);
    const pill = tags.getTagPill(title, UNCLASSIFIED_TAG);
    await expect(pill).toBeVisible();
    // Icon glyph rendered alongside the text (WCAG 1.4.1 — not color-only).
    await expect(pill.locator('[aria-hidden="true"]')).toContainText("?");
    const border = await pill.evaluate((el) => getComputedStyle(el).borderStyle);
    expect(border).toBe("dashed");
  });

  test("AC8: Unclassified tooltip frames state as transient", async ({ page }) => {
    const title = `e2e-tag-prov-transient-${Date.now()}`;
    const session = await client.createSession({ title, path: "/tmp", program: "bash" });
    await mockSessionTags(page, session.id, [UNCLASSIFIED_TAG], { [UNCLASSIFIED_TAG]: LLM_SENTINEL });

    const tags = await gotoDashboard(page, title, UNCLASSIFIED_TAG);
    const pill = tags.getTagPill(title, UNCLASSIFIED_TAG);
    await pill.hover();
    await expect(pill).toHaveAttribute("title", /will retry/);
    await expect(pill).toHaveAccessibleName(/auto-applied, will retry/);
  });

  // One test per theme rather than a loop over a shared `page` — a second
  // `page.goto()`/mock-route registration on the same page raced with the first
  // navigation's in-flight ListSessions fetch often enough to be flaky in practice
  // (confirmed empirically investigating an analogous case in
  // session-tag-removal-confirmation.spec.ts's AC11). Separate tests get separate pages
  // for free under Playwright's default per-test isolation.
  for (const theme of ["light", "dark"] as const) {
    test(`AC5: tag pill contrast meets WCAG AA in ${theme} theme`, async ({ page }) => {
      const title = `e2e-tag-prov-contrast-${theme}-${Date.now()}`;
      const session = await client.createSession({ title, path: "/tmp", program: "bash" });
      await mockSessionTags(page, session.id, ["RuleTag", UNCLASSIFIED_TAG], {
        RuleTag: LLM_SENTINEL,
        [UNCLASSIFIED_TAG]: LLM_SENTINEL,
      });

      await page.addInitScript((t) => {
        localStorage.setItem("stapler-theme", t);
        localStorage.setItem("stapler-squad:onboarded", "true");
      }, theme);
      await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
      const tags = new TagEditorPage(page);
      await tags.switchToBoardView();
      // See gotoDashboard()'s doc comment for why this reload-retry exists.
      await expect(async () => {
        if (!(await tags.getTagPill(title, "RuleTag").isVisible().catch(() => false))) {
          await page.reload({ waitUntil: "domcontentloaded" });
          await tags.switchToBoardView();
        }
        await expect(tags.getTagPill(title, "RuleTag")).toBeVisible({ timeout: 3000 });
      }).toPass({ timeout: 20000 });

      const results = await new AxeBuilder({ page })
        .include('[aria-label="Session tags"]')
        .withTags(["wcag2aa"])
        .analyze();
      const contrastViolations = results.violations.filter((v) => v.id === "color-contrast");
      expect(contrastViolations, `contrast violations under ${theme} theme: ${JSON.stringify(contrastViolations)}`).toEqual([]);
    });
  }
});
