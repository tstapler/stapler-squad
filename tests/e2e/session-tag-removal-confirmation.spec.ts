// @feature session:update
/**
 * E2E coverage for the TagEditor removal-confirmation flow (design/ux.md Surfaces 2–3,
 * AC7, AC9–AC13; validation.md's "UX Acceptance Tests" table).
 *
 * Same real-session + ListSessions-injection pattern as session-tag-provenance.spec.ts
 * (see that file's header) — `tags`/`ruleTagProvenance` are server-computed-only fields
 * this spec injects onto a real, API-created session so the real TagEditor component can
 * be exercised end to end without reproducing the live classification pipeline.
 */

import { test, expect, Page } from "@playwright/test";
import { SessionClient } from "./helpers/session-client";
import { TagEditorPage } from "./pages/TagEditorPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";
const LLM_SENTINEL = "llm";
const UNCLASSIFIED_TAG = "Unclassified";

async function mockSessionTags(page: Page, sessionId: string, tags: string[], ruleTagProvenance: Record<string, string>) {
  await page.route("**/api/session.v1.SessionService/ListSessions", async (route) => {
    const response = await route.fetch();
    const json = await response.json();
    const sessions = (json?.sessions ?? []) as Array<Record<string, unknown>>;
    const target = sessions.find((s) => s.id === sessionId);
    if (target) Object.assign(target, { tags, ruleTagProvenance });
    await route.fulfill({ response, json });
  });
  await page.route("**/api/session.v1.SessionService/WatchSessions", async (route) => {
    await route.abort();
  });
}

async function seedSession(
  client: SessionClient,
  page: Page,
  title: string,
  tags: string[],
  ruleTagProvenance: Record<string, string>,
) {
  const session = await client.createSession({ title, path: "/tmp", program: "bash" });
  await mockSessionTags(page, session.id, tags, ruleTagProvenance);
  await page.addInitScript(() => {
    localStorage.setItem("stapler-squad:onboarded", "true");
  });
  await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
  // TagEditor is opened from SessionCard, which only renders in Board view — the
  // default List view's SessionRow has no tag UI at all (see TagEditorPage.ts).
  const tagEditor = new TagEditorPage(page);
  await tagEditor.switchToBoardView();
  if (tags.length > 0) {
    // Rare race (observed under the chromium-dom project, ~2 in 50 runs): the very
    // first ListSessions fetch issued during hydration can occasionally resolve with
    // this session's tags not yet reflecting the mock. A reload re-issues the fetch
    // through the same still-registered route, giving the mock a second real chance
    // rather than just re-asserting against already-stale data.
    await expect(async () => {
      if (!(await tagEditor.getTagPill(title, tags[0]).isVisible().catch(() => false))) {
        await page.reload({ waitUntil: "domcontentloaded" });
        await tagEditor.switchToBoardView();
      }
      await expect(tagEditor.getTagPill(title, tags[0])).toBeVisible({ timeout: 3000 });
    }).toPass({ timeout: 20000 });
  }
}

test.describe("session-tag-removal-confirmation", () => {
  let client: SessionClient;

  test.beforeEach(() => {
    client = new SessionClient(BASE_URL);
  });

  test("AC7: Unclassified has no remove control, shows explanatory caption", async ({ page }) => {
    const title = `e2e-tag-removal-unclassified-${Date.now()}`;
    await seedSession(client, page, title, [UNCLASSIFIED_TAG], { [UNCLASSIFIED_TAG]: LLM_SENTINEL });

    const tagEditor = new TagEditorPage(page);
    const dialog = await tagEditor.open(title);

    await expect(tagEditor.getRemoveButton(UNCLASSIFIED_TAG)).toHaveCount(0);
    await expect(tagEditor.getUnclassifiedCaption()).toBeVisible();
    // exact:true — the session title itself contains "unclassified" as a substring
    // (case-insensitively, `getByText`'s default), which would otherwise also match.
    await expect(dialog.getByText(UNCLASSIFIED_TAG, { exact: true })).toBeVisible();
  });

  test("AC9: removing a provenanced tag takes exactly two actions", async ({ page }) => {
    const title = `e2e-tag-removal-twoactions-${Date.now()}`;
    await seedSession(client, page, title, ["Bugfix"], { Bugfix: "some-rule-id" });

    const tagEditor = new TagEditorPage(page);
    await tagEditor.open(title);

    await tagEditor.getRemoveButton("Bugfix").click();
    // Not removed after one click — the confirmation row is shown instead.
    await expect(tagEditor.getConfirmRow("Bugfix")).toBeVisible();
    await expect(tagEditor.getDialog().getByText("Bugfix")).toBeVisible();

    await tagEditor.getRemoveAnywayButton().click();
    await expect(tagEditor.getDialog().getByText("Bugfix", { exact: true })).toHaveCount(0);
  });

  test("AC10: removing a plain user tag takes exactly one action", async ({ page }) => {
    const title = `e2e-tag-removal-oneaction-${Date.now()}`;
    await seedSession(client, page, title, ["MyTag"], {});

    const tagEditor = new TagEditorPage(page);
    await tagEditor.open(title);

    await tagEditor.getRemoveButton("MyTag").click();
    await expect(tagEditor.getDialog().getByText("MyTag", { exact: true })).toHaveCount(0);
    await expect(tagEditor.getConfirmRow("MyTag")).toHaveCount(0);
  });

  // AC11 is split into two independent tests (each gets its own fresh `page`/browser
  // context, per Playwright's default test isolation) rather than two cases sharing one
  // page — a second `page.goto()` + `page.route()` registration on the same page raced
  // with the first navigation's in-flight ListSessions fetch often enough to be flaky in
  // practice (confirmed empirically), and each case is independently sufficient evidence
  // for AC11 anyway.
  test("AC11: confirmation names a resolvable rule", async ({ page }) => {
    const ruleId = `e2e-rule-${Date.now()}`;
    try {
      const ruleResp = await fetch(`${BASE_URL}/api/session.v1.SessionService/UpsertTaggingRule`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          rule: { id: ruleId, name: "e2e Named Rule", branchPattern: "^feature/", outputTag: "Feature", priority: 50, enabled: true, source: "user" },
        }),
      });
      expect(ruleResp.ok).toBe(true);

      const namedTitle = `e2e-tag-removal-named-${Date.now()}`;
      await seedSession(client, page, namedTitle, ["Feature"], { Feature: ruleId });
      const namedEditor = new TagEditorPage(page);
      await namedEditor.open(namedTitle);
      await namedEditor.getRemoveButton("Feature").click();
      await expect(namedEditor.getDialog().getByText(/Applied by rule "e2e Named Rule" — may reappear\./)).toBeVisible();
      expect(await namedEditor.getDialog().innerText()).not.toContain("undefined");
    } finally {
      await fetch(`${BASE_URL}/api/session.v1.SessionService/DeleteTaggingRule`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id: ruleId }),
      });
    }
  });

  test("AC11: confirmation degrades gracefully when the rule has been deleted — never blank", async ({ page }) => {
    const deletedTitle = `e2e-tag-removal-deletedrule-${Date.now()}`;
    await seedSession(client, page, deletedTitle, ["Ghost"], { Ghost: "deleted-rule-id-not-in-store" });
    const deletedEditor = new TagEditorPage(page);
    await deletedEditor.open(deletedTitle);
    await deletedEditor.getRemoveButton("Ghost").click();
    await expect(
      deletedEditor.getDialog().getByText("This tag was applied automatically and may reappear — remove anyway?"),
    ).toBeVisible();
    const bodyText = await deletedEditor.getDialog().innerText();
    expect(bodyText).not.toContain("undefined");
    expect(bodyText.trim().length).toBeGreaterThan(0);
  });

  test("AC12: Keep and Escape fully cancel with no side effects", async ({ page }) => {
    const title = `e2e-tag-removal-keepescape-${Date.now()}`;
    await seedSession(client, page, title, ["Bugfix"], { Bugfix: "some-rule-id" });

    const tagEditor = new TagEditorPage(page);
    await tagEditor.open(title);

    // Keep.
    await tagEditor.getRemoveButton("Bugfix").click();
    await expect(tagEditor.getConfirmRow("Bugfix")).toBeVisible();
    await tagEditor.getKeepButton().click();
    await expect(tagEditor.getConfirmRow("Bugfix")).toHaveCount(0);
    await expect(tagEditor.getRemoveButton("Bugfix")).toBeVisible();

    // Escape. The row's Escape handler is attached to the row div, so it only fires
    // while focus is inside it — wait for the documented auto-focus-to-Keep (AC13) to
    // actually land (it's applied via a 0ms setTimeout) before sending the key.
    await tagEditor.getRemoveButton("Bugfix").click();
    await expect(tagEditor.getConfirmRow("Bugfix")).toBeVisible();
    await expect(tagEditor.getKeepButton()).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(tagEditor.getConfirmRow("Bugfix")).toHaveCount(0);
    await expect(tagEditor.getRemoveButton("Bugfix")).toBeVisible();
  });

  test("AC13: entire removal flow is keyboard-operable with correct focus movement", async ({ page }) => {
    const title = `e2e-tag-removal-keyboard-${Date.now()}`;
    await seedSession(client, page, title, ["Bugfix"], { Bugfix: "some-rule-id" });

    const tagEditor = new TagEditorPage(page);
    await tagEditor.open(title);

    // Space, not Enter — confirmed empirically that this app's round icon buttons (this
    // one and "Remove anyway" below) only synthesize a click from Space in this
    // environment; validation.md's AC13 explicitly permits either ("activate via
    // Enter/Space"), so this is within spec, not a workaround for a real gap.
    await tagEditor.getRemoveButton("Bugfix").press(" ");
    await expect(tagEditor.getConfirmRow("Bugfix")).toBeVisible();
    // Focus lands on "Keep" (the non-destructive default — WCAG 2.4.3).
    await expect(tagEditor.getKeepButton()).toBeFocused();

    await page.keyboard.press("Tab");
    await expect(tagEditor.getRemoveAnywayButton()).toBeFocused();
    await tagEditor.getRemoveAnywayButton().press(" ");
    await expect(tagEditor.getDialog().getByText("Bugfix", { exact: true })).toHaveCount(0);
  });
});
